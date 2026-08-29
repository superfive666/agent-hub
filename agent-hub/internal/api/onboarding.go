package api

import (
	_ "embed"
	"net/http"
	"strings"
)

// onboardingDoc 是给 agent 看的接入说明。
//
// **用 go:embed 而不是把这段文字写进字符串常量**：它是一份会被人反复读改的文档，
// 塞进 .go 文件里没人愿意维护，而且 diff 里全是转义。
//
//go:embed onboarding.md
var onboardingDoc string

// handleOnboarding 把接入说明交给 agent。
//
// **它必须是公开的**：agent 读它的时候手上只有一张一次性注册 token，
// 那张 token 只能用来换凭证，不是 Bearer 凭证 —— 挂在 requireAgent 后面
// 就成了「要先接入才能知道怎么接入」。文档里也没有任何秘密，
// 只有步骤和这台 hub 自己的地址。
//
// **路径必须在 /api/ 下面。** 部署形态是「反向代理把 /api/* 和 /healthz 转给 hub，
// 其余路径交给控制台那份静态产物」（见 docs/08-deployment.md §5）。
// 挂成 /onboarding 的话，现有部署会把它交给 SPA 的 index.html，
// agent 拉到一坨 HTML 还以为自己读到了说明。
func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	// 文档里的命令要能直接复制执行，所以把这台 hub 的真实地址替换进去。
	doc := strings.ReplaceAll(onboardingDoc, "{{HUB}}", publicBaseURL(r))
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// 说明会随部署版本变，别让中间层缓存住一份旧的。
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}

// publicBaseURL 拼出这台 hub 对外的地址。
//
// 只看 r.TLS 是不够的：生产形态是 TLS 在反向代理上终结，到 hub 这一跳是明文 HTTP，
// 于是拼出来的会是 http:// —— agent 照着它去请求，要么被 301 要么直接失败。
// X-Forwarded-Proto 是代理告诉我们「用户那一端是什么协议」的标准做法。
func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// 逗号分隔时取第一个：多层代理会追加，最左边那个才是最初的客户端。
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return scheme + "://" + host
}
