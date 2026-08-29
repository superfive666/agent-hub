import { InboxEvent } from './types.js';

export interface InboxPage { events: InboxEvent[]; lastSeq: number }

export class ConnectionReplacedError extends Error {
  constructor() { super('同一身份已有挂起的长轮询请求，本次被顶替（ADR-0005）'); }
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
    const body = (await res.json()) as Partial<InboxPage>;
    const events = body.events ?? [];
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
