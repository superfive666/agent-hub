package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// createAgentFor 建一个 agent，返回 id 和注册 token（issueToken=false 时后者为空）。
// 三个管理接口的用例都从这里起步。
func createAgentFor(t *testing.T, c *http.Client, base, name string, issueToken bool) (string, string) {
	t.Helper()
	resp, body := doJSON(t, c, http.MethodPost, base+"/api/admin/agents", nil,
		map[string]any{"name": name, "purpose": "初始简介", "issueToken": issueToken})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 agent 失败: %d %s", resp.StatusCode, body)
	}
	var out struct {
		AgentID           string `json:"agentId"`
		RegistrationToken string `json:"registrationToken"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out.AgentID, out.RegistrationToken
}

// exchangeToken 把注册 token 换成长期凭证明文，之后就能以 agent 身份发请求。
func exchangeToken(t *testing.T, base, regToken string) string {
	t.Helper()
	resp, body := postJSON(t, base+"/api/agent/register", "",
		map[string]string{"registrationToken": regToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("换凭证失败: %d %s", resp.StatusCode, body)
	}
	var out struct{ Credential string }
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out.Credential
}

// agentRow 从 /api/admin/agents 里挑出一行。用列表接口而不是直接查库 ——
// 断言的是「控制台看到什么」，那才是这些接口存在的意义。
func agentRow(t *testing.T, c *http.Client, base, agentID string) map[string]any {
	t.Helper()
	resp, body := doJSON(t, c, http.MethodGet, base+"/api/admin/agents", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("列 agent 失败: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	for _, a := range out.Agents {
		if a["agentId"] == agentID {
			return a
		}
	}
	return nil
}

// 需求：已有的 agent 要能改简介。
func TestUpdateAgentPurpose(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"purpose": "改过的简介"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("改简介应当 200: %d %s", resp.StatusCode, body)
	}
	if got := agentRow(t, c, srv.URL, id)["purpose"]; got != "改过的简介" {
		t.Errorf("purpose = %v, want 改过的简介", got)
	}
}

// 需求：**名字不许改** —— 它是 @ 提及的唯一标识，改掉会让历史正文里的
// @old-name 静默失效。所以请求体里带 name 也不该有任何效果，
// 而不是「悄悄改掉」或者「报一个含糊的 500」。
func TestUpdateAgentIgnoresNameChange(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"name": "rover2", "purpose": "只有简介该生效"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("应当 200: %d %s", resp.StatusCode, body)
	}
	row := agentRow(t, c, srv.URL, id)
	if row["name"] != "rover" {
		t.Errorf("名字被改了：name = %v, want rover —— 改名会让历史 @ 全部失效", row["name"])
	}
	if row["purpose"] != "只有简介该生效" {
		t.Errorf("简介没生效: %v", row["purpose"])
	}
}

// 需求：只传 purpose 时不能顺手把 agent 停掉。
// enabled 用指针正是为了这条：零值 false 和「没传」必须分得开。
func TestUpdatePurposeDoesNotDisableAgent(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"purpose": "只改简介"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("改简介应当 200: %d %s", resp.StatusCode, body)
	}
	if got := agentRow(t, c, srv.URL, id)["status"]; got != "pending_registration" {
		t.Errorf("status = %v, want pending_registration —— 只改简介不该动状态", got)
	}
}

// 需求：停用要**立刻**让这个 agent 的凭证失效，不能只是界面上的一个标签。
// 这条和「吊销凭证」的区别是可逆：重新启用后原凭证继续能用，不必重新注册。
func TestDisableAgentKillsCredentialAndEnableRestoresIt(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, regToken := createAgentFor(t, c, srv.URL, "rover", true)

	// 走完注册换成长期凭证，这时它才是真正接入了的 active
	tok := exchangeToken(t, srv.URL, regToken)

	resp, _ := getWith(t, srv.URL+"/api/agent/me/inbox?after=0&limit=1", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("停用前应当能拉 inbox，实得 %d", resp.StatusCode)
	}

	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"enabled": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("停用应当 200: %d %s", resp.StatusCode, body)
	}
	var out struct{ Status string }
	_ = json.Unmarshal(body, &out)
	if out.Status != "disabled" {
		t.Errorf("status = %q, want disabled", out.Status)
	}

	resp, _ = getWith(t, srv.URL+"/api/agent/me/inbox?after=0&limit=1", tok)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("停用后凭证应当立刻失效（401），实得 %d —— 否则「停用」只是个标签", resp.StatusCode)
	}

	resp, body = doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"enabled": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("启用应当 200: %d %s", resp.StatusCode, body)
	}
	_ = json.Unmarshal(body, &out)
	if out.Status != "active" {
		t.Errorf("换过凭证的 agent 启用后 status = %q, want active", out.Status)
	}
	resp, _ = getWith(t, srv.URL+"/api/agent/me/inbox?after=0&limit=1", tok)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("启用后原凭证应当继续可用（这正是它和吊销的区别），实得 %d", resp.StatusCode)
	}
}

// 需求：没换过凭证的 agent 停用再启用，**不能**变成 active ——
// 它一次都没接进来过，显示成「已接入」是在骗人。
func TestEnableNeverRegisteredAgentGoesBackToPending(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	_, _ = doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"enabled": false})
	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"enabled": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("启用应当 200: %d %s", resp.StatusCode, body)
	}
	var out struct{ Status string }
	_ = json.Unmarshal(body, &out)
	if out.Status != "pending_registration" {
		t.Errorf("status = %q, want pending_registration —— 从没换过凭证不该显示成已接入", out.Status)
	}
}

// 需求：干净的 agent（建错名字、还没接入）能直接删掉。
func TestDeleteUnusedAgent(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "typo-name", true)

	resp, body := doJSON(t, c, http.MethodDelete, srv.URL+"/api/admin/agents/"+id, nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("没有留痕的 agent 应当能删: %d %s", resp.StatusCode, body)
	}
	if row := agentRow(t, c, srv.URL, id); row != nil {
		t.Errorf("删完还在列表里: %v", row)
	}
}

// 需求：背着 todo 的 agent **不能**物理删除。
//
// `todo.primary_agent_id NOT NULL REFERENCES agent(id)` 是硬约束 ——
// 「一条 todo 必须有且只有一个主 agent」。删掉它要么撞外键（500，看起来像 bug），
// 要么让那条 todo 失去主责人，而后者正是这条约束要防的事。
// 所以这里必须是一个能读懂的 409，并且**告诉调用方卡在哪、该改用什么**。
func TestDeleteAgentWithTodosIsRejectedWithCounts(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	resp, body := doJSON(t, c, http.MethodPost, srv.URL+"/api/admin/todos", nil,
		map[string]any{"title": "一件事", "body": "正文", "primaryAgentId": id})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 todo 失败: %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, c, http.MethodDelete, srv.URL+"/api/admin/agents/"+id, nil, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("背着 todo 的 agent 应当 409（不是 500 也不是删成功），实得 %d %s",
			resp.StatusCode, body)
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Refs    struct {
			Todos, Tweets, Steps int
		} `json:"refs"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != "agent_in_use" {
		t.Errorf("code = %q, want agent_in_use", e.Code)
	}
	if e.Refs.Todos != 1 {
		t.Errorf("refs.todos = %d, want 1 —— 要告诉调用方到底卡在哪", e.Refs.Todos)
	}
	if e.Message == "" {
		t.Error("要给一句人能看懂的说明，并指出改用停用")
	}

	// 删不掉，但停用一定要走得通 —— 否则用户就没有任何出路了
	resp, body = doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{"enabled": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("删不掉时至少要能停用: %d %s", resp.StatusCode, body)
	}
}

// 需求：对不存在的 agent 操作要 404，不是 500。
func TestManageMissingAgentIs404(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	const missing = "00000000-0000-0000-0000-000000000000"

	for _, tc := range []struct {
		name, method string
		body         any
	}{
		{"改简介", http.MethodPatch, map[string]any{"purpose": "x"}},
		{"停用", http.MethodPatch, map[string]any{"enabled": false}},
		{"删除", http.MethodDelete, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, c, tc.method, srv.URL+"/api/admin/agents/"+missing, nil, tc.body)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("应当 404，实得 %d %s", resp.StatusCode, body)
			}
		})
	}
}

// 需求：PATCH 什么都不带是没意义的请求，要 400 —— 静默返回 200
// 会让前端以为改成功了。
func TestUpdateAgentWithEmptyBodyIs400(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, _ := createAgentFor(t, c, srv.URL, "rover", false)

	resp, body := doJSON(t, c, http.MethodPatch, srv.URL+"/api/admin/agents/"+id, nil,
		map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("空 PATCH 应当 400，实得 %d %s", resp.StatusCode, body)
	}
}
