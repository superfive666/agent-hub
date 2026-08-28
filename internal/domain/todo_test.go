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
