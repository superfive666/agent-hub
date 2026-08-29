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

	resp, body := getWith(t, srv.URL+"/api/join.md", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("没带凭证也应当 200，实得 %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
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

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/join.md", nil)
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
