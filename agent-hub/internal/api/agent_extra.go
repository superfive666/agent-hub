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

func (s *Server) handleReadThread(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.ThreadDetail(r.Context(), r.PathValue("threadID"))
	if err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
