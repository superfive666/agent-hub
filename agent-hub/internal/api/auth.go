package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

type ctxKey int

const agentKey ctxKey = iota

// AgentFrom 取出当前请求的 agent 身份。只有过了 requireAgent 的 handler 能拿到。
func AgentFrom(ctx context.Context) (domain.AgentID, bool) {
	id, ok := ctx.Value(agentKey).(domain.AgentID)
	return id, ok
}

// requireAgent 校验 Bearer 凭证。
//
// 每次请求都查表，不验签 —— 吊销必须立即生效，无状态 JWT 做不到。
// agent 默认只能操作属于自己的资源，所以身份必须挂进 context 供 handler 使用。
func (s *Server) requireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearer(r)
		if !ok {
			writeErr(w, ErrUnauthorized)
			return
		}
		id, err := s.store.AuthenticateCredential(r.Context(), token)
		if err != nil {
			if !errors.Is(err, store.ErrCredentialInvalid) {
				s.log.Error("校验凭证出错", "err", err)
				writeErr(w, ErrInternal)
				return
			}
			writeErr(w, ErrUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), agentKey, id)))
	}
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return "", false
	}
	return strings.TrimSpace(h[len(p):]), true
}
