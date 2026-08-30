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
  /**
   * 与 messageField 一起用：塞进 body 的固定字段，比如 chat id、会话键、通道名。
   *
   * **字符串值支持占位符**，和 generic-shell 的命令模板同一套：
   * `{{threadId}}` `{{kind}}` `{{seq}}` `{{coalescedCount}}` `{{priority}}` `{{agentId}}`。
   * 这一条是给「常驻 runtime 自己按会话键分流」用的 —— 比如 hermes：
   * `{"session": "agent-hub/{{threadId}}"}` 能让每条 hub thread 落进它自己的一条会话，
   * 而不是把所有 hub 事件、连同人类正在聊的那条，全堆进同一个上下文里。
   *
   * 字段名各家不同，**我们不猜**：填错的表现是它照收不误但分流没生效，
   * 比每次唤起都失败更难发现。查你自己那套 webhook 的文档再填。
   */
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
    return { [this.m.messageField]: wakePrompt(p, this.hub), agentHub: p, ...this.extra(p) };
  }

  /** extraBody 里的字符串值做占位符替换；其它类型原样带过去。 */
  private extra(p: WakePayload): Record<string, unknown> | undefined {
    if (!this.m.extraBody) return undefined;
    const vars: Record<string, string> = {
      kind: p.kind,
      // 广播类事件没有 thread。留空会让模板塌成 "agent-hub/"，
      // 那是个合法但看不出含义的会话键；给它一个固定名字，
      // 广播就都落在同一条会话里，也不会去撞任何一条 thread 的会话。
      threadId: p.threadId ?? 'broadcast',
      seq: String(p.seq),
      coalescedCount: String(p.coalescedCount),
      priority: String(p.priority),
      agentId: this.hub?.agentId ?? '',
    };
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(this.m.extraBody)) {
      out[k] = typeof v === 'string' ? v.replace(/\{\{(\w+)\}\}/g, (_, n) => vars[n] ?? '') : v;
    }
    return out;
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
