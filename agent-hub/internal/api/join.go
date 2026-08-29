package api

import (
	"net/http"
	"regexp"
	"strings"

	agenthub "github.com/superfive666/agent-hub"
)

// tokenShape 限制能被替换进文档里的 token 长什么样。
//
// 注册 token 是 `ahr_reg_` + base64url，字符集就是下面这些。**这不是在校验 token
// 有没有效**（那要查库，而且会把一次性 token 的存在性变成一个可探测的信号）——
// 它只保证「被塞进正文的那段字符串是一段规整的 token 样子的东西」，
// 而不是别人从 query 里塞进来的一整段 markdown。
var tokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// handleJoinDoc 把 JOIN.md 交给 agent，并把它自己的 token / runtime 填进去。
//
// 控制台给出去的就是一句话 + 这个 URL：
//
//	Join agent-hub: read https://hub.example.com/api/join?token=…&runtime=…
//
// 所以 agent 拉到的文档里，命令是**可以直接跑的**，没有需要它自己填的占位符 ——
// 少一个「把 <你的token> 换成真值」的步骤，就少一个出错的地方。
//
// **必须公开。** agent 读它的时候手上只有一张一次性注册 token，那张 token 只能
// 用来换凭证、不是 Bearer 凭证 —— 挂在 requireAgent 后面就成了「要先接入才能
// 知道怎么接入」。
//
// **路径必须在 /api/ 下。** 部署形态是反向代理把 /api/* 与 /healthz 转给 hub、
// 其余路径交给控制台那份静态产物（docs/08-deployment.md §5）。挂成 /join
// 的话现有部署会把它交给 SPA 的 index.html，agent 拉到一坨 HTML
// 还以为自己读到了说明。
//
// ⚠️ token 在 query 里，**反向代理的 access log 会记下它**。可接受的前提是
// 它一次性、24 小时过期、而且本来就明文显示在控制台上；但 no-store 得加上，
// 别让它再多留一份在中间层的缓存里。
func (s *Server) handleJoinDoc(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	token := q.Get("token")
	if !tokenShape.MatchString(token) {
		// 没给或者形状不对：留一句人话，而不是把脏字符串塞进正文。
		token = "<ask your operator for a one-time registration token>"
	}

	// runtime 只认注册表里有的那些。塞一个不存在的进去，agent 照着跑
	// onboard.sh 会撞「不认识的 RUNTIME」，而它完全有理由以为是文档错了。
	runtime := q.Get("runtime")
	if !knownRuntimes[runtime] {
		runtime = "<claude-code | codex | opencode | openclaw | hermes | openhuman | generic-shell>"
	}

	doc := agenthub.JoinDoc
	doc = strings.ReplaceAll(doc, "{{HUB}}", publicBaseURL(r))
	doc = strings.ReplaceAll(doc, "{{TOKEN}}", token)
	doc = strings.ReplaceAll(doc, "{{RUNTIME}}", runtime)

	// text/plain 而不是 text/markdown：这份东西是给 agent 原样读的，
	// text/plain 在每个客户端里都直接呈现原文，而 text/markdown
	// 在部分浏览器里会触发下载。
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// 正文里带着一次性 token，别让任何中间层留副本。
	w.Header().Set("Cache-Control", "no-store")
	// 正文含 query 传进来的内容，禁掉内容嗅探，免得哪个客户端把它当 HTML 解释。
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}

// knownRuntimes 和 connector 的 builtinAdapters 对齐（connector/src/adapters/registry.ts）。
// 这里只用来判断「要不要把它填进文档」，不参与任何鉴权。
var knownRuntimes = map[string]bool{
	"claude-code": true, "claude": true, "claude-cli": true,
	"codex": true, "codex-cli": true,
	"opencode": true, "openclaw": true,
	"hermes": true, "openhuman": true,
	"generic-shell": true, "http-endpoint": true,
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
