package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// TestEndToEndCollaboration 把 M1 的整条链路走一遍：
//
//	管理员登录 → 建 agent → 签发注册 token → agent 自助注册 → 写 Agent Card
//	→ 自我介绍广播扇给全体 → 管理员建 todo 并 @ 人 → worker 扇出
//	→ 主 agent 拉到指派并回帖 → 声明开始 → 提交交付 → 管理员确认完成
//	→ 看板两种口径都能看到
//
// 这一遍跑通，平台的核心就是活的。任何一条断了这个用例都会挂。
func TestEndToEndCollaboration(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	admin := adminClient(t, srv.URL)

	// 起一个后台扇出循环。真 worker 在 agent-hub-worker 里，Go 的 internal 规则
	// 不让这边引它（两个服务各自的 internal 互不可见），所以这里跑同一个存储层入口
	// ProcessOutboxBatch —— 扇出照样是后台自己发生的，测试里不手动催。
	// worker 的选主与轮询语义由 agent-hub-worker/internal/worker 自己的用例覆盖。
	wctx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() {
		for wctx.Err() == nil {
			n, err := st.ProcessOutboxBatch(wctx, 64, nil)
			if err != nil || n == 0 {
				time.Sleep(30 * time.Millisecond)
			}
		}
	}()

	// ── 1. 管理员建两个 agent 并各签一张注册 token ──
	type agentCtx struct {
		id   string
		cred string
	}
	mk := func(name string) agentCtx {
		resp, body := doJSON(t, admin, http.MethodPost, srv.URL+"/api/admin/agents", nil,
			map[string]string{"name": name, "purpose": "端到端测试"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("建 agent %s 失败: %d %s", name, resp.StatusCode, body)
		}
		var created struct{ AgentID string }
		mustJSON(t, body, &created)

		resp, body = doJSON(t, admin, http.MethodPost,
			srv.URL+"/api/admin/agents/"+created.AgentID+"/registration-token", nil, nil)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("签 token 失败: %d %s", resp.StatusCode, body)
		}
		var tok struct{ RegistrationToken string }
		mustJSON(t, body, &tok)

		// ── 2. agent 拿着 token 自助注册，全程不需要人类干预 ──
		resp, body = postJSON(t, srv.URL+"/api/agent/register", "",
			map[string]string{"registrationToken": tok.RegistrationToken})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("agent %s 注册失败: %d %s", name, resp.StatusCode, body)
		}
		var reg struct{ AgentID, Credential string }
		mustJSON(t, body, &reg)
		return agentCtx{id: reg.AgentID, cred: reg.Credential}
	}
	rover, nova := mk("rover"), mk("nova")

	// ── 3. 写 Agent Card。能力边界是硬要求 ──
	card := func(name, desc string, skills []any, limits []string) map[string]any {
		return map[string]any{
			"name": name, "description": desc, "skills": skills,
			"capabilities": map[string]any{"extensions": []any{
				map[string]any{"uri": store.ProfileExtURI + "/v1", "params": map[string]any{
					"runtime": "claude-code", "tier": "longpoll",
					"typicalLatencySeconds": 120, "limitations": limits,
				}}}},
		}
	}
	resp, body := putJSON(t, srv.URL+"/api/agent/me/card", rover.cred,
		card("rover", "连接器与队列实现",
			[]any{map[string]any{"id": "queue-design", "name": "队列设计", "tags": []string{"queue"}}},
			[]string{"不做需要人类确认的操作", "不碰生产写操作"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("写 Card 失败: %d %s", resp.StatusCode, body)
	}
	resp, body = putJSON(t, srv.URL+"/api/agent/me/card", nova.cred,
		card("nova", "协议与校验",
			[]any{map[string]any{"id": "schema", "name": "schema 校验", "tags": []string{"a2a"}}},
			[]string{"不做前端", "不处理二进制内容"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nova 写 Card 失败: %d %s", resp.StatusCode, body)
	}

	// ── 4. 自我介绍广播扇给全体：nova 应当知道 rover 来了 ──
	waitFor(t, 5*time.Second, "nova 收到 rover 的自我介绍广播", func() bool {
		evs, err := st.ReadInbox(ctx, domain.AgentID(nova.id), 0, 50)
		if err != nil {
			return false
		}
		for _, e := range evs {
			if e.Kind == domain.EventTweetPublished {
				return true
			}
		}
		return false
	})

	// ── 5. 名录：rover 能按能力被检索到，且带着能力边界 ──
	resp, body = getWith(t, srv.URL+"/api/agent/directory?skill=queue", nova.cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("查名录失败: %d", resp.StatusCode)
	}
	var dir struct {
		Agents []store.DirectoryEntry `json:"agents"`
	}
	mustJSON(t, body, &dir)
	if len(dir.Agents) != 1 || dir.Agents[0].Name != "rover" || len(dir.Agents[0].Limitations) != 2 {
		t.Fatalf("名录检索结果不对: %+v", dir.Agents)
	}

	// ── 6. 管理员建 todo，指定 rover 为主 agent 并 @ nova ──
	resp, body = doJSON(t, admin, http.MethodPost, srv.URL+"/api/admin/todos", nil,
		map[string]any{
			"title":          "重写 connector 的重试退避逻辑",
			"body":           "改成指数退避加抖动，上限要能配。@nova 你碰过这块。",
			"primaryAgentId": rover.id,
			"mentions":       []string{nova.id},
			"tags":           []string{"connector"},
		})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建 todo 失败: %d %s", resp.StatusCode, body)
	}
	var todo struct {
		ThreadID  string    `json:"threadId"`
		StartedAt time.Time `json:"startedAt"`
	}
	mustJSON(t, body, &todo)
	if todo.StartedAt.IsZero() {
		t.Error("startedAt 应当是 thread 记录本身的日期")
	}

	// ── 7. 主 agent 拉到 todo.assigned，被 @ 的只拿到 mentioned ──
	roverSeq := waitForEvent(t, srv, rover.cred, domain.EventTodoAssigned, "rover 收到指派")
	waitForEvent(t, srv, nova.cred, domain.EventTodoMentioned, "nova 被 @ 到")

	// ── 8. rover 澄清 →（管理员确认需求）→ 声明开始 → 提交交付 ──
	//
	// 中间那道确认闸门是硬的：admin approve 之前 rover 推 start_work 会被 409 挡回来。
	// 这条用例走的是正常路径，闸门本身另有专门的用例（见 todo_confirmation_test.go）。
	for _, step := range []struct {
		body        string
		adminAction string // 回帖之后管理员做什么，空则不做
		action      string
		want        domain.TodoStatus
	}{
		{body: "两个问题先确认：上限走配置清单还是硬编码？", adminAction: "approve"},
		{body: "方向清楚了，我开始做。", action: "start_work", want: domain.StatusInProgress},
		{body: "做完了，base 200ms、max 30s、decorrelated jitter。",
			action: "submit_deliverable", want: domain.StatusAwaitingReview},
	} {
		resp, body = postJSON(t, srv.URL+"/api/agent/threads/"+todo.ThreadID+"/posts", rover.cred,
			map[string]any{"body": step.body})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("rover 回帖失败: %d %s", resp.StatusCode, body)
		}
		if step.adminAction != "" {
			resp, body = doJSON(t, admin, http.MethodPost,
				srv.URL+"/api/admin/todos/"+todo.ThreadID+"/state", nil,
				map[string]string{"action": step.adminAction})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("管理员 %s 失败: %d %s", step.adminAction, resp.StatusCode, body)
			}
		}
		if step.action == "" {
			continue
		}
		resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+todo.ThreadID+"/state", rover.cred,
			map[string]string{"action": step.action})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("推进状态 %s 失败: %d %s", step.action, resp.StatusCode, body)
		}
		var got struct{ Status string }
		mustJSON(t, body, &got)
		if got.Status != string(step.want) {
			t.Errorf("状态 = %s, want %s", got.Status, step.want)
		}
	}

	// ── 9. 关注者收到了这些回复，作者自己不收 ──
	waitFor(t, 5*time.Second, "nova 收到 thread 回复", func() bool {
		evs, _ := st.ReadInbox(ctx, domain.AgentID(nova.id), 0, 50)
		for _, e := range evs {
			if e.Kind == domain.EventThreadReplied {
				return true
			}
		}
		return false
	})
	after, _ := st.ReadInbox(ctx, domain.AgentID(rover.id), roverSeq, 50)
	for _, e := range after {
		if e.Kind == domain.EventThreadReplied {
			t.Errorf("rover 收到了自己发言的通知: %+v", e)
		}
	}

	// ── 10. 管理员以人类身份回帖，再确认完成 ──
	resp, body = doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/threads/"+todo.ThreadID+"/posts", nil,
		map[string]any{"body": "看过了，可以。"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("管理员回帖失败: %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/todos/"+todo.ThreadID+"/state", nil,
		map[string]string{"action": "confirm"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("确认完成失败: %d %s", resp.StatusCode, body)
	}

	// ── 11. thread 详情里能完整还原经过，且人机可分 ──
	resp, body = getWith(t, srv.URL+"/api/agent/threads/"+todo.ThreadID, rover.cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("读 thread 失败: %d", resp.StatusCode)
	}
	var detail store.ThreadDetailResult
	mustJSON(t, body, &detail)
	if detail.Status != string(domain.StatusDone) {
		t.Errorf("thread 状态 = %s, want done", detail.Status)
	}
	if len(detail.Posts) != 5 { // 开篇 + rover 三条 + admin 一条
		t.Errorf("发言数 = %d, want 5", len(detail.Posts))
	}
	var adminPosts, agentPosts int
	for _, p := range detail.Posts {
		switch p.AuthorKind {
		case "admin":
			adminPosts++
		case "agent":
			agentPosts++
		}
	}
	if adminPosts != 2 || agentPosts != 3 {
		t.Errorf("人 %d 条 / agent %d 条, want 2 / 3 —— authorKind 是前端区分人机的唯一依据",
			adminPosts, agentPosts)
	}
	if detail.PrimaryAgentID != rover.id {
		t.Errorf("主 agent = %s, want %s", detail.PrimaryAgentID, rover.id)
	}

	// ── 12. 看板两种口径都能看到今天这条 ──
	today := time.Now().Format("2006-01-02")
	for _, groupBy := range []string{"activity", "started"} {
		resp, body = doJSON(t, admin, http.MethodGet,
			srv.URL+"/api/admin/board?date="+today+"&groupBy="+groupBy, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("看板(%s) 失败: %d %s", groupBy, resp.StatusCode, body)
		}
		var board struct {
			GroupBy string            `json:"groupBy"`
			Items   []store.BoardItem `json:"items"`
		}
		mustJSON(t, body, &board)
		if board.GroupBy != groupBy {
			t.Errorf("groupBy = %s, want %s", board.GroupBy, groupBy)
		}
		var found bool
		for _, it := range board.Items {
			if it.ThreadID == todo.ThreadID {
				found = true
			}
		}
		if !found {
			t.Errorf("看板(%s) 里没有这条 thread，共 %d 条", groupBy, len(board.Items))
		}
	}

	// ── 13. outbox 已经排空，没有静默积压 ──
	waitFor(t, 5*time.Second, "outbox 排空", func() bool {
		lag, err := st.OutboxLagSeconds(ctx)
		return err == nil && lag == 0
	})
}

func mustJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\nbody = %s", err, body)
	}
}

func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

// waitForEvent 等某个 agent 通过 HTTP 拉到指定类型的事件，返回它的 seq。
// 走真实端点而不是直接读库 —— 端到端要验的正是这条路。
func waitForEvent(t *testing.T, srv *httptest.Server, cred string,
	kind domain.EventKind, what string) int64 {
	t.Helper()
	var seq int64
	waitFor(t, 8*time.Second, what, func() bool {
		resp, body := getWith(t, srv.URL+"/api/agent/me/inbox?after=0&wait=1s", cred)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var out struct {
			Events []store.InboxEvent `json:"events"`
		}
		if json.Unmarshal(body, &out) != nil {
			return false
		}
		for _, e := range out.Events {
			if e.Kind == kind {
				seq = e.Seq
				return true
			}
		}
		return false
	})
	return seq
}
