package domain

import (
	"errors"
	"strings"
	"testing"
)

// 需求：agent 名称是 @ 提及的唯一入口，所以字符集必须和前端的 mention token
// （/(^|\s)@([A-Za-z0-9_-]*)$/）对齐 —— 名字里带空格的 agent 根本 @ 不到，
// 而 @ 是这个平台上唯一的连接动作。
func TestValidateAgentName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		give    string
		want    string
		wantErr error
	}{
		{name: "普通名字原样通过", give: "rover", want: "rover"},
		{name: "允许下划线与连字符", give: "ci-runner_02", want: "ci-runner_02"},
		{name: "允许纯数字", give: "007", want: "007"},
		{name: "首尾空白被去掉", give: "  rover \n", want: "rover"},
		{name: "空字符串被拒", give: "", wantErr: ErrAgentNameRequired},
		{name: "只有空白等于空", give: "   ", wantErr: ErrAgentNameRequired},
		{
			name: "名字中间有空格：这样的名字 @ 不到，所以创建时就挡掉",
			give: "code reviewer", wantErr: ErrAgentNameCharset,
		},
		{name: "中文名同样 @ 不到", give: "巡检员", wantErr: ErrAgentNameCharset},
		{name: "点号不在 mention 字符集里", give: "rover.v2", wantErr: ErrAgentNameCharset},
		{name: "@ 本身不能出现在名字里", give: "@rover", wantErr: ErrAgentNameCharset},
		{name: "刚好到长度上限时通过", give: strings.Repeat("a", AgentNameMaxLen), want: strings.Repeat("a", AgentNameMaxLen)},
		{name: "超过长度上限被拒", give: strings.Repeat("a", AgentNameMaxLen+1), wantErr: ErrAgentNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateAgentName(tt.give)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateAgentName(%q) err = %v, want %v", tt.give, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateAgentName(%q) = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}
