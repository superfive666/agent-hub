package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	// ErrAgentNameRequired 名称是空的（或者只有空白）。
	ErrAgentNameRequired = errors.New("agent 名称不能为空")
	// ErrAgentNameTooLong 名称超长。
	ErrAgentNameTooLong = errors.New("agent 名称最长 64 个字符")
	// ErrAgentNameCharset 名称里有不允许的字符。
	ErrAgentNameCharset = errors.New("agent 名称只能包含字母、数字、下划线和连字符（@ 提及靠这个字符集匹配）")
)

// AgentNameMaxLen 是名称的长度上限。取 64 只是个不碍事的数：
// 名字要出现在正文的 `@name` 里、出现在列表的一列里，长到换行就没人用了。
const AgentNameMaxLen = 64

// ValidateAgentName 校验并规范化 agent 名称，返回去掉首尾空白后的名字。
//
// 字符集限制成 [A-Za-z0-9_-] 不是洁癖：**正文里的 @ 提及靠这个字符集匹配**
// （前端的 mention token 是 /(^|\s)@([A-Za-z0-9_-]*)$/）。
// 名字里带空格或中文的 agent 根本没法被 @ 到，而 @ 是这个平台上唯一的连接动作 ——
// 一个 @ 不到的 agent 等于接不进协作。所以这条限制要在创建时就挡住，
// 而不是等到有人发现「怎么 at 不上他」。
func ValidateAgentName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", ErrAgentNameRequired
	}
	if utf8.RuneCountInString(n) > AgentNameMaxLen {
		return "", ErrAgentNameTooLong
	}
	for _, c := range n {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return "", ErrAgentNameCharset
		}
	}
	return n, nil
}
