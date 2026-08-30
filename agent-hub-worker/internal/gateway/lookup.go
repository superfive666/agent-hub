package gateway

import (
	"context"
	"database/sql"

	"github.com/superfive666/agent-hub/internal/store"
)

// CardWebhookLookup 从 Agent Card 的扩展里取 webhook 地址。没配的返回空串。
//
// 这里的 JSON 路径有过一次静默失败，值得写下来：原先查的是
// `{capabilities,extensions,0,webhookUrl}`，而字段实际在 `extensions[i].params.webhookUrl`，
// 且正确的那个扩展是**按 URI 前缀**认的，不是固定第 0 个 —— A2A 允许别的扩展排在前面。
// 于是这个查询永远返回空串，Webhook 出口对每个 agent 都跳过，
// 而 deliver() 拿到空串就 return，一行日志都不留：控制台上一切正常，webhook 档从来没响过。
//
// 两条附加条件：
//
//   - tier = 'webhook'：别的档位填了 URL 是配置残留，照着推等于给一个没打算接收的端点发流量。
//   - runtime 不是聊天型 webhook：hermes / openhuman 的地址属于**它们自己的**消息通道，
//     hub 的信号格式它们不认得（见 store.ErrWebhookNotOurContract）。写 Card 时已经拦了
//     一道，这里再拦一道是为了老数据 —— 少推一次的代价，比把信号灌进别人正在给人用的
//     会话里小得多。
func CardWebhookLookup(db *sql.DB) EndpointLookup {
	return func(ctx context.Context, agent string) (string, error) {
		var runtime string
		var url sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT coalesce(c.runtime, ''), ext->'params'->>'webhookUrl'
			FROM agent_card c
			CROSS JOIN LATERAL jsonb_array_elements(
				coalesce(c.document #> '{capabilities,extensions}', '[]'::jsonb)) AS ext
			WHERE c.agent_id = $1
			  AND c.tier = 'webhook'
			  AND ext->>'uri' LIKE $2 || '%'
			  AND coalesce(ext->'params'->>'webhookUrl', '') <> ''
			ORDER BY c.version DESC
			LIMIT 1`, agent, store.ProfileExtURI).Scan(&runtime, &url)
		if err == sql.ErrNoRows {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if store.IsChatWebhookRuntime(runtime) {
			return "", nil
		}
		return url.String, nil
	}
}
