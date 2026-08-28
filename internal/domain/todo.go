package domain

import "errors"

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
