package store_test

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/superfive666/agent-hub/internal/store"
)

/*
需求：**inbox 事件发出去的字段名必须和契约一致。**

这条来自一次真实故障：`InboxEvent` 上一个 json tag 都没打，于是 hub 发的是 Go 的
导出名（`Seq` / `Kind` / `ThreadID`），而契约写的是 `seq` / `kind` / `threadId`。
按契约实现的客户端读到的每个字段都是 undefined —— connector 判不了优先级、
拼不出 threadId，事件却已经被算作「处理过」，cursor 照常推进。
**链路上每个环节都显示正常，只有 agent 一直不干活。**

**为什么以前一条 Go 用例都没发现**：`encoding/json` 解码时字段名是**大小写不敏感**的，
`{"Kind":…}` 照样能填进 `json:"kind"` 的字段。所以每一条用 Go 结构体读响应的用例
都照常通过 —— 只有非 Go 的客户端（TypeScript 写的 connector）才看得见。
所以这条用例比的是**原始 key**，不解码进结构体：解码进去就又看不见了。

预期直接取自契约，不手抄字段清单 —— 手抄的那份自己也会过期。
*/
func TestInboxEventWireFormatMatchesContract(t *testing.T) {
	spec, err := os.ReadFile("../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatalf("读契约: %v", err)
	}
	want := contractProps(t, string(spec), "InboxEvent")
	if len(want) == 0 {
		t.Fatal("契约里没解析出 InboxEvent 的字段 —— 用例本身失效了")
	}

	// handler 是 writeJSON(map[string]any{"events": []InboxEvent{…}})，
	// 每条事件的 key 就是这里编出来的这些。
	raw, err := json.Marshal(store.InboxEvent{Seq: 1, Kind: "tweet.mentioned", ThreadID: "th-1"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("契约里有 %q，实际发出去的没有。实际字段：%v", w, keys)
		}
	}
	for _, k := range keys {
		if k[:1] != strings.ToLower(k[:1]) {
			t.Errorf("字段 %q 大写开头 —— 漏了 json tag，Go 直接用了导出名", k)
		}
	}
}

// contractProps 从 openapi.yaml 里抠出某个 schema 的属性名。
func contractProps(t *testing.T, spec, schema string) []string {
	t.Helper()
	i := strings.Index(spec, "\n    "+schema+":\n")
	if i < 0 {
		t.Fatalf("契约里找不到 schema %s", schema)
	}
	rest := spec[i+1:]
	j := strings.Index(rest, "\n      properties:\n")
	if j < 0 {
		t.Fatalf("%s 下没有 properties", schema)
	}
	block := rest[j+len("\n      properties:\n"):]
	re := regexp.MustCompile(`^        ([a-zA-Z][a-zA-Z0-9]*):`)
	var out []string
	for _, line := range strings.Split(block, "\n") {
		// 缩进变浅就说明这个 schema 结束了
		if line != "" && !strings.HasPrefix(line, "        ") && !strings.HasPrefix(line, "          ") {
			break
		}
		if m := re.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}
