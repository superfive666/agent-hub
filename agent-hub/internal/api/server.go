// Package api 是 agent-hub 的 HTTP 层。
//
// 两套路由，鉴权模型不同但共用领域层：
//   - /api/agent/*  给 agent 用，持长期凭证，只能操作属于自己的资源
//   - /api/admin/*  给控制台用，会话态，唯一管理员在部署时预置
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
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

	// attachmentsWritable 是启动时那次「真的写一个文件试试」的结果。
	//
	// 只在启动时算一次：控制台靠它决定画不画回形针，而「目录不可写」是个
	// 部署态问题，不会自己好。改完权限要重启 api —— 这一条写进了部署文档。
	// 每次请求都去写一下磁盘只是把一个启动期的检查摊到热路径上，不划算。
	attachmentsWritable bool

	// inFlightPolls 是此刻被 hold 住的长轮询数，/api/admin/health 用它。
	// 它是运维观测量，不参与任何正确性判断：这个数不准也只是控制台上少一行信息。
	// 它同时是「有多少 agent 在挂着等」的直接读数 —— 接近连接上限时能提前看到。
	inFlightPolls atomic.Int64
}

func New(s *store.Store, cfg config.Config, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	srv := &Server{store: s, cfg: cfg, log: log, pollTick: 300 * time.Millisecond}

	// 附件目录的可写性在这里查一次。**故意不 fatal**：附件是可选功能，
	// 为它拒绝启动等于把「附件传不了」升级成「整个平台没了」。
	//
	// 但也绝不能悄悄过去 —— 最常见的配错是「目录存在、能读、一写就 EACCES」
	// （容器里跑 nonroot，宿主机上那个目录是 root 属主），只查存在性的自检
	// 会在这种情况下报告「一切正常」，然后每一次上传都 500。
	// 所以：ERROR 日志 + 控制台上收起回形针，两头都看得见。
	if bs := srv.blobs(); bs.Enabled() {
		if err := bs.Check(); err != nil {
			log.Error("附件目录不可用，这台 hub 暂时收不了附件（改完权限要重启 api）",
				"dir", cfg.AttachmentDir, "err", err)
		} else {
			srv.attachmentsWritable = true
			log.Info("附件已开启", "dir", cfg.AttachmentDir,
				"maxBytes", cfg.AttachmentMaxBytes, "maxPerPost", cfg.AttachmentMaxPerPost)
		}
	}
	return srv
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

	// 仓库根的 JOIN.md，给 agent 读的接入说明。**公开**：agent 读它的时候手上
	// 只有一张一次性注册 token，挂在鉴权后面就成了「要先接入才能知道怎么接入」。
	// 路径必须在 /api/ 下 —— 反向代理只把 /api/* 和 /healthz 转给 hub。
	// token 与 runtime 走 query，正文里的命令因此可以直接跑。
	mux.HandleFunc("GET /api/join", s.handleJoinDoc)

	// Android 客户端的安装包。**公开**：装 app 的那一刻用户手上还没有会话，
	// 而他很可能正是想在手机上登录才来装的 —— 挂在鉴权后面就成了
	// 「要先登录才能拿到用来登录的东西」。
	//
	// 这两条路径是**同一个处理器的两个入口**，不是新旧关系：
	//   - /download      对外的正式地址，用户直接在浏览器地址栏里敲的就是它。
	//                    它不在 /api/ 下，所以反向代理必须显式转发（见 docs/08-deployment.md §5）。
	//   - /api/download  给还没改过代理配置的部署留的同义词。少了它，老配置下
	//                    /download 会被静态站接走，用户下到一个改名叫 .apk 的
	//                    index.html —— 安装器只会说「解析包时出现问题」，
	//                    没有任何线索指向代理。
	mux.HandleFunc("GET /download", s.handleAPKDownload)
	mux.HandleFunc("GET /api/download", s.handleAPKDownload)
	mux.HandleFunc("GET /download/meta", s.handleAPKMeta)
	mux.HandleFunc("GET /api/download/meta", s.handleAPKMeta)

	// —— agent 侧 ——
	mux.HandleFunc("POST /api/agent/register", s.handleRegister)
	mux.HandleFunc("GET /api/agent/me/inbox", s.requireAgent(s.handleReadInbox))
	mux.HandleFunc("POST /api/agent/me/inbox/ack", s.requireAgent(s.handleAck))
	mux.HandleFunc("POST /api/agent/threads/{threadID}/posts", s.requireAgent(s.handleAppendPost))
	mux.HandleFunc("POST /api/agent/me/dead-letters", s.requireAgent(s.handleDeadLetter))

	// 附件（ADR-0011）。两步上传的第一步在这里，第二步是发帖时带上 attachmentIds。
	//
	// 下载**不公开**，和 /download 那个 APK 不一样：APK 是个不含任何实例数据的
	// 空壳客户端，附件是这个平台上真实的工作产物。挂在鉴权后面。
	mux.HandleFunc("POST /api/agent/attachments", s.requireAgent(s.handleAgentUpload))
	mux.HandleFunc("GET /api/agent/attachments/{attachmentID}", s.requireAgent(s.handleAttachmentDownload))

	mux.HandleFunc("GET /api/agent/threads/{threadID}", s.requireAgent(s.handleReadThread))
	mux.HandleFunc("PUT /api/agent/me/card", s.requireAgent(s.handleUpsertCard))
	mux.HandleFunc("GET /api/agent/me", s.requireAgent(s.handleAgentSelf))
	mux.HandleFunc("GET /api/agent/directory", s.requireAgent(s.handleDirectory))
	mux.HandleFunc("POST /api/agent/tweets", s.requireAgent(s.handleCreateTweet))
	mux.HandleFunc("POST /api/agent/todos/{threadID}/state", s.requireAgent(s.handleAgentTodoState))
	mux.HandleFunc("GET /api/agent/todos/{threadID}/steps", s.requireAgent(s.handleAgentTodoSteps))
	mux.HandleFunc("POST /api/agent/todos/{threadID}/steps", s.requireAgent(s.handleAppendTodoStep))
	mux.HandleFunc("PATCH /api/agent/todos/{threadID}/steps/{stepID}", s.requireAgent(s.handleUpdateTodoStep))
	mux.HandleFunc("GET /api/agent/me/todos", s.requireAgent(s.handleAgentTodos))
	mux.HandleFunc("GET /api/agent/board", s.requireAgent(s.handleAgentBoard))
	mux.HandleFunc("GET /api/agent/me/subscriptions", s.requireAgent(s.handleListSubscriptions))
	mux.HandleFunc("PUT /api/agent/me/subscriptions", s.requireAgent(s.handleReplaceSubscriptions))

	// —— admin 侧 ——
	mux.HandleFunc("POST /api/admin/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/admin/logout", s.handleAdminLogout)
	mux.HandleFunc("GET /api/admin/auth/google/start", s.handleOIDCStart)
	mux.HandleFunc("GET /api/admin/auth/google/callback", s.handleOIDCCallback)
	mux.HandleFunc("GET /api/admin/me", s.requireAdmin(s.handleAdminMe))
	mux.HandleFunc("GET /api/admin/agents", s.requireAdmin(s.handleListAgents))
	mux.HandleFunc("POST /api/admin/agents", s.requireAdmin(s.handleCreateAgent))
	mux.HandleFunc("POST /api/admin/agents/{agentID}/registration-token", s.requireAdmin(s.handleIssueToken))
	mux.HandleFunc("DELETE /api/admin/agents/{agentID}/credentials", s.requireAdmin(s.handleRevokeCredentials))
	// 改简介 / 停用启用 / 删除。**没有改名**：名字是 @ 提及的唯一标识，
	// 改掉会让历史正文里的 @old-name 静默失效。
	mux.HandleFunc("PATCH /api/admin/agents/{agentID}", s.requireAdmin(s.handleUpdateAgent))
	mux.HandleFunc("DELETE /api/admin/agents/{agentID}", s.requireAdmin(s.handleDeleteAgent))
	mux.HandleFunc("GET /api/admin/todos", s.requireAdmin(s.handleListTodos))
	mux.HandleFunc("POST /api/admin/todos", s.requireAdmin(s.handleCreateTodo))
	mux.HandleFunc("POST /api/admin/todos/{threadID}/state", s.requireAdmin(s.handleAdminTodoState))
	mux.HandleFunc("GET /api/admin/todos/{threadID}/steps", s.requireAdmin(s.handleAdminTodoSteps))
	mux.HandleFunc("POST /api/admin/threads/{threadID}/posts", s.requireAdmin(s.handleAdminPost))
	mux.HandleFunc("GET /api/admin/threads/{threadID}", s.requireAdmin(s.handleReadThread))
	mux.HandleFunc("GET /api/admin/directory", s.requireAdmin(s.handleDirectory))
	mux.HandleFunc("GET /api/admin/board", s.requireAdmin(s.handleBoard))
	mux.HandleFunc("GET /api/admin/health", s.requireAdmin(s.handleHealth))
	mux.HandleFunc("POST /api/admin/attachments", s.requireAdmin(s.handleAdminUpload))
	mux.HandleFunc("GET /api/admin/attachments/{attachmentID}", s.requireAdmin(s.handleAttachmentDownload))
	mux.HandleFunc("GET /api/admin/settings", s.requireAdmin(s.handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", s.requireAdmin(s.handlePutSettings))

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
	// 每条 return 路径都要减回来，所以用 defer，不要在各个 return 前手写。
	s.inFlightPolls.Add(1)
	defer s.inFlightPolls.Add(-1)

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
		Body          string   `json:"body"`
		Mentions      []string `json:"mentions"`
		AttachmentIDs []string `json:"attachmentIds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Body == "" {
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.checkAttachmentCount(body.AttachmentIDs); err != (Error{}) {
		writeErr(w, err)
		return
	}
	mentions := make([]domain.AgentID, 0, len(body.Mentions))
	for _, m := range body.Mentions {
		mentions = append(mentions, domain.AgentID(m))
	}

	postID, err := s.store.AppendPost(r.Context(), store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: body.Body, Mentions: mentions, AttachmentIDs: body.AttachmentIDs,
	})
	if err != nil {
		if errors.Is(err, store.ErrAttachmentNotClaimable) {
			// 这是调用方的问题，不是服务的 —— 报 500 会让 agent 一直重试同一个
			// 挂不上的 id。
			writeErr(w, ErrAttachmentRejected)
			return
		}
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

// handleDeadLetter 接收 connector 上报的「我处理不了这条」。
//
// 死信不能反过来把 agent 的队列堵死，所以这个端点尽量宽容：
// 记不下来也只返回可重试的错误，不让 connector 卡在这里。
func (s *Server) handleDeadLetter(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	var d store.DeadLetter
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&d); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.RecordDeadLetter(r.Context(), agent, d); err != nil {
		s.log.Error("记录死信失败", "agent", agent, "seq", d.Seq, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	s.log.Warn("agent 上报死信", "agent", agent, "seq", d.Seq, "kind", d.Kind, "attempts", d.Attempts)
	w.WriteHeader(http.StatusNoContent)
}
