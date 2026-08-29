// Package config 从环境变量读取部署级配置。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Google 的 OIDC 端点。写死默认值，配置里一般不用出现。
const (
	DefaultGoogleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultGoogleTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultGoogleUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
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

	// Google OIDC。只在 AuthMode == AuthOIDC 时有意义。
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	// 三个 Google 端点做成可配置，是为了让测试能指向一个假的授权服务器。
	// 生产不要动这三个 —— 指到别处等于把管理员会话交给别人签发。
	GoogleAuthURL     string
	GoogleTokenURL    string
	GoogleUserinfoURL string

	SessionSecret string
	LongPollMax   time.Duration
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

		GoogleClientID:     os.Getenv("GOOGLE_OIDC_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_OIDC_CLIENT_SECRET"),
		GoogleRedirectURI:  os.Getenv("GOOGLE_OIDC_REDIRECT_URI"),

		GoogleAuthURL:     envStr("GOOGLE_OIDC_AUTH_URL", DefaultGoogleAuthURL),
		GoogleTokenURL:    envStr("GOOGLE_OIDC_TOKEN_URL", DefaultGoogleTokenURL),
		GoogleUserinfoURL: envStr("GOOGLE_OIDC_USERINFO_URL", DefaultGoogleUserinfoURL),

		SessionSecret: os.Getenv("SESSION_SECRET"),
		LongPollMax:   envDuration("LONGPOLL_MAX_WAIT", 30*time.Second),
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

	if len(c.SessionSecret) < 16 {
		// 会话密钥太短等于没有：签出来的 cookie 可以被暴力伪造。
		return errors.New("SESSION_SECRET 至少需要 16 个字符")
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
		// 光有邮箱不够：没有 client 凭据和回调地址，登录流程根本走不完，
		// 服务会「起得来但没人进得去」。那是最糟的一种状态 —— 看起来正常，
		// 实际上没有任何人能管理这个实例。所以在启动时就拦下来。
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" || c.GoogleRedirectURI == "" {
			return fmt.Errorf("%w: oidc 模式还需要 GOOGLE_OIDC_CLIENT_ID / GOOGLE_OIDC_CLIENT_SECRET / GOOGLE_OIDC_REDIRECT_URI", ErrNoAdmin)
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
