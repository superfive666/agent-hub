package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/api"
	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

func newServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st := testdb.New(t)
	cfg := config.Config{
		DatabaseURL: "unused", Timezone: "UTC", AuthMode: config.AuthPassword,
		AdminUsername: "superfive", AdminPasswordHash: "hash",
		LongPollMax: 30 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置本身不合法: %v", err)
	}
	srv := httptest.NewServer(api.New(st, cfg, nil).Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

func postJSON(t *testing.T, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, req)
}

func getWith(t *testing.T, url, token string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, req)
}

func do(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.Bytes()
}

func register(t *testing.T, srv *httptest.Server, st *store.Store, name string) (domain.AgentID, string) {
	t.Helper()
	ctx := context.Background()
	id, err := st.CreateAgent(ctx, name, "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := st.IssueRegistrationToken(ctx, id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := postJSON(t, srv.URL+"/api/agent/register", "",
		map[string]string{"registrationToken": tok})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("注册失败: %d %s", resp.StatusCode, body)
	}
	var out struct{ AgentID, Credential string }
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Credential == "" {
		t.Fatal("注册没有返回长期凭证")
	}
	return id, out.Credential
}

// 需求：同一个注册 token 用第二次会被拒绝。
func TestRegistrationTokenIsSingleUse(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()

	id, err := st.CreateAgent(ctx, "rover", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := st.IssueRegistrationToken(ctx, id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	resp, _ := postJSON(t, srv.URL+"/api/agent/register", "", map[string]string{"registrationToken": tok})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("第一次注册应当成功，实得 %d", resp.StatusCode)
	}

	resp, body := postJSON(t, srv.URL+"/api/agent/register", "", map[string]string{"registrationToken": tok})
	if resp.StatusCode == http.StatusOK {
		t.Fatal("同一个注册 token 不该能用第二次")
	}
	var e api.Error
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("错误响应不是合法 JSON: %s", body)
	}
	if e.Code != "token_used" {
		t.Errorf("错误码 = %q, want token_used", e.Code)
	}
	if e.Retryable {
		t.Error("token 用过是永久失败，不该标成可重试 —— 否则 agent 会一直重试")
	}
}

// 需求：吊销凭证后 agent 的 API 调用立即失效。
func TestRevokedCredentialFailsImmediately(t *testing.T) {
	srv, st := newServer(t)
	id, cred := register(t, srv, st, "rover")

	resp, _ := getWith(t, srv.URL+"/api/agent/me/inbox", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("吊销前应当可用，实得 %d", resp.StatusCode)
	}

	if err := st.RevokeCredentials(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	resp, body := getWith(t, srv.URL+"/api/agent/me/inbox", cred)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("吊销后应当立即 401，实得 %d %s", resp.StatusCode, body)
	}
}

func TestMissingOrBogusCredentialRejected(t *testing.T) {
	srv, _ := newServer(t)
	for _, tok := range []string{"", "ahr_cred_不存在的东西"} {
		resp, _ := getWith(t, srv.URL+"/api/agent/me/inbox", tok)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("凭证 %q 应当被拒绝，实得 %d", tok, resp.StatusCode)
		}
	}
}

// 需求：有新事件时长轮询立刻返回，不用等满 wait。
func TestLongPollReturnsImmediatelyWhenEventsExist(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	id, cred := register(t, srv, st, "rover")

	if _, err := st.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: id}, CreatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	resp, body := getWith(t, srv.URL+"/api/agent/me/inbox?after=0&wait=10s", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d, body = %s", resp.StatusCode, body)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("已有事件时长轮询应当立刻返回，实际等了 %v", elapsed)
	}

	var out struct {
		Events  []store.InboxEvent `json:"events"`
		LastSeq int64              `json:"lastSeq"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 || out.Events[0].Kind != domain.EventTodoAssigned {
		t.Fatalf("事件 = %+v, want 一条 todo.assigned", out.Events)
	}
	if out.LastSeq != out.Events[0].Seq {
		t.Errorf("lastSeq = %d, want %d", out.LastSeq, out.Events[0].Seq)
	}
}

// 需求：没有新事件时 hold 到超时，返回空而不是报错。
func TestLongPollWaitsThenReturnsEmpty(t *testing.T) {
	srv, st := newServer(t)
	_, cred := register(t, srv, st, "rover")

	start := time.Now()
	resp, body := getWith(t, srv.URL+"/api/agent/me/inbox?after=0&wait=1s", cred)
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("超时返回空是正常情况，不该报错。状态码 = %d", resp.StatusCode)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("应当 hold 满 1s，实际只等了 %v", elapsed)
	}
	var out struct {
		Events []store.InboxEvent `json:"events"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 0 {
		t.Errorf("应当返回空事件列表，实得 %d 条", len(out.Events))
	}
}

// 长轮询期间产生的事件应当被捡到，而不是等到下一次请求。
func TestLongPollPicksUpEventArrivingDuringWait(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	id, cred := register(t, srv, st, "rover")

	go func() {
		time.Sleep(400 * time.Millisecond)
		if _, err := st.CreateTodo(ctx, store.CreateTodoParams{
			New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: id}, CreatedBy: "admin",
		}); err != nil {
			return
		}
		_, _ = st.ProcessOutboxBatch(ctx, 10, nil)
	}()

	start := time.Now()
	resp, body := getWith(t, srv.URL+"/api/agent/me/inbox?after=0&wait=10s", cred)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d", resp.StatusCode)
	}
	var out struct {
		Events []store.InboxEvent `json:"events"`
	}
	_ = json.Unmarshal(body, &out)
	if len(out.Events) != 1 {
		t.Fatalf("等待期间到达的事件应当被捡到，实得 %d 条", len(out.Events))
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("捡到事件后应当立刻返回，实际等了 %v", elapsed)
	}
}

// 需求：cursor 增量拉取不重不漏。
func TestInboxCursorPagination(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	id, cred := register(t, srv, st, "rover")

	for i := 0; i < 3; i++ {
		if _, err := st.CreateTodo(ctx, store.CreateTodoParams{
			New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: id}, CreatedBy: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}

	read := func(after int64) []store.InboxEvent {
		_, body := getWith(t, srv.URL+"/api/agent/me/inbox?after="+itoa(after), cred)
		var out struct {
			Events []store.InboxEvent `json:"events"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out.Events
	}
	all := read(0)
	if len(all) != 3 {
		t.Fatalf("应当拉到 3 条，实得 %d", len(all))
	}
	rest := read(all[0].Seq)
	if len(rest) != 2 || rest[0].Seq != all[1].Seq {
		t.Errorf("从 seq=%d 之后应当剩 2 条且不重复，实得 %+v", all[0].Seq, rest)
	}
}

func TestAckMovesCursor(t *testing.T) {
	srv, st := newServer(t)
	id, cred := register(t, srv, st, "rover")

	resp, body := postJSON(t, srv.URL+"/api/agent/me/inbox/ack", cred, map[string]int64{"cursor": 7})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ack 应当返回 204，实得 %d %s", resp.StatusCode, body)
	}
	got, err := st.Cursor(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Errorf("cursor = %d, want 7", got)
	}
}

// agent 回帖后，其他关注者应当收到通知，作者自己不收。
func TestAgentAppendPostFansOut(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	rover, roverCred := register(t, srv, st, "rover")
	nova, _ := register(t, srv, st, "nova")

	res, err := st.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover,
			Mentions: []domain.AgentID{nova}},
		CreatedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := st.ReadInbox(ctx, rover, 0, 50)

	resp, body := postJSON(t, srv.URL+"/api/agent/threads/"+res.ThreadID+"/posts", roverCred,
		map[string]any{"body": "我来看看"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("回帖应当 201，实得 %d %s", resp.StatusCode, body)
	}
	if _, err := st.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}

	after, _ := st.ReadInbox(ctx, rover, 0, 50)
	if len(after) != len(before) {
		t.Errorf("作者不该收到自己发言的通知：之前 %d 条，之后 %d 条", len(before), len(after))
	}
	novaEvents, _ := st.ReadInbox(ctx, nova, 0, 50)
	if len(novaEvents) != 2 {
		t.Errorf("关注者应当收到这条回复，共 2 条，实得 %d", len(novaEvents))
	}
}

func TestInternalErrorCarriesRetryHint(t *testing.T) {
	t.Parallel()
	// agent 要能自己判断能不能重试、多久后重试。
	if !api.ErrInternal.Retryable || api.ErrInternal.RetryAfter <= 0 {
		t.Error("可重试的错误必须给 RetryAfter，否则 agent 只能瞎猜，猜出来就是重试风暴")
	}
	if api.ErrUnauthorized.Retryable || api.ErrTokenUsed.Retryable {
		t.Error("永久失败不该标成可重试")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// connector 连续唤起失败后会把事件转入死信并上报。
// 这条链路断了的话，admin 永远不知道某个 agent 一直处理不了事件 —— 又是一种静默失败。
func TestDeadLetterReportIsRecordedAndIdempotent(t *testing.T) {
	srv, st := newServer(t)
	_, cred := register(t, srv, st, "rover")
	ctx := context.Background()

	report := map[string]any{"seq": 42, "kind": "todo.assigned", "attempts": 3, "error": "runtime 起不来"}
	resp, body := postJSON(t, srv.URL+"/api/agent/me/dead-letters", cred, report)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("上报死信应当 204，实得 %d %s", resp.StatusCode, body)
	}
	if n, err := st.DeadLetterCount(ctx); err != nil || n != 1 {
		t.Fatalf("死信数 = %d (err=%v), want 1", n, err)
	}

	// connector 重启后可能重报同一条，不该刷屏。
	resp, _ = postJSON(t, srv.URL+"/api/agent/me/dead-letters", cred, report)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("重复上报也应当 204，实得 %d", resp.StatusCode)
	}
	if n, _ := st.DeadLetterCount(ctx); n != 1 {
		t.Errorf("重复上报后死信数 = %d, want 1（应当幂等）", n)
	}
}

func TestDeadLetterRequiresCredential(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := postJSON(t, srv.URL+"/api/agent/me/dead-letters", "",
		map[string]any{"seq": 1, "kind": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("无凭证上报应当 401，实得 %d", resp.StatusCode)
	}
}
