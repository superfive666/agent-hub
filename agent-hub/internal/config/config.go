// Package config 从环境变量读取部署级配置。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// AuthMode 是管理员的认证方式，二选一，部署时定死。
type AuthMode string

const (
	AuthPassword AuthMode = "password"
	AuthOIDC     AuthMode = "oidc"
)

// Config 是服务启动所需的全部配置。
type Config struct {
	DatabaseURL string
	Addr        string
	Timezone    string

	AuthMode          AuthMode
	AdminUsername     string
	AdminPasswordHash string
	AdminGoogleEmail  string

	LongPollMax time.Duration
}

// ErrNoAdmin 是那条硬约束的出口：**没有预置管理员时服务必须启动失败**。
//
// 不能给默认值，也不能"先跑起来再说" —— 那会悄悄跑起一个谁都能进的实例，
// 而且没人会发现，因为它看起来一切正常。
var ErrNoAdmin = errors.New("没有预置管理员，拒绝启动")

// Load 读取并校验配置。任何一项不合法都返回错误，调用方应当直接退出。
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Addr:        envStr("APP_ADDR", ":8080"),
		Timezone:    envStr("PLATFORM_TIMEZONE", "Asia/Singapore"),
		AuthMode:    AuthMode(strings.ToLower(envStr("ADMIN_AUTH_MODE", string(AuthPassword)))),

		AdminUsername:     os.Getenv("ADMIN_USERNAME"),
		AdminPasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
		AdminGoogleEmail:  os.Getenv("ADMIN_GOOGLE_EMAIL"),

		LongPollMax: envDuration("LONGPOLL_MAX_WAIT", 30*time.Second),
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate 检查配置是否足以安全地启动。
func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("必须配置 DATABASE_URL")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		// 时区决定看板按什么切分「一天」。配错了每个人看到的「今天」会不一样，
		// 这是最难查的一类问题，所以在启动时就拦下来。
		return fmt.Errorf("PLATFORM_TIMEZONE 无效: %w", err)
	}

	switch c.AuthMode {
	case AuthPassword:
		if c.AdminUsername == "" || c.AdminPasswordHash == "" {
			return fmt.Errorf("%w: password 模式需要 ADMIN_USERNAME 与 ADMIN_PASSWORD_HASH", ErrNoAdmin)
		}
	case AuthOIDC:
		if c.AdminGoogleEmail == "" {
			return fmt.Errorf("%w: oidc 模式需要 ADMIN_GOOGLE_EMAIL", ErrNoAdmin)
		}
	default:
		return fmt.Errorf("ADMIN_AUTH_MODE 只能是 password 或 oidc，得到 %q", c.AuthMode)
	}
	return nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
