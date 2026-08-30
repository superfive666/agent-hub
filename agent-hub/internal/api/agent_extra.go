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
		if errors.Is(err, store.ErrWebhookNotOurContract) {
			// 填错的人以为自己在「省掉 connector」，实际是在把 hub 的信号
			// 灌进别人家的聊天通道。错误正文里已经写了该怎么改。
			writeErr(w, Error{Code: "webhook_not_our_contract", Message: err.Error()})
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

// handleAgentSelf 回答「我是谁」。
//
// agent 手上只有一个凭证，而写 Card 时 name 必须和管理员注册的对得上 ——
// 写错了自我介绍广播里就是个别人不认识的名字。名录接口给的是所有人，
// 要从里面捞自己得先知道自己的 agentId，绕一圈还是要先回答「我是谁」。
// cardVersion 也只有这里有：agent 靠它分辨首次撰写还是更新。
func (s *Server) handleAgentSelf(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	self, err := s.store.SelfOf(r.Context(), agent)
	if err != nil {
		s.log.Error("查 agent 自身失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, self)
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
//
// **用户确认闸门在这里**：管理员 approve 之前，start_work / submit_deliverable
// 一律 409。未确认阶段主 agent 该做的是把疑问问清楚 —— 那些动作全都不受影响：
// 发帖、追加 clarification 步骤、把状态设成 clarifying 都照常可用。
func (s *Server) handleAgentTodoState(w http.ResponseWriter, r *http.Request) {
	agent, _ := AgentFrom(r.Context())
	threadID := r.PathValue("threadID")

	var body struct{ Action, Note string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}

	if !s.requirePrimaryAgent(w, r, threadID, agent) {
		return
	}

	var next domain.TodoStatus
	switch body.Action {
	case "start_work":
		next = domain.StatusInProgress
	case "submit_deliverable":
		next = domain.StatusAwaitingReview
	case "clarify":
		// 「我看了，有问题要问」。闸门之前唯一能推的状态，也是最该推的那个 ——
		// 管理员在列表上一眼能看出这条 todo 已经被接手、正在澄清，
		// 而不是躺在 awaiting_response 里像没人管。
		next = domain.StatusClarifying
	case "decline":
		next = domain.StatusAwaitingResponse
	default:
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.AgentSetTodoStatus(r.Context(), threadID, next); err != nil {
		if errors.Is(err, domain.ErrTodoNotConfirmed) {
			e := ErrTodoNotConfirmed
			e.Message = err.Error()
			writeErr(w, e)
			return
		}
		if errors.Is(err, store.ErrTodoNotFound) {
			writeErr(w, ErrNotFound)
			return
		}
		s.log.Error("推进 todo 状态失败", "agent", agent, "thread", threadID, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": string(next)})
}

// requirePrimaryAgent 校验调用者是这条 todo 的主 agent。不是就已经把错误写出去了。
//
// 抽出来是因为「步骤」的两个写接口和状态推进用的是同一条规则：
// agent 只能操作属于自己的资源，而一条 todo 属于且只属于它的主 agent。
func (s *Server) requirePrimaryAgent(
	w http.ResponseWriter, r *http.Request, threadID string, me domain.AgentID,
) bool {
	primary, _, err := s.store.TodoOwner(r.Context(), threadID)
	if err != nil {
		writeErr(w, ErrNotFound)
		return false
	}
	if primary != me {
		writeErr(w, Error{Code: "not_primary_agent",
			Message: "只有这条 todo 的主 agent 能推进它"})
		return false
	}
	return true
}

// handleAppendTodoStep 追加一条「任务处理详情步骤」。
//
// **只有主 agent 能写，关注者只读。** 理由：这张表回答的是「这件事推进到哪一步了」，
// 而这个问题只该有一个答案 —— 它的责任人给出的那个。关注者要补充什么就在 thread 里发言，
// 那条路径有通知、有 @、有作者身份。放开写权限的结果是同一条 todo 上出现几条
// 互相矛盾的进度叙述，而看的人分不出哪条算数。
//
// 追加步骤**不发 inbox 事件**：它是过程记录，不是通知。真正需要别人知道的事情，
// 主 agent 会在 thread 里说一句 —— 那条路径本来就带扇出。步骤每加一条就吵一次的话，
// 关注者的 inbox 会被一条 todo 的内部流水淹掉。
func (s *Server) handleAppendTodoStep(w http.ResponseWriter, r *http.Request) {
	me := agentFrom(r)
	threadID := r.PathValue("threadID")

	var body struct {
		Kind   string `json:"kind"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Status string `json:"status"`
		PostID string `json:"postId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if !s.requirePrimaryAgent(w, r, threadID, me) {
		return
	}
	// confirmation 是管理员的确认动作，由 hub 自己写。让 agent 能写这一类，
	// 等于给了它一个「自己给自己放行」的入口，闸门就白设了。
	if domain.TodoStepKind(body.Kind) == domain.StepConfirmation {
		writeErr(w, Error{Code: "bad_request",
			Message: "confirmation 类型的步骤由 hub 在管理员确认时自动记录，不能由 agent 写入"})
		return
	}

	step, err := s.store.AppendTodoStep(r.Context(), store.AppendStepParams{
		ThreadID:  threadID,
		ActorKind: "agent", ActorAgentID: me, PostID: body.PostID,
		Step: domain.NewTodoStep{
			Kind: domain.TodoStepKind(body.Kind), Title: body.Title,
			Detail: body.Detail, Status: domain.TodoStepStatus(body.Status),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTodoStepKind), errors.Is(err, domain.ErrTodoStepStatus),
			errors.Is(err, domain.ErrTodoStepTitle):
			writeErr(w, Error{Code: "bad_request", Message: err.Error()})
		case errors.Is(err, store.ErrTodoNotFound):
			writeErr(w, ErrNotFound)
		default:
			s.log.Error("写处理步骤失败", "agent", me, "thread", threadID, "err", err)
			writeErr(w, ErrInternal)
		}
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

// handleUpdateTodoStep 改一条步骤的状态或说明（比如把预先铺好的 pending 改成 done）。
func (s *Server) handleUpdateTodoStep(w http.ResponseWriter, r *http.Request) {
	me := agentFrom(r)
	threadID := r.PathValue("threadID")

	var body struct {
		Status *string `json:"status"`
		Detail *string `json:"detail"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if body.Status == nil && body.Detail == nil {
		writeErr(w, Error{Code: "bad_request", Message: "至少要改 status 或 detail 之一"})
		return
	}
	if !s.requirePrimaryAgent(w, r, threadID, me) {
		return
	}

	var status *domain.TodoStepStatus
	if body.Status != nil {
		v := domain.TodoStepStatus(*body.Status)
		status = &v
	}
	step, err := s.store.UpdateTodoStep(r.Context(), store.UpdateStepParams{
		ThreadID: threadID, StepID: r.PathValue("stepID"),
		Status: status, Detail: body.Detail,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTodoStepStatus):
			writeErr(w, Error{Code: "bad_request", Message: err.Error()})
		case errors.Is(err, store.ErrStepNotFound):
			writeErr(w, ErrNotFound)
		default:
			s.log.Error("更新处理步骤失败", "agent", me, "thread", threadID, "err", err)
			writeErr(w, ErrInternal)
		}
		return
	}
	writeJSON(w, http.StatusOK, step)
}

// handleAgentTodoSteps 读步骤。**关注者也读得到** —— 想帮上忙就得先看得见
// 别人做到哪儿了；和 GET /api/agent/threads/{threadID} 的可见范围保持一致。
func (s *Server) handleAgentTodoSteps(w http.ResponseWriter, r *http.Request) {
	s.listTodoSteps(w, r)
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
