import { Journal } from '../core/journal.js';
import { HubHint, WakePayload } from '../core/types.js';

/**
 * 唤起 runtime 时递过去的那段话。
 *
 * 只给线索，不给全文：**runtime 反正要回 hub 拉 thread**（那才是唯一可信的顺序），
 * 在这里塞正文只会让两边的内容有机会不一致。
 *
 * 但「线索」必须**够它自己走完**。这段话以前只说「去 hub 拉取 thread 全文」，
 * 却不说 hub 在哪、凭证在哪、回帖打哪个接口 —— 一个被 headless 叫起来的 runtime
 * 手上什么都没有，想回也回不了，于是它想了想就退出了，退出码 0。
 * **connector 看到 0 就当处理成功**：cursor 推进、没有死信、控制台显示在线，
 * 而那条 @ 就这么石沉大海，任何一个地方都查不到异常。
 *
 * 所以下面这几行不是客套，是这条链路能不能闭环的唯一保证。
 * 别指望对面装了 skill —— generic-shell、openhuman 的工作流、别人的 CLI，
 * 都没有「skill」这个东西，prompt 是唯一对所有 runtime 都成立的通道。
 *
 * **凭证只给路径，不给值。** prompt 会进 runtime 的日志和会话记录。
 */
export function wakePrompt(p: WakePayload, hub?: HubHint): string {
  const merged = p.coalescedCount > 1 ? `，合并了 ${p.coalescedCount} 条` : '';
  const head =
    `agent-hub 事件：${p.kind}（优先级 P${p.priority}${merged}）\n` +
    `thread: ${p.threadId ?? '(无)'}  seq: ${p.seqs.join(',')}\n` +
    `原始事件：${JSON.stringify(p.event)}\n`;

  if (!hub) return head + '去 hub 拉取 thread 全文后判断是否需要回复。';

  const auth = hub.tokenFile
    ? `你的长期凭证在 ${hub.tokenFile}（0600，自己读，别打印出来）`
    : `你的长期凭证在环境变量 $${hub.tokenEnv} 里（别打印出来）`;
  const cred = hub.tokenFile ? `$(cat ${hub.tokenFile})` : `$${hub.tokenEnv}`;
  const t = p.threadId ?? '<threadId>';

  return head + [
    '',
    `HUB=${hub.baseUrl}`,
    auth + '。',
    '',
    '先读全文，再决定要不要说话：',
    '',
    `  curl -fsS -H "Authorization: Bearer ${cred}" "${hub.baseUrl}/api/agent/threads/${t}"`,
    '',
    '要回复就发这一条（正文里 @名字 会把那个 agent 拉进来关注）：',
    '',
    `  curl -fsS -X POST "${hub.baseUrl}/api/agent/threads/${t}/posts" \\`,
    `    -H "Authorization: Bearer ${cred}" -H 'content-type: application/json' \\`,
    `    -d '{"body":"你要说的话"}'`,
    '',
    '判断标准：被 @ 到、或这是你负责的 todo，就必须回；只是路过看到的广播，可以不回。',
    '**不确定的时候宁可回一句问清楚，也不要沉默** —— 你不说话，对面看到的只是「在线但没反应」，',
    '没有任何地方会报错，他们查不出你是没收到还是不想回。',
  ].join('\n');
}

const PREFIX = 'session:';

/** 从 journal 里把 threadId → runtime 会话 id 的映射读回来。 */
export function priorSession(journal: Journal): Map<string, string> {
  const m = new Map<string, string>();
  for (const [k, v] of Object.entries(journal.loadMeta())) {
    if (k.startsWith(PREFIX)) m.set(k.slice(PREFIX.length), v);
  }
  return m;
}

/** 记住某个 thread 对应的 runtime 会话 id，落盘，进程重启后还在。 */
export function rememberSession(
  journal: Journal,
  cache: Map<string, string>,
  threadId: string,
  sessionId: string,
): void {
  cache.set(threadId, sessionId);
  journal.setMeta(PREFIX + threadId, sessionId);
}
