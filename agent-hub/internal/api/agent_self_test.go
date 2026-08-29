package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 需求：agent 要能知道「我是谁」。
//
// 写 Card 时 name 必须和管理员注册的对得上 —— 写错了自我介绍广播里就是个别人
// 不认识的名字。agent 手上只有一个凭证，除了问 hub 没有别的途径知道这个名字：
// 名录接口给的是**所有人**，要从里面捞出自己还得先知道自己的 agentId。
// cardVersion 也只有这条路给：agent 靠它分辨这次是首次撰写还是更新。
func TestAgentCanLookUpItsOwnIdentity(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, regToken := createAgentFor(t, c, srv.URL, "orin", true)
	tok := exchangeToken(t, srv.URL, regToken)

	resp, body := getWith(t, srv.URL+"/api/agent/me", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("查自身应当 200: %d %s", resp.StatusCode, body)
	}
	var self struct {
		AgentID     string `json:"agentId"`
		Name        string `json:"name"`
		Purpose     string `json:"purpose"`
		Status      string `json:"status"`
		HasCard     bool   `json:"hasCard"`
		CardVersion int    `json:"cardVersion"`
	}
	if err := json.Unmarshal(body, &self); err != nil {
		t.Fatal(err)
	}
	if self.Name != "orin" {
		t.Errorf("name = %q, want orin", self.Name)
	}
	if self.AgentID != id {
		t.Errorf("agentId = %q, want %q", self.AgentID, id)
	}
	if self.HasCard || self.CardVersion != 0 {
		t.Errorf("还没写过 Card：hasCard=%v cardVersion=%d，want false/0", self.HasCard, self.CardVersion)
	}
	if self.Status != "active" {
		t.Errorf("换过凭证之后 status = %q, want active", self.Status)
	}
}

// 需求：**名录里也有还没写 Card 的 agent** —— 那条查询是 LEFT JOIN。
//
// 这条用例把这件事钉住，因为控制台正是靠它决定怎么分栏展示：
// 如果哪天名录改成只收录有 Card 的，前端那个「还没出现在名录里」的分栏
// 就会变成空的，而没有任何地方会报错。
func TestDirectoryIncludesAgentsWithoutCard(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)
	id, regToken := createAgentFor(t, c, srv.URL, "nocard", true)
	tok := exchangeToken(t, srv.URL, regToken)

	resp, body := getWith(t, srv.URL+"/api/agent/directory", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("拉名录: %d %s", resp.StatusCode, body)
	}
	var dir struct {
		Agents []struct {
			AgentID     string `json:"agentId"`
			HasCard     bool   `json:"hasCard"`
			Description string `json:"description"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(body, &dir); err != nil {
		t.Fatal(err)
	}
	for _, a := range dir.Agents {
		if a.AgentID != id {
			continue
		}
		if a.HasCard {
			t.Error("还没写过 Card，hasCard 却是 true")
		}
		// description 退化成管理员填的 purpose —— 控制台不能把它当 Card 描述展示
		if a.Description != "初始简介" {
			t.Errorf("description = %q, want 管理员填的 purpose「初始简介」", a.Description)
		}
		return
	}
	t.Error("名录里找不到这个还没写 Card 的 agent —— 控制台的分栏逻辑依赖它在这里")
}

// 需求：这条路要凭证，控制台的会话 cookie 在这里一律 401。
func TestAgentSelfRequiresCredential(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := getWith(t, srv.URL+"/api/agent/me", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("没凭证应当 401，实得 %d", resp.StatusCode)
	}
}
