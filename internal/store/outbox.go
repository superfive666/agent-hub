package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/superfive666/agent-hub/internal/domain"
)

// Notification 是提交之后要发出去的一条通知信号。
// 它只带 agent 和序号，不带内容 —— 正确性在 inbox 里，通知丢了不影响。见 ADR-0006。
type Notification struct {
	AgentID domain.AgentID
	Seq     int64
}

// ProcessOutboxBatch 认领一批 outbox 事件，扇出写进各 agent 的 inbox，标记完成。
//
// **事务纪律 ②**：认领、扇出、标记完成在同一个事务里。worker 中途崩了事务回滚，
// outbox 行回到 pending 下次重跑 —— 所以写进 inbox 这一段是 exactly-once，
// at-least-once 只发生在 hub→agent 那一段。
//
// **事务纪律 ③**：notify 在 Commit 之后才被调用。如果在事务里就通知，
// agent 收到信号立刻来拉，而事务还没提交 —— 它什么都拉不到，要等下次轮询才发现。
// 这个坑在低负载下几乎不出现，所以只能靠纪律，不能靠调试发现。
//
// 认领用 FOR UPDATE SKIP LOCKED，代码因此是 N-worker 安全的；但部署只跑一个实例，
// 因为多 worker 会打乱 per-agent 的因果顺序。见 ADR-0004。
func (s *Store) ProcessOutboxBatch(
	ctx context.Context,
	limit int,
	notify func(context.Context, []Notification),
) (processed int, err error) {
	var pending []Notification

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, thread_id, post_id, actor_agent_id, payload
			FROM outbox_event
			WHERE status = 'pending' AND next_attempt_at <= now()
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return fmt.Errorf("认领 outbox: %w", err)
		}

		type claimed struct {
			id       int64
			threadID string
			postID   sql.NullString
			actor    sql.NullString
			payload  []byte
		}
		var batch []claimed
		for rows.Next() {
			var c claimed
			if err := rows.Scan(&c.id, &c.threadID, &c.postID, &c.actor, &c.payload); err != nil {
				rows.Close()
				return fmt.Errorf("读 outbox 行: %w", err)
			}
			batch = append(batch, c)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("遍历 outbox: %w", err)
		}
		rows.Close()

		for _, c := range batch {
			in, err := loadFanoutInput(ctx, tx, c.threadID, c.postID, c.actor, c.payload)
			if err != nil {
				return err
			}
			for _, d := range domain.Fanout(in) {
				seq, err := allocSeq(ctx, tx, d.AgentID)
				if err != nil {
					return err
				}
				inserted, err := insertInbox(ctx, tx, d, seq, c.threadID, c.postID)
				if err != nil {
					return err
				}
				if inserted {
					pending = append(pending, Notification{AgentID: d.AgentID, Seq: seq})
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE outbox_event SET status='done', processed_at=now() WHERE id=$1`, c.id); err != nil {
				return fmt.Errorf("标记 outbox 完成: %w", err)
			}
			processed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// —— 分界线：到这里事务已经提交，agent 现在来拉一定拉得到 ——
	if notify != nil && len(pending) > 0 {
		notify(ctx, pending)
	}
	return processed, nil
}

// loadFanoutInput 读出算收件人所需的 thread 状态。
//
// 注意读的是**扇出时刻**的关注者，不是发帖时刻的快照。两者在正常延迟下没有区别，
// 而读当前状态更简单也更自愈 —— 中途加入的关注者会从下一条事件开始收到通知。
func loadFanoutInput(
	ctx context.Context, tx *sql.Tx,
	threadID string, postID, actor sql.NullString, payload []byte,
) (domain.FanoutInput, error) {
	var meta struct {
		ThreadKind string `json:"threadKind"`
		IsOpening  bool   `json:"isOpening"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &meta); err != nil {
			return domain.FanoutInput{}, fmt.Errorf("解析 outbox payload: %w", err)
		}
	}

	in := domain.FanoutInput{
		ThreadKind:      domain.ThreadKind(meta.ThreadKind),
		IsThreadOpening: meta.IsOpening,
	}
	if actor.Valid {
		in.Actor = domain.AgentID(actor.String)
	}

	if in.ThreadKind == domain.ThreadTodo {
		var primary sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT primary_agent_id FROM todo WHERE thread_id = $1`, threadID).Scan(&primary)
		if err != nil && err != sql.ErrNoRows {
			return domain.FanoutInput{}, fmt.Errorf("读主 agent: %w", err)
		}
		if primary.Valid {
			in.PrimaryAgentID = domain.AgentID(primary.String)
		}
	}

	if in.ThreadKind == domain.ThreadTweet && in.IsThreadOpening {
		audience, err := broadcastAudience(ctx, tx, threadID)
		if err != nil {
			return domain.FanoutInput{}, err
		}
		in.BroadcastTo = audience
	}

	if postID.Valid {
		rows, err := tx.QueryContext(ctx,
			`SELECT agent_id FROM mention WHERE post_id = $1`, postID.String)
		if err != nil {
			return domain.FanoutInput{}, fmt.Errorf("读 mention: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return domain.FanoutInput{}, err
			}
			in.Mentions = append(in.Mentions, domain.AgentID(id))
		}
		rows.Close()
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT agent_id, reason FROM thread_watcher WHERE thread_id = $1`, threadID)
	if err != nil {
		return domain.FanoutInput{}, fmt.Errorf("读 thread_watcher: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, reason string
		if err := rows.Scan(&id, &reason); err != nil {
			return domain.FanoutInput{}, err
		}
		in.Watchers = append(in.Watchers, domain.Watcher{
			AgentID: domain.AgentID(id), Reason: domain.WatchReason(reason),
		})
	}
	return in, rows.Err()
}

// allocSeq 给某个 agent 分配下一个 inbox 序号。
// upsert 让第一次投递时自动建好 agent_inbox_state 行，不需要注册时预建。
func allocSeq(ctx context.Context, tx *sql.Tx, id domain.AgentID) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO agent_inbox_state (agent_id, last_seq) VALUES ($1, 1)
		ON CONFLICT (agent_id) DO UPDATE
			SET last_seq = agent_inbox_state.last_seq + 1, updated_at = now()
		RETURNING last_seq`, string(id)).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("分配 seq: %w", err)
	}
	return seq, nil
}

// insertInbox 写一条 inbox 事件。
//
// ON CONFLICT DO NOTHING 是兜底：Fanout 已经保证了「一条 post 对一个 agent 最多一条」，
// 唯一约束挡的是任何绕过它的路径。返回值告诉调用方这次是否真的插进去了 ——
// 没插进去就不该发通知。
func insertInbox(
	ctx context.Context, tx *sql.Tx,
	d domain.Delivery, seq int64, threadID string, postID sql.NullString,
) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO inbox_event (agent_id, seq, kind, priority, thread_id, post_id)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT DO NOTHING`,
		string(d.AgentID), seq, string(d.Kind), d.Priority(), threadID, postID)
	if err != nil {
		return false, fmt.Errorf("写 inbox_event: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// OutboxPendingCount 返回还没扇出的事件条数。
//
// 和 OutboxLagSeconds 是一对：滞后秒数说明「最老的那条等了多久」，
// 条数说明「积了多少」。worker 刚挂时滞后还是 0，条数已经在涨了。
func (s *Store) OutboxPendingCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM outbox_event WHERE status = 'pending'`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计待扇出事件: %w", err)
	}
	return n, nil
}

// OutboxLagSeconds 返回最老一条待扇出事件已经等了多久。
//
// **这是唯一能发现 worker 静默死亡的指标。** worker 挂掉时不会有任何报错：
// 帖子照常能发（写 outbox 成功），agent 照常能拉 inbox（只是拉不到新东西），
// 整个平台看起来「很安静」。没有这条告警，可能几小时后才有人发现。
// 没有待扇出事件时返回 0。
func (s *Store) OutboxLagSeconds(ctx context.Context) (float64, error) {
	var lag sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
		SELECT EXTRACT(EPOCH FROM (now() - min(occurred_at)))
		FROM outbox_event WHERE status = 'pending'`).Scan(&lag)
	if err != nil {
		return 0, fmt.Errorf("查 outbox lag: %w", err)
	}
	if !lag.Valid {
		return 0, nil
	}
	return lag.Float64, nil
}
