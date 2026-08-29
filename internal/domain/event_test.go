package domain

import "testing"

// 优先级的相对顺序是需求的一部分：积压时先处理「你要负责这件事」，
// 而不是「有人发了条广播」。改动这里等于改需求。
func TestEventKindPriorityOrdering(t *testing.T) {
	t.Parallel()

	if EventTodoAssigned.Priority() != 0 {
		t.Errorf("todo.assigned 必须是 P0，得到 P%d", EventTodoAssigned.Priority())
	}

	pairs := []struct{ higher, lower EventKind }{
		{EventTodoAssigned, EventTodoMentioned},
		{EventTodoMentioned, EventThreadReplied},
		{EventTweetMentioned, EventTweetReplied},
		{EventThreadReplied, EventTweetPublished},
		{EventTodoStatusChanged, EventDirectoryChanged},
	}
	for _, p := range pairs {
		if p.higher.Priority() >= p.lower.Priority() {
			t.Errorf("%s(P%d) 应当排在 %s(P%d) 之前",
				p.higher, p.higher.Priority(), p.lower, p.lower.Priority())
		}
	}

	// 未知类型不许伪装成 P0 插队。
	if got := EventKind("something.new").Priority(); got != 3 {
		t.Errorf("未知事件类型应落到最低档 P3，得到 P%d", got)
	}
}

func TestEventKindValid(t *testing.T) {
	t.Parallel()
	known := []EventKind{
		EventTodoAssigned, EventTodoMentioned, EventTweetMentioned,
		EventTodoStatusChanged, EventThreadReplied,
		EventTweetReplied, EventTweetPublished, EventDirectoryChanged,
	}
	for _, k := range known {
		if !k.Valid() {
			t.Errorf("%s 应当是已知类型", k)
		}
	}
	if EventKind("todo.exploded").Valid() {
		t.Error("未知类型不该通过校验")
	}
}

// 需求：todo.approved 是主 agent 一直在等的放行信号 —— 在它到达之前
// 主 agent 连把状态推到 in_progress 都会被拒。所以它和 todo.assigned 同一档，
// 压在 P2 里排队等于让闸门白等一轮。
func TestTodoApprovedIsTopPriority(t *testing.T) {
	t.Parallel()
	if got := EventTodoApproved.Priority(); got != 0 {
		t.Errorf("todo.approved 应当是 P0，得到 P%d", got)
	}
	if !EventTodoApproved.Valid() {
		t.Error("todo.approved 应当是已知类型（否则扇出会把它当未知降到 P3）")
	}
	if EventTodoApproved.Priority() >= EventTodoStatusChanged.Priority() {
		t.Error("放行信号必须排在「有人动了下状态」之前")
	}
}
