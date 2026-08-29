package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// 建一个 agent 并让它自助注册，返回 (agentID, 凭证)。
func mkAgent(t *testing.T, srv, name string, admin *http.Client) (string, string) {
	t.Helper()
	resp, body := doJSON(t, admin, http.MethodPost, srv+"/api/admin/agents", nil,
		map[string]string{"name": name, "purpose": "scope 测试"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 agent 失败: %d %s", resp.StatusCode, body)
	}
	var created struct{ AgentID string }
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	resp, body = doJSON(t, admin, http.MethodPost,
		srv+"/api/admin/agents/"+created.AgentID+"/registration-token", nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("签 token 失败: %d %s", resp.StatusCode, body)
	}
	var tok struct{ RegistrationToken string }
	if err := json.Unmarshal(body, &tok); err != nil {
		t.Fatal(err)
	}
	resp, body = postJSON(t, srv+"/api/agent/register", "",
		map[string]string{"registrationToken": tok.RegistrationToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("注册失败: %d %s", resp.StatusCode, body)
	}
	var reg struct{ AgentID, Credential string }
	if err := json.Unmarshal(body, &reg); err != nil {
		t.Fatal(err)
	}
	return reg.AgentID, reg.Credential
}

// getJSONAs 用 agent 凭证发 GET。token 为空则不带 Authorization 头。
func getJSONAs(t *testing.T, url, token string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, buf
}

type todoList struct {
	Todos []struct {
		ThreadID string `json:"threadId"`
		Title    string `json:"title"`
	} `json:"todos"`
}

func agentTodos(t *testing.T, srv, cred string) todoList {
	t.Helper()
	resp, body := getJSONAs(t, srv+"/api/agent/me/todos", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("拉队列失败: %d %s", resp.StatusCode, body)
	}
	var out todoList
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("解析队列失败: %v — %s", err, body)
	}
	return out
}

// 需求模块 1 的验收标准：todo 建好之后主 agent 拉自己的队列能看到它，
// 而**被 @ 的关注者拉不到** —— 关注者在 inbox 里收到 mention 事件，
// 但队列里没有这条。队列的含义是「该我做的事」，不是「和我有关的事」。
func TestAgentQueueOnlyHasMyOwnTodos(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	_, primaryCred := mkAgent(t, srv.URL, "rover", admin)
	_, watcherCred := mkAgent(t, srv.URL, "nova", admin)

	// 从 admin 列表里找出主 agent 的 id —— 顺带验了 /api/admin/agents 的响应体
	resp, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/agents", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("列 agent 失败: %d %s", resp.StatusCode, body)
	}
	var agents struct {
		Agents []struct {
			AgentID string `json:"agentId"`
			Name    string `json:"name"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("解析 agent 列表失败: %v — %s", err, body)
	}
	var roverID string
	for _, a := range agents.Agents {
		if a.Name == "rover" {
			roverID = a.AgentID
		}
	}
	if roverID == "" {
		t.Fatal("列表里没有 rover —— /api/admin/agents 的响应体和契约对不上")
	}

	resp, body = doJSON(t, admin, http.MethodPost, srv.URL+"/api/admin/todos", nil, map[string]any{
		"title": "把队列写完", "body": "@nova 帮忙看下边界情况", "primaryAgentId": roverID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 todo 失败: %d %s", resp.StatusCode, body)
	}

	if got := agentTodos(t, srv.URL, primaryCred); len(got.Todos) != 1 || got.Todos[0].Title != "把队列写完" {
		t.Fatalf("主 agent 的队列里应当有且只有这一条，得到 %+v", got.Todos)
	}
	if got := agentTodos(t, srv.URL, watcherCred); len(got.Todos) != 0 {
		t.Fatalf("被 @ 的关注者队列里不该有这条（他不是负责人），得到 %+v", got.Todos)
	}
}

// 订阅表本来就有，扇出查询也早就读它了，缺的只是写入口。
// 没有这个端点的话 subscription 永远是空的，带标签的广播一个人都收不到。
func TestSubscriptionsRoundTrip(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	_, cred := mkAgent(t, srv.URL, "rover", admin)

	resp, body := getJSONAs(t, srv.URL+"/api/agent/me/subscriptions", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("读订阅失败: %d %s", resp.StatusCode, body)
	}
	var empty struct {
		Subscriptions []map[string]string `json:"subscriptions"`
	}
	if err := json.Unmarshal(body, &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.Subscriptions) != 0 {
		t.Fatalf("新 agent 不该有任何订阅，得到 %+v", empty.Subscriptions)
	}

	resp, body = putJSON(t, srv.URL+"/api/agent/me/subscriptions", cred, map[string]any{
		"subscriptions": []map[string]string{
			{"kind": "tag", "value": "queue"},
			{"kind": "tag", "value": "storage"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("写订阅失败: %d %s", resp.StatusCode, body)
	}

	// 整份覆盖，不是增量：第二次只提交一条，之前那两条就该没了。
	resp, body = putJSON(t, srv.URL+"/api/agent/me/subscriptions", cred, map[string]any{
		"subscriptions": []map[string]string{{"kind": "tag", "value": "queue"}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("覆盖订阅失败: %d %s", resp.StatusCode, body)
	}
	var after struct {
		Subscriptions []map[string]string `json:"subscriptions"`
	}
	if err := json.Unmarshal(body, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Subscriptions) != 1 || after.Subscriptions[0]["value"] != "queue" {
		t.Fatalf("整份覆盖后应当只剩 queue，得到 %+v", after.Subscriptions)
	}
}

func TestSubscriptionsRejectBadKind(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	_, cred := mkAgent(t, srv.URL, "rover", admin)

	resp, body := putJSON(t, srv.URL+"/api/agent/me/subscriptions", cred, map[string]any{
		"subscriptions": []map[string]string{{"kind": "everything", "value": "x"}},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("kind 只能是 tag/agent，应当 400，得到 %d %s", resp.StatusCode, body)
	}
}

// 控制台带的是会话 cookie，不是 Bearer。少了 admin 侧这条路由，
// 前端只能去打 agent 侧端点然后被 401 挡回来。
func TestAdminCanReadThreadWithSessionCookie(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	_, _ = mkAgent(t, srv.URL, "rover", admin)

	resp, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/agents", nil, nil)
	var agents struct {
		Agents []struct {
			AgentID string `json:"agentId"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(body, &agents); err != nil || len(agents.Agents) == 0 {
		t.Fatalf("拿不到 agent 列表: %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, admin, http.MethodPost, srv.URL+"/api/admin/todos", nil, map[string]any{
		"title": "看看线程", "body": "正文", "primaryAgentId": agents.Agents[0].AgentID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 todo 失败: %d %s", resp.StatusCode, body)
	}
	var created struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	resp, body = doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/threads/"+created.ThreadID, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("管理员用 cookie 读 thread 应当 200，得到 %d %s", resp.StatusCode, body)
	}
}

// agent 侧看板：需求模块 4 要求「双端可见」。
func TestAgentCanReadBoard(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	_, cred := mkAgent(t, srv.URL, "rover", admin)

	resp, body := getJSONAs(t, srv.URL+"/api/agent/board?groupBy=started", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent 拉看板应当 200，得到 %d %s", resp.StatusCode, body)
	}
	var out struct {
		GroupBy string `json:"groupBy"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.GroupBy != "started" || out.Date == "" {
		t.Fatalf("看板应当回显口径与日期，得到 %+v", out)
	}
}

// 这几条都是 agent 侧路由，没有凭证一律 401。
func TestNewAgentRoutesRequireCredential(t *testing.T) {
	srv, _ := newServer(t)
	for _, path := range []string{
		"/api/agent/me/todos", "/api/agent/board", "/api/agent/me/subscriptions",
	} {
		resp, _ := getJSONAs(t, srv.URL+path, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 无凭证应当 401，得到 %d", path, resp.StatusCode)
		}
	}
}

// 订阅端点存在的意义，全在这条用例里：带标签的广播只投给订阅了该标签的 agent，
// 没订阅的拉不到（需求模块 6 的验收标准）。
//
// 在补上 PUT /me/subscriptions 之前，subscription 表没有任何写入口，
// 于是这条断言的前半截根本不可能成立 —— 带标签的广播谁都收不到。
func TestTaggedBroadcastReachesOnlySubscribers(t *testing.T) {
	srv, st := newServer(t)
	admin := adminClient(t, srv.URL)
	_, subCred := mkAgent(t, srv.URL, "subscriber", admin)
	_, plainCred := mkAgent(t, srv.URL, "bystander", admin)
	_, authorCred := mkAgent(t, srv.URL, "author", admin)

	resp, body := putJSON(t, srv.URL+"/api/agent/me/subscriptions", subCred, map[string]any{
		"subscriptions": []map[string]string{{"kind": "tag", "value": "queue"}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("写订阅失败: %d %s", resp.StatusCode, body)
	}

	resp, body = postJSON(t, srv.URL+"/api/agent/tweets", authorCred, map[string]any{
		"body": "队列这块我踩了个坑，记一下", "tags": []string{"queue"},
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("发广播失败: %d %s", resp.StatusCode, body)
	}

	if _, err := st.ProcessOutboxBatch(t.Context(), 64, nil); err != nil {
		t.Fatalf("扇出失败: %v", err)
	}

	count := func(cred string) int {
		resp, body := getJSONAs(t, srv.URL+"/api/agent/me/inbox", cred)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("读 inbox 失败: %d %s", resp.StatusCode, body)
		}
		var out struct {
			Events []struct {
				Kind string `json:"kind"`
			} `json:"events"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("解析 inbox 失败: %v — %s", err, body)
		}
		n := 0
		for _, e := range out.Events {
			if e.Kind == "tweet.published" {
				n++
			}
		}
		return n
	}

	if got := count(subCred); got != 1 {
		t.Fatalf("订阅了 queue 的 agent 应当收到这条广播，收到 %d 条", got)
	}
	if got := count(plainCred); got != 0 {
		t.Fatalf("没订阅 queue 的 agent 不该收到，收到 %d 条", got)
	}
}
