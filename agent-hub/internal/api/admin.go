package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "hub_session"

// adminSubject 是会话里那个唯一管理员的标识。
//
// 两种模式下身份的载体不同：口令模式是用户名，OIDC 模式是那个预置的 Google 邮箱。
// 会话签名和校验都必须用同一个值，否则 OIDC 登完立刻就会被自己的 requireAdmin 拒掉
// （AdminUsername 在 OIDC 模式下是空的）。
func (s *Server) adminSubject() string {
	if s.cfg.AuthMode == config.AuthOIDC {
		return s.cfg.AdminGoogleEmail
	}
	return s.cfg.AdminUsername
}

// requireAdmin 校验会话 cookie。
//
// 这个平台只有一个管理员，凭据在部署时预置。**不在预置名单里的账号根本进不来** ——
// 不是「登录后无权限」，是连会话都拿不到。
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.validSession(c.Value) {
			writeErr(w, ErrUnauthorized)
			return
		}
		next(w, r)
	}
}

// signSession 签一个带过期时间的会话串。用 HMAC 而不是存库：
// 只有一个管理员，会话表的运维成本换不到任何东西。
func (s *Server) signSession(username string, exp time.Time) string {
	payload := username + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSession(v string) bool {
	parts := strings.SplitN(v, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	mac.Write(payload)
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return false
	}
	fields := strings.SplitN(string(payload), "|", 2)
	if len(fields) != 2 || subtle.ConstantTimeCompare([]byte(fields[0]), []byte(s.adminSubject())) != 1 {
		return false
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	return err == nil && time.Now().Unix() < exp
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if s.cfg.AuthMode != config.AuthPassword {
		writeErr(w, Error{Code: "unauthorized", Message: "本实例配置为 Google OIDC 登录"})
		return
	}
	// 用户名不对也走一遍 bcrypt，避免用响应时间区分「用户名不存在」和「密码错误」。
	nameOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.cfg.AdminUsername)) == 1
	passOK := bcrypt.CompareHashAndPassword([]byte(s.cfg.AdminPasswordHash), []byte(body.Password)) == nil
	if !nameOK || !passOK {
		s.log.Warn("管理员登录失败", "username", body.Username, "remote", r.RemoteAddr)
		writeErr(w, ErrUnauthorized)
		return
	}

	exp := time.Now().Add(12 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.signSession(s.adminSubject(), exp),
		Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil,
	})
	s.store.Audit(r.Context(), s.adminSubject(), "login", "", map[string]any{"mode": "password"})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"username": s.adminSubject(), "authMode": string(s.cfg.AuthMode), "timezone": s.cfg.Timezone,
	})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAgents(r.Context())
	if err != nil {
		s.log.Error("查 agent 列表失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": rows})
}

// handleCreateAgent 建一条 agent 记录，可选地在同一个响应里连注册 token 一起给出。
//
// 「像注册 CI runner 那样」：控制台上填个名字，拿到一串一次性 token，
// 复制到那台机器上跑一下就接进来了。issueToken 为 true 时省掉第二次往返，
// 也顺带消掉「记录建好了但 token 没签出来」那个中间态 —— 两件事在一个事务里。
func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Purpose    string `json:"purpose"`
		Owner      string `json:"owner"`
		IssueToken bool   `json:"issueToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if body.Owner == "" {
		body.Owner = s.adminSubject()
	}

	res, err := s.store.CreateAgentWithToken(r.Context(), store.CreateAgentParams{
		Name: body.Name, Purpose: body.Purpose, Owner: body.Owner,
		IssueToken: body.IssueToken, TokenTTL: store.DefaultRegistrationTokenTTL,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAgentNameTaken):
			// 撞名是调用方能自己修的事，不是服务器错误 —— 以前这里会 500。
			writeErr(w, ErrAgentNameTaken)
		case errors.Is(err, domain.ErrAgentNameRequired),
			errors.Is(err, domain.ErrAgentNameTooLong),
			errors.Is(err, domain.ErrAgentNameCharset):
			writeErr(w, Error{Code: "bad_request", Message: err.Error()})
		default:
			s.log.Error("创建 agent 失败", "err", err)
			writeErr(w, ErrInternal)
		}
		return
	}

	s.store.Audit(r.Context(), s.adminSubject(), "create_agent", string(res.AgentID),
		map[string]any{"name": body.Name, "issueToken": body.IssueToken})
	out := map[string]any{"agentId": string(res.AgentID)}
	if res.RegistrationToken != "" {
		// 明文只在这里出现一次，库里只有哈希。签发也要单独留一条审计 ——
		// 「谁在什么时候给这个 agent 发过 token」和「谁建了它」是两件事。
		s.store.Audit(r.Context(), s.adminSubject(), "issue_registration_token",
			string(res.AgentID), map[string]any{"via": "create_agent"})
		out["registrationToken"] = res.RegistrationToken
		out["expiresAt"] = res.ExpiresAt
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	agent := domain.AgentID(r.PathValue("agentID"))
	plain, exp, err := s.store.IssueRegistrationToken(r.Context(), agent, store.DefaultRegistrationTokenTTL)
	if err != nil {
		s.log.Error("签发注册 token 失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "issue_registration_token", string(agent), nil)
	// 明文只在这里返回一次，库里只有哈希。关掉页面就再也看不到，只能作废重发。
	writeJSON(w, http.StatusCreated, map[string]any{
		"registrationToken": plain, "expiresAt": exp,
	})
}

func (s *Server) handleRevokeCredentials(w http.ResponseWriter, r *http.Request) {
	agent := domain.AgentID(r.PathValue("agentID"))
	if err := s.store.RevokeCredentials(r.Context(), agent); err != nil {
		s.log.Error("吊销凭证失败", "agent", agent, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "revoke_credentials", string(agent), nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateAgent 改简介、停用 / 启用。
//
// **没有改名这条路，这是故意的。** 名字是 `@` 提及的唯一标识，改掉之后正文里
// 那些已经写好的 `@old-name` 会静默失效（解析不到就当普通文本），
// 没有任何地方会报错，只是那些 agent 从此收不到本该属于它们的通知。
func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agent := domain.AgentID(r.PathValue("agentID"))
	var body struct {
		Purpose *string `json:"purpose"`
		// Enabled 为 nil 表示这次不动状态。用指针而不是 bool ——
		// 零值 false 和「没传」必须分得开，否则只想改简介的请求会顺手把 agent 停掉。
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if body.Purpose == nil && body.Enabled == nil {
		writeErr(w, Error{Code: "bad_request", Message: "至少要给 purpose 或 enabled 其中之一"})
		return
	}

	if body.Purpose != nil {
		if err := s.store.UpdateAgentPurpose(r.Context(), agent, *body.Purpose); err != nil {
			s.agentWriteErr(w, "改 agent 简介", agent, err)
			return
		}
		s.store.Audit(r.Context(), s.adminSubject(), "update_agent_purpose", string(agent), nil)
	}

	status := ""
	if body.Enabled != nil {
		next, err := s.store.SetAgentEnabled(r.Context(), agent, *body.Enabled)
		if err != nil {
			s.agentWriteErr(w, "改 agent 状态", agent, err)
			return
		}
		status = next
		// 停用会立刻让这个 agent 的凭证认证不过（AuthenticateCredential 要求
		// status='active'），所以这是一条安全相关的操作，必须留痕。
		s.store.Audit(r.Context(), s.adminSubject(), "set_agent_enabled", string(agent),
			map[string]any{"enabled": *body.Enabled, "status": next})
	}

	out := map[string]any{"agentId": string(agent)}
	if status != "" {
		out["status"] = status
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteAgent 物理删除，只在没有内容留痕时允许；有留痕就让调用方去停用。
func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agent := domain.AgentID(r.PathValue("agentID"))
	refs, err := s.store.DeleteAgent(r.Context(), agent)
	if err != nil {
		if errors.Is(err, store.ErrAgentInUse) {
			// 把计数一起回去 —— 只说「删不掉」的话，调用方只能自己去库里查为什么。
			writeJSON(w, http.StatusConflict, map[string]any{
				"code": "agent_in_use",
				"message": "这个 agent 已经在内容里留下痕迹，删掉会让已经发生的事失去主语。" +
					"改用停用：它会立刻下线，凭证保留，随时能再启用。",
				"retryable": false,
				"refs":      refs,
			})
			return
		}
		s.agentWriteErr(w, "删除 agent", agent, err)
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "delete_agent", string(agent), nil)
	w.WriteHeader(http.StatusNoContent)
}

// agentWriteErr 把 store 的错误翻成 HTTP。三个写接口共用，省得每处都抄一遍
// 「不存在就 404，其余记日志再 500」。
func (s *Server) agentWriteErr(w http.ResponseWriter, what string, agent domain.AgentID, err error) {
	if errors.Is(err, store.ErrAgentNotFound) {
		writeErr(w, ErrNotFound)
		return
	}
	s.log.Error(what+"失败", "agent", agent, "err", err)
	writeErr(w, ErrInternal)
}

func (s *Server) handleListTodos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, err := s.store.ListTodos(r.Context(), q.Get("status"), q.Get("primaryAgentId"))
	if err != nil {
		s.log.Error("查 todo 列表失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todos": rows})
}

func (s *Server) handleCreateTodo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title          string   `json:"title"`
		Body           string   `json:"body"`
		PrimaryAgentID string   `json:"primaryAgentId"`
		Mentions       []string `json:"mentions"`
		Tags           []string `json:"tags"`
		DueAt          *string  `json:"dueAt"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	mentions := make([]domain.AgentID, 0, len(body.Mentions))
	for _, m := range body.Mentions {
		mentions = append(mentions, domain.AgentID(m))
	}
	var due *time.Time
	if body.DueAt != nil && *body.DueAt != "" {
		if t, err := time.Parse(time.RFC3339, *body.DueAt); err == nil {
			due = &t
		}
	}

	res, err := s.store.CreateTodo(r.Context(), store.CreateTodoParams{
		New: domain.NewTodo{
			Title: body.Title, Body: body.Body,
			PrimaryAgentID: domain.AgentID(body.PrimaryAgentID), Mentions: mentions,
		},
		CreatedBy: s.adminSubject(), DueAt: due, Tags: body.Tags,
	})
	if err != nil {
		// 主 agent 必选是业务规则，不是服务器错误 —— 要让调用方看懂。
		if errors.Is(err, domain.ErrPrimaryAgentRequired) ||
			errors.Is(err, domain.ErrTitleRequired) || errors.Is(err, domain.ErrBodyRequired) {
			writeErr(w, Error{Code: "bad_request", Message: err.Error()})
			return
		}
		s.log.Error("创建 todo 失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "create_todo", res.ThreadID,
		map[string]any{"primaryAgentId": body.PrimaryAgentID})
	writeJSON(w, http.StatusCreated, map[string]any{
		"threadId": res.ThreadID, "startedAt": res.StartedAt,
	})
}

// handleAdminTodoState 是管理员对一条 todo 的四个动作。
//
// 四个动作分属两个阶段，不要混淆：
//   - approve —— **确认需求，可以开工**。这是「用户确认闸门」：在它之前主 agent
//     只能提问、澄清，推不动状态；在它之后才允许干活。
//   - confirm / reject —— 交付之后的确认完成 / 打回。reject 只在 awaiting_review 上成立。
//   - cancel —— 任何阶段都能撤。
func (s *Server) handleAdminTodoState(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("threadID")
	var body struct{ Action, Note string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}

	if body.Action == "approve" {
		s.approveTodo(w, r, threadID)
		return
	}

	var next domain.TodoStatus
	switch body.Action {
	case "confirm":
		next = domain.StatusDone
	case "reject":
		// 打回只在「交上来了」这个状态上有确定含义，见 store.RejectTodo。
		if err := s.store.RejectTodo(r.Context(), threadID); err != nil {
			if errors.Is(err, store.ErrInvalidTodoTransition) {
				writeErr(w, Error{Code: "invalid_todo_transition",
					Message: "只有待确认（awaiting_review）的 todo 才能打回；" +
						"确认之前请在 thread 里发帖说明，并暂不点确认"})
				return
			}
			writeErr(w, ErrNotFound)
			return
		}
		s.store.Audit(r.Context(), s.adminSubject(), "todo_reject", threadID,
			map[string]any{"note": body.Note})
		writeJSON(w, http.StatusOK, map[string]string{"status": string(domain.StatusInProgress)})
		return
	case "cancel":
		next = domain.StatusCancelled
	default:
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.SetTodoStatus(r.Context(), threadID, next); err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "todo_"+body.Action, threadID,
		map[string]any{"note": body.Note})
	writeJSON(w, http.StatusOK, map[string]string{"status": string(next)})
}

// approveTodo 打开确认闸门。重复 approve 是幂等的：不改时间戳、不重复发事件。
func (s *Server) approveTodo(w http.ResponseWriter, r *http.Request, threadID string) {
	res, err := s.store.ApproveTodo(r.Context(), threadID, s.adminSubject())
	if err != nil {
		if errors.Is(err, store.ErrTodoNotFound) {
			writeErr(w, ErrNotFound)
			return
		}
		s.log.Error("确认 todo 失败", "thread", threadID, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	if !res.AlreadyConfirmed {
		// 重复 approve 不再记一条审计 —— 审计要能回答「这条 todo 是谁、什么时候放行的」，
		// 同一个答案记三遍只会让人以为放行过三次。
		s.store.Audit(r.Context(), s.adminSubject(), "todo_approve", threadID, nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": string(res.Status), "confirmedAt": res.ConfirmedAt,
		"confirmedBy": res.ConfirmedBy, "alreadyConfirmed": res.AlreadyConfirmed,
	})
}

// handleAdminTodoSteps 读一条 todo 的处理详情步骤。
func (s *Server) handleAdminTodoSteps(w http.ResponseWriter, r *http.Request) {
	s.listTodoSteps(w, r)
}

func (s *Server) listTodoSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := s.store.ListTodoSteps(r.Context(), r.PathValue("threadID"))
	if err != nil {
		s.log.Error("查处理步骤失败", "thread", r.PathValue("threadID"), "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps})
}

// handleAdminPost 让管理员以人类身份回帖。
// authorKind 落成 admin，前端据此换字体、换气泡底色、靠右并挂「人类」标签。
func (s *Server) handleAdminPost(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("threadID")
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
		ThreadID: threadID, AuthorKind: "admin", Body: body.Body, Mentions: mentions,
	})
	if err != nil {
		s.log.Error("管理员回帖失败", "thread", threadID, "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"postId": postID})
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) { s.board(w, r) }

// board 是看板的实现，admin 侧和 agent 侧共用。
//
// 需求模块 4 要求「双端可见」：同一个日期、同一个时区口径、同一份聚合。
// 两边各写一份的话，「管理员看到的今天」和「agent 看到的今天」迟早会不一样。
func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	loc, err := time.LoadLocation(s.cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	day := time.Now().In(loc)
	if v := q.Get("date"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			day = t
		}
	}
	groupBy := q.Get("groupBy")
	if groupBy == "" {
		groupBy = "activity"
	}
	items, err := s.store.Board(r.Context(), day, groupBy, loc)
	if err != nil {
		s.log.Error("查看板失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"groupBy": groupBy, "date": day.Format("2006-01-02"), "items": items,
	})
}

// handleHealth 是控制台唯一的运行状态探针，契约见 openapi.yaml 的 /api/admin/health。
//
// **五个字段必须全部返回，一个都不能省。** 前端对缺失字段的兜底是「读不到就当坏了」
// （`data.workerAlive ?? false`），这个语义本身是对的 —— 探针读不到就该报警，
// 不能因为字段缺失而显示「一切正常」。代价是：这里少返回一个字段，
// 控制台就会**永远**挂着一条假告警，而底下的 worker 其实活得好好的。
// 假告警和漏告警一样坏：第三次「又是误报」之后，就没人再看这条横幅了。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lag, err := s.store.OutboxLagSeconds(ctx)
	if err != nil {
		s.log.Error("查 outbox 滞后失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	pending, err := s.store.OutboxPendingCount(ctx)
	if err != nil {
		s.log.Error("统计待扇出事件失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	dead, err := s.store.DeadLetterCount(ctx)
	if err != nil {
		s.log.Error("统计死信失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	// worker 的存活判据是它那把 advisory lock 还在不在，见 store.WorkerAlive。
	alive, err := s.store.WorkerAlive(ctx)
	if err != nil {
		s.log.Error("查 worker 存活失败", "err", err)
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"outboxLagSeconds": lag,
		"outboxPending":    pending,
		"outboxDead":       dead,
		"workerAlive":      alive,
		"pendingLongPolls": s.inFlightPolls.Load(),
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), s.cfg.Timezone)
	if err != nil {
		writeErr(w, ErrInternal)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeErr(w, ErrBadRequest)
		return
	}
	if err := s.store.PutSettings(r.Context(), in); err != nil {
		writeErr(w, Error{Code: "bad_request", Message: err.Error()})
		return
	}
	s.store.Audit(r.Context(), s.adminSubject(), "update_settings", "", nil)
	writeJSON(w, http.StatusOK, in)
}
