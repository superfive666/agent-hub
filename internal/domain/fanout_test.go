package domain

import (
	"reflect"
	"testing"
)

// 这些用例来自 docs/01-requirements.md 的验收标准，不是照着实现补的。
// 每条规则一个用例，即使实现看起来「显然」—— 显然的地方最容易出事。

func TestFanout(t *testing.T) {
	t.Parallel()

	const (
		admin = AgentID("") // admin 发帖时 Actor 为空
		rover = AgentID("rover")
		nova  = AgentID("nova")
		kilo  = AgentID("kilo")
		pico  = AgentID("pico")
	)

	tests := []struct {
		name string
		give FanoutInput
		want []Delivery
	}{
		{
			name: "开篇：主 agent 拿到 assigned，被 @ 的只拿到 mentioned",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover, IsThreadOpening: true,
				Mentions: []AgentID{nova, kilo},
				Watchers: []Watcher{{rover, WatchPrimary}, {nova, WatchMentioned}, {kilo, WatchMentioned}},
				Actor:    admin,
			},
			want: []Delivery{
				{kilo, EventTodoMentioned},
				{nova, EventTodoMentioned},
				{rover, EventTodoAssigned},
			},
		},
		{
			name: "一条 post 里 @ 同一个 agent 两次，只产生一条通知",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover,
				Mentions: []AgentID{nova, nova, nova},
				Actor:    admin,
			},
			want: []Delivery{
				{nova, EventTodoMentioned},
				{rover, EventThreadReplied},
			},
		},
		{
			name: "主 agent 同时被 @：只产生一条，取优先级最高的 assigned",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover, IsThreadOpening: true,
				Mentions: []AgentID{rover},
				Watchers: []Watcher{{rover, WatchPrimary}},
				Actor:    admin,
			},
			want: []Delivery{{rover, EventTodoAssigned}},
		},
		{
			name: "三重身份（主 agent + 被 @ + 老关注者）仍然只产生一条",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover, IsThreadOpening: true,
				Mentions: []AgentID{rover, rover},
				Watchers: []Watcher{{rover, WatchPrimary}, {rover, WatchReplied}},
				Actor:    admin,
			},
			want: []Delivery{{rover, EventTodoAssigned}},
		},
		{
			name: "作者不收自己的通知",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover,
				Mentions: []AgentID{rover, nova},
				Watchers: []Watcher{{rover, WatchPrimary}, {nova, WatchMentioned}},
				Actor:    rover,
			},
			want: []Delivery{{nova, EventTodoMentioned}},
		},
		{
			name: "关注者发言时，主 agent 收到的是 thread.replied 而不是 assigned",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover, IsThreadOpening: false,
				Watchers: []Watcher{{rover, WatchPrimary}, {nova, WatchMentioned}, {kilo, WatchMentioned}},
				Actor:    nova,
			},
			want: []Delivery{
				{kilo, EventThreadReplied},
				{rover, EventThreadReplied},
			},
		},
		{
			name: "被 @ 的优先级高于单纯的关注：同一条 post 里取 mentioned",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover,
				Mentions: []AgentID{pico},
				Watchers: []Watcher{{pico, WatchReplied}, {rover, WatchPrimary}},
				Actor:    admin,
			},
			want: []Delivery{
				{pico, EventTodoMentioned},
				{rover, EventThreadReplied},
			},
		},
		{
			name: "广播里的 @ 用 tweet.mentioned，回复用 tweet.replied",
			give: FanoutInput{
				ThreadKind: ThreadTweet,
				Mentions:   []AgentID{kilo},
				Watchers:   []Watcher{{kilo, WatchMentioned}, {pico, WatchReplied}},
				Actor:      nova,
			},
			want: []Delivery{
				{kilo, EventTweetMentioned},
				{pico, EventTweetReplied},
			},
		},
		{
			name: "广播没有主责人：即使传了 PrimaryAgentID 也不产生 assigned",
			give: FanoutInput{
				ThreadKind: ThreadTweet, PrimaryAgentID: rover, IsThreadOpening: true,
				Watchers: []Watcher{{rover, WatchReplied}},
				Actor:    nova,
			},
			want: []Delivery{{rover, EventTweetReplied}},
		},
		{
			name: "广播开篇：投递范围里的人拿到 tweet.published，被 @ 的仍然是 P1",
			give: FanoutInput{
				ThreadKind: ThreadTweet, IsThreadOpening: true,
				Mentions:    []AgentID{kilo},
				BroadcastTo: []AgentID{rover, nova, kilo},
				Actor:       nova,
			},
			want: []Delivery{
				{kilo, EventTweetMentioned}, // 被 @ 优先于被广播
				{rover, EventTweetPublished},
			},
		},
		{
			name: "广播的回复不再刷全平台，只通知参与者",
			give: FanoutInput{
				ThreadKind: ThreadTweet, IsThreadOpening: false,
				BroadcastTo: []AgentID{rover, nova, kilo, pico},
				Watchers:    []Watcher{{nova, WatchReplied}, {rover, WatchReplied}},
				Actor:       nova,
			},
			want: []Delivery{{rover, EventTweetReplied}},
		},
		{
			name: "没有收件人时返回空，而不是 nil 之外的怪东西",
			give: FanoutInput{ThreadKind: ThreadTodo, PrimaryAgentID: rover, Actor: rover},
			want: []Delivery{},
		},
		{
			name: "空 AgentID 被忽略，不会写出一条无主的事件",
			give: FanoutInput{
				ThreadKind: ThreadTodo, PrimaryAgentID: rover,
				Mentions: []AgentID{"", nova},
				Actor:    admin,
			},
			want: []Delivery{
				{nova, EventTodoMentioned},
				{rover, EventThreadReplied},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Fanout(tt.give)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fanout()\n got = %v\nwant = %v", got, tt.want)
			}
		})
	}
}

// Delivery.Priority 会被 store 直接写进 inbox_event.priority 这一列，
// 所以它必须和 EventKind 的优先级对得上，不能只在类型上看着对。
func TestDeliveryPriorityMatchesKind(t *testing.T) {
	t.Parallel()
	got := Fanout(FanoutInput{
		ThreadKind: ThreadTodo, PrimaryAgentID: "rover", IsThreadOpening: true,
		Mentions: []AgentID{"nova"},
		Watchers: []Watcher{{"kilo", WatchReplied}},
	})
	want := map[AgentID]int{"rover": 0, "nova": 1, "kilo": 2}
	if len(got) != len(want) {
		t.Fatalf("收件人数量 = %d, want %d（%v）", len(got), len(want), got)
	}
	for _, d := range got {
		if d.Priority() != want[d.AgentID] {
			t.Errorf("%s 的优先级 = P%d, want P%d", d.AgentID, d.Priority(), want[d.AgentID])
		}
		if d.Priority() != d.Kind.Priority() {
			t.Errorf("%s: Delivery.Priority()=%d 与 Kind.Priority()=%d 不一致",
				d.AgentID, d.Priority(), d.Kind.Priority())
		}
	}
}

// worker 依赖顺序稳定来复现 seq 分配，所以顺序本身要有用例盯着。
func TestFanoutIsDeterministic(t *testing.T) {
	t.Parallel()
	in := FanoutInput{
		ThreadKind: ThreadTodo, PrimaryAgentID: "rover", IsThreadOpening: true,
		Mentions: []AgentID{"zeta", "nova", "kilo", "pico"},
		Watchers: []Watcher{{"mu", WatchReplied}, {"rover", WatchPrimary}},
	}
	first := Fanout(in)
	for i := 0; i < 200; i++ {
		if got := Fanout(in); !reflect.DeepEqual(got, first) {
			t.Fatalf("第 %d 次结果与首次不同\n got = %v\nwant = %v", i, got, first)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].AgentID >= first[i].AgentID {
			t.Errorf("结果未按 AgentID 升序：%v", first)
		}
	}
}

func TestDedupMentions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		give []AgentID
		want []AgentID
	}{
		{"重复的 @ 收敛成一个", []AgentID{"nova", "nova", "kilo"}, []AgentID{"kilo", "nova"}},
		{"空 id 被丢掉", []AgentID{"", "nova", ""}, []AgentID{"nova"}},
		{"空输入", nil, []AgentID{}},
		{"已经唯一时保持有序", []AgentID{"b", "a", "c"}, []AgentID{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DedupMentions(tt.give); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DedupMentions(%v) = %v, want %v", tt.give, got, tt.want)
			}
		})
	}
}
