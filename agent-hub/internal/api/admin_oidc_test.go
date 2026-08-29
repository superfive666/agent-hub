package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/api"
	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

// fakeGoogle 是一个假的授权服务器：换 token、发 userinfo，就这两件事。
// email 决定它会声称登录的人是谁，verified 决定 Google 有没有验过这个邮箱。
func fakeGoogle(t *testing.T, email string, verified bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-` + r.Form.Get("code") + `"}`))
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer at-") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		v := "false"
		if verified {
			v = "true"
		}
		_, _ = w.Write([]byte(`{"email":"` + email + `","email_verified":` + v + `}`))
	})
	g := httptest.NewServer(mux)
	t.Cleanup(g.Close)
	return g
}

// oidcServer 起一个配成 OIDC 模式的 hub，三个 Google 端点指向 fakeGoogle。
// 返回的 client 不自动跟随重定向 —— 我们要看的正是那几个 302 本身。
func oidcServer(t *testing.T, google *httptest.Server) (*httptest.Server, *http.Client) {
	t.Helper()
	return oidcServerOn(t, testdb.New(t), google)
}

// oidcServerOn 在一个**已有的** store 上起 OIDC 实例。
// testdb.New 会独占测试库直到用例结束，一个用例里调两次会卡死，
// 所以要在同一个用例里同时起口令实例和 OIDC 实例，必须共用同一个 store。
func oidcServerOn(t *testing.T, st *store.Store, google *httptest.Server) (*httptest.Server, *http.Client) {
	t.Helper()
	cfg := config.Config{
		DatabaseURL: "unused", Timezone: "UTC", AuthMode: config.AuthOIDC,
		AdminGoogleEmail:   "superfive@zephyr.org.sg",
		GoogleClientID:     "cid",
		GoogleClientSecret: "csecret",
		GoogleRedirectURI:  "https://hub.example/api/admin/auth/google/callback",
		GoogleAuthURL:      google.URL + "/auth",
		GoogleTokenURL:     google.URL + "/token",
		GoogleUserinfoURL:  google.URL + "/userinfo",
		SessionSecret:      "test-secret-0123456789", LongPollMax: 30 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("OIDC 测试配置本身不合法: %v", err)
	}
	srv := httptest.NewServer(api.New(st, cfg, nil).Handler())
	t.Cleanup(srv.Close)
	jar := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return srv, jar
}

// start 走一遍 /start，返回 Google 那边收到的 state 和该带回去的 cookie。
func startOIDC(t *testing.T, srv *httptest.Server, c *http.Client) (state string, cookie *http.Cookie) {
	t.Helper()
	resp, err := c.Get(srv.URL + "/api/admin/auth/google/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start 应当 302，得到 %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state = loc.Query().Get("state")
	if state == "" {
		t.Fatal("跳转地址里没有 state")
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == "hub_oidc_state" {
			cookie = ck
		}
	}
	if cookie == nil {
		t.Fatal("没有下发 state cookie")
	}
	return state, cookie
}

func callback(t *testing.T, srv *httptest.Server, c *http.Client, state string, ck *http.Cookie, code string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		srv.URL+"/api/admin/auth/google/callback?code="+code+"&state="+url.QueryEscape(state), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ck != nil {
		req.AddCookie(ck)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func sessionCookieOf(resp *http.Response) *http.Cookie {
	for _, ck := range resp.Cookies() {
		if ck.Name == "hub_session" && ck.Value != "" {
			return ck
		}
	}
	return nil
}

// 预置的那个邮箱能登进来，而且拿到的会话真的能过 requireAdmin。
// 光看 302 不够 —— OIDC 模式下 AdminUsername 是空的，会话主体一旦取错，
// 登录会「成功」但下一个请求立刻 401。
func TestOIDCLoginGrantsUsableSession(t *testing.T) {
	srv, c := oidcServer(t, fakeGoogle(t, "superfive@zephyr.org.sg", true))
	state, ck := startOIDC(t, srv, c)

	resp := callback(t, srv, c, state, ck, "good-code")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("回调应当 302 回首页，得到 %d", resp.StatusCode)
	}
	sc := sessionCookieOf(resp)
	if sc == nil {
		t.Fatal("登录成功却没有下发会话 cookie")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/me", nil)
	req.AddCookie(sc)
	me, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("OIDC 会话应当能过 requireAdmin，得到 %d", me.StatusCode)
	}
}

// 邮箱大小写不该成为门槛。
func TestOIDCLoginEmailCaseInsensitive(t *testing.T) {
	srv, c := oidcServer(t, fakeGoogle(t, "SuperFive@Zephyr.Org.SG", true))
	state, ck := startOIDC(t, srv, c)
	if sessionCookieOf(callback(t, srv, c, state, ck, "good-code")) == nil {
		t.Fatal("大小写不同的同一个邮箱应当能登进来")
	}
}

// 不在预置名单里的邮箱，连会话都拿不到 —— 不是登录后没权限。
func TestOIDCRejectsUnlistedEmail(t *testing.T) {
	srv, c := oidcServer(t, fakeGoogle(t, "someone-else@evil.example", true))
	state, ck := startOIDC(t, srv, c)
	resp := callback(t, srv, c, state, ck, "good-code")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("外人应当 401，得到 %d", resp.StatusCode)
	}
	if sessionCookieOf(resp) != nil {
		t.Fatal("外人竟然拿到了会话 cookie")
	}
}

// 未验证的邮箱不算身份：任何人都能在自己的 Google 账号上填别人的邮箱。
func TestOIDCRejectsUnverifiedEmail(t *testing.T) {
	srv, c := oidcServer(t, fakeGoogle(t, "superfive@zephyr.org.sg", false))
	state, ck := startOIDC(t, srv, c)
	resp := callback(t, srv, c, state, ck, "good-code")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未验证邮箱应当 401，得到 %d", resp.StatusCode)
	}
	if sessionCookieOf(resp) != nil {
		t.Fatal("未验证邮箱竟然拿到了会话 cookie")
	}
}

// state 对不上就拒绝。这是防 CSRF 的那一环：攻击者能诱导浏览器打开我们的回调，
// 但没法让浏览器带上他不知道的 state cookie。
func TestOIDCRejectsBadState(t *testing.T) {
	srv, c := oidcServer(t, fakeGoogle(t, "superfive@zephyr.org.sg", true))
	_, ck := startOIDC(t, srv, c)

	t.Run("state 与 cookie 不符", func(t *testing.T) {
		if got := callback(t, srv, c, "not-the-state", ck, "good-code").StatusCode; got != http.StatusUnauthorized {
			t.Fatalf("应当 401，得到 %d", got)
		}
	})
	t.Run("完全没有 cookie", func(t *testing.T) {
		if got := callback(t, srv, c, "whatever", nil, "good-code").StatusCode; got != http.StatusUnauthorized {
			t.Fatalf("应当 401，得到 %d", got)
		}
	})
}

// 口令模式的实例不应该开着 OIDC 的门，反之亦然。
// 两条路都通意味着预置管理员的约束有两个口子。
func TestAuthModesAreMutuallyExclusive(t *testing.T) {
	pwSrv, st := newServer(t)
	oidcSrv, _ := oidcServerOn(t, st, fakeGoogle(t, "superfive@zephyr.org.sg", true))

	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(pwSrv.URL + "/api/admin/auth/google/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("口令模式下 OIDC 入口应当 401，得到 %d", resp.StatusCode)
	}

	r2, body := postJSON(t, oidcSrv.URL+"/api/admin/login", "",
		map[string]string{"username": "superfive", "password": "hunter2"})
	if r2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("OIDC 模式下口令登录应当 401，得到 %d：%s", r2.StatusCode, body)
	}
}
