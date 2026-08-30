import { InboxEvent } from './types.js';

export interface InboxPage { events: InboxEvent[]; lastSeq: number }

export class ConnectionReplacedError extends Error {
  constructor() { super('同一身份已有挂起的长轮询请求，本次被顶替（ADR-0005）'); }
}

/**
 * 把一条 inbox 事件归一成契约里的形状。
 *
 * **为什么要容错**：契约（openapi.yaml#InboxEvent）写的是小写字段，但 hub 的 Go
 * 结构体一度没打 json tag，实际发出去的是 Go 的导出名（`Seq` / `Kind` / `ThreadID`）。
 * hub 那边已经修了，这里仍然两种都认 —— **connector 装在别人机器上，和 hub 不会同步升级**。
 *
 * 只认一种的下场很难查：字段全 undefined，kind 判不了优先级、threadId 拼不进唤起
 * prompt，而事件照样被算作「处理过」，cursor 照常推进 —— 链路上每一环都显示正常。
 *
 * （Go 那边一条用例都没发现这个，是因为 `encoding/json` 解码时字段名大小写不敏感，
 * 只有非 Go 的客户端才看得见。）
 */
function normalizeEvent(e: Record<string, unknown>): InboxEvent {
  const pick = <T>(...ks: string[]): T | undefined => {
    for (const k of ks) if (e[k] !== undefined && e[k] !== null) return e[k] as T;
    return undefined;
  };
  return {
    seq: pick<number>('seq', 'Seq') ?? 0,
    kind: pick<string>('kind', 'Kind') ?? '',
    priority: pick<number>('priority', 'Priority'),
    threadId: pick<string>('threadId', 'ThreadID') || undefined,
    postId: pick<string>('postId', 'PostID') || undefined,
    payload: pick<Record<string, unknown>>('payload', 'Payload'),
    createdAt: pick<string>('createdAt', 'CreatedAt'),
  };
}

/** hub 的 agent API 客户端。只用内置 fetch，不引 HTTP 库。 */
export class HubClient {
  constructor(
    private baseUrl: string,
    private token: string,
    private timeoutMs: number,
  ) {}

  async #req(path: string, init: RequestInit, timeoutMs = this.timeoutMs): Promise<Response> {
    const ac = new AbortController();
    const t = setTimeout(() => ac.abort(), timeoutMs);
    try {
      return await fetch(this.baseUrl + path, {
        ...init,
        signal: ac.signal,
        headers: {
          authorization: `Bearer ${this.token}`,
          'content-type': 'application/json',
          ...(init.headers as Record<string, string> | undefined),
        },
      });
    } finally {
      clearTimeout(t);
    }
  }

  /** 拉 inbox。waitSeconds > 0 就是长轮询（同一个端点，只多一个 wait 参数）。 */
  async fetchInbox(after: number, limit: number, waitSeconds = 0): Promise<InboxPage> {
    const q = new URLSearchParams({ after: String(after), limit: String(limit) });
    if (waitSeconds > 0) q.set('wait', `${waitSeconds}s`);
    const res = await this.#req(`/api/agent/me/inbox?${q}`, { method: 'GET' },
      waitSeconds > 0 ? waitSeconds * 1000 + this.timeoutMs : this.timeoutMs);
    if (res.status === 409) { await res.text(); throw new ConnectionReplacedError(); }
    if (!res.ok) throw new Error(`GET inbox 失败: ${res.status} ${await res.text()}`);
    const body = (await res.json()) as { events?: Record<string, unknown>[]; lastSeq?: number };
    const events = (body.events ?? []).map(normalizeEvent);
    return { events, lastSeq: body.lastSeq ?? (events.length ? events[events.length - 1].seq : after) };
  }

  async ack(cursor: number): Promise<void> {
    const res = await this.#req('/api/agent/me/inbox/ack', { method: 'POST', body: JSON.stringify({ cursor }) });
    if (!res.ok && res.status !== 204) throw new Error(`ack 失败: ${res.status}`);
    await res.text().catch(() => undefined);
  }

  /**
   * 上报死信。openapi.yaml 目前没有这个端点（见 README「取舍」），
   * 404/405 只记一次日志就当上报过 —— 死信不能反过来把队列堵死。
   */
  async reportDeadLetter(path: string, body: unknown): Promise<'ok' | 'unsupported' | 'failed'> {
    try {
      const res = await this.#req(path, { method: 'POST', body: JSON.stringify(body) });
      await res.text().catch(() => undefined);
      if (res.ok || res.status === 204) return 'ok';
      if (res.status === 404 || res.status === 405 || res.status === 501) return 'unsupported';
      return 'failed';
    } catch {
      return 'failed';
    }
  }
}
