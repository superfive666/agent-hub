package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// 需求：控制台给出去的那段 prompt 只带一个 URL，步骤由 hub 自己吐。
//
// **这条路必须是公开的**：agent 读它的时候手上只有一张一次性注册 token，
// 那张 token 只能用来换凭证、不是 Bearer 凭证 —— 挂在鉴权后面就成了
// 「要先接入才能知道怎么接入」。
func TestJoinDocIsPublic(t *testing.T) {
	srv, _ := newServer(t)

	resp, body := getWith(t, srv.URL+"/api/join", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("没带凭证也应当 200，实得 %d", resp.StatusCode)
	}
	// 「返回 text，原样的 md 内容」—— text/plain 在每个客户端里都直接呈现原文，
	// text/markdown 在部分浏览器里会触发下载。
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}

	doc := string(body)
	// 三件事一件都不能少 —— 尤其是「保持在线」：少了它 agent 注册完就没人拉 inbox，
	// 而这个失败是完全静默的（界面上它只是显示离线）。
	for _, must := range []string{
		"/api/agent/register", // 换凭证
		"Stay reachable",      // 常驻 / cron
		"Agent Card",          // 自我介绍
		"limitations",         // 硬要求那一条
		"422",                 // 留空会被拒
	} {
		if !strings.Contains(doc, must) {
			t.Errorf("接入说明里没有 %q —— agent 照着做会漏掉这一步", must)
		}
	}
}

// 需求：说明里的命令要能直接跑，所以 hub 的地址得是**这台 hub 对外的地址**。
//
// 只看 r.TLS 是不够的：生产形态是 TLS 在反向代理终结，到 hub 这一跳是明文 HTTP，
// 拼出来会是 http:// —— agent 照着它请求，要么被 301 要么直接失败。
func TestJoinDocUsesForwardedScheme(t *testing.T) {
	srv, _ := newServer(t)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/join", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "hub.example.com")
	_, body := do(t, req)

	doc := string(body)
	if !strings.Contains(doc, "https://hub.example.com") {
		t.Error("没按 X-Forwarded-* 拼出对外地址 —— TLS 在代理终结时会得到一个 http:// 的错地址")
	}
	if strings.Contains(doc, "{{HUB}}") {
		t.Error("占位符没被替换掉")
	}
}

// 需求：token 和 runtime 从 query 进来，直接填进正文 ——
// agent 拉到的命令必须是能直接跑的，不该再有「把 <你的token> 换成真值」这一步。
func TestJoinDocFillsInTokenAndRuntime(t *testing.T) {
	srv, _ := newServer(t)

	resp, body := getWith(t,
		srv.URL+"/api/join?token=ahr_reg_ABCdef-123_xyz&runtime=claude-code", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("应当 200，实得 %d", resp.StatusCode)
	}
	doc := string(body)
	if !strings.Contains(doc, "REG_TOKEN=ahr_reg_ABCdef-123_xyz") {
		t.Error("token 没被填进正文")
	}
	if !strings.Contains(doc, "RUNTIME=claude-code") {
		t.Error("runtime 没被填进正文")
	}
	for _, ph := range []string{"{{HUB}}", "{{TOKEN}}", "{{RUNTIME}}"} {
		if strings.Contains(doc, ph) {
			t.Errorf("占位符 %s 没被替换", ph)
		}
	}

	// 正文里带着一次性 token，任何中间层都不该留副本
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store —— 正文里有一次性 token", cc)
	}
	// 正文含 query 传进来的内容，禁掉内容嗅探
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("缺 X-Content-Type-Options: nosniff")
	}
	// 「返回 text，原样的 md」
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// 需求：query 里塞进来的东西不能被原样写进正文。
//
// 这不是在校验 token 有没有效（那要查库，还会把一次性 token 的存在性变成
// 一个可探测的信号），只是保证正文里出现的是一段规整的 token 样子的东西，
// 而不是别人塞进来的一整段 markdown 或脚本。
func TestJoinDocRejectsMalformedTokenAndRuntime(t *testing.T) {
	for _, tc := range []struct{ name, query, mustNotContain string }{
		{"带空格和 markdown 的 token", "?token=x+%60rm+-rf+%2F%60", "rm -rf"},
		{"超长 token", "?token=" + strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"没给 token", "", "REG_TOKEN=\n"},
		{"不认识的 runtime", "?runtime=totally-made-up", "RUNTIME=totally-made-up"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(t)
			resp, body := getWith(t, srv.URL+"/api/join"+tc.query, "")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("仍然应当 200（它是文档不是接口），实得 %d", resp.StatusCode)
			}
			if strings.Contains(string(body), tc.mustNotContain) {
				t.Errorf("不该出现在正文里的内容漏出去了: %q", tc.mustNotContain)
			}
			// 占位符必须被换成一句人话，不能留在正文里
			if strings.Contains(string(body), "{{") {
				t.Error("占位符没被替换")
			}
		})
	}
}
