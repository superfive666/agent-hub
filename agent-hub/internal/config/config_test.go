package config_test

import (
	"errors"
	"testing"

	"github.com/superfive666/agent-hub/agent-hub/internal/config"
)

// **硬约束**：没有预置管理员时服务必须启动失败。
// 不能默认放行 —— 那会悄悄跑起一个谁都能进的实例，而且看起来一切正常。
func TestValidateRejectsMissingAdmin(t *testing.T) {
	t.Parallel()
	base := config.Config{DatabaseURL: "postgres://x", Timezone: "UTC", SessionSecret: "0123456789abcdef"}

	tests := []struct {
		name string
		give config.Config
		want error
	}{
		{
			name: "password 模式什么都没配",
			give: func() config.Config { c := base; c.AuthMode = config.AuthPassword; return c }(),
			want: config.ErrNoAdmin,
		},
		{
			name: "password 模式只给了用户名",
			give: func() config.Config {
				c := base
				c.AuthMode, c.AdminUsername = config.AuthPassword, "superfive"
				return c
			}(),
			want: config.ErrNoAdmin,
		},
		{
			name: "password 模式只给了密码哈希",
			give: func() config.Config {
				c := base
				c.AuthMode, c.AdminPasswordHash = config.AuthPassword, "$2a$..."
				return c
			}(),
			want: config.ErrNoAdmin,
		},
		{
			name: "oidc 模式没给邮箱",
			give: func() config.Config { c := base; c.AuthMode = config.AuthOIDC; return c }(),
			want: config.ErrNoAdmin,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.give.Validate()
			if !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want 包含 %v", err, tt.want)
			}
		})
	}
}

func TestValidateAcceptsCompleteConfig(t *testing.T) {
	t.Parallel()
	for _, c := range []config.Config{
		{DatabaseURL: "postgres://x", Timezone: "UTC", SessionSecret: "0123456789abcdef", AuthMode: config.AuthPassword,
			AdminUsername: "superfive", AdminPasswordHash: "$2a$..."},
		{DatabaseURL: "postgres://x", Timezone: "Asia/Singapore", SessionSecret: "0123456789abcdef", AuthMode: config.AuthOIDC,
			AdminGoogleEmail: "s@zephyr.org.sg"},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("完整配置不该被拒绝: %v", err)
		}
	}
}

func TestValidateRejectsBadInputs(t *testing.T) {
	t.Parallel()
	ok := config.Config{DatabaseURL: "postgres://x", Timezone: "UTC", SessionSecret: "0123456789abcdef",
		AuthMode: config.AuthPassword, AdminUsername: "u", AdminPasswordHash: "h"}

	noDSN := ok
	noDSN.DatabaseURL = ""
	if err := noDSN.Validate(); err == nil {
		t.Error("缺 DATABASE_URL 应当被拒绝")
	}

	// 时区决定看板按什么切分「一天」，配错了每个人看到的「今天」会不一样。
	badTZ := ok
	badTZ.Timezone = "Mars/Olympus"
	if err := badTZ.Validate(); err == nil {
		t.Error("无效时区应当在启动时就被拦下")
	}

	badMode := ok
	badMode.AuthMode = "magic"
	if err := badMode.Validate(); err == nil {
		t.Error("未知的认证模式应当被拒绝")
	}
}

// 会话密钥太短等于没有：签出来的 cookie 可以被暴力伪造。
func TestValidateRejectsWeakSessionSecret(t *testing.T) {
	t.Parallel()
	c := config.Config{DatabaseURL: "postgres://x", Timezone: "UTC",
		AuthMode: config.AuthPassword, AdminUsername: "u", AdminPasswordHash: "h",
		SessionSecret: "short"}
	if err := c.Validate(); err == nil {
		t.Error("过短的 SESSION_SECRET 应当被拒绝")
	}
}
