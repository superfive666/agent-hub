package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/config"
)

const oidcStateCookie = "hub_oidc_state"

// oidcHTTP 是走 Google 那两跳用的客户端。给死超时：授权回调是同步的，
// 挂在一个没有超时的外部请求上会把管理员的浏览器一起吊住。
var oidcHTTP = &http.Client{Timeout: 10 * time.Second}

// handleOIDCStart 把管理员踢去 Google。
//
// state 存在一个短命的 HttpOnly cookie 里，回调时比对 —— 这是防 CSRF 的那一环：
// 攻击者可以诱导浏览器打开我们的回调地址，但没法让浏览器带上他不知道的 state cookie。
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthMode != config.AuthOIDC {
		writeErr(w, Error{Code: "unauthorized", Message: "本实例配置为用户名口令登录"})
		return
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		s.log.Error("生成 OIDC state 失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: state, Path: "/",
		MaxAge: 300, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
	})

	q := url.Values{
		"client_id":     {s.cfg.GoogleClientID},
		"redirect_uri":  {s.cfg.GoogleRedirectURI},
		"response_type": {"code"},
		"scope":         {"openid email"},
		"state":         {state},
		// 只有一个管理员，账号选择器每次都弹出来比默默用错账号要好。
		"prompt": {"select_account"},
	}
	http.Redirect(w, r, s.cfg.GoogleAuthURL+"?"+q.Encode(), http.StatusFound)
}

// handleOIDCCallback 收 Google 的回调，换 token，取邮箱，比对预置管理员。
//
// **不在预置名单里的邮箱拿不到会话** —— 不是登录后没权限，是压根不发 cookie。
// 这条和口令模式是同一条硬约束：这个实例只认部署时预置的那一个管理员。
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthMode != config.AuthOIDC {
		writeErr(w, Error{Code: "unauthorized", Message: "本实例配置为用户名口令登录"})
		return
	}
	// 无论走到哪个分支，这张一次性的 state cookie 都要作废。
	defer http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})

	c, err := r.Cookie(oidcStateCookie)
	got := r.URL.Query().Get("state")
	if err != nil || got == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(got)) != 1 {
		s.log.Warn("OIDC state 校验失败", "remote", r.RemoteAddr)
		writeErr(w, ErrUnauthorized)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeErr(w, ErrBadRequest)
		return
	}

	email, err := s.oidcResolveEmail(r.Context(), code)
	if err != nil {
		s.log.Warn("OIDC 换取身份失败", "err", err, "remote", r.RemoteAddr)
		writeErr(w, ErrUnauthorized)
		return
	}
	// 邮箱大小写不敏感；两侧都归一化后再比，避免 Admin@x 进不来。
	if !strings.EqualFold(email, s.cfg.AdminGoogleEmail) {
		s.log.Warn("非预置管理员尝试登录", "email", email, "remote", r.RemoteAddr)
		writeErr(w, ErrUnauthorized)
		return
	}

	exp := time.Now().Add(12 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.signSession(s.adminSubject(), exp),
		Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil,
	})
	s.store.Audit(r.Context(), s.adminSubject(), "login", "", map[string]any{"mode": "oidc"})
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcResolveEmail 用授权码换 access_token，再拿它读 userinfo。
//
// 这里**故意不去验 id_token 的签名**：token 是我们自己向 Google 的 token 端点
// 发起的 TLS 请求直接拿回来的，不经过浏览器，没有被掉包的机会。验签是给
// 「从不可信通道收到 JWT」那种场景用的，在这条链路上只会引入一套 JWKS 缓存
// 和它的过期逻辑，换不来任何安全性。
func (s *Server) oidcResolveEmail(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {s.cfg.GoogleClientID},
		"client_secret": {s.cfg.GoogleClientSecret},
		"redirect_uri":  {s.cfg.GoogleRedirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.GoogleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oidcHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errStatus(resp.StatusCode, "token 端点")
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10)).Decode(&tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", errStatus(0, "token 响应里没有 access_token")
	}

	ureq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.GoogleUserinfoURL, nil)
	if err != nil {
		return "", err
	}
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := oidcHTTP.Do(ureq)
	if err != nil {
		return "", err
	}
	defer uresp.Body.Close()
	if uresp.StatusCode != http.StatusOK {
		return "", errStatus(uresp.StatusCode, "userinfo 端点")
	}
	var info struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, uresp.Body, 64<<10)).Decode(&info); err != nil {
		return "", err
	}
	if info.Email == "" {
		return "", errStatus(0, "userinfo 里没有 email")
	}
	// 未验证的邮箱不算身份 —— 任何人都能在自己的账号上填一个别人的邮箱，
	// 只有 Google 标记为已验证的那个才代表他真的控制这个邮箱。
	if !info.Verified {
		return "", errStatus(0, "邮箱未经 Google 验证")
	}
	return info.Email, nil
}

type oidcError struct {
	status int
	what   string
}

func (e oidcError) Error() string {
	if e.status == 0 {
		return e.what
	}
	return e.what + " 返回了非 200"
}

func errStatus(status int, what string) error { return oidcError{status: status, what: what} }
