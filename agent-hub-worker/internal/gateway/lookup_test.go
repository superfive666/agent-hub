package gateway_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/gateway"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

// card 造一份最小可用的 A2A Card。**前面故意排一个别的扩展** ——
// 这正是原来那条查询翻车的地方：它取 extensions[0]，而我们的扩展未必排在第 0 位。
func card(runtime, tier, webhookURL string) []byte {
	params := map[string]any{
		"runtime": runtime, "tier": tier,
		"limitations": []string{"不碰生产写操作"},
	}
	if webhookURL != "" {
		params["webhookUrl"] = webhookURL
	}
	doc := map[string]any{
		"name": "t", "description": "d",
		"capabilities": map[string]any{"extensions": []any{
			map[string]any{"uri": "https://example.com/ext/other", "params": map[string]any{"x": 1}},
			map[string]any{"uri": store.ProfileExtURI + "/v1", "params": params},
		}},
	}
	b, _ := json.Marshal(doc)
	return b
}

func mk(t *testing.T, s *store.Store, name, runtime, tier, url string) domain.AgentID {
	t.Helper()
	id, err := s.CreateAgent(context.Background(), name, "测试用 agent", "superfive")
	if err != nil {
		t.Fatalf("建 agent: %v", err)
	}
	if _, err := s.UpsertCard(context.Background(), id, card(runtime, tier, url)); err != nil {
		t.Fatalf("写 Card: %v", err)
	}
	return id
}

// 需求：webhook 档的 agent 填了地址，投递侧就要能查到它。
//
// 这条用例存在的原因是它曾经**永远查不到**：查询写的是
// {capabilities,extensions,0,webhookUrl}，而字段在 extensions[i].params.webhookUrl，
// 于是 webhook 档从来没响过，而且一行日志都没有。
func TestLookupFindsWebhookURLBehindOtherExtensions(t *testing.T) {
	s := testdb.New(t)
	id := mk(t, s, "wh", "http-endpoint", "webhook", "http://127.0.0.1:8787/notify")

	got, err := gateway.CardWebhookLookup(s.DB())(context.Background(), string(id))
	if err != nil {
		t.Fatalf("查地址: %v", err)
	}
	if got != "http://127.0.0.1:8787/notify" {
		t.Errorf("查到 %q，want 本机 /notify —— 查不到就是静默不投递", got)
	}
}

// 需求：hermes / openhuman 的地址属于它们自己的聊天通道，hub 绝不能直连。
// 写 Card 时已经拦了一道，这里是老数据的第二道 —— 直接往库里塞一条绕过校验的 Card。
func TestLookupRefusesChatRuntimeEvenIfStored(t *testing.T) {
	s := testdb.New(t)
	id := mk(t, s, "herm", "hermes", "longpoll", "")

	// 绕过 UpsertCard 的校验，模拟校验上线之前写进去的那种行。
	if _, err := s.DB().ExecContext(context.Background(), `
		UPDATE agent_card SET tier = 'webhook', document = $2 WHERE agent_id = $1`,
		string(id), card("hermes", "webhook", "http://127.0.0.1:8080/webhook/xxx")); err != nil {
		t.Fatalf("塞老数据: %v", err)
	}

	got, err := gateway.CardWebhookLookup(s.DB())(context.Background(), string(id))
	if err != nil {
		t.Fatalf("查地址: %v", err)
	}
	if got != "" {
		t.Errorf("查到 %q —— hermes 的通道不能被 hub 直连，它同时在给人用", got)
	}
}

// 需求：longpoll 档填了地址是配置残留，不能照着推 —— 那个端点根本没打算接收。
func TestLookupIgnoresNonWebhookTier(t *testing.T) {
	s := testdb.New(t)
	id := mk(t, s, "lp", "http-endpoint", "longpoll", "http://127.0.0.1:8787/notify")

	got, err := gateway.CardWebhookLookup(s.DB())(context.Background(), string(id))
	if err != nil {
		t.Fatalf("查地址: %v", err)
	}
	if got != "" {
		t.Errorf("查到 %q，longpoll 档不该被直连", got)
	}
}

// 需求：没写过 Card 的 agent 查出空串，不能报错 —— 报错会让整批通知都停下来。
func TestLookupReturnsEmptyForCardlessAgent(t *testing.T) {
	s := testdb.New(t)
	id, err := s.CreateAgent(context.Background(), "nocard", "测试用 agent", "superfive")
	if err != nil {
		t.Fatalf("建 agent: %v", err)
	}
	got, err := gateway.CardWebhookLookup(s.DB())(context.Background(), string(id))
	if err != nil || got != "" {
		t.Errorf("得到 (%q, %v)，want (\"\", nil)", got, err)
	}
}
