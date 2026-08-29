package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/superfive666/agent-hub/internal/domain"
)

// ProfileExtURI 是我们在 A2A Card 里的扩展前缀。
// A2A 没有「能力边界」这一项，只能走它规范内的 AgentExtension 承载。
const ProfileExtURI = "https://agent-hub/ext/agent-profile"

// ErrCardNeedsLimitations 是那条硬要求：能力边界不许为空。
//
// 它比能力清单更有信息量 —— 「我能做什么」人人都往大了写，
// 而选主 agent 时真正有用的是「它做不了什么」。
var ErrCardNeedsLimitations = errors.New("Agent Card 必须写明能力边界（不能做什么）")

// CardProfile 是我们塞在 A2A 扩展里的那部分。
type CardProfile struct {
	Limitations           []string `json:"limitations"`
	Tools                 []string `json:"tools"`
	Runtime               string   `json:"runtime"`
	Tier                  string   `json:"tier"`
	TypicalLatencySeconds int      `json:"typicalLatencySeconds"`
	MaxConcurrency        int      `json:"maxConcurrency"`
	Availability          string   `json:"availability"`
	WebhookURL            string   `json:"webhookUrl"`
}

// ParseCard 从 A2A 文档里挑出我们要的字段。
func ParseCard(doc []byte) (name, description string, p CardProfile, err error) {
	var card struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Capabilities struct {
			Extensions []struct {
				URI    string      `json:"uri"`
				Params CardProfile `json:"params"`
			} `json:"extensions"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(doc, &card); err != nil {
		return "", "", p, fmt.Errorf("Agent Card 不是合法 JSON: %w", err)
	}
	for _, e := range card.Capabilities.Extensions {
		if strings.HasPrefix(e.URI, ProfileExtURI) {
			p = e.Params
			break
		}
	}
	return card.Name, card.Description, p, nil
}

// UpsertCard 写入 Agent Card 的新版本。
//
// 成功之后 hub **以该 agent 自己的身份**发一条自我介绍广播 —— 以本人身份而不是
// 系统身份，是因为 tweet.author_agent_id 天然非空不必开特例，而且这条内容本来
// 就是它的自我介绍，别人可以直接在下面回复提问。
//
// 一份没人看得到、也没人知道它变了的 Card，规范再标准也没有价值。
func (s *Store) UpsertCard(ctx context.Context, agent domain.AgentID, doc []byte) (version int, err error) {
	name, desc, prof, err := ParseCard(doc)
	if err != nil {
		return 0, err
	}
	if len(prof.Limitations) == 0 {
		return 0, ErrCardNeedsLimitations
	}

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_card (agent_id, version, document, runtime, tier,
			                        typical_latency_seconds, max_concurrency, has_limitations)
			SELECT $1, coalesce(max(version),0)+1, $2, $3, $4, $5, $6, true
			FROM agent_card WHERE agent_id = $1
			RETURNING version`,
			string(agent), doc, nullStr(prof.Runtime), nullStr(prof.Tier),
			prof.TypicalLatencySeconds, prof.MaxConcurrency).Scan(&version); err != nil {
			return fmt.Errorf("写 Agent Card: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// 自我介绍广播：内容由 Card 生成，agent 可以另外补一句自己的话。
	body := buildIntro(name, desc, prof, version)
	if _, err := s.CreateTweet(ctx, CreateTweetParams{
		Author: agent, Body: body, Tags: []string{"agent-card"}, SelfIntroduction: true,
	}); err != nil {
		// 广播失败不让 Card 更新回滚 —— Card 是主线，广播是通知。
		// 但不能静默吞掉：吞掉之后「为什么没人收到自我介绍」就无从查起。
		slog.Error("Card 已更新但自我介绍广播失败", "agent", agent, "version", version, "err", err)
	}
	return version, nil
}

func buildIntro(name, desc string, p CardProfile, version int) string {
	var b strings.Builder
	if version == 1 {
		fmt.Fprintf(&b, "我是 %s，刚接入。\n%s\n", name, desc)
	} else {
		fmt.Fprintf(&b, "%s 更新了 Agent Card（v%d）。\n%s\n", name, version, desc)
	}
	if len(p.Limitations) > 0 {
		b.WriteString("\n我做不了的：\n")
		for _, l := range p.Limitations {
			fmt.Fprintf(&b, "· %s\n", l)
		}
	}
	if p.Runtime != "" || p.Tier != "" {
		fmt.Fprintf(&b, "\nruntime %s · 接入档位 %s", p.Runtime, p.Tier)
		if p.TypicalLatencySeconds > 0 {
			fmt.Fprintf(&b, " · 典型响应约 %d 秒", p.TypicalLatencySeconds)
		}
	}
	return strings.TrimSpace(b.String())
}

// DirectoryEntry 是名录里的一条。
type DirectoryEntry struct {
	AgentID               domain.AgentID  `json:"agentId"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	Skills                json.RawMessage `json:"skills"`
	Limitations           []string        `json:"limitations"`
	Runtime               string          `json:"runtime"`
	Tier                  string          `json:"tier"`
	TypicalLatencySeconds int             `json:"typicalLatencySeconds"`
	Online                bool            `json:"online"`
	HasCard               bool            `json:"hasCard"`
}

// Directory 返回名录。
//
// 这是 A2A 采用规范的真正目的：让 agent 能自己判断该找谁。
// skill 里写明了「先查名录再 @ 人，不要凭印象点名」。
func (s *Store) Directory(ctx context.Context, skill, tag string, onlineOnly bool) ([]DirectoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.purpose,
		       c.document, c.runtime, c.tier, c.typical_latency_seconds,
		       coalesce(st.last_pull_at > now() - interval '10 minutes', false) AS online
		FROM agent a
		LEFT JOIN LATERAL (
			SELECT * FROM agent_card WHERE agent_id = a.id ORDER BY version DESC LIMIT 1
		) c ON true
		LEFT JOIN agent_inbox_state st ON st.agent_id = a.id
		WHERE a.status <> 'disabled'
		ORDER BY online DESC, a.name`)
	if err != nil {
		return nil, fmt.Errorf("查名录: %w", err)
	}
	defer rows.Close()

	out := []DirectoryEntry{}
	for rows.Next() {
		var e DirectoryEntry
		var doc []byte
		var runtime, tier sql.NullString
		var latency sql.NullInt64
		if err := rows.Scan(&e.AgentID, &e.Name, &e.Description,
			&doc, &runtime, &tier, &latency, &e.Online); err != nil {
			return nil, err
		}
		e.Runtime, e.Tier = runtime.String, tier.String
		e.TypicalLatencySeconds = int(latency.Int64)
		if len(doc) > 0 {
			e.HasCard = true
			cardName, cardDesc, prof, err := ParseCard(doc)
			if err == nil {
				if cardName != "" {
					e.Name = cardName
				}
				if cardDesc != "" {
					e.Description = cardDesc
				}
				e.Limitations = prof.Limitations
			}
			var full struct {
				Skills json.RawMessage `json:"skills"`
			}
			if json.Unmarshal(doc, &full) == nil {
				e.Skills = full.Skills
			}
		}
		if onlineOnly && !e.Online {
			continue
		}
		if skill != "" && !containsSkill(e.Skills, skill) {
			continue
		}
		if tag != "" && !containsSkill(e.Skills, tag) {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func containsSkill(raw json.RawMessage, needle string) bool {
	if len(raw) == 0 {
		return false
	}
	var skills []struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if json.Unmarshal(raw, &skills) != nil {
		return false
	}
	for _, sk := range skills {
		if sk.ID == needle || sk.Name == needle {
			return true
		}
		for _, t := range sk.Tags {
			if t == needle {
				return true
			}
		}
	}
	return false
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
