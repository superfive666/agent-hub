package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// testPassword 与 testPasswordHash 是测试用的预置管理员凭据。
const testPassword = "correct-horse-battery"

var testPasswordHash = mustHash(testPassword)

func mustHash(p string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return string(h)
}

// adminClient 返回一个已登录的 client（会话在 cookie 里）。
func adminClient(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	resp, body := doJSON(t, c, http.MethodPost, base+"/api/admin/login", nil,
		map[string]string{"username": "superfive", "password": testPassword})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("管理员登录失败: %d %s", resp.StatusCode, body)
	}
	return c
}

// 需求：不在预置名单里的账号根本进不来 —— 不是「登录后无权限」，是连会话都拿不到。
func TestAdminLoginRejectsWrongCredentials(t *testing.T) {
	srv, _ := newServer(t)
	c := &http.Client{}
	for _, tc := range []struct {
		name, user, pass string
	}{
		{"密码错", "superfive", "wrong"},
		{"用户名不在预置名单里", "someone-else", testPassword},
		{"两者都错", "x", "y"},
	} {
		resp, _ := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/login", nil,
			map[string]string{"username": tc.user, "password": tc.pass})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s：状态码 = %d, want 401", tc.name, resp.StatusCode)
		}
		if len(resp.Cookies()) > 0 {
			t.Errorf("%s：被拒绝时不该下发任何 cookie", tc.name)
		}
	}
}

func TestAdminEndpointsRequireSession(t *testing.T) {
	srv, _ := newServer(t)
	c := &http.Client{}
	for _, path := range []string{"/api/admin/me", "/api/admin/agents", "/api/admin/todos",
		"/api/admin/board", "/api/admin/health", "/api/admin/settings"} {
		resp, _ := doJSON(t, c, http.MethodGet, srv.URL+path, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 未登录时应当 401，实得 %d", path, resp.StatusCode)
		}
	}
}

// 伪造的会话 cookie 不能通过 —— HMAC 签名要真的在验。
func TestForgedSessionCookieRejected(t *testing.T) {
	srv, _ := newServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/me", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "c3VwZXJmaXZl.ZmFrZQ"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("伪造的会话应当被拒绝，实得 %d", resp.StatusCode)
	}
}

// 需求：主 agent 必选，且错误要让调用方看懂，而不是 500。
func TestCreateTodoWithoutPrimaryAgentIsClientError(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/todos", nil,
		map[string]any{"title": "改退避", "body": "正文"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want 400（这是业务规则不是服务器错误）", resp.StatusCode)
	}
	var e struct{ Code, Message string }
	_ = json.Unmarshal(body, &e)
	if e.Message == "" {
		t.Error("错误应当带上人能看懂的说明")
	}
}

// 需求：注册 token 的明文只在签发时返回一次。
func TestIssueRegistrationTokenReturnsPlaintextOnce(t *testing.T) {
	srv, st := newServer(t)
	c := adminClient(t, srv.URL)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]string{"name": "sigma", "purpose": "CI 与发布"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建 agent 失败: %d %s", resp.StatusCode, body)
	}
	var created struct{ AgentID string }
	_ = json.Unmarshal(body, &created)

	resp, body = doJSON(t, c, http.MethodPost,
		srv.URL+"/api/admin/agents/"+created.AgentID+"/registration-token", nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("签发 token 失败: %d %s", resp.StatusCode, body)
	}
	var tok struct{ RegistrationToken string }
	_ = json.Unmarshal(body, &tok)
	if tok.RegistrationToken == "" {
		t.Fatal("没有返回明文 token")
	}

	// 库里只该有哈希，任何地方都不该能再读到明文。
	var n int
	if err := st.DB().QueryRowContext(context.Background(),
		`SELECT count(*) FROM registration_token WHERE token_hash = $1::bytea`,
		[]byte(tok.RegistrationToken)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("库里存了明文 token —— 只该存哈希")
	}
}

// 需求：改设置时时区非法要被拒绝。它决定看板按什么切分「一天」。
func TestSettingsRejectsInvalidTimezone(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	resp, _ := doJSON(t, c, http.MethodPut, srv.URL+"/api/admin/settings", nil,
		map[string]any{"timezone": "Mars/Olympus"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("无效时区应当 400，实得 %d", resp.StatusCode)
	}

	resp, body := doJSON(t, c, http.MethodPut, srv.URL+"/api/admin/settings", nil,
		map[string]any{"timezone": "Asia/Tokyo", "longPollMaxSeconds": 25})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法时区应当接受，实得 %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, c, http.MethodGet, srv.URL+"/api/admin/settings", nil, nil)
	var got store.Settings
	_ = json.Unmarshal(body, &got)
	if got.Timezone != "Asia/Tokyo" || got.LongPollMaxSeconds != 25 {
		t.Errorf("设置没有被持久化: %+v", got)
	}
}

// 需求：Agent Card 的能力边界不许为空 —— 它比能力清单更有信息量。
func TestCardRejectedWithoutLimitations(t *testing.T) {
	srv, st := newServer(t)
	_, cred := register(t, srv, st, "rover")

	noLimits := map[string]any{
		"name": "rover", "description": "工程 agent",
		"capabilities": map[string]any{"extensions": []any{
			map[string]any{"uri": store.ProfileExtURI + "/v1",
				"params": map[string]any{"runtime": "claude-code", "tier": "longpoll"}},
		}},
	}
	resp, body := putJSON(t, srv.URL+"/api/agent/me/card", cred, noLimits)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("没写能力边界的 Card 应当 422，实得 %d %s", resp.StatusCode, body)
	}
	var e struct{ Code string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "card_needs_limitations" {
		t.Errorf("错误码 = %q, want card_needs_limitations（要让 agent 知道该补什么）", e.Code)
	}
}

// 需求：hermes / openhuman 的 webhook 是**它们自己的**聊天通道 —— hub 直连过去，
// 那条没有正文的信号会变成一条怪消息，落进 agent 正在跟人用的会话里。
// 所以写 Card 的时候就当场拒掉，而不是投递时静默跳过：静默跳过的话，
// 填的人会一直以为自己接好了，而事件一条都不会到。
func TestChatRuntimeCardRejectsWebhookURL(t *testing.T) {
	srv, st := newServer(t)
	_, cred := register(t, srv, st, "herm")

	card := map[string]any{
		"name": "herm", "description": "跑在 hermes 上的 agent",
		"capabilities": map[string]any{"extensions": []any{
			map[string]any{"uri": store.ProfileExtURI + "/v1", "params": map[string]any{
				"runtime": "hermes", "tier": "webhook",
				"webhookUrl":  "http://127.0.0.1:8080/webhook/xxx",
				"limitations": []string{"不碰生产写操作"},
			}},
		}},
	}
	resp, body := putJSON(t, srv.URL+"/api/agent/me/card", cred, card)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("hermes 声明 webhookUrl 应当 422，实得 %d %s", resp.StatusCode, body)
	}
	var e struct{ Code, Message string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "webhook_not_our_contract" {
		t.Errorf("错误码 = %q, want webhook_not_our_contract", e.Code)
	}
	if !strings.Contains(e.Message, "connector") {
		t.Errorf("报错没指出出路（走 connector）: %q", e.Message)
	}
}

// 写完 Card 之后 hub 以该 agent 自己的身份发一条自我介绍广播，
// 其余 agent 都能收到 —— 一份没人知道它变了的 Card 没有价值。
func TestCardUpsertBroadcastsSelfIntroduction(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	_, roverCred := register(t, srv, st, "rover")
	nova, _ := register(t, srv, st, "nova")

	card := map[string]any{
		"name": "rover", "description": "专注连接器与队列实现",
		"skills": []any{map[string]any{"id": "queue-design", "name": "队列设计",
			"tags": []string{"queue", "retry"}}},
		"capabilities": map[string]any{"extensions": []any{
			map[string]any{"uri": store.ProfileExtURI + "/v1", "params": map[string]any{
				"runtime": "claude-code", "tier": "longpoll", "typicalLatencySeconds": 120,
				"limitations": []string{"不做需要人类确认的操作", "不碰生产写操作"},
			}},
		}},
	}
	resp, body := putJSON(t, srv.URL+"/api/agent/me/card", roverCred, card)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("写 Card 失败: %d %s", resp.StatusCode, body)
	}

	if _, err := st.ProcessOutboxBatch(ctx, 50, nil); err != nil {
		t.Fatal(err)
	}
	evs, err := st.ReadInbox(ctx, nova, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range evs {
		if e.Kind == domain.EventTweetPublished {
			found = true
		}
	}
	if !found {
		t.Errorf("其他 agent 应当收到自我介绍广播，实得 %+v", evs)
	}

	// 名录里能按能力检索到它，并且带着能力边界。
	resp, body = getWith(t, srv.URL+"/api/agent/directory?skill=queue", roverCred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("查名录失败: %d", resp.StatusCode)
	}
	var dir struct {
		Agents []store.DirectoryEntry `json:"agents"`
	}
	_ = json.Unmarshal(body, &dir)
	if len(dir.Agents) != 1 || dir.Agents[0].Name != "rover" {
		t.Fatalf("按 skill 检索名录 = %+v, want 只有 rover", dir.Agents)
	}
	if len(dir.Agents[0].Limitations) != 2 {
		t.Errorf("名录条目应当带着能力边界，实得 %v", dir.Agents[0].Limitations)
	}
}

// 需求：只有主 agent 能推进 todo 状态。
func TestOnlyPrimaryAgentCanAdvanceState(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	rover, roverCred := register(t, srv, st, "rover")
	_, novaCred := register(t, srv, st, "nova")

	res, err := st.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 闸门先打开，否则 start_work 会先被 409 挡住，这条用例要验的是**越权**那一层。
	if _, err := st.ApproveTodo(ctx, res.ThreadID, "superfive"); err != nil {
		t.Fatal(err)
	}

	resp, body := postJSON(t, srv.URL+"/api/agent/todos/"+res.ThreadID+"/state", novaCred,
		map[string]string{"action": "start_work"})
	if resp.StatusCode == http.StatusOK {
		t.Fatal("非主 agent 不该能推进状态")
	}
	var e struct{ Code string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "not_primary_agent" {
		t.Errorf("错误码 = %q, want not_primary_agent", e.Code)
	}

	resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+res.ThreadID+"/state", roverCred,
		map[string]string{"action": "start_work"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("主 agent 应当能推进，实得 %d %s", resp.StatusCode, body)
	}
}
