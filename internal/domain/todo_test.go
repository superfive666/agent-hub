package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewTodoValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		give    NewTodo
		wantErr error
	}{
		{
			name:    "不指定主 agent 时拒绝创建",
			give:    NewTodo{Title: "改退避", Body: "改成指数退避"},
			wantErr: ErrPrimaryAgentRequired,
		},
		{
			name:    "只 @ 了人但没选主 agent，同样拒绝 —— @ 不等于指派",
			give:    NewTodo{Title: "改退避", Body: "@nova 看下", Mentions: []AgentID{"nova"}},
			wantErr: ErrPrimaryAgentRequired,
		},
		{
			name:    "没有标题",
			give:    NewTodo{Body: "正文", PrimaryAgentID: "rover"},
			wantErr: ErrTitleRequired,
		},
		{
			name:    "没有描述",
			give:    NewTodo{Title: "标题", PrimaryAgentID: "rover"},
			wantErr: ErrBodyRequired,
		},
		{
			name: "齐全时通过",
			give: NewTodo{Title: "改退避", Body: "改成指数退避", PrimaryAgentID: "rover"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.give.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewTodoInitialWatchers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		give NewTodo
		want []Watcher
	}{
		{
			name: "主 agent 与被 @ 的人各自入列",
			give: NewTodo{PrimaryAgentID: "rover", Mentions: []AgentID{"nova", "kilo"}},
			want: []Watcher{{"rover", WatchPrimary}, {"kilo", WatchMentioned}, {"nova", WatchMentioned}},
		},
		{
			name: "@ 到主 agent 自己时不重复入列，身份仍是 primary",
			give: NewTodo{PrimaryAgentID: "rover", Mentions: []AgentID{"rover", "nova"}},
			want: []Watcher{{"rover", WatchPrimary}, {"nova", WatchMentioned}},
		},
		{
			name: "重复 @ 同一个人只入列一次",
			give: NewTodo{PrimaryAgentID: "rover", Mentions: []AgentID{"nova", "nova"}},
			want: []Watcher{{"rover", WatchPrimary}, {"nova", WatchMentioned}},
		},
		{
			name: "没有 @ 任何人时只有主 agent",
			give: NewTodo{PrimaryAgentID: "rover"},
			want: []Watcher{{"rover", WatchPrimary}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.give.InitialWatchers(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InitialWatchers()\n got = %v\nwant = %v", got, tt.want)
			}
		})
	}
}

// 需求：所有待办都要用户先做一个确认动作，agent 才继续往下做；
// 未确认之前 agent 仍然可以回复、提问、要澄清信息。
// 所以闸门只挡「往下做」的那三个状态，不挡澄清。
func TestNeedsConfirmation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		give TodoStatus
		want bool
	}{
		{name: "开工要先被确认", give: StatusInProgress, want: true},
		{name: "交付要先被确认", give: StatusAwaitingReview, want: true},
		{name: "完成要先被确认", give: StatusDone, want: true},
		{name: "澄清中不挡 —— 这正是确认之前该发生的事", give: StatusClarifying, want: false},
		{name: "退回待响应不挡：agent 说「这活我接不了」不需要许可", give: StatusAwaitingResponse, want: false},
		{name: "取消不挡：那是管理员的动作", give: StatusCancelled, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NeedsConfirmation(tt.give); got != tt.want {
				t.Errorf("NeedsConfirmation(%s) = %v, want %v", tt.give, got, tt.want)
			}
		})
	}
}

func TestNewTodoStepValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		give       NewTodoStep
		wantErr    error
		wantStatus TodoStepStatus
	}{
		{
			name:       "不填状态时默认 done —— 绝大多数步骤是做完了才记一笔",
			give:       NewTodoStep{Kind: StepProgress, Title: "跑通了重试退避"},
			wantStatus: StepDone,
		},
		{
			name:       "显式给 pending：主 agent 一次铺好计划，做一步改一步",
			give:       NewTodoStep{Kind: StepPlan, Title: "补并发用例", Status: StepPending},
			wantStatus: StepPending,
		},
		{
			name:    "没有标题时拒绝 —— 一条没标题的步骤在界面上是一行空白",
			give:    NewTodoStep{Kind: StepProgress, Title: "   "},
			wantErr: ErrTodoStepTitle,
		},
		{
			name:    "未知类型被拒",
			give:    NewTodoStep{Kind: "brainstorm", Title: "想一想"},
			wantErr: ErrTodoStepKind,
		},
		{
			name:    "未知状态被拒",
			give:    NewTodoStep{Kind: StepProgress, Title: "干活", Status: "almost"},
			wantErr: ErrTodoStepStatus,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			give := tt.give
			err := give.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && give.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", give.Status, tt.wantStatus)
			}
		})
	}
}
