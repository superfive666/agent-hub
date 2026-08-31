package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

// InboxEvent 是 agent 拉到的一条事件。
// json tag **必须**和契约（docs/api/openapi.yaml#InboxEvent）的小写字段一致。
//
// 少了 tag 时 Go 发出去的是导出名（`Seq` / `Kind` / `ThreadID`），
// 而按契约实现的客户端读 `seq` / `kind` / `threadId` 全是 undefined ——
// **然后 cursor 照常推进，没有任何一环报错**：connector 拿不到 kind 就判不了优先级，
// 拿不到 threadId 就拼不出唤起 prompt 里的地址，事件却已经算「处理过」了。
// 这条链路上每个环节都显示正常，只有 agent 一直不干活。
type InboxEvent struct {
	Seq       int64            `json:"seq"`
	Kind      domain.EventKind `json:"kind"`
	Priority  int              `json:"priority"`
	ThreadID  string           `json:"threadId"`
	PostID    string           `json:"postId"`
	Payload   json.RawMessage  `json:"payload"`
	CreatedAt time.Time        `json:"createdAt"`
}

// ReadInbox 按 cursor 增量拉取。
//
// 这是**正确性的唯一来源**：通知可以丢，只要 agent 还能按 cursor 拉，
// 断线十分钟后重连也能一条不少地补齐。after 传上次处理到的 seq，0 表示从头。
func (s *Store) ReadInbox(ctx context.Context, id domain.AgentID, after int64, limit int) ([]InboxEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, kind, priority, thread_id, post_id, payload, created_at
		FROM inbox_event
		WHERE agent_id = $1 AND seq > $2
		ORDER BY seq
		LIMIT $3`, string(id), after, limit)
	if err != nil {
		return nil, fmt.Errorf("读 inbox: %w", err)
	}
	defer rows.Close()

	out := make([]InboxEvent, 0, limit)
	for rows.Next() {
		var e InboxEvent
		var threadID, postID sql.NullString
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Priority, &threadID, &postID, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描 inbox 行: %w", err)
		}
		e.ThreadID, e.PostID = threadID.String, postID.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 记一次拉取时间：在线判定靠它，而不是靠连接是否存在。
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_inbox_state (agent_id, last_pull_at) VALUES ($1, now())
		ON CONFLICT (agent_id) DO UPDATE SET last_pull_at = now()`, string(id)); err != nil {
		return nil, fmt.Errorf("更新拉取时间: %w", err)
	}
	return out, nil
}

// AckCursor 上报 agent 已经处理到哪。
// cursor 只许前进 —— 迟到的 ack 不能把它拽回去，否则会重放已处理的事件。
func (s *Store) AckCursor(ctx context.Context, id domain.AgentID, cursor int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_inbox_state (agent_id, cursor) VALUES ($1,$2)
		ON CONFLICT (agent_id) DO UPDATE
			SET cursor = GREATEST(agent_inbox_state.cursor, EXCLUDED.cursor), updated_at = now()`,
		string(id), cursor)
	if err != nil {
		return fmt.Errorf("更新 cursor: %w", err)
	}
	return nil
}

// Cursor 返回 agent 当前的 cursor。
func (s *Store) Cursor(ctx context.Context, id domain.AgentID) (int64, error) {
	var c int64
	err := s.db.QueryRowContext(ctx,
		`SELECT cursor FROM agent_inbox_state WHERE agent_id = $1`, string(id)).Scan(&c)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读 cursor: %w", err)
	}
	return c, nil
}

// DeadLetter 是 connector 上报的一条「我处理不了」。
type DeadLetter struct {
	Seq       int64  `json:"seq"`
	Kind      string `json:"kind"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"error"`
}

// RecordDeadLetter 记下某个 agent 处理不了的事件。
//
// 幂等：connector 重启后可能重报同一条，重复上报只留一行，不刷屏。
//
// 存下来的理由和 outbox_lag 是同一个：死信只留在 agent 自己机器上的话，
// admin 永远不知道「这个 agent 一直处理不了事件」—— 那又是一种静默失败。
func (s *Store) RecordDeadLetter(ctx context.Context, id domain.AgentID, d DeadLetter) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_dead_letter (agent_id, seq, kind, attempts, last_error)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (agent_id, seq) DO NOTHING`,
		string(id), d.Seq, d.Kind, d.Attempts, d.LastError)
	if err != nil {
		return fmt.Errorf("记录死信: %w", err)
	}
	return nil
}

// DeadLetterCount 返回死信总数。非零时控制台要显眼提示。
func (s *Store) DeadLetterCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM agent_dead_letter`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计死信: %w", err)
	}
	return n, nil
}

// OwedAgent 是「欠着事件、而且已经有一阵没来拉」的一个 agent。
type OwedAgent struct {
	AgentID domain.AgentID
	// LastSeq 是已经分配给它的最大 seq。重发信号时报这个数。
	LastSeq int64
	Cursor  int64
	Tier    string
	// SilentFor 是距上次拉取多久。从来没拉过的话，是距记录建出来多久。
	SilentFor time.Duration
}

// AgentsOwingEvents 找出 inbox 里还压着东西、而且超过判定窗口没来拉的 agent。
//
// **这是「断线重连之后自动补上」的那一半。** 另一半在 connector 里：
// cursor 落盘，重连后按 cursor 续拉，期间的事件一条不少。
// 但那一半只在「有人在拉」时成立 —— 走 webhook 契约的端点是被动的，
// 信号在它下线那一刻发出、丢掉，之后**没有第二次**：
// 事件就一直躺在 inbox 里，而它以为自己没事可做。
//
// 所以 worker 定期扫一遍，对这些 agent 重发一次信号。等它的端点活过来，
// 下一轮信号就落到它头上，它按 cursor 一拉就把断线期间的全补上了。
//
// 判据里的「超过判定窗口没拉」很重要：不加这一条，正在正常处理积压的 agent
// 会被每轮都戳一次 —— 它本来就在拉，戳它只是噪音。
func (s *Store) AgentsOwingEvents(ctx context.Context, limit int) ([]OwedAgent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.agent_id::text, s.last_seq, s.cursor, coalesce(c.tier, 'longpoll'),
		       extract(epoch FROM (now() - coalesce(s.last_pull_at, a.created_at)))
		FROM agent_inbox_state s
		JOIN agent a ON a.id = s.agent_id
		LEFT JOIN LATERAL (
			SELECT tier FROM agent_card WHERE agent_id = a.id ORDER BY version DESC LIMIT 1
		) c ON true
		WHERE a.status = 'active' AND s.last_seq > s.cursor
		ORDER BY s.last_seq - s.cursor DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("查欠账 agent: %w", err)
	}
	defer rows.Close()

	var out []OwedAgent
	for rows.Next() {
		var o OwedAgent
		var id string
		var silent float64
		if err := rows.Scan(&id, &o.LastSeq, &o.Cursor, &o.Tier, &silent); err != nil {
			return nil, err
		}
		o.AgentID = domain.AgentID(id)
		o.SilentFor = time.Duration(silent * float64(time.Second))
		// 窗口内刚拉过的不算「失联」—— 它正在处理，戳它只是噪音。
		if o.SilentFor < onlineWindow(o.Tier) {
			continue
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PurgeAckedInboxEvents 清掉**已经 ack 且超过保留期**的 inbox 事件。
//
// `seq <= cursor` 这个条件不能少，而且不是为了省事：cursor 之后的事件正是
// 「agent 还没处理」的那些，一个断线两周的 agent 全部的救命数据都在那儿。
// 按时间一刀切会把它们一起删掉 —— 而 agent 重连之后只会拉到一个空 inbox，
// **不报错、不重试，就是什么都没有**，它永远不知道自己错过了什么。
// 所以：过期只是允许删的前提，已被确认才是删的理由。
//
// 返回删掉的条数，给日志和指标用。
func (s *Store) PurgeAckedInboxEvents(ctx context.Context, retain time.Duration) (int64, error) {
	if retain <= 0 {
		return 0, nil // 保留期没配就什么都不做，别把「没设置」当成「立刻删」
	}
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM inbox_event e
		USING agent_inbox_state s
		WHERE s.agent_id = e.agent_id
		  AND e.seq <= s.cursor
		  AND e.created_at < now() - make_interval(secs => $1)`, retain.Seconds())
	if err != nil {
		return 0, fmt.Errorf("清理 inbox: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
