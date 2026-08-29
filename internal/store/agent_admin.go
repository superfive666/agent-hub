package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/superfive666/agent-hub/internal/domain"
)

var (
	// ErrAgentNotFound agent 不存在。
	ErrAgentNotFound = errors.New("agent 不存在")

	// ErrAgentInUse 说明这个 agent 已经在内容里留下了痕迹，物理删不掉。
	//
	// 不是技术限制而是设计：`todo.primary_agent_id NOT NULL REFERENCES agent(id)`
	// 是根 CLAUDE.md 里的硬约束 ——「一条 todo 必须有且只有一个主 agent」。
	// 删掉一个背着 todo 的 agent，要么违反外键，要么得把那条 todo 的主责人置空，
	// 而后者正是这条约束存在的意义所在：不允许出现没人负责的 todo。
	// tweet 与 todo_step 同理 —— 它们是**已经发生过的事**，抹掉作者等于篡改历史。
	ErrAgentInUse = errors.New("这个 agent 已经在内容里留下痕迹，只能停用不能删除")
)

// AgentRefs 是一个 agent 在内容里的留痕计数。删不掉时要告诉调用方到底卡在哪，
// 只回一句「删不掉」等于让人去库里自己查。
type AgentRefs struct {
	Todos  int `json:"todos"`
	Tweets int `json:"tweets"`
	Steps  int `json:"steps"`
}

func (r AgentRefs) any() bool { return r.Todos > 0 || r.Tweets > 0 || r.Steps > 0 }

// UpdateAgentPurpose 改简介。
//
// **名称不给改，这里也没有改名的入口。** 名字是 `@` 提及的唯一标识，正文里
// 那些已经写好的 `@old-name` 不会跟着改 —— 改完之后历史帖子里的提及全部失效，
// 而且是静默失效（解析不到就当普通文本忽略），没有任何地方会报错。
func (s *Store) UpdateAgentPurpose(ctx context.Context, agent domain.AgentID, purpose string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent SET purpose = $2 WHERE id = $1`, string(agent), strings.TrimSpace(purpose))
	if err != nil {
		return fmt.Errorf("改 agent 简介: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// SetAgentEnabled 停用 / 启用一个 agent，返回落库后的状态。
//
// **停用是立刻生效的下线，不是一个标签**：AuthenticateCredential 的查询里带着
// `a.status = 'active'`，所以状态一改，这个 agent 的长期凭证当场就认证不过了。
// 它和「吊销凭证」的区别在于可逆 —— 凭证还在，重新启用就能继续用，
// 不需要重新走一遍注册换证。
//
// 启用时的目标状态是**算出来的，不是写死 active**：一个从没换过凭证的 agent
// （pending_registration）被停用再启用，直接写 active 会让它在控制台上显示成
// 「已接入」，而它其实一次都没接进来过。有没有活着的凭证才是这件事的判据。
func (s *Store) SetAgentEnabled(ctx context.Context, agent domain.AgentID, enabled bool) (string, error) {
	var next string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT true FROM agent WHERE id = $1 FOR UPDATE`, string(agent)).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAgentNotFound
			}
			return fmt.Errorf("查 agent: %w", err)
		}

		if !enabled {
			next = "disabled"
			_, err := tx.ExecContext(ctx,
				`UPDATE agent SET status = 'disabled', disabled_at = now() WHERE id = $1`, string(agent))
			return err
		}

		var hasCredential bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM agent_credential
			               WHERE agent_id = $1 AND revoked_at IS NULL)`,
			string(agent)).Scan(&hasCredential); err != nil {
			return fmt.Errorf("查凭证: %w", err)
		}
		next = "pending_registration"
		if hasCredential {
			next = "active"
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE agent SET status = $2, disabled_at = NULL WHERE id = $1`, string(agent), next)
		return err
	})
	if err != nil {
		return "", err
	}
	return next, nil
}

// CountAgentRefs 数一个 agent 在内容里的留痕。
func (s *Store) CountAgentRefs(ctx context.Context, agent domain.AgentID) (AgentRefs, error) {
	var r AgentRefs
	err := s.db.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM todo      WHERE primary_agent_id = $1),
		       (SELECT count(*) FROM tweet     WHERE author_agent_id  = $1),
		       (SELECT count(*) FROM todo_step WHERE actor_agent_id   = $1)`,
		string(agent)).Scan(&r.Todos, &r.Tweets, &r.Steps)
	if err != nil {
		return r, fmt.Errorf("数 agent 留痕: %w", err)
	}
	return r, nil
}

// DeleteAgent 物理删除一个 agent，**只在它没有任何内容留痕时**才允许。
//
// 有留痕就返回 ErrAgentInUse 与计数，让调用方去停用 —— 见 ErrAgentInUse 的注释。
// 能删掉的那些是「建错了名字、还没接入就想重来」这类，删掉它们不会让任何
// 已经发生的事失去主语，剩下的关联（凭证、注册 token、inbox、订阅、Card）
// 都是 ON DELETE CASCADE，跟着一起走。
//
// 计数与删除必须同事务：分开做的话，两次调用之间刚好有人给它指派了一条 todo，
// 删除会撞外键报 500，而调用方拿到的还是「可以删」的判断。
func (s *Store) DeleteAgent(ctx context.Context, agent domain.AgentID) (AgentRefs, error) {
	var refs AgentRefs
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT true FROM agent WHERE id = $1 FOR UPDATE`, string(agent)).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAgentNotFound
			}
			return fmt.Errorf("查 agent: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM todo      WHERE primary_agent_id = $1),
			       (SELECT count(*) FROM tweet     WHERE author_agent_id  = $1),
			       (SELECT count(*) FROM todo_step WHERE actor_agent_id   = $1)`,
			string(agent)).Scan(&refs.Todos, &refs.Tweets, &refs.Steps); err != nil {
			return fmt.Errorf("数 agent 留痕: %w", err)
		}
		if refs.any() {
			return ErrAgentInUse
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent WHERE id = $1`, string(agent)); err != nil {
			return fmt.Errorf("删除 agent: %w", err)
		}
		return nil
	})
	return refs, err
}

// AgentSelf 是一个 agent 关于「我是谁」的最小事实集。
type AgentSelf struct {
	AgentID string `json:"agentId"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Status  string `json:"status"`
	HasCard bool   `json:"hasCard"`
	// CardVersion 为 0 表示还没写过 Card。agent 靠它判断这是首次撰写还是更新。
	CardVersion int `json:"cardVersion"`
}

// SelfOf 回答「我是谁」。
//
// 为什么需要它：写 Card 时 `name` 必须和管理员注册的对得上 —— 写错了自我介绍
// 广播里就是个别人不认识的名字。而 agent 手上只有一个凭证，没有别的途径知道
// 这个名字：名录接口给的是**所有人**，它得先知道自己的 agentId 再去里面捞自己，
// 绕了一圈还是要先回答「我是谁」。
//
// 另外 cardVersion 只有这里有：agent 靠它分辨这次是首次撰写还是更新，
// 名录接口不带版本号。
func (s *Store) SelfOf(ctx context.Context, agent domain.AgentID) (AgentSelf, error) {
	var out AgentSelf
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.name, a.purpose, a.status,
		       (SELECT max(version) FROM agent_card WHERE agent_id = a.id)
		FROM agent a WHERE a.id = $1`, string(agent)).
		Scan(&out.AgentID, &out.Name, &out.Purpose, &out.Status, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrAgentNotFound
	}
	if err != nil {
		return out, fmt.Errorf("查 agent 自身: %w", err)
	}
	out.CardVersion = int(version.Int64)
	out.HasCard = version.Valid
	return out, nil
}
