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
type InboxEvent struct {
	Seq       int64
	Kind      domain.EventKind
	Priority  int
	ThreadID  string
	PostID    string
	Payload   json.RawMessage
	CreatedAt time.Time
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
