package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// 需求（模块 3）：管理员在控制台上「像注册 CI runner 那样」新建 agent。
// 名字撞了是调用方能自己修的事 —— 换个名字重试 —— 所以要给它一个能读懂的 409，
// 而不是 500。500 会让前端只能显示「服务器错误」，用户根本不知道该改什么。
func TestCreateAgentRejectsDuplicateNameWith409(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]string{"name": "rover", "purpose": "连接器"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("首次创建应当成功: %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]string{"name": "rover", "purpose": "另一个"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("重名应当 409（不是 500），实得 %d %s", resp.StatusCode, body)
	}
	var e struct{ Code, Message string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "agent_name_taken" {
		t.Errorf("错误码 = %q, want agent_name_taken", e.Code)
	}
	if e.Message == "" {
		t.Error("错误要带一句人能看懂的说明")
	}
}

// 需求：名字是 @ 提及的唯一入口，字符集必须和正文里的 mention token 对齐。
// 名字里带空格的 agent 根本 @ 不到，而 @ 是平台上唯一的连接动作 ——
// 所以这条校验发生在创建时，而不是等到有人发现「怎么 at 不上他」。
func TestCreateAgentValidatesName(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	tests := []struct {
		name       string
		give       string
		wantStatus int
		wantStored string // 期望落库的名字，空表示不该创建成功
	}{
		{name: "普通名字", give: "rover", wantStatus: http.StatusCreated, wantStored: "rover"},
		{name: "下划线与连字符可用", give: "ci-runner_02", wantStatus: http.StatusCreated, wantStored: "ci-runner_02"},
		{name: "首尾空白被去掉后照常创建", give: "  nova  ", wantStatus: http.StatusCreated, wantStored: "nova"},
		{name: "空名字被拒", give: "", wantStatus: http.StatusBadRequest},
		{name: "只有空白等于空名字", give: "   ", wantStatus: http.StatusBadRequest},
		{name: "名字中间有空格：这样的名字 @ 不到", give: "code reviewer", wantStatus: http.StatusBadRequest},
		{name: "中文名同样 @ 不到", give: "巡检员", wantStatus: http.StatusBadRequest},
		{name: "超长名字被拒", give: strings.Repeat("a", 65), wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
				map[string]string{"name": tt.give, "purpose": "校验用"})
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("状态码 = %d, want %d —— %s", resp.StatusCode, tt.wantStatus, body)
			}
			if tt.wantStatus != http.StatusCreated {
				var e struct{ Message string }
				_ = json.Unmarshal(body, &e)
				if e.Message == "" {
					t.Error("被拒时要说清楚为什么，否则用户只能猜")
				}
				return
			}
			if got := agentNameByID(t, srv.URL, c, agentIDOf(t, body)); got != tt.wantStored {
				t.Errorf("落库的名字 = %q, want %q", got, tt.wantStored)
			}
		})
	}
}

// 需求：建 agent 时可以顺手把注册 token 一起签出来 —— 少一次往返，
// 也不会出现「记录建好了但 token 没签出来」那个中间态。
func TestCreateAgentCanIssueTokenInSameResponse(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	// 默认不签，保持向后兼容。
	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]any{"name": "quiet", "purpose": "不要 token"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建失败: %d %s", resp.StatusCode, body)
	}
	var plain struct {
		AgentID           string `json:"agentId"`
		RegistrationToken string `json:"registrationToken"`
	}
	_ = json.Unmarshal(body, &plain)
	if plain.RegistrationToken != "" {
		t.Error("没要求签发时不该返回 token —— issueToken 默认为 false")
	}

	resp, body = doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]any{"name": "runner", "purpose": "CI", "issueToken": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建失败: %d %s", resp.StatusCode, body)
	}
	var withToken struct {
		AgentID           string    `json:"agentId"`
		RegistrationToken string    `json:"registrationToken"`
		ExpiresAt         time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(body, &withToken); err != nil {
		t.Fatal(err)
	}
	if withToken.AgentID == "" || withToken.RegistrationToken == "" {
		t.Fatalf("issueToken=true 时要在同一个响应里给出 token: %s", body)
	}
	if !withToken.ExpiresAt.After(time.Now()) {
		t.Errorf("expiresAt = %v，应当是未来时间", withToken.ExpiresAt)
	}

	// 拿到的 token 必须真能换凭证 —— 不然这个字段就是个摆设。
	resp, body = postJSON(t, srv.URL+"/api/agent/register", "",
		map[string]string{"registrationToken": withToken.RegistrationToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("同响应里给出的 token 应当能换凭证: %d %s", resp.StatusCode, body)
	}
}

// 需求：控制台要能画出「未接入 / 已接入」两态。
// 建好的记录还没换过长期凭证，它就是 pending_registration；
// 换完才是 active。建完就写 active 的话这两态在数据上根本区分不出来。
func TestAgentStatusDistinguishesRegisteredFromPending(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]any{"name": "pending", "purpose": "还没接进来", "issueToken": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建失败: %d %s", resp.StatusCode, body)
	}
	var created struct {
		AgentID           string `json:"agentId"`
		RegistrationToken string `json:"registrationToken"`
	}
	_ = json.Unmarshal(body, &created)

	if got := agentStatusByID(t, srv.URL, c, created.AgentID); got != "pending_registration" {
		t.Errorf("刚建好的 agent status = %q, want pending_registration", got)
	}

	resp, body = postJSON(t, srv.URL+"/api/agent/register", "",
		map[string]string{"registrationToken": created.RegistrationToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("注册失败: %d %s", resp.StatusCode, body)
	}
	if got := agentStatusByID(t, srv.URL, c, created.AgentID); got != "active" {
		t.Errorf("换过凭证之后 status = %q, want active", got)
	}
}

// 需求（模块 3 与模块 7）：创建 agent、签发 token 都要留审计。
func TestCreateAgentWithTokenIsAudited(t *testing.T) {
	srv, st := newServer(t)
	c := adminClient(t, srv.URL)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/agents", nil,
		map[string]any{"name": "audited", "purpose": "审计", "issueToken": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建失败: %d %s", resp.StatusCode, body)
	}
	for _, action := range []string{"create_agent", "issue_registration_token"} {
		var n int
		if err := st.DB().QueryRowContext(t.Context(),
			`SELECT count(*) FROM audit_log WHERE action = $1`, action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("审计里 %s = %d 条, want 1", action, n)
		}
	}
}

// —— 小工具 ——

func agentIDOf(t *testing.T, body []byte) string {
	t.Helper()
	var created struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("解析创建响应失败: %v — %s", err, body)
	}
	return created.AgentID
}

type adminAgentRow struct {
	AgentID   string    `json:"agentId"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

func adminAgents(t *testing.T, base string, c *http.Client) []adminAgentRow {
	t.Helper()
	resp, body := doJSON(t, c, http.MethodGet, base+"/api/admin/agents", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("列 agent 失败: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Agents []adminAgentRow `json:"agents"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("解析 agent 列表失败: %v — %s", err, body)
	}
	return out.Agents
}

func agentRowByID(t *testing.T, base string, c *http.Client, id string) adminAgentRow {
	t.Helper()
	for _, a := range adminAgents(t, base, c) {
		if a.AgentID == id {
			return a
		}
	}
	t.Fatalf("agent 列表里找不到 %s", id)
	return adminAgentRow{}
}

func agentNameByID(t *testing.T, base string, c *http.Client, id string) string {
	t.Helper()
	return agentRowByID(t, base, c, id).Name
}

func agentStatusByID(t *testing.T, base string, c *http.Client, id string) string {
	t.Helper()
	return agentRowByID(t, base, c, id).Status
}
