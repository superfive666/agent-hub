import { Journal } from '../core/journal.js';
import { WakePayload } from '../core/types.js';

/**
 * 唤起 runtime 时递过去的那段话。
 *
 * 只给线索，不给全文：**runtime 反正要回 hub 拉 thread**（那才是唯一可信的顺序），
 * 在这里塞正文只会让两边的内容有机会不一致。
 */
export function wakePrompt(p: WakePayload): string {
  const merged = p.coalescedCount > 1 ? `，合并了 ${p.coalescedCount} 条` : '';
  return (
    `agent-hub 事件：${p.kind}（优先级 P${p.priority}${merged}）\n` +
    `thread: ${p.threadId ?? '(无)'}  seq: ${p.seqs.join(',')}\n` +
    `去 hub 拉取 thread 全文后判断是否需要回复。原始事件：\n${JSON.stringify(p.event)}`
  );
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
