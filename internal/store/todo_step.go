package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

var (
	// ErrStepNotFound 步骤不存在，或者不属于这条 todo。
	// 两种情况合成一个错误：告诉调用方「这个 id 在别的 todo 下面」是在帮人枚举。
	ErrStepNotFound = errors.New("步骤不存在")
	// ErrTodoNotFound todo 不存在。
	ErrTodoNotFound = errors.New("todo 不存在")
)

// TodoStepRow 是「任务处理详情步骤」里的一条。
type TodoStepRow struct {
	ID           string    `json:"id"`
	ThreadID     string    `json:"threadId"`
	Seq          int       `json:"seq"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	Detail       string    `json:"detail"`
	Status       string    `json:"status"`
	ActorKind    string    `json:"actorKind"`
	ActorAgentID string    `json:"actorAgentId,omitempty"`
	ActorName    string    `json:"actorName"`
	PostID       string    `json:"postId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// AppendStepParams 是追加一条步骤的输入。
type AppendStepParams struct {
	ThreadID string
	Step     domain.NewTodoStep
	// ActorKind 是 "agent" 或 "admin"。admin 的那条由 hub 自己写（确认动作）。
	ActorKind    string
	ActorAgentID domain.AgentID
	PostID       string // 可选：这一步对应 thread 里哪条发言
}

// AppendTodoStep 追加一条处理步骤，返回它在这条 todo 内的序号。
//
// seq 的分配必须在事务里，而且要先把 todo 行锁住再算 max(seq)+1。
// 只算不锁的话，两个并发请求会读到同一个 max：一条插进去，另一条撞唯一约束报错 ——
// 不会写重复的 seq（约束挡住了），但**会吞掉一条步骤**，而调用方看到的是一个
// 莫名其妙的数据库错误。锁 todo 行把并发追加排成队，两条都能写进去，序号连续。
//
// 锁的是 todo 而不是 todo_step：新表没有可以先锁的行（第一条步骤还不存在），
// 而 todo 行一定在 —— 顺带也就校验了「这条 todo 存在」。
func (s *Store) AppendTodoStep(ctx context.Context, p AppendStepParams) (TodoStepRow, error) {
	if err := p.Step.Validate(); err != nil {
		return TodoStepRow{}, err
	}
	var out TodoStepRow
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		row, err := appendStep(ctx, tx, p)
		out = row
		return err
	})
	return out, err
}

func appendStep(ctx context.Context, tx *sql.Tx, p AppendStepParams) (TodoStepRow, error) {
	var out TodoStepRow
	var exists string
	err := tx.QueryRowContext(ctx,
		`SELECT thread_id FROM todo WHERE thread_id = $1 FOR UPDATE`, p.ThreadID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrTodoNotFound
	}
	if err != nil {
		return out, fmt.Errorf("锁定 todo 行: %w", err)
	}

	var actorID, postID any
	if p.ActorKind == "agent" {
		actorID = string(p.ActorAgentID)
	}
	if p.PostID != "" {
		postID = p.PostID
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO todo_step (id, thread_id, seq, kind, title, detail, status,
		                       actor_kind, actor_agent_id, post_id)
		SELECT gen_random_uuid(), $1, coalesce(max(seq), 0) + 1, $2, $3, $4, $5, $6, $7::uuid, $8::uuid
		FROM todo_step WHERE thread_id = $1
		RETURNING id, thread_id, seq, kind, title, detail, status, actor_kind,
		          coalesce(actor_agent_id::text, ''), coalesce(post_id::text, ''),
		          created_at, updated_at`,
		p.ThreadID, string(p.Step.Kind), p.Step.Title, p.Step.Detail, string(p.Step.Status),
		p.ActorKind, actorID, postID).
		Scan(&out.ID, &out.ThreadID, &out.Seq, &out.Kind, &out.Title, &out.Detail,
			&out.Status, &out.ActorKind, &out.ActorAgentID, &out.PostID,
			&out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, fmt.Errorf("写 todo_step: %w", err)
	}
	return out, nil
}

// UpdateStepParams 是更新一条步骤的输入。两个字段都是可选的，nil 表示不动。
type UpdateStepParams struct {
	ThreadID string
	StepID   string
	Status   *domain.TodoStepStatus
	Detail   *string
}

// UpdateTodoStep 改一条步骤的状态或说明（比如 pending → done）。
//
// 条件里同时带 thread_id：步骤 id 是全局唯一的 uuid，但**不能允许 A 拿着 B 的 step id
// 就把 B 的记录改了** —— 越权检查在 handler 层是按 thread 做的，这里的条件
// 保证那层检查不会被一个来自别的 thread 的 id 绕过去。
func (s *Store) UpdateTodoStep(ctx context.Context, p UpdateStepParams) (TodoStepRow, error) {
	if p.Status != nil && !domain.ValidStepStatus(*p.Status) {
		return TodoStepRow{}, domain.ErrTodoStepStatus
	}
	var status, detail any
	if p.Status != nil {
		status = string(*p.Status)
	}
	if p.Detail != nil {
		detail = *p.Detail
	}

	var out TodoStepRow
	err := s.db.QueryRowContext(ctx, `
		UPDATE todo_step
		SET status = coalesce($3::text, status),
		    detail = coalesce($4::text, detail),
		    updated_at = now()
		WHERE id = $2 AND thread_id = $1
		RETURNING id, thread_id, seq, kind, title, detail, status, actor_kind,
		          coalesce(actor_agent_id::text, ''), coalesce(post_id::text, ''),
		          created_at, updated_at`,
		p.ThreadID, p.StepID, status, detail).
		Scan(&out.ID, &out.ThreadID, &out.Seq, &out.Kind, &out.Title, &out.Detail,
			&out.Status, &out.ActorKind, &out.ActorAgentID, &out.PostID,
			&out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TodoStepRow{}, ErrStepNotFound
	}
	if err != nil {
		return TodoStepRow{}, fmt.Errorf("更新 todo_step: %w", err)
	}
	return out, nil
}

// ListTodoSteps 按 seq 升序读一条 todo 的全部步骤。
func (s *Store) ListTodoSteps(ctx context.Context, threadID string) ([]TodoStepRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT st.id, st.thread_id, st.seq, st.kind, st.title, st.detail, st.status,
		       st.actor_kind, coalesce(st.actor_agent_id::text, ''),
		       coalesce(a.name, ''), coalesce(st.post_id::text, ''),
		       st.created_at, st.updated_at
		FROM todo_step st
		LEFT JOIN agent a ON a.id = st.actor_agent_id
		WHERE st.thread_id = $1
		ORDER BY st.seq`, threadID)
	if err != nil {
		return nil, fmt.Errorf("查 todo_step: %w", err)
	}
	defer rows.Close()

	out := []TodoStepRow{}
	for rows.Next() {
		var r TodoStepRow
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.Seq, &r.Kind, &r.Title, &r.Detail,
			&r.Status, &r.ActorKind, &r.ActorAgentID, &r.ActorName, &r.PostID,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApproveResult 是一次确认动作的结果。
type ApproveResult struct {
	Status      domain.TodoStatus
	ConfirmedAt time.Time
	ConfirmedBy string
	// AlreadyConfirmed 为 true 说明这次是重复 approve：什么都没改，也没有发事件。
	AlreadyConfirmed bool
}

// ApproveTodo 是「用户确认需求，可以开工」这个动作。
//
// 一个事务里做四件事：写 confirmed_at/confirmed_by、把状态推到 in_progress、
// 追加一条 kind='confirmation' 的步骤、写一条 todo.approved 的 outbox 事件。
// **事务纪律 ①**：状态和 outbox 同事务 —— 分开写就会出现「确认了但主 agent
// 永远收不到放行信号」，而它此刻正被闸门挡着，等于这条 todo 静默死掉。
//
// 幂等：已经确认过的再 approve 直接返回，不改时间戳、不重复发事件、不再加一条步骤。
// 控制台上那个按钮被点两下、或者请求重试，都不该在 thread 里留下两条确认记录。
// 条件更新（confirmed_at IS NULL）让并发的两次 approve 只有一次能生效。
func (s *Store) ApproveTodo(ctx context.Context, threadID, by string) (ApproveResult, error) {
	var res ApproveResult
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var confirmedAt sql.NullTime
		var confirmedBy sql.NullString
		var status string
		err := tx.QueryRowContext(ctx, `
			SELECT status, confirmed_at, confirmed_by FROM todo WHERE thread_id = $1 FOR UPDATE`,
			threadID).Scan(&status, &confirmedAt, &confirmedBy)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTodoNotFound
		}
		if err != nil {
			return fmt.Errorf("读 todo: %w", err)
		}
		if confirmedAt.Valid {
			res = ApproveResult{
				Status: domain.TodoStatus(status), ConfirmedAt: confirmedAt.Time,
				ConfirmedBy: confirmedBy.String, AlreadyConfirmed: true,
			}
			return nil
		}

		if err := tx.QueryRowContext(ctx, `
			UPDATE todo SET confirmed_at = now(), confirmed_by = $2,
			                status = $3, updated_at = now()
			WHERE thread_id = $1
			RETURNING confirmed_at`, threadID, by, string(domain.StatusInProgress)).
			Scan(&res.ConfirmedAt); err != nil {
			return fmt.Errorf("确认 todo: %w", err)
		}
		res.Status, res.ConfirmedBy = domain.StatusInProgress, by

		if _, err := appendStep(ctx, tx, AppendStepParams{
			ThreadID:  threadID,
			ActorKind: "admin",
			Step: domain.NewTodoStep{
				Kind:   domain.StepConfirmation,
				Title:  "管理员确认需求，可以开工",
				Status: domain.StepDone,
			},
		}); err != nil {
			return err
		}

		// 没有 post_id：这不是一条发言，是一次状态动作。收件人的构成因此也不一样，
		// 见 domain.FanoutInput.TodoEvent。
		return insertOutbox(ctx, tx, outboxRow{
			Kind:     string(domain.EventTodoApproved),
			ThreadID: threadID,
			Payload: map[string]any{
				"threadKind": string(domain.ThreadTodo),
				"todoEvent":  string(domain.EventTodoApproved),
				"status":     string(domain.StatusInProgress),
			},
		})
	})
	if err != nil {
		return ApproveResult{}, err
	}
	return res, nil
}

// AgentSetTodoStatus 是主 agent 推进状态的入口，闸门在这里。
//
// **硬规则**：confirmed_at 为空时不允许推到 in_progress / awaiting_review / done。
// 未确认之前 agent 依然可以发帖提问、可以把状态设成 clarifying、可以追加步骤 ——
// 挡的是「往下做」，不是「说话」。
//
// 判据读的是 confirmed_at 这个事实，不是「状态机允不允许」：状态会被反复推来推去，
// 「有没有被人确认过」只发生一次且不可回退，用它当闸门才挡得住。
func (s *Store) AgentSetTodoStatus(ctx context.Context, threadID string, next domain.TodoStatus) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var confirmedAt sql.NullTime
		err := tx.QueryRowContext(ctx,
			`SELECT confirmed_at FROM todo WHERE thread_id = $1 FOR UPDATE`, threadID).Scan(&confirmedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTodoNotFound
		}
		if err != nil {
			return fmt.Errorf("读 todo: %w", err)
		}
		if !confirmedAt.Valid && domain.NeedsConfirmation(next) {
			return domain.ErrTodoNotConfirmed
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE todo SET status = $2, updated_at = now() WHERE thread_id = $1`,
			threadID, string(next)); err != nil {
			return fmt.Errorf("更新 todo 状态: %w", err)
		}
		return nil
	})
}

// TodoConfirmedAt 返回一条 todo 的确认时间，没确认过就是 nil。
func (s *Store) TodoConfirmedAt(ctx context.Context, threadID string) (*time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT confirmed_at FROM todo WHERE thread_id = $1`, threadID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTodoNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("读确认时间: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	v := t.Time
	return &v, nil
}
