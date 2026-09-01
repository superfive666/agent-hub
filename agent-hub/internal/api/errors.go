package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error 是返回给 agent 的错误。
//
// **agent 要能自己读懂并据此决策**：能不能重试、多久后重试、是不是永久失败。
// 用裸状态码打发它等于让它自己猜，猜出来的行为就是重试风暴。
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RetryAfter int    `json:"retryAfter,omitempty"` // 秒
}

func (e Error) Error() string { return e.Code + ": " + e.Message }

// 常用错误。Retryable 为 true 的必须给 RetryAfter，否则 agent 只能瞎猜。
var (
	ErrUnauthorized = Error{Code: "unauthorized", Message: "凭证无效或已被吊销", Retryable: false}
	ErrTokenUsed    = Error{Code: "token_used", Message: "注册 token 无效、已用过或已过期", Retryable: false}
	ErrNotFound     = Error{Code: "not_found", Message: "对象不存在", Retryable: false}
	ErrBadRequest   = Error{Code: "bad_request", Message: "请求不合法", Retryable: false}
	ErrInternal     = Error{Code: "internal", Message: "服务内部错误", Retryable: true, RetryAfter: 5}

	// ErrAgentNameTaken 名字撞了。调用方换个名字就能成功，所以不是 500 也不是重试能解决的。
	ErrAgentNameTaken = Error{Code: "agent_name_taken",
		Message: "这个名称已经被占用了，换一个", Retryable: false}

	// ErrTodoNotConfirmed 是「用户确认闸门」拦下来的请求。
	//
	// Retryable 是 false：这不是等一会儿就好的暂时性失败，它需要**人**做一个动作。
	// 标成可重试等于放任 agent 在那儿空转，而它真正该做的是在 thread 里
	// 把需求问清楚，然后等 todo.approved 事件。
	ErrTodoNotConfirmed = Error{Code: "todo_not_confirmed", Retryable: false}
)

func status(e Error) int {
	switch e.Code {
	case "unauthorized":
		return http.StatusUnauthorized
	case "token_used", "bad_request", "attachment_rejected", "too_many_attachments", "attachment_empty":
		return http.StatusBadRequest
	case "card_needs_limitations", "webhook_not_our_contract":
		return http.StatusUnprocessableEntity
	case "not_primary_agent":
		return http.StatusForbidden
	case "agent_name_taken", "todo_not_confirmed", "invalid_todo_transition":
		return http.StatusConflict
	case "not_found":
		return http.StatusNotFound
	case "rate_limited":
		return http.StatusTooManyRequests
	// 413 而不是 400：400 会让 agent 以为请求写错了，去改请求体的形状；
	// 真正该做的是把文件切小或压缩。状态码本身就该指对方向。
	case "attachment_too_large":
		return http.StatusRequestEntityTooLarge
	// 503 而不是 404：端点在，只是这台 hub 现在拿不出构建产物。
	// 404 会把运维引去查路由，而真正要查的是部署。
	case "apk_unavailable", "attachments_unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeErr(w http.ResponseWriter, e Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if e.Retryable && e.RetryAfter > 0 {
		w.Header().Set("Retry-After", itoa(e.RetryAfter))
	}
	w.WriteHeader(status(e))
	_ = json.NewEncoder(w).Encode(e)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("写响应失败", "err", err)
	}
}

func itoa(n int) string {
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
