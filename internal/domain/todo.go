package domain

import (
	"errors"
	"strings"
)

// TodoStatus 是 todo 的生命周期。状态由 thread 里的动作驱动，
// 没有独立于 thread 之外的状态操作面板。
type TodoStatus string

const (
	StatusAwaitingResponse TodoStatus = "awaiting_response"
	StatusClarifying       TodoStatus = "clarifying"
	StatusInProgress       TodoStatus = "in_progress"
	StatusAwaitingReview   TodoStatus = "awaiting_review"
	StatusDone             TodoStatus = "done"
	StatusCancelled        TodoStatus = "cancelled"
)

var (
	// ErrPrimaryAgentRequired 是「主 agent 必选」这条硬规则在应用层的出口。
	// 数据库层还有 primary_agent_id NOT NULL 兜底，两道都要在。
	ErrPrimaryAgentRequired = errors.New("todo 必须指定一个主 agent")
	ErrTitleRequired        = errors.New("todo 必须有标题")
	ErrBodyRequired         = errors.New("todo 必须有描述")
)

// NewTodo 是创建一条 todo 所需的输入。
type NewTodo struct {
	Title          string
	Body           string
	PrimaryAgentID AgentID
	Mentions       []AgentID
}

// Validate 检查创建 todo 的硬规则。
//
// 「一件事必须有且只有一个人负责」—— 没有主 agent 的 todo 要么没人管，
// 要么所有人互相等，所以这里直接拒绝，不给默认值。
func (n NewTodo) Validate() error {
	if n.Title == "" {
		return ErrTitleRequired
	}
	if n.Body == "" {
		return ErrBodyRequired
	}
	if n.PrimaryAgentID == "" {
		return ErrPrimaryAgentRequired
	}
	return nil
}

// InitialWatchers 算出 todo 建好时的初始关注者集合。
//
// 主 agent 以 primary 身份关注；正文里 @ 到的人以 mentioned 身份关注。
// 如果 @ 的正好是主 agent，它只会出现一次，且身份是 primary ——
// 被 @ 不会把主 agent 变成两个人，也不会让它重复入队。
func (n NewTodo) InitialWatchers() []Watcher {
	out := []Watcher{{AgentID: n.PrimaryAgentID, Reason: WatchPrimary}}
	for _, id := range DedupMentions(n.Mentions) {
		if id == n.PrimaryAgentID {
			continue
		}
		out = append(out, Watcher{AgentID: id, Reason: WatchMentioned})
	}
	return out
}

// —— 用户确认闸门 ——
//
// 需求原话：「所有待办都需要用户有一个确认动作，agent 才继续往下做。」
// 未确认之前 agent 依然可以在这条 task 上回复、提问、要澄清信息 ——
// 闸门挡的是「往下做」，不是「说话」。

var (
	// ErrTodoNotConfirmed 是闸门在应用层的出口。
	// 消息写给 agent 看：它要能自己读懂下一步该干什么，而不是盲目重试。
	ErrTodoNotConfirmed = errors.New(
		"这条 todo 还没有得到管理员确认：先在 thread 里把需求问清楚，" +
			"管理员确认后你会收到 todo.approved 事件，届时再推进状态")
	// ErrTodoStepKind / ErrTodoStepStatus 是步骤记录的取值约束，DB 的 CHECK 是第二道。
	ErrTodoStepKind   = errors.New("步骤类型必须是 clarification/plan/progress/blocked/deliverable/confirmation 之一")
	ErrTodoStepStatus = errors.New("步骤状态必须是 pending/in_progress/done/blocked 之一")
	ErrTodoStepTitle  = errors.New("步骤必须有标题")
)

// NeedsConfirmation 报告推进到 next 是否需要先有管理员的确认动作。
//
// 三个「往下做」的状态被挡住：in_progress（开工）、awaiting_review（交付）、done（完成）。
// clarifying 和 awaiting_response 不挡 —— 澄清阶段本来就是确认之前该发生的事，
// 挡住它等于连问都不让问。
func NeedsConfirmation(next TodoStatus) bool {
	switch next {
	case StatusInProgress, StatusAwaitingReview, StatusDone:
		return true
	default:
		return false
	}
}

// TodoStepKind 是一条处理步骤的类型。
type TodoStepKind string

const (
	StepClarification TodoStepKind = "clarification" // 提出/得到澄清
	StepPlan          TodoStepKind = "plan"          // 打算怎么做
	StepProgress      TodoStepKind = "progress"      // 做到哪了
	StepBlocked       TodoStepKind = "blocked"       // 卡住了，缺什么
	StepDeliverable   TodoStepKind = "deliverable"   // 交付物
	StepConfirmation  TodoStepKind = "confirmation"  // 管理员的确认动作，由 hub 自己写
)

// TodoStepStatus 是一条处理步骤的完成度。
type TodoStepStatus string

const (
	StepPending       TodoStepStatus = "pending"
	StepInProgress    TodoStepStatus = "in_progress"
	StepDone          TodoStepStatus = "done"
	StepStatusBlocked TodoStepStatus = "blocked"
)

// ValidStepKind 报告 k 是否是已知的步骤类型。
func ValidStepKind(k TodoStepKind) bool {
	switch k {
	case StepClarification, StepPlan, StepProgress, StepBlocked, StepDeliverable, StepConfirmation:
		return true
	}
	return false
}

// ValidStepStatus 报告 s 是否是已知的步骤状态。
func ValidStepStatus(s TodoStepStatus) bool {
	switch s {
	case StepPending, StepInProgress, StepDone, StepStatusBlocked:
		return true
	}
	return false
}

// NewTodoStep 是追加一条处理步骤的输入。
type NewTodoStep struct {
	Kind   TodoStepKind
	Title  string
	Detail string
	Status TodoStepStatus // 留空按 done 处理：绝大多数步骤是「已经做完了才记一笔」
}

// Validate 校验并补全默认值。
func (n *NewTodoStep) Validate() error {
	if strings.TrimSpace(n.Title) == "" {
		return ErrTodoStepTitle
	}
	n.Title = strings.TrimSpace(n.Title)
	if !ValidStepKind(n.Kind) {
		return ErrTodoStepKind
	}
	if n.Status == "" {
		n.Status = StepDone
	}
	if !ValidStepStatus(n.Status) {
		return ErrTodoStepStatus
	}
	return nil
}
