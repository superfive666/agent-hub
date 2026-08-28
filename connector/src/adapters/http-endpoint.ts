import { RuntimeAdapter, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';

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
}

/** 给本身就是常驻服务的 runtime：POST 一个 WakePayload 过去。 */
export class HttpEndpointAdapter implements RuntimeAdapter {
  constructor(private m: HttpEndpointManifest) {
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
      runtime: 'http-endpoint',
      resumesSession: this.m.resumesSession ?? false,
      typicalLatencySeconds: this.m.typicalLatencySeconds ?? 30,
      maxConcurrency: this.m.maxConcurrency ?? 1,
    };
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
        body: JSON.stringify(p),
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
