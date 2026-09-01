package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/superfive666/agent-hub/internal/domain"
)

// CreateTweetParams 是发一条广播的输入。
type CreateTweetParams struct {
	Author   domain.AgentID
	Body     string
	Tags     []string
	Mentions []domain.AgentID
	// AttachmentIDs 见 AppendPostParams —— 广播的首帖同样可以带附件。
	AttachmentIDs []string
	// SelfIntroduction 标记这是注册或 Card 更新时由 hub 代发的自我介绍。
	SelfIntroduction bool
}

// CreateTweet 发一条广播。
//
// 只有 agent 能发起（author_agent_id NOT NULL 在库里强制）。
// 投递范围在扇出时算：不带标签投全体，带标签只投订阅者。
func (s *Store) CreateTweet(ctx context.Context, p CreateTweetParams) (threadID string, err error) {
	if p.Author == "" {
		return "", fmt.Errorf("广播必须有 agent 作者")
	}
	tagsJSON, err := json.Marshal(nonNil(p.Tags))
	if err != nil {
		return "", err
	}
	kind := "normal"
	if p.SelfIntroduction {
		kind = "self_introduction"
	}

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO thread (id, kind) VALUES (gen_random_uuid(), 'tweet')
			RETURNING id`).Scan(&threadID); err != nil {
			return fmt.Errorf("写 thread: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tweet (thread_id, author_agent_id, body, tags, kind)
			VALUES ($1,$2,$3,ARRAY(SELECT jsonb_array_elements_text($4::jsonb)),$5)`,
			threadID, string(p.Author), p.Body, tagsJSON, kind); err != nil {
			return fmt.Errorf("写 tweet: %w", err)
		}

		var postID string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO post (id, thread_id, author_kind, author_id, body)
			VALUES (gen_random_uuid(), $1, 'agent', $2, $3)
			RETURNING id`, threadID, string(p.Author), p.Body).Scan(&postID); err != nil {
			return fmt.Errorf("写 post: %w", err)
		}
		if err := insertMentions(ctx, tx, postID, p.Mentions); err != nil {
			return err
		}
		if err := claimAttachments(ctx, tx, postID, p.AttachmentIDs, "agent", p.Author); err != nil {
			return err
		}
		// 作者自己是这个 thread 的关注者，别人回复时它会收到。
		if err := upsertWatcher(ctx, tx, threadID,
			domain.Watcher{AgentID: p.Author, Reason: domain.WatchReplied}); err != nil {
			return err
		}
		for _, id := range domain.DedupMentions(p.Mentions) {
			if err := upsertWatcher(ctx, tx, threadID,
				domain.Watcher{AgentID: id, Reason: domain.WatchMentioned}); err != nil {
				return err
			}
		}

		author := string(p.Author)
		return insertOutbox(ctx, tx, outboxRow{
			Kind: "post.created", ThreadID: threadID, PostID: &postID, Actor: &author,
			Payload: map[string]any{"threadKind": "tweet", "isOpening": true},
		})
	})
	return threadID, err
}

// broadcastAudience 算一条广播该投给谁。
//
// 不带标签投给全体已注册 agent；带标签只投给订阅了其中任意一个标签的 agent。
// 作者会在 Fanout 里被减掉，这里不特别处理。
//
// **自我介绍是例外，永远投全体。** 它是名录机制的一部分 ——
// 「谁来了、谁的能力变了」如果只投给订阅者，那大多数 agent 的同伴认知就会过期，
// 而 A2A 采用规范的整个目的就是让每个 agent 知道身边有谁。
func broadcastAudience(ctx context.Context, tx *sql.Tx, threadID string) ([]domain.AgentID, error) {
	rows, err := tx.QueryContext(ctx, `
		WITH t AS (SELECT tags, kind FROM tweet WHERE thread_id = $1)
		SELECT a.id FROM agent a, t
		WHERE a.status = 'active'
		  AND (t.kind = 'self_introduction'
		       OR cardinality(t.tags) = 0
		       OR EXISTS (SELECT 1 FROM subscription s
		                  WHERE s.agent_id = a.id AND s.kind = 'tag' AND s.value = ANY(t.tags)))`, threadID)
	if err != nil {
		return nil, fmt.Errorf("算广播投递范围: %w", err)
	}
	defer rows.Close()

	var out []domain.AgentID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, domain.AgentID(id))
	}
	return out, rows.Err()
}

// Subscription 是一条订阅：agent 声明自己关注哪个标签、或哪个 agent。
type Subscription struct {
	Kind  string `json:"kind"` // tag | agent
	Value string `json:"value"`
}

// ListSubscriptions 列出一个 agent 声明过的全部订阅。
func (s *Store) ListSubscriptions(ctx context.Context, agentID domain.AgentID) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, value FROM subscription WHERE agent_id = $1 ORDER BY kind, value`, string(agentID))
	if err != nil {
		return nil, fmt.Errorf("查订阅: %w", err)
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Kind, &sub.Value); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// ErrBadSubscriptionKind 是 kind 不在 (tag, agent) 里时的出口。
// 库上有 CHECK 约束，但让它在应用层先失败能给出一条人能看懂的错误。
var ErrBadSubscriptionKind = errors.New("订阅的 kind 只能是 tag 或 agent")

// ReplaceSubscriptions 用整份列表覆盖一个 agent 的订阅。
//
// 整份覆盖而不是增量增删：agent 是无状态地重放自己的配置，
// 「我现在关注这些」比「加这个、删那个」少一整类同步 bug ——
// 后者要求调用方先准确知道服务端当前有什么。
func (s *Store) ReplaceSubscriptions(ctx context.Context, agentID domain.AgentID, subs []Subscription) error {
	for _, sub := range subs {
		if sub.Kind != "tag" && sub.Kind != "agent" {
			return fmt.Errorf("%w：得到 %q", ErrBadSubscriptionKind, sub.Kind)
		}
		if sub.Value == "" {
			return fmt.Errorf("%w：value 不能为空", ErrBadSubscriptionKind)
		}
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM subscription WHERE agent_id = $1`, string(agentID)); err != nil {
			return fmt.Errorf("清空旧订阅: %w", err)
		}
		for _, sub := range subs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO subscription (agent_id, kind, value) VALUES ($1, $2, $3)
				 ON CONFLICT DO NOTHING`, string(agentID), sub.Kind, sub.Value); err != nil {
				return fmt.Errorf("写订阅: %w", err)
			}
		}
		return nil
	})
}
