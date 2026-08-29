package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// mkTodoFor 让管理员建一条 todo 指定 primary 为主 agent，返回 thread id。
func mkTodoFor(t *testing.T, base string, admin *http.Client, primaryID, title string) string {
	t.Helper()
	resp, body := doJSON(t, admin, http.MethodPost, base+"/api/admin/todos", nil, map[string]any{
		"title": title, "body": "正文", "primaryAgentId": primaryID,
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
	return created.ThreadID
}

func patchJSON(t *testing.T, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, req)
}

// 需求：用户没确认之前，agent 不能把这条 todo 往下推。
// 报错要让 agent 自己读懂下一步该干什么，而不是盲目重试。
func TestAgentCannotAdvanceBeforeAdminApproval(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)

	for _, action := range []string{"start_work", "submit_deliverable"} {
		threadID := mkTodoFor(t, srv.URL, admin, roverID, "未确认就想开工")
		resp, body := postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/state", roverCred,
			map[string]string{"action": action})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("%s 在未确认时应当 409，实得 %d %s", action, resp.StatusCode, body)
		}
		var e struct {
			Code, Message string
			Retryable     bool
		}
		_ = json.Unmarshal(body, &e)
		if e.Code != "todo_not_confirmed" {
			t.Errorf("错误码 = %q, want todo_not_confirmed", e.Code)
		}
		if e.Message == "" {
			t.Error("要告诉 agent「等管理员确认需求」，不能只给个状态码")
		}
		if e.Retryable {
			t.Error("这不是等一会儿就好的失败，标成可重试会让 agent 空转")
		}
	}
}

// 需求：未确认之前 agent 仍然可以在这条 task 上回复、提问、要更多澄清信息。
// 闸门挡的是「往下做」，不是「说话」。
func TestAgentCanStillClarifyBeforeApproval(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "先问清楚")

	// ① 发帖提问
	resp, body := postJSON(t, srv.URL+"/api/agent/threads/"+threadID+"/posts", roverCred,
		map[string]any{"body": "上限走配置还是硬编码？"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("未确认时应当照常能回帖，实得 %d %s", resp.StatusCode, body)
	}

	// ② 把状态设成 clarifying
	resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/state", roverCred,
		map[string]string{"action": "clarify"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("未确认时应当能进入澄清中，实得 %d %s", resp.StatusCode, body)
	}
	var got struct{ Status string }
	_ = json.Unmarshal(body, &got)
	if got.Status != string(domain.StatusClarifying) {
		t.Errorf("status = %q, want clarifying", got.Status)
	}

	// ③ 追加 clarification 类型的步骤
	resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", roverCred,
		map[string]any{"kind": "clarification", "title": "问了两个问题", "detail": "等回复"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("未确认时应当能追加澄清步骤，实得 %d %s", resp.StatusCode, body)
	}
}

// 需求：管理员 approve 之后，主 agent 才能继续；同时它要收到放行事件。
func TestApproveOpensTheGateAndNotifiesPrimaryAgent(t *testing.T) {
	srv, st := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "确认后开工")

	resp, body := doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": "approve"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve 失败: %d %s", resp.StatusCode, body)
	}
	var approved struct {
		Status           string `json:"status"`
		ConfirmedBy      string `json:"confirmedBy"`
		AlreadyConfirmed bool   `json:"alreadyConfirmed"`
	}
	_ = json.Unmarshal(body, &approved)
	if approved.Status != string(domain.StatusInProgress) {
		t.Errorf("approve 后 status = %q, want in_progress", approved.Status)
	}
	if approved.ConfirmedBy != "superfive" {
		t.Errorf("confirmedBy = %q, want superfive", approved.ConfirmedBy)
	}

	// 主 agent 拉得到放行信号 —— 收不到的话它会一直被闸门挡着，这条 todo 静默死掉。
	if _, err := st.ProcessOutboxBatch(t.Context(), 64, nil); err != nil {
		t.Fatal(err)
	}
	resp, body = getJSONAs(t, srv.URL+"/api/agent/me/inbox", roverCred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("读 inbox 失败: %d %s", resp.StatusCode, body)
	}
	var inbox struct {
		Events []struct {
			Kind     string `json:"kind"`
			Priority int    `json:"priority"`
		} `json:"events"`
	}
	_ = json.Unmarshal(body, &inbox)
	var found bool
	for _, e := range inbox.Events {
		if e.Kind == string(domain.EventTodoApproved) {
			found = true
			if e.Priority != 0 {
				t.Errorf("todo.approved 的优先级 = %d, want 0", e.Priority)
			}
		}
	}
	if !found {
		t.Fatalf("主 agent 应当收到 todo.approved，实得 %+v", inbox.Events)
	}

	// 闸门开了，开工与交付都通了。
	for _, action := range []string{"start_work", "submit_deliverable"} {
		resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/state", roverCred,
			map[string]string{"action": action})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("确认之后 %s 应当成功，实得 %d %s", action, resp.StatusCode, body)
		}
	}
}

// 需求：重复 approve 要幂等 —— 不报错、不重复发事件。
func TestApproveIsIdempotentOverHTTP(t *testing.T) {
	srv, st := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, _ := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "点两下确认")

	var confirmedAt [2]string
	for i := range 2 {
		resp, body := doJSON(t, admin, http.MethodPost,
			srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": "approve"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("第 %d 次 approve 失败: %d %s", i+1, resp.StatusCode, body)
		}
		var out struct {
			ConfirmedAt      string `json:"confirmedAt"`
			AlreadyConfirmed bool   `json:"alreadyConfirmed"`
		}
		_ = json.Unmarshal(body, &out)
		confirmedAt[i] = out.ConfirmedAt
		if (i == 1) != out.AlreadyConfirmed {
			t.Errorf("第 %d 次 alreadyConfirmed = %v", i+1, out.AlreadyConfirmed)
		}
	}
	if confirmedAt[0] != confirmedAt[1] {
		t.Errorf("确认时间被改写: %s → %s", confirmedAt[0], confirmedAt[1])
	}

	var n int
	if err := st.DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM outbox_event WHERE kind = 'todo.approved'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("todo.approved 事件 = %d 条, want 1", n)
	}
}

// 需求：reject 只用于 awaiting_review 打回。未确认阶段管理员靠发帖 + 不 approve 表达，
// 那条路径本来就有通知、有留痕，不需要第二套语义模糊的按钮。
func TestRejectOnlyAppliesToSubmittedWork(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "还没交东西")

	resp, body := doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": "reject"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("未提交交付时打回应当 409，实得 %d %s", resp.StatusCode, body)
	}
	var e struct{ Code, Message string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "invalid_todo_transition" || e.Message == "" {
		t.Errorf("错误 = %+v, want invalid_todo_transition 且带说明", e)
	}

	// 走完确认 → 开工 → 交付，再打回就通了。
	for _, step := range []struct{ who, action string }{
		{"admin", "approve"}, {"agent", "start_work"}, {"agent", "submit_deliverable"},
	} {
		if step.who == "admin" {
			resp, body = doJSON(t, admin, http.MethodPost,
				srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": step.action})
		} else {
			resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/state", roverCred,
				map[string]string{"action": step.action})
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s 失败: %d %s", step.who, step.action, resp.StatusCode, body)
		}
	}
	resp, body = doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": "reject"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("待确认的 todo 应当能打回，实得 %d %s", resp.StatusCode, body)
	}
}

// 需求：处理过程记录在「任务处理详情步骤」里。
// 只有主 agent 能写，关注者只读；步骤按 seq 升序返回；admin 侧和 agent 侧读的是同一份。
func TestTodoStepsWriteScopeAndOrdering(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)
	_, novaCred := mkAgent(t, srv.URL, "nova", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "记录处理过程")

	// 关注者写不了 —— agent 只能操作属于自己的资源。
	resp, body := postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", novaCred,
		map[string]any{"kind": "progress", "title": "我也来记一笔"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("非主 agent 写步骤应当 403，实得 %d %s", resp.StatusCode, body)
	}
	var e struct{ Code string }
	_ = json.Unmarshal(body, &e)
	if e.Code != "not_primary_agent" {
		t.Errorf("错误码 = %q, want not_primary_agent", e.Code)
	}

	// confirmation 是管理员的确认动作，agent 写不了 —— 否则等于给了它自己放行的入口。
	resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", roverCred,
		map[string]any{"kind": "confirmation", "title": "我批准我自己"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("agent 写 confirmation 步骤应当被拒，实得 %d %s", resp.StatusCode, body)
	}

	// 主 agent 依次追加三条，seq 从 1 开始连续。
	titles := []string{"问清楚需求", "打算怎么做", "做完了"}
	kinds := []string{"clarification", "plan", "progress"}
	var firstStepID string
	for i, title := range titles {
		resp, body = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", roverCred,
			map[string]any{"kind": kinds[i], "title": title})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("追加步骤失败: %d %s", resp.StatusCode, body)
		}
		var st store.TodoStepRow
		if err := json.Unmarshal(body, &st); err != nil {
			t.Fatal(err)
		}
		if st.Seq != i+1 {
			t.Errorf("第 %d 条步骤的 seq = %d, want %d", i, st.Seq, i+1)
		}
		if i == 0 {
			firstStepID = st.ID
		}
	}

	// 关注者读得到 —— 想帮上忙就得先看得见别人做到哪儿了。
	if got := listSteps(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", novaCred, nil); len(got) != 3 {
		t.Errorf("关注者读到的步骤数 = %d, want 3", len(got))
	}
	// admin 侧读的是同一份。
	adminSteps := listSteps(t, srv.URL+"/api/admin/todos/"+threadID+"/steps", "", admin)
	if len(adminSteps) != 3 {
		t.Fatalf("管理员读到的步骤数 = %d, want 3", len(adminSteps))
	}
	for i, st := range adminSteps {
		if st.Seq != i+1 || st.Title != titles[i] {
			t.Fatalf("步骤没有按 seq 升序返回: %+v", adminSteps)
		}
		if st.ActorKind != "agent" || st.ActorName != "rover" {
			t.Errorf("步骤要能看出是谁记的: %+v", st)
		}
	}

	// 改状态：pending → done。
	resp, body = patchJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps/"+firstStepID, roverCred,
		map[string]any{"status": "done", "detail": "两个问题都问了"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("更新步骤失败: %d %s", resp.StatusCode, body)
	}
	var updated store.TodoStepRow
	_ = json.Unmarshal(body, &updated)
	if updated.Status != "done" || updated.Detail != "两个问题都问了" {
		t.Errorf("更新结果 = %+v", updated)
	}

	// 关注者改不了。
	resp, _ = patchJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps/"+firstStepID, novaCred,
		map[string]any{"status": "blocked"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("非主 agent 改步骤应当 403，实得 %d", resp.StatusCode)
	}
}

// 需求：追加步骤是过程记录，不是通知 —— 不该产生 inbox 事件。
// 真正要让别人知道的事，主 agent 在 thread 里说一句，那条路径本来就带扇出。
func TestAppendingStepsDoesNotFanOutEvents(t *testing.T) {
	srv, st := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, roverCred := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "安静地记录")

	if _, err := st.ProcessOutboxBatch(t.Context(), 64, nil); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := st.DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM outbox_event`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		resp, body := postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", roverCred,
			map[string]any{"kind": "progress", "title": "又干了一点"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("追加步骤失败: %d %s", resp.StatusCode, body)
		}
	}

	var after int
	if err := st.DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM outbox_event`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("追加步骤产生了 %d 条 outbox 事件，want 0 —— 步骤是过程记录，不是通知",
			after-before)
	}
}

// 需求：ThreadDetail 与 TodoSummary 都要带 confirmedAt，前端靠它决定画不画确认按钮。
func TestConfirmedAtSurfacedInThreadAndList(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, _ := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "确认前后")

	detail := func() store.ThreadDetailResult {
		t.Helper()
		resp, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/threads/"+threadID, nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("读 thread 失败: %d %s", resp.StatusCode, body)
		}
		var d store.ThreadDetailResult
		if err := json.Unmarshal(body, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	summary := func() store.TodoRow {
		t.Helper()
		resp, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/todos", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("读 todo 列表失败: %d %s", resp.StatusCode, body)
		}
		var out struct {
			Todos []store.TodoRow `json:"todos"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Todos) != 1 {
			t.Fatalf("todo 数 = %d, want 1", len(out.Todos))
		}
		return out.Todos[0]
	}

	if detail().ConfirmedAt != nil || summary().ConfirmedAt != nil {
		t.Fatal("确认之前 confirmedAt 必须为空 —— 否则前端会把确认按钮藏起来")
	}

	resp, body := doJSON(t, admin, http.MethodPost,
		srv.URL+"/api/admin/todos/"+threadID+"/state", nil, map[string]string{"action": "approve"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve 失败: %d %s", resp.StatusCode, body)
	}

	if detail().ConfirmedAt == nil {
		t.Error("确认之后 ThreadDetail.confirmedAt 应当非空")
	}
	if summary().ConfirmedAt == nil {
		t.Error("确认之后 TodoSummary.confirmedAt 应当非空")
	}
}

// 步骤端点也要有鉴权：没凭证一律 401。
func TestTodoStepRoutesRequireCredential(t *testing.T) {
	srv, _ := newServer(t)
	admin := adminClient(t, srv.URL)
	roverID, _ := mkAgent(t, srv.URL, "rover", admin)
	threadID := mkTodoFor(t, srv.URL, admin, roverID, "鉴权")

	resp, _ := getJSONAs(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("无凭证读步骤应当 401，实得 %d", resp.StatusCode)
	}
	resp, _ = postJSON(t, srv.URL+"/api/agent/todos/"+threadID+"/steps", "",
		map[string]any{"kind": "progress", "title": "偷偷写一条"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("无凭证写步骤应当 401，实得 %d", resp.StatusCode)
	}

	c := &http.Client{}
	resp, _ = doJSON(t, c, http.MethodGet, srv.URL+"/api/admin/todos/"+threadID+"/steps", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("未登录读 admin 侧步骤应当 401，实得 %d", resp.StatusCode)
	}
}

// listSteps 读步骤：给 token 就走 agent 侧，给 client 就走 admin 侧。
func listSteps(t *testing.T, url, token string, c *http.Client) []store.TodoStepRow {
	t.Helper()
	var body []byte
	var resp *http.Response
	if c != nil {
		resp, body = doJSON(t, c, http.MethodGet, url, nil, nil)
	} else {
		resp, body = getJSONAs(t, url, token)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("读步骤失败: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Steps []store.TodoStepRow `json:"steps"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("解析步骤失败: %v — %s", err, body)
	}
	return out.Steps
}
