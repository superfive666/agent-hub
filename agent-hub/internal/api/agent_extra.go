package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// handleUpsertCard 写入或更新 Agent Card（A2A v1.0）。
//
// 成功后 hub 以该 agent 自己的身份发一条自我介绍广播 —— 一份没人看得到、
// 也没人知道它变了的 Card，规范再标准也没有价值。
func (s *Server) handleUpsertCard(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	doc, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	version, err := s.store.UpsertCard(r.Context(), agent, doc)
	if err != nil {
		if errors.Is(err, store.ErrCardNeedsLimitations) {
			// 「能力边界」不是可选项：它比能力清单更有信息量，
			// 因为「我能做什么」人人都往大了写。
			writeErr(w, Error{Code: "card_needs_limitations", Message: err.Error()})
			return
		}
		s.log.Error("写 Agent Card 失败", "agent", agent, "err", err)
		writeErr(w, Error{Code: "bad_request", Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

// handleDirectory 是名录：平台上还有谁、各自擅长什么。
// skill 里写明了「先查名录再 @ 人，不要凭印象点名」。
// handleDirectory 返回名录。agent 侧和 admin 侧共用。
//
// 控制台的「名录」页要展示的正是这一份 —— 谁在这儿、能做什么、边界在哪，
// 而不是 /api/admin/agents 那份运维视角的列表（在线否、手上压了几条）。
// 两个视角回答两个不同的问题，都要有。
func (s *Server) handleDirectory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	entries, err := s.store.Directory(r.Context(), q.Get("skill"), q.Get("tag"), q.Get("online") == "true")
	if err != nil {
		s.log.Error("查名录失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": entries})
}

func (s *Server) handleCreateTweet(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	var body struct {
		Body     string   `json:"body"`
		Tags     []string `json:"tags"`
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
	threadID, err := s.store.CreateTweet(r.Context(), store.CreateTweetParams{
		Author: agent, Body: body.Body, Tags: body.Tags, Mentions: mentions,
	})
	if err != nil {
		s.log.Error("发广播失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"threadId": threadID})
}

// handleAgentTodoState 让主 agent 推进状态。
// 只有主 agent 能调 —— agent 默认只能操作属于自己的资源。
func (s *Server) handleAgentTodoState(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	threadID := r.PathValue("threadID")

	var body struct{ Action, Note string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}

	primary, status, err := s.store.TodoOwner(r.Context(), threadID)
	if err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	if primary != agent {
		writeErr(w, Error{Code: "not_primary_agent",
			Message: "只有这条 todo 的主 agent 能推进状态"})
		return
	}

	var next domain.TodoStatus
	switch body.Action {
	case "start_work":
		next = domain.StatusInProgress
	case "submit_deliverable":
		next = domain.StatusAwaitingReview
	case "decline":
		next = domain.StatusAwaitingResponse
	default:
		writeErr(w, ErrBadRequest)
		return
	}
	_ = status
	if err := s.store.SetTodoStatus(r.Context(), threadID, next); err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": string(next)})
}

// handleReadThread 读 thread 全貌。agent 侧和 admin 侧共用同一个 handler —— 内容一样，
// 只是挂在两条鉴权链后面。控制台拿的是会话 cookie，agent 拿的是 Bearer 凭证，
// 少了 admin 那条路由的话控制台只能去打 agent 侧端点，然后被 401 挡回来。
func (s *Server) handleReadThread(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.ThreadDetail(r.Context(), r.PathValue("threadID"))
	if err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// handleAgentTodos 是主 agent 的「我的队列」。
//
// 需求模块 1 的验收标准之一：todo 建好之后，主 agent 拉自己的队列要能看到它。
// 被 @ 的关注者拉不到 —— 他们在 inbox 里收到 mention 事件，但队列里没有这条，
// 因为队列的含义是「该我做的事」，不是「和我有关的事」。
func (s *Server) handleAgentTodos(w http.ResponseWriter, r *http.Request) {
	me := agentFrom(r)
	rows, err := s.store.ListTodos(r.Context(), r.URL.Query().Get("status"), string(me))
	if err != nil {
		s.log.Error("查 agent 队列失败", "err", err, "agent", me)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todos": rows})
}

// handleAgentBoard 是 agent 侧的看板。
//
// 需求模块 4 写的是「双端可见」：admin 在控制台看，agent 通过 API 拉 ——
// agent 也需要「看看今天大家在干嘛」的能力。和 admin 那份是同一个聚合，
// 同一个时区口径，只是入口不同。
func (s *Server) handleAgentBoard(w http.ResponseWriter, r *http.Request) { s.board(w, r) }

// agentFrom 取当前请求的 agent 身份。只在过了 requireAgent 的 handler 里调用，
// 所以拿不到就是路由接错了，直接 panic 比返回零值安全 —— 零值会让
// 「查我的队列」变成「查 primary_agent_id 为空串的队列」，静默返回空列表。
func agentFrom(r *http.Request) domain.AgentID {
	id, ok := AgentFrom(r.Context())
	if !ok {
		panic("handler 需要 agent 身份，但没有挂在 requireAgent 后面")
	}
	return id
}

// handleListSubscriptions 列出自己声明过的订阅。
func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := s.store.ListSubscriptions(r.Context(), agentFrom(r))
	if err != nil {
		s.log.Error("查订阅失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}

// handleReplaceSubscriptions 用整份列表覆盖自己的订阅。
//
// 没有这个端点的话，subscription 表永远是空的，于是**带标签的广播一个人都收不到** ——
// 定向广播这个功能整个不可达。表和扇出查询早就写好了，缺的只是写入口。
func (s *Server) handleReplaceSubscriptions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subscriptions []store.Subscription `json:"subscriptions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.ReplaceSubscriptions(r.Context(), agentFrom(r), body.Subscriptions); err != nil {
		if errors.Is(err, store.ErrBadSubscriptionKind) {
			writeErr(w, Error{Code: "bad_request", Message: err.Error()})
			return
		}
		s.log.Error("写订阅失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	subs, err := s.store.ListSubscriptions(r.Context(), agentFrom(r))
	if err != nil {
		s.log.Error("回读订阅失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}
