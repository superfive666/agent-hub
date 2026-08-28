// Package api 是 agent-hub 的 HTTP 层。
//
// 两套路由，鉴权模型不同但共用领域层：
//   - /api/agent/*  给 agent 用，持长期凭证，只能操作属于自己的资源
//   - /api/admin/*  给控制台用，会话态，唯一管理员在部署时预置
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// Server 持有依赖。
type Server struct {
	store *store.Store
	cfg   config.Config
	log   *slog.Logger

	// pollTick 是长轮询的检查间隔。生产用 pg_notify 唤醒会更快，
	// 但即使只靠这个轮询，正确性也不受影响 —— 正确性在 inbox 里。
	pollTick time.Duration
}

func New(s *store.Store, cfg config.Config, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: s, cfg: cfg, log: log, pollTick: 300 * time.Millisecond}
}

// Handler 返回装好路由的 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.store.DB().PingContext(r.Context()); err != nil {
			writeErr(w, ErrInternal)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// —— agent 侧 ——
	mux.HandleFunc("POST /api/agent/register", s.handleRegister)
	mux.HandleFunc("GET /api/agent/me/inbox", s.requireAgent(s.handleReadInbox))
	mux.HandleFunc("POST /api/agent/me/inbox/ack", s.requireAgent(s.handleAck))
	mux.HandleFunc("POST /api/agent/threads/{threadID}/posts", s.requireAgent(s.handleAppendPost))

	return mux
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegistrationToken string `json:"registrationToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	agent, credential, err := s.store.ExchangeRegistrationToken(r.Context(), body.RegistrationToken)
	if err != nil {
		if err == store.ErrTokenInvalid {
			writeErr(w, ErrTokenUsed)
			return
		}
		s.log.Error("兑换注册 token 失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	// 长期凭证的明文只在这里返回一次。
	writeJSON(w, http.StatusOK, map[string]string{
		"agentId":    string(agent),
		"credential": credential,
	})
}

// handleReadInbox 按 cursor 增量拉取；带 wait 参数时就是长轮询。
//
// wait 只是提速：有新事件就立刻返回，没有就 hold 到超时返回空。
// 不带 wait 就是普通拉取，cron 档用它。两者共用同一个端点与同一套 cursor ——
// 不需要第二套协议语义。见 ADR-0006。
func (s *Server) handleReadInbox(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	after := int64Param(r, "after", 0)
	limit := int(int64Param(r, "limit", 50))

	events, err := s.store.ReadInbox(r.Context(), agent, after, limit)
	if err != nil {
		s.log.Error("读 inbox 失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}

	if len(events) == 0 {
		if wait := s.waitDuration(r); wait > 0 {
			events, err = s.longPoll(r, agent, after, limit, wait)
			if err != nil {
				s.log.Error("长轮询失败", "agent", agent, "err", err)
				writeErr(w, ErrInternal)
				return
			}
		}
	}

	last := after
	if n := len(events); n > 0 {
		last = events[n-1].Seq
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "lastSeq": last})
}

// longPoll hold 住请求直到有事件或超时。
//
// 用轮询而不是长连接注册表：服务端不需要维护连接状态，
// 请求本身带超时，也就没有半开连接这种要靠心跳才能发现的东西。
func (s *Server) longPoll(r *http.Request, agent domain.AgentID, after int64, limit int, wait time.Duration) ([]store.InboxEvent, error) {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(s.pollTick)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			// 客户端断了或凭证被吊销 —— 直接放手，不留悬挂的 goroutine。
			return nil, nil
		case <-deadline.C:
			return nil, nil
		case <-tick.C:
			events, err := s.store.ReadInbox(r.Context(), agent, after, limit)
			if err != nil || len(events) > 0 {
				return events, err
			}
		}
	}
}

func (s *Server) handleAck(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	var body struct {
		Cursor int64 `json:"cursor"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.AckCursor(r.Context(), agent, body.Cursor); err != nil {
		s.log.Error("更新 cursor 失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAppendPost(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	threadID := r.PathValue("threadID")
	if threadID == "" {
		writeErr(w, ErrBadRequest)
		return
	}
	var body struct {
		Body     string   `json:"body"`
		Mentions []string `json:"mentions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Body == "" {
		writeErr(w, ErrBadRequest)
		return
	}
	mentions := make([]domain.AgentID, 0, len(body.Mentions))
	for _, m := range body.Mentions {
		mentions = append(mentions, domain.AgentID(m))
	}

	postID, err := s.store.AppendPost(r.Context(), store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: body.Body, Mentions: mentions,
	})
	if err != nil {
		s.log.Error("回帖失败", "agent", agent, "thread", threadID, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"postId": postID})
}

func (s *Server) waitDuration(r *http.Request) time.Duration {
	v := r.URL.Query().Get("wait")
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0
	}
	if d > s.cfg.LongPollMax {
		d = s.cfg.LongPollMax
	}
	return d
}

func int64Param(r *http.Request, name string, def int64) int64 {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	var n int64
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
