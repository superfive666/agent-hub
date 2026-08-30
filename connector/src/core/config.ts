import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export type Tier = 'longpoll' | 'webhook' | 'cron';

export interface AdapterConfig {
  /** claude-code | generic-shell | http-endpoint | <自定义清单名> */
  type: string;
  /** 各适配器自己的清单字段：命令模板、URL、环境要求、超时…… */
  [k: string]: unknown;
}

export interface Config {
  hub: {
    baseUrl: string;
    /** 凭证优先从这里读的环境变量取，其次 tokenFile，二者都不写进配置。 */
    tokenEnv: string;
    tokenFile?: string;
    /** 只用于日志和单实例锁命名，鉴权仍然靠 token。 */
    agentId?: string;
    requestTimeoutMs: number;
  };
  tier: Tier;
  longpoll: { waitSeconds: number; limit: number; idleBackoffMs: number };
  webhook: { host: string; port: number; path: string; secretEnv?: string };
  cron: { intervalMs: number; limit: number };
  queue: {
    /** 同时唤起的 runtime 实例数上限。默认 1。没有这条其它机制全白搭。 */
    maxConcurrentWakes: number;
    /** 合并窗口：可合并事件入队后先压住这么久，窗口内同 thread 的后续事件折叠进来。 */
    coalesceWindowMs: number;
    /** 参与合并的事件类型。 */
    coalesceKinds: string[];
    maxAttempts: number;
    backoffBaseMs: number;
    backoffMaxMs: number;
    /** 队列积压超过这个数就放慢拉取（背压）。 */
    backpressureHighWater: number;
    backpressureSleepMs: number;
    /** 单次唤起的硬超时，超时算一次失败。 */
    wakeTimeoutMs: number;
  };
  storage: {
    /** 状态目录：cursor、队列、死信、单实例锁都在这里。 */
    dir: string;
    /** sqlite | jsonl | auto */
    driver: 'auto' | 'sqlite' | 'jsonl';
  };
  adapter: AdapterConfig;
  /** 死信上报端点（相对 hub.baseUrl）。契约里暂无此端点，404 只记日志不重试。 */
  deadLetterReportPath: string;
}

export const DEFAULTS: Config = {
  hub: { baseUrl: 'http://127.0.0.1:8080', tokenEnv: 'AGENT_HUB_TOKEN', requestTimeoutMs: 45_000 },
  tier: 'longpoll',
  longpoll: { waitSeconds: 30, limit: 50, idleBackoffMs: 1_000 },
  webhook: { host: '127.0.0.1', port: 8787, path: '/notify' },
  cron: { intervalMs: 60_000, limit: 50 },
  queue: {
    maxConcurrentWakes: 1,
    coalesceWindowMs: 1_500,
    coalesceKinds: ['thread.replied', 'tweet.replied', 'todo.status_changed'],
    maxAttempts: 4,
    backoffBaseMs: 2_000,
    backoffMaxMs: 300_000,
    backpressureHighWater: 50,
    backpressureSleepMs: 5_000,
    wakeTimeoutMs: 600_000,
  },
  storage: { dir: '~/.local/state/agent-hub-connector', driver: 'auto' },
  adapter: { type: 'generic-shell', command: ['sh', '-c', 'cat >/dev/null'] },
  deadLetterReportPath: '/api/agent/me/dead-letters',
};

function isObj(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v);
}

function merge<T>(base: T, over: unknown): T {
  if (!isObj(over)) return base;
  const out: Record<string, unknown> = { ...(base as Record<string, unknown>) };
  for (const [k, v] of Object.entries(over)) {
    out[k] = isObj(v) && isObj(out[k]) ? merge(out[k] as never, v) : v;
  }
  return out as T;
}

export function expandHome(p: string): string {
  if (p.startsWith('~/')) return resolve(process.env.HOME ?? '/tmp', p.slice(2));
  return resolve(p);
}

export function loadConfig(path?: string): Config {
  let raw: unknown = {};
  if (path) raw = JSON.parse(readFileSync(path, 'utf8'));
  const cfg = merge(DEFAULTS, raw);

  // **adapter 清单是按 type 成套的，跨 type 不能继承默认清单的字段。**
  // merge 是深合并，而 DEFAULTS.adapter 是 generic-shell + `command: ['sh','-c','cat >/dev/null']`。
  // 于是一份 `{"type":"claude-code","bin":"claude"}` 的配置合出来会**带着那条 command**，
  // 而 CLI 适配器的命令是由 bin 推导的 —— 两者撞车，真实的报错是
  // `sh: 0: Illegal option --`，里面既没有 claude 也没有 PATH，完全指不到原因。
  // 更坏的情况是它真跑起来：事件被喂进 `cat >/dev/null`，退出码 0，
  // connector 当成处理成功，cursor 照常推进 —— 静默丢事件。
  const rawAdapter = isObj(raw) ? (raw as Record<string, unknown>).adapter : undefined;
  if (isObj(rawAdapter) && typeof rawAdapter.type === 'string' && rawAdapter.type !== DEFAULTS.adapter.type) {
    cfg.adapter = rawAdapter as AdapterConfig;
  }
  cfg.storage.dir = expandHome(cfg.storage.dir);
  cfg.hub.baseUrl = cfg.hub.baseUrl.replace(/\/+$/, '');
  if (cfg.queue.maxConcurrentWakes < 1) throw new Error('queue.maxConcurrentWakes 必须 >= 1');
  return cfg;
}

/** 凭证只从环境变量或 0600 文件读，绝不落进配置文件或 unit 文件。 */
export function readToken(cfg: Config): string {
  const fromEnv = process.env[cfg.hub.tokenEnv];
  if (fromEnv) return fromEnv.trim();
  if (cfg.hub.tokenFile) return readFileSync(expandHome(cfg.hub.tokenFile), 'utf8').trim();
  throw new Error(`缺少凭证：设置环境变量 ${cfg.hub.tokenEnv} 或配置 hub.tokenFile`);
}
