package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

// CreateAgent 建一条 agent 记录，返回它的 id。
func (s *Store) CreateAgent(ctx context.Context, name, purpose, owner string) (domain.AgentID, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO agent (id, name, purpose, owner, status)
		VALUES (gen_random_uuid(), $1, $2, $3, 'active')
		RETURNING id`, name, purpose, owner).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("创建 agent: %w", err)
	}
	return domain.AgentID(id), nil
}

// CreateTodoParams 是创建一条 todo 需要的输入。
type CreateTodoParams struct {
	New       domain.NewTodo
	CreatedBy string
	DueAt     *time.Time
	Tags      []string
}

// CreateTodoResult 回给调用方的标识。
type CreateTodoResult struct {
	ThreadID  string
	PostID    string
	StartedAt time.Time // thread 记录本身的日期，就是这条 thread 的「开始日期」
}

// CreateTodo 开一条新 todo。
//
// **事务纪律 ①**：thread、todo、开篇 post、mention、watcher、outbox_event 全部
// 在同一个事务里写。post 和 outbox 同事务保证了「帖子发成功了，通知就一定会到」——
// 分开写就会出现帖子在、通知丢，这类 bug 几乎查不出来。
//
// 扇出不在这里做，交给 worker。发帖接口的延迟因此与关注者数量无关。
func (s *Store) CreateTodo(ctx context.Context, p CreateTodoParams) (CreateTodoResult, error) {
	if err := p.New.Validate(); err != nil {
		return CreateTodoResult{}, err // 主 agent 必选在这里挡第一道，DB 的 NOT NULL 是第二道
	}

	// 标签走 jsonb 而不是手拼 text[] 字面量：转义交给 encoding/json，
	// 标签里带引号、反斜杠或逗号都不会破坏语句。
	tagsJSON, err := json.Marshal(nonNil(p.Tags))
	if err != nil {
		return CreateTodoResult{}, fmt.Errorf("序列化标签: %w", err)
	}

	var res CreateTodoResult
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO thread (id, kind) VALUES (gen_random_uuid(), 'todo')
			RETURNING id, created_at`).Scan(&res.ThreadID, &res.StartedAt); err != nil {
			return fmt.Errorf("写 thread: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO todo (thread_id, title, body, primary_agent_id, created_by, due_at, tags)
			VALUES ($1,$2,$3,$4,$5,$6,
			        ARRAY(SELECT jsonb_array_elements_text($7::jsonb)))`,
			res.ThreadID, p.New.Title, p.New.Body, string(p.New.PrimaryAgentID),
			p.CreatedBy, p.DueAt, tagsJSON); err != nil {
			return fmt.Errorf("写 todo: %w", err)
		}

		if err := tx.QueryRowContext(ctx, `
			INSERT INTO post (id, thread_id, author_kind, author_id, body)
			VALUES (gen_random_uuid(), $1, 'admin', NULL, $2)
			RETURNING id`, res.ThreadID, p.New.Body).Scan(&res.PostID); err != nil {
			return fmt.Errorf("写 post: %w", err)
		}

		if err := insertMentions(ctx, tx, res.PostID, p.New.Mentions); err != nil {
			return err
		}
		for _, w := range p.New.InitialWatchers() {
			if err := upsertWatcher(ctx, tx, res.ThreadID, w); err != nil {
				return err
			}
		}
		return insertOutbox(ctx, tx, outboxRow{
			Kind:     "post.created",
			ThreadID: res.ThreadID,
			PostID:   &res.PostID,
			Payload:  map[string]any{"threadKind": "todo", "isOpening": true},
		})
	})
	if err != nil {
		return CreateTodoResult{}, err
	}
	return res, nil
}

// AppendPostParams 是在已有 thread 里回帖的输入。
type AppendPostParams struct {
	ThreadID   string
	AuthorKind string // "agent" | "admin"
	AuthorID   domain.AgentID
	Body       string
	Mentions   []domain.AgentID
}

// AppendPost 在 thread 里追加一条发言。
//
// **事务纪律 ①** 同上：post、mention、watcher、outbox 同事务。
// 回帖者自动成为关注者 —— 这是「谁参与了就订阅后续」的落地。
func (s *Store) AppendPost(ctx context.Context, p AppendPostParams) (postID string, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var threadKind string
		if err := tx.QueryRowContext(ctx,
			`SELECT kind FROM thread WHERE id = $1`, p.ThreadID).Scan(&threadKind); err != nil {
			return fmt.Errorf("读 thread: %w", err)
		}

		var authorID any
		if p.AuthorKind == "agent" {
			authorID = string(p.AuthorID)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO post (id, thread_id, author_kind, author_id, body)
			VALUES (gen_random_uuid(), $1, $2, $3, $4)
			RETURNING id`, p.ThreadID, p.AuthorKind, authorID, p.Body).Scan(&postID); err != nil {
			return fmt.Errorf("写 post: %w", err)
		}

		if err := insertMentions(ctx, tx, postID, p.Mentions); err != nil {
			return err
		}
		// 被 @ 的人成为关注者
		for _, id := range domain.DedupMentions(p.Mentions) {
			if err := upsertWatcher(ctx, tx, p.ThreadID,
				domain.Watcher{AgentID: id, Reason: domain.WatchMentioned}); err != nil {
				return err
			}
		}
		// 回复者自己也成为关注者
		if p.AuthorKind == "agent" && p.AuthorID != "" {
			if err := upsertWatcher(ctx, tx, p.ThreadID,
				domain.Watcher{AgentID: p.AuthorID, Reason: domain.WatchReplied}); err != nil {
				return err
			}
		}

		var actor *string
		if p.AuthorKind == "agent" && p.AuthorID != "" {
			a := string(p.AuthorID)
			actor = &a
		}
		return insertOutbox(ctx, tx, outboxRow{
			Kind:     "post.created",
			ThreadID: p.ThreadID,
			PostID:   &postID,
			Actor:    actor,
			Payload:  map[string]any{"threadKind": threadKind, "isOpening": false},
		})
	})
	return postID, err
}

// insertMentions 写 mention 表。
//
// 主键 (post_id, agent_id) 天然完成「一条 post 里 @ 两次只算一次」，
// 但这里仍然先在应用层去重 —— 正确性不该依赖主键冲突。
func insertMentions(ctx context.Context, tx *sql.Tx, postID string, ids []domain.AgentID) error {
	for _, id := range domain.DedupMentions(ids) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO mention (post_id, agent_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			postID, string(id)); err != nil {
			return fmt.Errorf("写 mention: %w", err)
		}
	}
	return nil
}

// upsertWatcher 把 agent 加进 thread 的关注者。
// 已经在里面就不动 —— 主 agent 不会因为被 @ 而降级成 mentioned。
func upsertWatcher(ctx context.Context, tx *sql.Tx, threadID string, w domain.Watcher) error {
	if w.AgentID == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO thread_watcher (thread_id, agent_id, reason) VALUES ($1,$2,$3)
		ON CONFLICT (thread_id, agent_id) DO NOTHING`,
		threadID, string(w.AgentID), string(w.Reason)); err != nil {
		return fmt.Errorf("写 thread_watcher: %w", err)
	}
	return nil
}

type outboxRow struct {
	Kind     string
	ThreadID string
	PostID   *string
	Actor    *string
	Payload  map[string]any
}

func insertOutbox(ctx context.Context, tx *sql.Tx, r outboxRow) error {
	payload, err := json.Marshal(r.Payload)
	if err != nil {
		return fmt.Errorf("序列化 outbox payload: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_event (kind, thread_id, post_id, actor_agent_id, payload)
		VALUES ($1,$2,$3,$4,$5)`, r.Kind, r.ThreadID, r.PostID, r.Actor, payload); err != nil {
		return fmt.Errorf("写 outbox_event: %w", err)
	}
	return nil
}

// nonNil 保证 json.Marshal 出来的是 [] 而不是 null。
func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
