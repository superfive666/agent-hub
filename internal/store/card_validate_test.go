package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/superfive666/agent-hub/internal/store"
)

// 需求：能力边界是硬要求 —— 「我能做什么」人人都往大了写，选主 agent 时
// 真正有用的是「它做不了什么」。
func TestCardWithoutLimitationsIsRejected(t *testing.T) {
	err := store.ValidateProfile(store.CardProfile{Runtime: "claude-code", Tier: "longpoll"})
	if !errors.Is(err, store.ErrCardNeedsLimitations) {
		t.Fatalf("没写能力边界应当被拒，得到 %v", err)
	}
}

// 需求：hermes / openhuman 的 webhook 是**它们自己的**聊天通道，
// hub 直连过去只会把一条没有正文的信号灌进 agent 正在用的会话。
// 这类地址只能给同机的 connector 用，写进 Card 就要当场报错 ——
// 拦在投递时是静默的，填的人会一直以为自己接好了。
func TestChatRuntimeCannotDeclareWebhookURL(t *testing.T) {
	for _, rt := range []string{"hermes", "openhuman"} {
		p := store.CardProfile{
			Runtime: rt, Tier: "webhook",
			WebhookURL:  "http://127.0.0.1:8080/webhook/xxx",
			Limitations: []string{"不碰生产写操作"},
		}
		err := store.ValidateProfile(p)
		if !errors.Is(err, store.ErrWebhookNotOurContract) {
			t.Errorf("runtime=%s 声明 webhookUrl 应当被拒，得到 %v", rt, err)
		}
		// 报错正文得说清楚该怎么改，否则填的人只会把 URL 换个写法再试一遍。
		if err != nil && !strings.Contains(err.Error(), "connector") {
			t.Errorf("runtime=%s 的报错没指出出路: %v", rt, err)
		}
	}
}

// 需求：同样这两个 runtime，走 connector（不声明 webhookUrl）是正常接法，不能误伤。
func TestChatRuntimeViaConnectorIsAccepted(t *testing.T) {
	p := store.CardProfile{
		Runtime: "hermes", Tier: "longpoll",
		Limitations: []string{"不碰生产写操作"},
	}
	if err := store.ValidateProfile(p); err != nil {
		t.Fatalf("hermes 走 connector 应当被接受，得到 %v", err)
	}
}

// 需求：自己写的常驻服务认得 hub 的信号格式，webhook 档照常可用。
func TestOwnServiceMayDeclareWebhookURL(t *testing.T) {
	p := store.CardProfile{
		Runtime: "http-endpoint", Tier: "webhook",
		WebhookURL:  "http://127.0.0.1:8787/notify",
		Limitations: []string{"不做前端"},
	}
	if err := store.ValidateProfile(p); err != nil {
		t.Fatalf("自己写的端点应当被接受，得到 %v", err)
	}
}
