import { HubHint, RuntimeAdapter, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { wakePrompt } from './prompt.js';

export interface HttpEndpointManifest {
  url: string;
  method?: string;
  headers?: Record<string, string>;
  /** 从环境变量取 bearer token，不写进配置文件。 */
  tokenEnv?: string;
  timeoutSeconds?: number;
  maxConcurrency?: number;
  typicalLatencySeconds?: number;
  resumesSession?: boolean;
  /** 启动时探活的 URL，可选。 */
  healthUrl?: string;
  /**
   * 对方是「聊天型」webhook（hermes 的 Webhook 通道、openhuman 的工作流触发器
   * 都属于这类）时填这个：body 变成 `{ [messageField]: "<提示词>", agentHub: <原始负载> }`。
   *
   * 不填就原样 POST WakePayload —— 那适合你自己写的服务，它认得我们的字段。
   * 别人的通用 webhook 认不得，只会看一个它自己约定的文本字段。
   */
  messageField?: string;
  /** 与 messageField 一起用：固定塞进 body 的字段，比如 chat id、通道名。 */
  extraBody?: Record<string, unknown>;
  /** 上报给 hub 的 runtime 名，默认 http-endpoint。 */
  runtimeName?: string;
}

/** 给本身就是常驻服务的 runtime：POST 一个 WakePayload 过去。 */
export class HttpEndpointAdapter implements RuntimeAdapter {
  constructor(private m: HttpEndpointManifest, private hub?: HubHint) {
    if (!m.url) throw new Error('http-endpoint 需要 url');
  }
  async start(): Promise<void> {
    if (!this.m.healthUrl) return;
    const res = await fetch(this.m.healthUrl).catch((e) => { throw new Error(`runtime 探活失败: ${e}`); });
    if (!res.ok) throw new Error(`runtime 探活失败: ${res.status}`);
  }
  async stop(): Promise<void> {}
  capabilities(): RuntimeCapabilities {
    return {
      runtime: this.m.runtimeName ?? 'http-endpoint',
      resumesSession: this.m.resumesSession ?? false,
      typicalLatencySeconds: this.m.typicalLatencySeconds ?? 30,
      maxConcurrency: this.m.maxConcurrency ?? 1,
    };
  }
  private body(p: WakePayload): unknown {
    if (!this.m.messageField) return p;
    return { [this.m.messageField]: wakePrompt(p, this.hub), agentHub: p, ...this.m.extraBody };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const ac = new AbortController();
    const t = setTimeout(() => ac.abort(), (this.m.timeoutSeconds ?? 300) * 1000);
    try {
      const token = this.m.tokenEnv ? process.env[this.m.tokenEnv] : undefined;
      const res = await fetch(this.m.url, {
        method: this.m.method ?? 'POST',
        signal: ac.signal,
        headers: {
          'content-type': 'application/json',
          ...(token ? { authorization: `Bearer ${token}` } : {}),
          ...this.m.headers,
        },
        body: JSON.stringify(this.body(p)),
      });
      const text = await res.text().catch(() => '');
      // 4xx 是对方明确说「这个我处理不了」，重试没意义，直接进死信路径。
      if (!res.ok) return { ok: false, detail: `${res.status} ${text.slice(0, 500)}`, retryable: res.status >= 500 };
      return { ok: true, detail: text.slice(0, 2000) };
    } catch (e) {
      return { ok: false, detail: String(e), retryable: true };
    } finally {
      clearTimeout(t);
    }
  }
}
