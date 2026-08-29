package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

// AgentRow 是控制台 agent 列表里的一行。
type AgentRow struct {
	AgentID    domain.AgentID `json:"agentId"`
	Name       string         `json:"name"`
	Purpose    string         `json:"purpose"`
	Status     string         `json:"status"`
	Runtime    string         `json:"runtime"`
	Tier       string         `json:"tier"`
	Online     bool           `json:"online"`
	LastPullAt *time.Time     `json:"lastPullAt,omitempty"`
	OpenTodos  int            `json:"openTodos"`
	HasCard    bool           `json:"hasCard"`
}

// ListAgents 给控制台用。
//
// 在线判定不是「连接是否存在」而是「最近一次拉取在窗口内」，
// **窗口按接入档位取值** —— 否则 cron 档的 agent 明明工作得好好的，界面上会永远显示离线。
func (s *Store) ListAgents(ctx context.Context) ([]AgentRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.purpose, a.status,
		       coalesce(c.runtime,''), coalesce(c.tier,''), c.agent_id IS NOT NULL,
		       st.last_pull_at,
		       coalesce(t.open_count, 0)
		FROM agent a
		LEFT JOIN LATERAL (
			SELECT * FROM agent_card WHERE agent_id = a.id ORDER BY version DESC LIMIT 1
		) c ON true
		LEFT JOIN agent_inbox_state st ON st.agent_id = a.id
		LEFT JOIN LATERAL (
			SELECT count(*) AS open_count FROM todo
			WHERE primary_agent_id = a.id AND status NOT IN ('done','cancelled')
		) t ON true
		ORDER BY a.name`)
	if err != nil {
		return nil, fmt.Errorf("查 agent 列表: %w", err)
	}
	defer rows.Close()

	out := []AgentRow{}
	for rows.Next() {
		var r AgentRow
		var lastPull sql.NullTime
		if err := rows.Scan(&r.AgentID, &r.Name, &r.Purpose, &r.Status,
			&r.Runtime, &r.Tier, &r.HasCard, &lastPull, &r.OpenTodos); err != nil {
			return nil, err
		}
		if lastPull.Valid {
			t := lastPull.Time
			r.LastPullAt = &t
			r.Online = time.Since(t) < onlineWindow(r.Tier)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// onlineWindow 按接入档位取不同的在线判定窗口。
func onlineWindow(tier string) time.Duration {
	switch tier {
	case "webhook":
		return 5 * time.Minute
	case "cron":
		return 12 * time.Minute // 默认 5 分钟轮询的两倍多一点
	default: // longpoll
		return 2 * time.Minute
	}
}

// TodoRow 是 todo 列表里的一行。
type TodoRow struct {
	ThreadID           string     `json:"threadId"`
	Title              string     `json:"title"`
	Status             string     `json:"status"`
	PrimaryAgentID     string     `json:"primaryAgentId"`
	PrimaryAgentName   string     `json:"primaryAgentName"`
	PrimaryAgentOnline bool       `json:"primaryAgentOnline"`
	Watchers           []string   `json:"watchers"`
	StartedAt          time.Time  `json:"startedAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	DueAt              *time.Time `json:"dueAt,omitempty"`
	ReplyCount         int        `json:"replyCount"`
}

// ListTodos 给控制台的 todo 列表。startedAt 取的是 thread 记录本身的日期。
func (s *Store) ListTodos(ctx context.Context, status string) ([]TodoRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT th.id, td.title, td.status, td.primary_agent_id, pa.name,
		       st.last_pull_at, coalesce(c.tier,''),
		       th.created_at, td.updated_at, td.due_at,
		       coalesce(pc.n, 0),
		       coalesce(array_agg(wa.name) FILTER (WHERE wa.id IS NOT NULL AND wa.id <> td.primary_agent_id), '{}')
		FROM todo td
		JOIN thread th ON th.id = td.thread_id
		JOIN agent pa ON pa.id = td.primary_agent_id
		LEFT JOIN agent_inbox_state st ON st.agent_id = pa.id
		LEFT JOIN LATERAL (
			SELECT tier FROM agent_card WHERE agent_id = pa.id ORDER BY version DESC LIMIT 1
		) c ON true
		LEFT JOIN LATERAL (SELECT count(*) AS n FROM post WHERE thread_id = th.id) pc ON true
		LEFT JOIN thread_watcher tw ON tw.thread_id = th.id
		LEFT JOIN agent wa ON wa.id = tw.agent_id
		WHERE ($1 = '' OR td.status = $1)
		GROUP BY th.id, td.title, td.status, td.primary_agent_id, pa.name,
		         st.last_pull_at, c.tier, th.created_at, td.updated_at, td.due_at, pc.n
		ORDER BY td.updated_at DESC`, status)
	if err != nil {
		return nil, fmt.Errorf("查 todo 列表: %w", err)
	}
	defer rows.Close()

	out := []TodoRow{}
	for rows.Next() {
		var r TodoRow
		var lastPull sql.NullTime
		var tier string
		var due sql.NullTime
		var watchers []byte
		if err := rows.Scan(&r.ThreadID, &r.Title, &r.Status, &r.PrimaryAgentID, &r.PrimaryAgentName,
			&lastPull, &tier, &r.StartedAt, &r.UpdatedAt, &due, &r.ReplyCount, &watchers); err != nil {
			return nil, err
		}
		if lastPull.Valid {
			r.PrimaryAgentOnline = time.Since(lastPull.Time) < onlineWindow(tier)
		}
		if due.Valid {
			t := due.Time
			r.DueAt = &t
		}
		r.Watchers = parsePGArray(string(watchers))
		out = append(out, r)
	}
	return out, rows.Err()
}

// parsePGArray 解析 text[] 的字面量输出。只用于读，写路径一律走 jsonb。
func parsePGArray(s string) []string {
	if len(s) < 2 || s == "{}" {
		return []string{}
	}
	s = s[1 : len(s)-1]
	out := []string{}
	var cur []rune
	inQuote, esc := false, false
	for _, c := range s {
		switch {
		case esc:
			cur = append(cur, c)
			esc = false
		case c == '\\':
			esc = true
		case c == '"':
			inQuote = !inQuote
		case c == ',' && !inQuote:
			out = append(out, string(cur))
			cur = nil
		default:
			cur = append(cur, c)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// SetTodoStatus 推进 todo 状态。状态由 thread 里的动作驱动，这里只落库。
func (s *Store) SetTodoStatus(ctx context.Context, threadID string, status domain.TodoStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE todo SET status = $2, updated_at = now() WHERE thread_id = $1`, threadID, string(status))
	if err != nil {
		return fmt.Errorf("更新 todo 状态: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// BoardItem 是看板上的一条。
type BoardItem struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	ThreadID   string    `json:"threadId"`
	ThreadKind string    `json:"threadKind"`
	AuthorKind string    `json:"authorKind"`
	AuthorName string    `json:"authorName"`
	Summary    string    `json:"summary"`
	// 「按开始」口径独有：显示的是**当前**状态与累计统计，不是当天快照
	Status         string     `json:"status,omitempty"`
	ReplyCount     int        `json:"replyCount,omitempty"`
	LastActivityAt *time.Time `json:"lastActivityAt,omitempty"`
}

// Board 返回某一天的活动。
//
// 两种口径回答两个不同的问题：
//   - activity：这一天**发生了什么**。以 post.created_at 分桶，一条 thread 会跨多天反复出现
//   - started：这一天**开了哪些事、现在怎么样了**。以 thread.created_at 分桶，只出现一次
func (s *Store) Board(ctx context.Context, day time.Time, groupBy string, loc *time.Location) ([]BoardItem, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)

	if groupBy == "started" {
		return s.boardByStart(ctx, start, end)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.created_at, th.kind, th.id, p.author_kind,
		       coalesce(a.name, 'superfive'), left(p.body, 200)
		FROM post p
		JOIN thread th ON th.id = p.thread_id
		LEFT JOIN agent a ON a.id = p.author_id
		WHERE p.created_at >= $1 AND p.created_at < $2
		ORDER BY p.created_at`, start, end)
	if err != nil {
		return nil, fmt.Errorf("查看板: %w", err)
	}
	defer rows.Close()

	out := []BoardItem{}
	for rows.Next() {
		var it BoardItem
		if err := rows.Scan(&it.At, &it.ThreadKind, &it.ThreadID,
			&it.AuthorKind, &it.AuthorName, &it.Summary); err != nil {
			return nil, err
		}
		it.Kind = it.ThreadKind
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) boardByStart(ctx context.Context, start, end time.Time) ([]BoardItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT th.created_at, th.kind, th.id,
		       coalesce(td.title, left(tw.body, 200)),
		       coalesce(td.status, ''),
		       coalesce(pa.name, aw.name, ''),
		       stats.reply_count, stats.last_activity_at
		FROM thread th
		LEFT JOIN todo td ON td.thread_id = th.id
		LEFT JOIN tweet tw ON tw.thread_id = th.id
		LEFT JOIN agent pa ON pa.id = td.primary_agent_id
		LEFT JOIN agent aw ON aw.id = tw.author_agent_id
		LEFT JOIN LATERAL (
			SELECT count(*) AS reply_count, max(created_at) AS last_activity_at
			FROM post WHERE thread_id = th.id
		) stats ON true
		WHERE th.created_at >= $1 AND th.created_at < $2
		ORDER BY th.created_at`, start, end)
	if err != nil {
		return nil, fmt.Errorf("查看板（按开始）: %w", err)
	}
	defer rows.Close()

	out := []BoardItem{}
	for rows.Next() {
		var it BoardItem
		var last sql.NullTime
		if err := rows.Scan(&it.At, &it.ThreadKind, &it.ThreadID, &it.Summary,
			&it.Status, &it.AuthorName, &it.ReplyCount, &last); err != nil {
			return nil, err
		}
		it.Kind = it.ThreadKind
		if last.Valid {
			t := last.Time
			it.LastActivityAt = &t // 很可能落在别的日期上 —— 这正是这个视图的用处
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Settings 是部署级配置里可以热改的那部分。
type Settings struct {
	Timezone           string         `json:"timezone"`
	LongPollMaxSeconds int            `json:"longPollMaxSeconds"`
	InboxRetentionDays int            `json:"inboxRetentionDays"`
	OnlineWindow       map[string]int `json:"onlineWindowSeconds"`
	RateLimits         map[string]int `json:"rateLimits"`
}

func defaultSettings(tz string) Settings {
	return Settings{
		Timezone: tz, LongPollMaxSeconds: 30, InboxRetentionDays: 30,
		OnlineWindow: map[string]int{"longpoll": 120, "webhook": 300, "cron": 720},
		RateLimits: map[string]int{
			"tweetsPerHour": 10, "inboxWritesPerMinute": 200, "apiRequestsPerMinute": 600},
	}
}

// GetSettings 读配置，没写过就返回默认值。
func (s *Store) GetSettings(ctx context.Context, fallbackTZ string) (Settings, error) {
	var raw []byte
	var tz string
	err := s.db.QueryRowContext(ctx,
		`SELECT timezone, config FROM platform_config WHERE id = true`).Scan(&tz, &raw)
	if err == sql.ErrNoRows {
		return defaultSettings(fallbackTZ), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("读配置: %w", err)
	}
	out := defaultSettings(tz)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	out.Timezone = tz
	return out, nil
}

// PutSettings 写配置。时区非法直接拒绝 —— 它决定看板按什么切分「一天」，
// 配错了每个人看到的「今天」会不一样，这是最难查的一类问题。
func (s *Store) PutSettings(ctx context.Context, in Settings) error {
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return fmt.Errorf("时区无效: %w", err)
	}
	blob, err := json.Marshal(in)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO platform_config (id, timezone, config) VALUES (true, $1, $2)
		ON CONFLICT (id) DO UPDATE SET timezone = $1, config = $2, updated_at = now()`,
		in.Timezone, blob)
	if err != nil {
		return fmt.Errorf("写配置: %w", err)
	}
	return nil
}

// Audit 记一条管理员操作。只记管理员的写操作 ——
// agent 的动作在看板和各自的 thread 里，混进来会让审计失去意义。
func (s *Store) Audit(ctx context.Context, actor, action, target string, detail map[string]any) {
	blob, _ := json.Marshal(detail)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor, action, target, detail) VALUES ($1,$2,$3,$4)`,
		actor, action, target, blob); err != nil {
		// 审计失败不该阻断业务，但要留下痕迹。
		fmt.Printf("审计写入失败: %v\n", err)
	}
}

// TodoOwner 返回一条 todo 的主 agent 与当前状态。
func (s *Store) TodoOwner(ctx context.Context, threadID string) (domain.AgentID, domain.TodoStatus, error) {
	var id, status string
	err := s.db.QueryRowContext(ctx,
		`SELECT primary_agent_id, status FROM todo WHERE thread_id = $1`, threadID).Scan(&id, &status)
	if err != nil {
		return "", "", err
	}
	return domain.AgentID(id), domain.TodoStatus(status), nil
}

// ThreadPost 是 thread 详情里的一条发言。
type ThreadPost struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"threadId"`
	AuthorKind string    `json:"authorKind"`
	AuthorID   string    `json:"authorId,omitempty"`
	AuthorName string    `json:"authorName"`
	Body       string    `json:"body"`
	Mentions   []string  `json:"mentions"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ThreadDetailResult 是一条 thread 的全貌。todo 与 tweet 共用。
type ThreadDetailResult struct {
	ThreadID       string       `json:"threadId"`
	Kind           string       `json:"kind"`
	StartedAt      time.Time    `json:"startedAt"`
	Title          string       `json:"title,omitempty"`
	Status         string       `json:"status,omitempty"`
	PrimaryAgentID string       `json:"primaryAgentId,omitempty"`
	DueAt          *time.Time   `json:"dueAt,omitempty"`
	Tags           []string     `json:"tags"`
	Watchers       []WatcherRow `json:"watchers"`
	Posts          []ThreadPost `json:"posts"`
}

// WatcherRow 是关注者。reason 为 primary 的必须响应，另两种只是关注。
type WatcherRow struct {
	AgentID string `json:"agentId"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	Online  bool   `json:"online"`
}

// ThreadDetail 读一条 thread 的全貌。
func (s *Store) ThreadDetail(ctx context.Context, threadID string) (ThreadDetailResult, error) {
	var d ThreadDetailResult
	var title, status, primary sql.NullString
	var due sql.NullTime
	var tags []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT th.id, th.kind, th.created_at,
		       td.title, td.status, td.primary_agent_id, td.due_at,
		       coalesce(td.tags, tw.tags, '{}')
		FROM thread th
		LEFT JOIN todo td ON td.thread_id = th.id
		LEFT JOIN tweet tw ON tw.thread_id = th.id
		WHERE th.id = $1`, threadID).
		Scan(&d.ThreadID, &d.Kind, &d.StartedAt, &title, &status, &primary, &due, &tags)
	if err != nil {
		return d, err
	}
	d.Title, d.Status, d.PrimaryAgentID = title.String, status.String, primary.String
	if due.Valid {
		t := due.Time
		d.DueAt = &t
	}
	d.Tags = parsePGArray(string(tags))

	prows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.author_kind, coalesce(p.author_id::text,''), coalesce(a.name,'superfive'),
		       p.body, p.created_at,
		       coalesce(array_agg(m.agent_id::text) FILTER (WHERE m.agent_id IS NOT NULL), '{}')
		FROM post p
		LEFT JOIN agent a ON a.id = p.author_id
		LEFT JOIN mention m ON m.post_id = p.id
		WHERE p.thread_id = $1
		GROUP BY p.id, p.author_kind, p.author_id, a.name, p.body, p.created_at
		ORDER BY p.created_at`, threadID)
	if err != nil {
		return d, fmt.Errorf("读 thread 发言: %w", err)
	}
	defer prows.Close()
	d.Posts = []ThreadPost{}
	for prows.Next() {
		var p ThreadPost
		var mentions []byte
		if err := prows.Scan(&p.ID, &p.AuthorKind, &p.AuthorID, &p.AuthorName,
			&p.Body, &p.CreatedAt, &mentions); err != nil {
			return d, err
		}
		p.ThreadID = threadID
		p.Mentions = parsePGArray(string(mentions))
		d.Posts = append(d.Posts, p)
	}

	wrows, err := s.db.QueryContext(ctx, `
		SELECT tw.agent_id::text, a.name, tw.reason, st.last_pull_at, coalesce(c.tier,'')
		FROM thread_watcher tw
		JOIN agent a ON a.id = tw.agent_id
		LEFT JOIN agent_inbox_state st ON st.agent_id = a.id
		LEFT JOIN LATERAL (
			SELECT tier FROM agent_card WHERE agent_id = a.id ORDER BY version DESC LIMIT 1
		) c ON true
		WHERE tw.thread_id = $1
		ORDER BY tw.reason = 'primary' DESC, a.name`, threadID)
	if err != nil {
		return d, fmt.Errorf("读关注者: %w", err)
	}
	defer wrows.Close()
	d.Watchers = []WatcherRow{}
	for wrows.Next() {
		var wr WatcherRow
		var lastPull sql.NullTime
		var tier string
		if err := wrows.Scan(&wr.AgentID, &wr.Name, &wr.Reason, &lastPull, &tier); err != nil {
			return d, err
		}
		if lastPull.Valid {
			wr.Online = time.Since(lastPull.Time) < onlineWindow(tier)
		}
		d.Watchers = append(d.Watchers, wr)
	}
	return d, nil
}
