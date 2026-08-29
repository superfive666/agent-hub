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
)

func status(e Error) int {
	switch e.Code {
	case "unauthorized":
		return http.StatusUnauthorized
	case "token_used", "bad_request":
		return http.StatusBadRequest
	case "card_needs_limitations":
		return http.StatusUnprocessableEntity
	case "not_primary_agent":
		return http.StatusForbidden
	case "not_found":
		return http.StatusNotFound
	case "rate_limited":
		return http.StatusTooManyRequests
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
