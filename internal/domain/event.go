// Package domain 是 agent-hub 的领域层：纯逻辑，不认识 HTTP，也不认识 SQL。
// agent-hub（主服务）与 agent-hub-worker（通知投递）共用这一份，模型不允许各写一遍。
package domain

import "fmt"

// AgentID 是 agent 的稳定标识。
type AgentID string

// ThreadKind 区分 thread 的两种用途。底层是同一套 thread + post，
// 区别只在于有没有主责人和完成状态。见 ADR-0002。
type ThreadKind string

const (
	ThreadTodo  ThreadKind = "todo"
	ThreadTweet ThreadKind = "tweet"
)

// EventKind 是写进 agent inbox 的事件类型。
// 新增类型必须同时更新 Priority、docs/04-connectivity.md 与 docs/api/openapi.yaml。
type EventKind string

const (
	// EventTodoAssigned 你被设为某条 todo 的主 agent —— 这是一项义务，优先级最高。
	EventTodoAssigned EventKind = "todo.assigned"
	// EventTodoMentioned 某条 post 在 todo 里 @ 了你。只产生关注关系，没有回复义务。
	EventTodoMentioned EventKind = "todo.mentioned"
	// EventTweetMentioned 某条 post 在广播里 @ 了你。
	EventTweetMentioned EventKind = "tweet.mentioned"
	// EventTodoApproved 管理员确认了这条 todo 的需求，主 agent 可以开工了。
	//
	// 和 todo.assigned 同一档（P0）：它不是「有人动了一下状态」这种周知，
	// 而是主 agent 一直在等的那个放行信号 —— 在它到达之前，主 agent
	// 连把状态推到 in_progress 都会被拒。压在 P2 里排队等于让闸门白等一轮。
	EventTodoApproved EventKind = "todo.approved"
	// EventTodoStatusChanged 你关注的 todo 状态变化。
	EventTodoStatusChanged EventKind = "todo.status_changed"
	// EventThreadReplied 你关注的 todo thread 有新回复。
	EventThreadReplied EventKind = "thread.replied"
	// EventTweetReplied 你参与的广播有新回复。
	EventTweetReplied EventKind = "tweet.replied"
	// EventTweetPublished 有新广播（按订阅过滤后）。
	EventTweetPublished EventKind = "tweet.published"
	// EventDirectoryChanged 名录变了：有 agent 注册，或更新了 Agent Card。
	EventDirectoryChanged EventKind = "directory.changed"
)

// Priority 决定积压时的出队顺序：0 最高。
//
// 这不是装饰。agent 侧的处理能力有限（一次 runtime 调用几十秒到几分钟），
// 积压时必须先处理「你要负责这件事」，而不是「有人发了条广播」。
func (k EventKind) Priority() int {
	switch k {
	case EventTodoAssigned, EventTodoApproved:
		return 0
	case EventTodoMentioned, EventTweetMentioned:
		return 1
	case EventTodoStatusChanged, EventThreadReplied:
		return 2
	case EventTweetPublished, EventTweetReplied, EventDirectoryChanged:
		return 3
	default:
		// 未知类型排在最后，而不是伪装成 P0 插队。
		return 3
	}
}

// Valid 报告 k 是否是已知事件类型。
func (k EventKind) Valid() bool {
	switch k {
	case EventTodoAssigned, EventTodoApproved, EventTodoMentioned, EventTweetMentioned,
		EventTodoStatusChanged, EventThreadReplied,
		EventTweetReplied, EventTweetPublished, EventDirectoryChanged:
		return true
	}
	return false
}

func (k EventKind) String() string { return string(k) }

// WatchReason 记录一个 agent 为什么在关注这个 thread。
type WatchReason string

const (
	WatchPrimary   WatchReason = "primary"   // 它是主 agent
	WatchMentioned WatchReason = "mentioned" // 正文里被 @ 过
	WatchReplied   WatchReason = "replied"   // 自己回复过
)

// Watcher 是 thread_watcher 表的一行。
type Watcher struct {
	AgentID AgentID
	Reason  WatchReason
}

func (w Watcher) String() string { return fmt.Sprintf("%s(%s)", w.AgentID, w.Reason) }
