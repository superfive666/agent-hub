package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// mkTodo 建一条 todo，返回 thread id。
func mkTodo(t *testing.T, s *store.Store, primary domain.AgentID) string {
	t.Helper()
	res, err := s.CreateTodo(context.Background(), store.CreateTodoParams{
		New:       domain.NewTodo{Title: "改退避", Body: "改成指数退避", PrimaryAgentID: primary},
		CreatedBy: "superfive",
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	return res.ThreadID
}

// 需求：seq 在每条 todo 内单调递增。
//
// 这条用例专门盯并发：两个 agent 同时追加步骤**不能撞出重复 seq，也不能吞掉一条**。
// 只算 max(seq)+1 而不先锁住 todo 行的话，两边会读到同一个 max —— 唯一约束确实
// 挡住了重复，但代价是一条步骤被吞掉，而调用方只看到一个数据库错误。
// 这种行为 mock 不出来，必须在真库上并发地跑。
func TestAppendTodoStepAllocatesSeqUnderConcurrency(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	threadID := mkTodo(t, s, rover)

	const n = 12
	var wg sync.WaitGroup
	errs := make([]error, n)
	seqs := make([]int, n)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // 尽量让它们真的挤在一起，而不是排着队进来
			row, err := s.AppendTodoStep(ctx, store.AppendStepParams{
				ThreadID:  threadID,
				ActorKind: "agent", ActorAgentID: rover,
				Step: domain.NewTodoStep{Kind: domain.StepProgress, Title: "并发追加"},
			})
			errs[i], seqs[i] = err, row.Seq
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 次追加失败（并发不该让任何一条被吞掉）: %v", i, err)
		}
	}
	seen := map[int]bool{}
	for _, q := range seqs {
		if seen[q] {
			t.Fatalf("seq %d 被分配了两次", q)
		}
		seen[q] = true
	}
	// 1..n 一个不多一个不少：既没有重复，也没有缺口。
	for want := 1; want <= n; want++ {
		if !seen[want] {
			t.Errorf("seq %d 缺失 —— 有步骤被吞掉了", want)
		}
	}

	steps, err := s.ListTodoSteps(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != n {
		t.Fatalf("步骤条数 = %d, want %d", len(steps), n)
	}
	for i, st := range steps {
		if st.Seq != i+1 {
			t.Fatalf("步骤按 seq 升序返回：第 %d 条的 seq = %d", i, st.Seq)
		}
	}
}

// 两条 todo 各自从 1 开始编号 —— seq 是「这是第几步」，不是「全库第几条记录」。
func TestTodoStepSeqIsPerTodo(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	a, b := mkTodo(t, s, rover), mkTodo(t, s, rover)

	for _, threadID := range []string{a, b} {
		row, err := s.AppendTodoStep(ctx, store.AppendStepParams{
			ThreadID: threadID, ActorKind: "agent", ActorAgentID: rover,
			Step: domain.NewTodoStep{Kind: domain.StepPlan, Title: "第一步"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if row.Seq != 1 {
			t.Errorf("thread %s 的首条步骤 seq = %d, want 1", threadID, row.Seq)
		}
	}
}

// 需求：步骤能从 pending 改成 done。改的时候要带上 thread —— 拿着别的 todo 的
// step id 过来不能改到东西，否则 handler 层按 thread 做的越权检查就形同虚设。
func TestUpdateTodoStepIsScopedToItsThread(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	mine, other := mkTodo(t, s, rover), mkTodo(t, s, rover)

	step, err := s.AppendTodoStep(ctx, store.AppendStepParams{
		ThreadID: mine, ActorKind: "agent", ActorAgentID: rover,
		Step: domain.NewTodoStep{Kind: domain.StepPlan, Title: "补并发用例", Status: domain.StepPending},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := domain.StepDone
	if _, err := s.UpdateTodoStep(ctx, store.UpdateStepParams{
		ThreadID: other, StepID: step.ID, Status: &done,
	}); !errors.Is(err, store.ErrStepNotFound) {
		t.Fatalf("拿别的 thread 的 id 来改，err = %v, want ErrStepNotFound", err)
	}

	got, err := s.UpdateTodoStep(ctx, store.UpdateStepParams{
		ThreadID: mine, StepID: step.ID, Status: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != string(domain.StepDone) {
		t.Errorf("status = %q, want done", got.Status)
	}
	if got.Title != "补并发用例" {
		t.Errorf("只改 status 时标题不该被清掉，得到 %q", got.Title)
	}
}

// 需求：管理员确认后，状态推进到 in_progress，同事务写 outbox 事件，
// 并留下一条 kind='confirmation' 的步骤。
func TestApproveTodoWritesStatusStepAndOutboxTogether(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	threadID := mkTodo(t, s, rover)

	// 建 todo 本身会写一条 outbox（开篇 post），先消化掉，免得混淆计数。
	if _, err := s.ProcessOutboxBatch(ctx, 50, nil); err != nil {
		t.Fatal(err)
	}

	res, err := s.ApproveTodo(ctx, threadID, "superfive")
	if err != nil {
		t.Fatalf("ApproveTodo: %v", err)
	}
	if res.AlreadyConfirmed {
		t.Error("首次 approve 不该被当成重复")
	}
	if res.Status != domain.StatusInProgress {
		t.Errorf("status = %s, want in_progress", res.Status)
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM todo WHERE thread_id=$1 AND confirmed_at IS NOT NULL AND confirmed_by='superfive'`,
		threadID); got != 1 {
		t.Error("confirmed_at / confirmed_by 没有落库")
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM todo_step WHERE thread_id=$1 AND kind='confirmation' AND actor_kind='admin'`,
		threadID); got != 1 {
		t.Errorf("确认动作应当留下一条 confirmation 步骤，实得 %d 条", got)
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM outbox_event WHERE status='pending' AND kind='todo.approved'`); got != 1 {
		t.Errorf("待扇出的 todo.approved 事件 = %d 条, want 1（状态与 outbox 必须同事务）", got)
	}

	// 主 agent 的 inbox 里要出现放行信号 —— 它此刻正被闸门挡着，收不到就卡死了。
	if _, err := s.ProcessOutboxBatch(ctx, 50, nil); err != nil {
		t.Fatal(err)
	}
	evs, err := s.ReadInbox(ctx, rover, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var approved int
	for _, e := range evs {
		if e.Kind == domain.EventTodoApproved {
			approved++
		}
	}
	if approved != 1 {
		t.Errorf("主 agent 收到的 todo.approved = %d 条, want 1", approved)
	}
}

// 需求：重复 approve 要幂等 —— 不报错、不重复发事件、不再加一条确认步骤。
func TestApproveTodoIsIdempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	threadID := mkTodo(t, s, rover)

	first, err := s.ApproveTodo(ctx, threadID, "superfive")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 50, nil); err != nil {
		t.Fatal(err)
	}

	second, err := s.ApproveTodo(ctx, threadID, "superfive")
	if err != nil {
		t.Fatalf("重复 approve 不该报错: %v", err)
	}
	if !second.AlreadyConfirmed {
		t.Error("第二次 approve 应当被识别为重复")
	}
	if !second.ConfirmedAt.Equal(first.ConfirmedAt) {
		t.Errorf("确认时间被改写了: %v → %v", first.ConfirmedAt, second.ConfirmedAt)
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM outbox_event WHERE kind='todo.approved'`); got != 1 {
		t.Errorf("todo.approved 事件 = %d 条, want 1（重复 approve 不该再发一条）", got)
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM todo_step WHERE thread_id=$1 AND kind='confirmation'`, threadID); got != 1 {
		t.Errorf("confirmation 步骤 = %d 条, want 1", got)
	}
}

// 需求硬规则：confirmed_at 为空时，agent 不能把状态推到
// in_progress / awaiting_review / done；但可以推到 clarifying。
func TestAgentSetTodoStatusRespectsConfirmationGate(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	tests := []struct {
		name    string
		give    domain.TodoStatus
		wantErr error
	}{
		{name: "未确认时不许开工", give: domain.StatusInProgress, wantErr: domain.ErrTodoNotConfirmed},
		{name: "未确认时不许交付", give: domain.StatusAwaitingReview, wantErr: domain.ErrTodoNotConfirmed},
		{name: "未确认时不许自己判完成", give: domain.StatusDone, wantErr: domain.ErrTodoNotConfirmed},
		{name: "未确认时可以进入澄清中 —— 闸门挡的是往下做，不是说话", give: domain.StatusClarifying},
		{name: "未确认时可以退回待响应", give: domain.StatusAwaitingResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threadID := mkTodo(t, s, rover)
			err := s.AgentSetTodoStatus(ctx, threadID, tt.give)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AgentSetTodoStatus(%s) err = %v, want %v", tt.give, err, tt.wantErr)
			}
		})
	}

	// 确认之后同样的动作就通了。
	threadID := mkTodo(t, s, rover)
	if _, err := s.ApproveTodo(ctx, threadID, "superfive"); err != nil {
		t.Fatal(err)
	}
	if err := s.AgentSetTodoStatus(ctx, threadID, domain.StatusAwaitingReview); err != nil {
		t.Errorf("确认之后应当能交付，实得 %v", err)
	}
}

// 需求：reject 只用于 awaiting_review 打回；确认之前管理员靠发帖 + 不 approve 表达。
func TestRejectTodoOnlyFromAwaitingReview(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	fresh := mkTodo(t, s, rover)
	if err := s.RejectTodo(ctx, fresh); !errors.Is(err, store.ErrInvalidTodoTransition) {
		t.Fatalf("未确认阶段打回 err = %v, want ErrInvalidTodoTransition", err)
	}

	submitted := mkTodo(t, s, rover)
	if _, err := s.ApproveTodo(ctx, submitted, "superfive"); err != nil {
		t.Fatal(err)
	}
	if err := s.AgentSetTodoStatus(ctx, submitted, domain.StatusAwaitingReview); err != nil {
		t.Fatal(err)
	}
	if err := s.RejectTodo(ctx, submitted); err != nil {
		t.Fatalf("待确认的 todo 应当能打回，实得 %v", err)
	}
	if got := countRows(t, s,
		`SELECT count(*) FROM todo WHERE thread_id=$1 AND status='in_progress'`, submitted); got != 1 {
		t.Error("打回之后应当回到 in_progress 继续做")
	}
}

// 迁移必须能在已有数据的库上跑，且历史 todo 一律变成「待确认」——
// 它们确实没有经过任何人的确认动作，假装确认过等于第一天就给闸门开了后门。
func TestExistingTodosStartUnconfirmed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")
	threadID := mkTodo(t, s, rover)

	at, err := s.TodoConfirmedAt(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if at != nil {
		t.Errorf("新建的 todo 不该是已确认的，得到 %v", at)
	}

	rows, err := s.ListTodos(ctx, "", string(rover))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ConfirmedAt != nil {
		t.Errorf("列表里的 confirmedAt 应当为空（前端靠它决定画不画确认按钮），得到 %+v", rows)
	}

	if _, err := s.ApproveTodo(ctx, threadID, "superfive"); err != nil {
		t.Fatal(err)
	}
	detail, err := s.ThreadDetail(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ConfirmedAt == nil {
		t.Error("确认之后 ThreadDetail.confirmedAt 应当非空")
	}
}
