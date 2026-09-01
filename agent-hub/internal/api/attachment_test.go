package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/api"
	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

// 用例按需求写：
//   1. agent 传上来的产物，人在控制台里点得开、下得下来（两侧对称）
//   2. 上传方给的东西一律不可信：文件名进不了路径、进不了响应头、类型不回显
//   3. 没配 ATTACHMENT_DIR 是正常状态，要给一个说得清楚的 503

// attachServer 起一个开了附件的 server，返回它、store 和附件目录。
func attachServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	st := testdb.New(t)
	dir := t.TempDir()
	cfg := config.Config{
		DatabaseURL: "unused", Timezone: "UTC", AuthMode: config.AuthPassword,
		AdminUsername: "superfive", AdminPasswordHash: testPasswordHash,
		SessionSecret: "test-secret-0123456789", LongPollMax: 30 * time.Second,
		AttachmentDir: dir, AttachmentMaxBytes: 1 << 20, AttachmentMaxPerPost: 3,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置本身不合法: %v", err)
	}
	srv := httptest.NewServer(api.New(st, cfg, nil).Handler())
	t.Cleanup(srv.Close)
	return srv, st, dir
}

// uploadFile 发一个 multipart 上传。filename / contentType 原样送出去 ——
// 这两个正是「上传方说了算」的不可信输入，测试必须能塞进恶意值。
func uploadFile(t *testing.T, url, token, filename, contentType string, content []byte) (*http.Response, []byte) {
	t.Helper()
	body, ct := multipartBody(t, filename, contentType, content)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", ct)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(t, req)
}

// multipartBody 拼一个 file part。**filename 与 Content-Type 原样送出去** ——
// 这两个正是「上传方说了算」的不可信输入，用例必须能往里塞恶意值。
func multipartBody(t *testing.T, filename, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition",
		`form-data; name="file"; filename="`+strings.ReplaceAll(filename, `"`, `\"`)+`"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) > 0 {
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// regAgent 建一个 agent 并注册，返回它的长期凭证。
func regAgent(t *testing.T, srv *httptest.Server, st *store.Store, name string) string {
	t.Helper()
	_, cred := register(t, srv, st, name)
	return cred
}

func decodeAttachment(t *testing.T, body []byte) store.Attachment {
	t.Helper()
	var a store.Attachment
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatalf("解上传响应: %v (%s)", err, body)
	}
	return a
}

// 端到端：agent 传产物 → 挂到帖子上 → 人在控制台里读到并下下来。
func TestAgentUploadsArtifactAndHumanDownloadsIt(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "builder")
	const content = "构建产物：一切正常\n"

	resp, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"构建报告.txt", "text/plain", []byte(content))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("上传应该 201，得到 %d: %s", resp.StatusCode, body)
	}
	up := decodeAttachment(t, body)
	if up.ID == "" || up.SizeBytes != int64(len(content)) {
		t.Fatalf("上传响应不完整: %+v", up)
	}
	if up.Filename != "构建报告.txt" {
		t.Errorf("文件名应该原样留着，得到 %q", up.Filename)
	}

	threadID := mkTweet(t, srv, token, "活干完了")
	resp, body = postJSON(t, srv.URL+"/api/agent/threads/"+threadID+"/posts", token,
		map[string]any{"body": "报告在附件里", "attachmentIds": []string{up.ID}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("带附件回帖应该 201，得到 %d: %s", resp.StatusCode, body)
	}

	// 人在控制台里看到它
	admin := adminClient(t, srv.URL)
	resp, body = doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/threads/"+threadID, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("读 thread 应该 200，得到 %d: %s", resp.StatusCode, body)
	}
	var detail struct {
		Posts []struct {
			Body        string             `json:"body"`
			Attachments []store.Attachment `json:"attachments"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("解 thread: %v", err)
	}
	last := detail.Posts[len(detail.Posts)-1]
	if len(last.Attachments) != 1 || last.Attachments[0].ID != up.ID {
		t.Fatalf("控制台该看到那个附件，得到 %+v", last.Attachments)
	}

	// 点下去真的能拿到原始字节
	resp, body = doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/attachments/"+up.ID, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("下载应该 200，得到 %d", resp.StatusCode)
	}
	if string(body) != content {
		t.Errorf("下下来的内容不一致: %q", body)
	}
}

// 反方向也要成立：人把一份规格书递给 agent。
func TestHumanUploadsAndAgentDownloads(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "worker")
	admin := adminClient(t, srv.URL)
	threadID := mkTweet(t, srv, token, "开个头")
	const spec = "规格书 v1"

	resp, body := uploadAs(t, admin, srv.URL+"/api/admin/attachments",
		"spec.md", "text/markdown", []byte(spec))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("管理员上传应该 201，得到 %d: %s", resp.StatusCode, body)
	}
	up := decodeAttachment(t, body)

	resp, body = doJSON(t, admin, http.MethodPost, srv.URL+"/api/admin/threads/"+threadID+"/posts", nil,
		map[string]any{"body": "照这个做", "attachmentIds": []string{up.ID}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("管理员带附件回帖应该 201，得到 %d: %s", resp.StatusCode, body)
	}

	resp, body = getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent 下载应该 200，得到 %d", resp.StatusCode)
	}
	if string(body) != spec {
		t.Errorf("agent 下到的内容不一致: %q", body)
	}
}

// 这是这个功能的安全边界（ADR-0011 第四条）。
//
// 附件和控制台**同源**。一个 agent 上传一份 HTML，如果浏览器肯把它当页面渲染，
// 那就是一个挂在管理员会话上的存储型 XSS。三道头一道都不能少。
func TestDownloadNeverRendersInTheBrowser(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "attacker")

	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"payload.html", "text/html", []byte(`<script>alert(document.cookie)</script>`))
	up := decodeAttachment(t, body)

	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)

	// ① 声明的 text/html 绝不能被回显 —— 白名单里没有它
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		t.Errorf("text/html 必须被降级成 octet-stream，得到 %q", ct)
	}
	// ② 直接访问就是下载，不是渲染
	if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("Content-Disposition 必须是 attachment，得到 %q", cd)
	}
	// ③ 不许浏览器忽略我们给的类型自己猜
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options 必须是 nosniff，得到 %q", got)
	}
	// ④ 前三道万一被绕过，这一层里没有脚本、没有外链、没有同源身份
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Errorf("CSP 必须同时有 default-src 'none' 和 sandbox，得到 %q", csp)
	}
}

// SVG 是 XML，能带脚本。把它降级成 octet-stream 的代价只是「界面上不给它画缩略图」，
// 太便宜了，不值得赌。
func TestSVGIsNotServedAsImage(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "designer")

	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"logo.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	up := decodeAttachment(t, body)

	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "svg") {
		t.Errorf("SVG 不该按图片类型回显，得到 %q", ct)
	}
}

// 认得出的图片类型要原样回显，否则界面上一张缩略图都画不出来。
func TestKnownImageTypesAreEchoed(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "grapher")

	// 一个最小的合法 PNG 头就够了 —— 这里测的是类型协商，不是解码
	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"chart.png", "image/png", []byte("\x89PNG\r\n\x1a\n0123456789"))
	up := decodeAttachment(t, body)
	if up.ContentType != "image/png" {
		t.Errorf("png 的类型应该留着，得到 %q", up.ContentType)
	}

	// 客户端没带 Content-Type 时按扩展名兜底 —— 很多 CLI 上传工具就是不带
	_, body = uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"shot.png", "", []byte("\x89PNG\r\n\x1a\nabcdefghij"))
	if got := decodeAttachment(t, body).ContentType; got != "image/png" {
		t.Errorf("没带 Content-Type 时应该按扩展名认出 png，得到 %q", got)
	}
}

// 文件名会被放进 Content-Disposition，而 HTTP 头是用换行分隔的 ——
// 一个带 \r\n 的文件名可以在这里劈开响应头，塞进任意一个新头。
func TestHostileFilenameCannotSplitTheResponseHeader(t *testing.T) {
	srv, st, dir := attachServer(t)
	token := regAgent(t, srv, st, "attacker")

	hostile := "a\r\nX-Injected: yes\r\n\r\n<script>.txt"
	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		hostile, "text/plain", []byte("内容"))
	up := decodeAttachment(t, body)

	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)
	if resp.Header.Get("X-Injected") != "" {
		t.Fatal("文件名把响应头劈开了 —— 注入成功")
	}
	cd := resp.Header.Get("Content-Disposition")
	if strings.ContainsAny(cd, "\r\n") {
		t.Errorf("Content-Disposition 里不该有换行: %q", cd)
	}
	// 存下来的名字里也不该留控制字符：它们在界面上是隐形的
	if strings.ContainsAny(up.Filename, "\r\n") {
		t.Errorf("存下来的文件名里不该有换行: %q", up.Filename)
	}

	// 而且不管文件名多恶意，磁盘上永远只有 <ab>/<cd>/<64位十六进制>
	assertOnlyHashedPaths(t, dir)
}

// 文件名带路径时，磁盘上也必须只出现哈希路径 —— 这是 ADR-0011 第二条的落地检查。
func TestFilenameNeverBecomesAPath(t *testing.T) {
	srv, st, dir := attachServer(t)
	token := regAgent(t, srv, st, "attacker")

	for _, name := range []string{
		"../../../etc/cron.d/pwn",
		`C:\Windows\System32\evil.dll`,
		"..%2f..%2fpwn",
		"....//pwn",
		"/absolute/pwn",
	} {
		resp, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
			name, "text/plain", []byte("内容"+name))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("上传 %q 应该正常成功（安全由寻址方式保证，不靠拒绝）：%d %s",
				name, resp.StatusCode, body)
		}
		got := decodeAttachment(t, body).Filename
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("存下来的文件名里不该有分隔符: %q", got)
		}
	}
	assertOnlyHashedPaths(t, dir)
}

// assertOnlyHashedPaths 走一遍附件目录，确认每一个文件都躺在
// <ab>/<cd>/<64 位十六进制> 上 —— 一个例外都不许有。
func assertOnlyHashedPaths(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 || len(parts[0]) != 2 || len(parts[1]) != 2 || len(parts[2]) != 64 {
			t.Errorf("磁盘上出现了不是哈希路径的东西: %s", rel)
			return nil
		}
		if !strings.HasPrefix(parts[2], parts[0]+parts[1]) {
			t.Errorf("分片目录和内容哈希对不上: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫附件目录: %v", err)
	}
}

func TestUploadRejectsOversizeWithTheLimitInTheMessage(t *testing.T) {
	srv, st, dir := attachServer(t)
	token := regAgent(t, srv, st, "spammer")

	resp, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"big.bin", "application/octet-stream", bytes.Repeat([]byte("x"), (1<<20)+1))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限应该 413（400 会让 agent 以为请求写错了去改形状），得到 %d: %s",
			resp.StatusCode, body)
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code != "attachment_too_large" {
		t.Errorf("错误码应该是 attachment_too_large，得到 %q", e.Code)
	}
	// 不带上限值的话 agent 只能二分猜
	if !strings.Contains(e.Message, "MiB") {
		t.Errorf("错误消息里要写清上限是多少，得到 %q", e.Message)
	}
	// 半个文件不许留下
	assertOnlyHashedPaths(t, dir)
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("超限之后目录里应该什么都不剩，实际有 %d 个文件", n)
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestTooManyAttachmentsOnOnePost(t *testing.T) {
	srv, st, _ := attachServer(t) // MaxPerPost = 3
	token := regAgent(t, srv, st, "hoarder")
	threadID := mkTweet(t, srv, token, "开个头")

	ids := []string{}
	for i := 0; i < 4; i++ {
		_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
			"f.txt", "text/plain", []byte(strings.Repeat("x", i+1)))
		ids = append(ids, decodeAttachment(t, body).ID)
	}

	resp, body := postJSON(t, srv.URL+"/api/agent/threads/"+threadID+"/posts", token,
		map[string]any{"body": "全塞进来", "attachmentIds": ids})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("超过每帖上限应该 400，得到 %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "3") {
		t.Errorf("错误消息里要说清上限是几个，得到 %s", body)
	}
}

// 挂不上的附件是**调用方**的问题。报 500 会让 agent 一直重试同一个挂不上的 id。
func TestClaimingSomeoneElsesUploadIs400NotServerError(t *testing.T) {
	srv, st, _ := attachServer(t)
	alice := regAgent(t, srv, st, "alice")
	bob := regAgent(t, srv, st, "bob")
	threadID := mkTweet(t, srv, alice, "开个头")

	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", alice,
		"alice.txt", "text/plain", []byte("alice 的东西"))
	up := decodeAttachment(t, body)

	resp, body := postJSON(t, srv.URL+"/api/agent/threads/"+threadID+"/posts", bob,
		map[string]any{"body": "偷用", "attachmentIds": []string{up.ID}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("挂别人的上传应该 400，得到 %d: %s", resp.StatusCode, body)
	}
	var e struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code != "attachment_rejected" {
		t.Errorf("错误码应该是 attachment_rejected，得到 %q", e.Code)
	}
	if e.Retryable {
		t.Error("挂不上的附件重试多少次都一样，retryable 必须是 false")
	}
}

func TestDownloadRequiresAuth(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "owner")
	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"secret.txt", "text/plain", []byte("工作产物"))
	up := decodeAttachment(t, body)

	// 附件是这个平台上真实的工作产物，不是那个「不含任何实例数据的空壳 APK」。
	// 两个入口都必须挡住没凭证的请求。
	for _, path := range []string{"/api/agent/attachments/", "/api/admin/attachments/"} {
		resp, _ := getWith(t, srv.URL+path+up.ID, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 无凭证访问应该 401，得到 %d", path, resp.StatusCode)
		}
	}
}

func TestDownloadUnknownIDIs404(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "someone")
	for _, id := range []string{
		"00000000-0000-0000-0000-000000000000",
		"不是-uuid", // 不该变成 500
	} {
		resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+id, token)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("下载 %q 应该 404，得到 %d", id, resp.StatusCode)
		}
	}
}

// 没配 ATTACHMENT_DIR 是**正常的部署状态**。要给一个说得清楚的 503，
// 不是 404（那会把人引去查路由，而真正要查的是部署），也不是 500。
func TestAttachmentsOffGivesAClear503(t *testing.T) {
	srv, st := newServer(t) // 这个 harness 不配 AttachmentDir
	token := regAgent(t, srv, st, "hopeful")

	resp, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"x.txt", "text/plain", []byte("内容"))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("没开附件时上传应该 503，得到 %d: %s", resp.StatusCode, body)
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code != "attachments_unavailable" {
		t.Errorf("错误码应该是 attachments_unavailable，得到 %q", e.Code)
	}
	// 运维读了这条就该知道下一步做什么
	if !strings.Contains(e.Message, "ATTACHMENT_DIR") {
		t.Errorf("错误消息里要点名那个环境变量，得到 %q", e.Message)
	}
}

// 控制台靠 /api/admin/me 决定画不画回形针。没开附件时必须是 false，
// 否则界面上会有一个点下去必然失败的按钮。
// 一个用例只能调一次 testdb.New（它会独占整个测试库直到用例结束），
// 所以「开着」和「没开」拆成两个用例，不是一个。
func TestAdminMeReportsAttachmentsOffWhenNotConfigured(t *testing.T) {
	srv, _ := newServer(t)
	if meAttachmentsEnabled(t, srv) {
		t.Error("没配 ATTACHMENT_DIR 时 me.attachments.enabled 应该是 false —— " +
			"否则界面上会有一个点下去必然失败的回形针")
	}
}

func TestAdminMeReportsAttachmentsOnWhenConfigured(t *testing.T) {
	srv, _, _ := attachServer(t)
	if !meAttachmentsEnabled(t, srv) {
		t.Error("配了可写目录时 me.attachments.enabled 应该是 true")
	}
}

func meAttachmentsEnabled(t *testing.T, srv *httptest.Server) bool {
	t.Helper()
	admin := adminClient(t, srv.URL)
	_, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/me", nil, nil)
	var me struct {
		Attachments struct {
			Enabled    bool  `json:"enabled"`
			MaxBytes   int64 `json:"maxBytes"`
			MaxPerPost int   `json:"maxPerPost"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("解 me: %v (%s)", err, body)
	}
	if me.Attachments.Enabled && (me.Attachments.MaxBytes <= 0 || me.Attachments.MaxPerPost <= 0) {
		t.Errorf("开着的时候两个上限都要给出来，前端才画得出提示: %+v", me.Attachments)
	}
	return me.Attachments.Enabled
}

// 同一份内容传两次，磁盘上只留一份 —— 但两条元数据行各是各的，
// 两条帖子各挂各的，谁都不会看到别人的文件名。
func TestSameContentTwiceSharesOneBlobButNotOneRow(t *testing.T) {
	srv, st, dir := attachServer(t)
	token := regAgent(t, srv, st, "duper")
	const content = "一模一样的产物"

	_, b1 := uploadFile(t, srv.URL+"/api/agent/attachments", token, "第一次.txt", "text/plain", []byte(content))
	_, b2 := uploadFile(t, srv.URL+"/api/agent/attachments", token, "第二次.txt", "text/plain", []byte(content))
	a1, a2 := decodeAttachment(t, b1), decodeAttachment(t, b2)

	if a1.ID == a2.ID {
		t.Error("两次上传应该是两条独立的元数据行")
	}
	if a1.SHA256 != a2.SHA256 {
		t.Error("同样的内容应该落到同一个 blob 上")
	}
	if n := countFiles(t, dir); n != 1 {
		t.Errorf("磁盘上应该只有 1 份内容，实际 %d 份", n)
	}
	// 各自的文件名互不影响
	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+a2.ID, token)
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "UTF-8''") {
		t.Errorf("中文文件名要走 RFC 5987 的 filename*，得到 %q", cd)
	}
}

// 内容寻址意味着同一个 id 的字节永远不变 —— ETag 命中时应该是 304，
// 十几 MB 的产物在 thread 里反复出现时这一条省的是真流量。
func TestDownloadSupportsConditionalGet(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "cacher")
	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"big.txt", "text/plain", []byte(strings.Repeat("x", 4096)))
	up := decodeAttachment(t, body)

	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("下载响应要带 ETag")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/agent/attachments/"+up.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-None-Match", etag)
	resp2, _ := do(t, req)
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("ETag 命中应该 304，得到 %d", resp2.StatusCode)
	}
}

// 上传成功但没发帖 —— 那份内容在磁盘上，但谁都看不见它，
// 直到 worker 的 GC 把它收走。这里只确认它确实没出现在任何 thread 里。
func TestUnclaimedUploadIsInvisible(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "quitter")
	threadID := mkTweet(t, srv, token, "开个头")

	_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"没发出去.txt", "text/plain", []byte("草稿"))
	up := decodeAttachment(t, body)

	admin := adminClient(t, srv.URL)
	_, body = doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/threads/"+threadID, nil, nil)
	if strings.Contains(string(body), up.ID) {
		t.Error("没挂到帖子上的附件不该出现在 thread 详情里")
	}

	// 但传它的人自己还能下到 —— 它就是没被认领，不是被没收了
	resp, _ := getWith(t, srv.URL+"/api/agent/attachments/"+up.ID, token)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("未认领的附件按 id 还是拿得到，得到 %d", resp.StatusCode)
	}
}

// 请求里没有 file 字段 —— 400，而不是挂在那儿读到超时。
func TestUploadWithoutFileFieldIs400(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "confused")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("notafile", "x")
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/agent/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := do(t, req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("没有 file 字段应该 400，得到 %d", resp.StatusCode)
	}
}

// 空文件是**调用方的错**，不是部署的错。
// 报 500 的话运维会去查 ATTACHMENT_DIR 的权限，而权限一点问题都没有。
func TestEmptyFileIsRejectedAsClientError(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "empty")
	resp, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
		"empty.txt", "text/plain", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("空附件应该 400，得到 %d: %s", resp.StatusCode, body)
	}
	var e struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code != "attachment_empty" {
		t.Errorf("错误码应该是 attachment_empty，得到 %q", e.Code)
	}
	if e.Retryable {
		t.Error("同一个空文件重试多少次都一样，retryable 必须是 false")
	}
}

// 一条帖子挂多个附件时，顺序要稳定 —— 界面上今天这个在前、明天那个在前，
// 是那种「说不上哪里不对但就是不对」的毛病。
func TestMultipleAttachmentsKeepAStableOrder(t *testing.T) {
	srv, st, _ := attachServer(t)
	token := regAgent(t, srv, st, "multi")
	threadID := mkTweet(t, srv, token, "开个头")

	var ids []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		_, body := uploadFile(t, srv.URL+"/api/agent/attachments", token,
			n, "text/plain", []byte("内容"+n))
		ids = append(ids, decodeAttachment(t, body).ID)
	}
	resp, body := postJSON(t, srv.URL+"/api/agent/threads/"+threadID+"/posts", token,
		map[string]any{"body": "三个文件", "attachmentIds": ids})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("应该 201，得到 %d: %s", resp.StatusCode, body)
	}

	admin := adminClient(t, srv.URL)
	var first []string
	for i := 0; i < 3; i++ {
		_, body := doJSON(t, admin, http.MethodGet, srv.URL+"/api/admin/threads/"+threadID, nil, nil)
		var detail struct {
			Posts []struct {
				Attachments []store.Attachment `json:"attachments"`
			} `json:"posts"`
		}
		if err := json.Unmarshal(body, &detail); err != nil {
			t.Fatalf("解 thread: %v", err)
		}
		last := detail.Posts[len(detail.Posts)-1]
		var got []string
		for _, a := range last.Attachments {
			got = append(got, a.Filename)
		}
		if len(got) != 3 {
			t.Fatalf("应该有 3 个附件，得到 %v", got)
		}
		if first == nil {
			first = got
			continue
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Errorf("多次读到的顺序不一致：%v vs %v", first, got)
		}
	}
}

// ── 下面几个是这份文件自己要用的小工具 ──────────────────────────────

// uploadAs 用一个已登录的 client 发 multipart 上传。
// part 的头自己拼，不用 CreateFormFile —— 那个会按扩展名塞一个类型，
// 而用例要测的正是「上传方声明什么类型」这件事。
func uploadAs(t *testing.T, c *http.Client, url, filename, contentType string, content []byte) (*http.Response, []byte) {
	t.Helper()
	body, ct := multipartBody(t, filename, contentType, content)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", ct)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

// mkTweet 发一条广播，返回 threadID。附件用例只需要一条能回帖的 thread。
func mkTweet(t *testing.T, srv *httptest.Server, token, body string) string {
	t.Helper()
	resp, raw := postJSON(t, srv.URL+"/api/agent/tweets", token, map[string]any{"body": body})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("发广播失败: %d %s", resp.StatusCode, raw)
	}
	var out struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解广播响应: %v", err)
	}
	return out.ThreadID
}
