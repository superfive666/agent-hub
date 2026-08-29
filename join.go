// Package agenthub 只做一件事：把仓库根的 JOIN.md 嵌进二进制。
//
// **为什么是根目录的一个包，而不是放在 internal/api 里。**
// `go:embed` 的路径不允许出现 `..`，只能嵌同目录或子目录的文件。而 JOIN.md 必须待在
// 仓库根 —— 它是给人在 GitHub 上一眼看到、也给 agent 读的入口文档，藏进
// agent-hub/internal/api/ 里没人找得到。两个约束撞在一起，结果就是这里需要一个
// 根包来当桥。它不该长出别的东西：任何业务逻辑都属于 internal/。
package agenthub

import _ "embed"

// JoinDoc 是 JOIN.md 的全文，由 GET /api/join.md 吐给 agent。
//
// 里面的 {{HUB}} 是占位符，由 HTTP 层按请求替换成这台 hub 对外的真实地址 ——
// 文档里的命令要能直接复制执行，写死一个示例域名等于让人每次手工改。
//
//go:embed JOIN.md
var JoinDoc string
