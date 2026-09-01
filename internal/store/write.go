package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/superfive666/agent-hub/internal/domain"
)

// ErrAgentNameTaken 说明这个名字已经被占了。
//
// 名字是 @ 提及的唯一入口，所以它必须唯一（DB 上是 UNIQUE 约束）。
// 撞名是调用方能自己修的事 —— 换个名字重试 —— 所以它是 409 而不是 500。
var ErrAgentNameTaken = errors.New("这个名称已经被占用了，换一个")

// CreateAgentParams 是新建一个 agent 记录的输入。
type CreateAgentParams struct {
	Name    string
	Purpose string
	Owner   string
	// IssueToken 为 true 时在**同一个事务里**顺手签一张注册 token。
	// 分两次调用会出现「agent 建好了但 token 没签出来」的中间态 ——
	// 控制台上那一行既不能用也不知道该不该删。合成一个事务就没有这个态。
	IssueToken bool
	TokenTTL   time.Duration
}

// CreateAgentResult 是新建 agent 的结果。
type CreateAgentResult struct {
	AgentID domain.AgentID
	// RegistrationToken 的**明文只在这里出现一次**，库里只有哈希。
	// 没要求签发时为空。
	RegistrationToken string
	ExpiresAt         *time.Time
}

// CreateAgentWithToken 建一条 agent 记录，可选地在同一事务里签发注册 token。
//
// 新记录的状态是 pending_registration 而不是 active：这条记录此刻只是一个占位，
// 它还没有换过长期凭证，也就还没有真的接进来。控制台的「未接入 / 已接入」两态
// 直接读这个字段 —— 建完就写 active 的话，两态在数据上根本区分不出来。
// 状态在 ExchangeRegistrationToken 里才翻成 active。
func (s *Store) CreateAgentWithToken(ctx context.Context, p CreateAgentParams) (CreateAgentResult, error) {
	name, err := domain.ValidateAgentName(p.Name)
	if err != nil {
		return CreateAgentResult{}, err
	}

	var res CreateAgentResult
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx, `
			INSERT INTO agent (id, name, purpose, owner, status)
			VALUES (gen_random_uuid(), $1, $2, $3, 'pending_registration')
			RETURNING id`, name, p.Purpose, p.Owner).Scan(&id)
		if isUniqueViolation(err) {
			return ErrAgentNameTaken
		}
		if err != nil {
			return fmt.Errorf("创建 agent: %w", err)
		}
		res.AgentID = domain.AgentID(id)

		if !p.IssueToken {
			return nil
		}
		plain, exp, err := issueRegistrationToken(ctx, tx, res.AgentID, p.TokenTTL)
		if err != nil {
			return err
		}
		res.RegistrationToken, res.ExpiresAt = plain, &exp
		return nil
	})
	if err != nil {
		return CreateAgentResult{}, err
	}
	return res, nil
}

// CreateAgent 建一条 agent 记录，返回它的 id。CreateAgentWithToken 的简写。
func (s *Store) CreateAgent(ctx context.Context, name, purpose, owner string) (domain.AgentID, error) {
	res, err := s.CreateAgentWithToken(ctx, CreateAgentParams{Name: name, Purpose: purpose, Owner: owner})
	return res.AgentID, err
}

// isUniqueViolation 判断错误是不是唯一约束冲突（PostgreSQL 23505）。
// 靠错误码而不是匹配错误文案 —— 文案会随 PostgreSQL 版本和 locale 变。
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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
	// AttachmentIDs 是两步上传的第二步：先 POST /attachments 拿到 id，
	// 发帖时把 id 带上。空是常态 —— 绝大多数帖子没有附件。
	AttachmentIDs []string
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
		// 附件和帖子同事务：分成两笔的话，中间失败就留下一条「说了有附件、
		// 但附件没挂上」的帖子，而界面上看不出区别。
		if err := claimAttachments(ctx, tx, postID, p.AttachmentIDs,
			p.AuthorKind, p.AuthorID); err != nil {
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
