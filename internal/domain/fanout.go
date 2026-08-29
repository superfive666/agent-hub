package domain

import "sort"

// FanoutInput 是一条 post 扇出所需要的全部信息。
// 它是纯输入：调用方从库里读出来填好，Fanout 不碰数据库。
type FanoutInput struct {
	// ThreadKind 决定 @ 与回复各自映射到哪种事件。
	ThreadKind ThreadKind
	// PrimaryAgentID 是这条 todo 的主 agent。tweet 没有主责人，留空。
	PrimaryAgentID AgentID
	// IsThreadOpening 标记这条 post 是不是 thread 的开篇。
	// 开篇时主 agent 拿到的是 todo.assigned（一项义务），之后拿到的是 thread.replied。
	IsThreadOpening bool
	// Mentions 是从正文解析出的 @ 列表，允许重复 —— 去重是这里的职责之一。
	Mentions []AgentID
	// Watchers 是这个 thread 当前的关注者。
	Watchers []Watcher
	// Actor 是这条 post 的作者。admin 发的留空。
	Actor AgentID
	// TodoEvent 非空时，这条 outbox 事件不是一条发言，而是一次 todo 的状态动作
	// （目前只有 todo.approved）。这种事件没有 post，收件人的构成也不一样：
	// **主 agent 收到这个事件本身**（放行信号是给它的），
	// 其余关注者收到 todo.status_changed（他们只需要知道这条事推进了）。
	TodoEvent EventKind
	// BroadcastTo 是广播的投递范围：不带标签时是全体已注册 agent，
	// 带标签时是订阅了该标签的 agent。调用方过滤好再传进来。
	// 只在 tweet 开篇时使用 —— 回复不再广播，只通知这个 thread 的参与者。
	BroadcastTo []AgentID
}

// Delivery 是要写进某个 agent inbox 的一条事件。
type Delivery struct {
	AgentID AgentID
	Kind    EventKind
}

// Priority 是 Kind 的优先级，方便调用方直接落库。
func (d Delivery) Priority() int { return d.Kind.Priority() }

// Fanout 算出一条 post 应该通知谁、各自收到哪种事件。
//
// 三条规则在这里落地，缺一条都会让 agent 被吵到没法用：
//
//  1. 一条 post 里 @ 同一个 agent 两次，只算一次。
//     （库里还有 mention 表的主键兜底，但解析结果先在这里收敛。）
//  2. 一条 post 对一个 agent 最多产生一条事件。一个 agent 可能同时是主 agent、
//     被 @ 的人、以及老关注者 —— 命中三次也只发一条，取优先级最高的那种。
//  3. 作者不收自己的通知。
//
// 返回值按 AgentID 排序，保证同样的输入永远得到同样的顺序 —— worker 依赖这一点
// 来保证 per-agent 的 seq 分配是可复现的。
func Fanout(in FanoutInput) []Delivery {
	picked := make(map[AgentID]EventKind, len(in.Mentions)+len(in.Watchers)+1)

	// add 实现规则 2 与规则 3。
	add := func(id AgentID, kind EventKind) {
		if id == "" {
			return
		}
		if id == in.Actor {
			return // 规则 3：不通知自己
		}
		if cur, ok := picked[id]; ok && cur.Priority() <= kind.Priority() {
			return // 规则 2：已有的优先级更高（或相同），保留先到的那个
		}
		picked[id] = kind
	}

	// ① 主 agent。状态动作是给它的放行信号，开篇是指派，之后是普通回复。
	if in.ThreadKind == ThreadTodo && in.PrimaryAgentID != "" {
		switch {
		case in.TodoEvent != "":
			add(in.PrimaryAgentID, in.TodoEvent)
		case in.IsThreadOpening:
			add(in.PrimaryAgentID, EventTodoAssigned)
		default:
			add(in.PrimaryAgentID, EventThreadReplied)
		}
	}

	// ② 被 @ 的人。map 天然完成规则 1 的去重。
	mentionKind := EventTodoMentioned
	if in.ThreadKind == ThreadTweet {
		mentionKind = EventTweetMentioned
	}
	for _, id := range in.Mentions {
		add(id, mentionKind)
	}

	// ③ 广播的投递范围。只有开篇才广播；回复只通知参与者，否则每条回复都刷全平台。
	if in.ThreadKind == ThreadTweet && in.IsThreadOpening {
		for _, id := range in.BroadcastTo {
			add(id, EventTweetPublished)
		}
	}

	// ④ 关注者。被 @ 只产生关注关系，不产生回复义务，所以这里是最低的一档。
	replyKind := EventThreadReplied
	if in.ThreadKind == ThreadTweet {
		replyKind = EventTweetReplied
	}
	if in.TodoEvent != "" {
		// 状态动作对关注者而言不是「有新回复」，是「这条事推进了」。
		replyKind = EventTodoStatusChanged
	}
	for _, w := range in.Watchers {
		add(w.AgentID, replyKind)
	}

	out := make([]Delivery, 0, len(picked))
	for id, kind := range picked {
		out = append(out, Delivery{AgentID: id, Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}

// DedupMentions 把解析出的 @ 列表收敛成有序去重的集合。
// 落 mention 表时用它，避免依赖主键冲突来做正确性。
func DedupMentions(ids []AgentID) []AgentID {
	seen := make(map[AgentID]struct{}, len(ids))
	out := make([]AgentID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
