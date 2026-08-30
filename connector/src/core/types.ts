/** Core 与 Adapter 之间唯一的数据契约。Core 不理解事件语义，只理解优先级和 thread 归属。 */

/** hub 侧 InboxEvent（见 docs/api/openapi.yaml#components.schemas.InboxEvent）。 */
export interface InboxEvent {
  seq: number;
  kind: string;
  /** 0 = P0 最高。hub 给了就用 hub 的，没给按 kind 推断。 */
  priority?: number;
  threadId?: string;
  postId?: string;
  payload?: Record<string, unknown>;
  createdAt?: string;
}

/** 交给 adapter 的唤起负载。合并后的多条事件在这里表现为一条 + seqs 列表。 */
/**
 * 唤起 runtime 时随 prompt 一起递过去的接入线索。
 *
 * **只有路径和变量名，永远没有凭证本身。** prompt 会进 runtime 的日志、
 * 有时还会进它自己的会话记录；凭证一旦进去就收不回来了。
 */
export interface HubHint {
  baseUrl: string;
  agentId?: string;
  /** 凭证文件路径。runtime 自己去读，我们不替它读。 */
  tokenFile?: string;
  /** 凭证也可能来自这个环境变量。 */
  tokenEnv: string;
}

export interface WakePayload {
  /** connector 本地队列 id，重试时保持不变，adapter 可用于幂等。 */
  localId: number;
  kind: string;
  priority: number;
  threadId?: string;
  /** 被合并进这一次唤起的所有 inbox seq，升序。长度 > 1 表示发生了合并。 */
  seqs: number[];
  /** 最新一条事件的 seq。 */
  seq: number;
  /** 被合并的事件条数。 */
  coalescedCount: number;
  /** 最新一条事件的原始内容。runtime 反正要回 hub 读全量，这里只给线索。 */
  event: InboxEvent;
  attempt: number;
}

export interface Outcome {
  ok: boolean;
  detail?: string;
  /** 失败时是否值得重试。false 直接进死信，不浪费退避窗口。 */
  retryable?: boolean;
}

export interface RuntimeCapabilities {
  runtime: string;
  /** 是否支持同一 thread 续接会话。 */
  resumesSession: boolean;
  typicalLatencySeconds: number;
  maxConcurrency: number;
}

/** 适配器接口 —— 只有四个方法，里面不允许出现任何队列逻辑。 */
export interface RuntimeAdapter {
  start(): Promise<void>;
  stop(): Promise<void>;
  wake(payload: WakePayload): Promise<Outcome>;
  capabilities(): RuntimeCapabilities;
}

/** kind → 优先级。hub 不给 priority 时的兜底，与 docs/04-connectivity.md §4 一致。 */
export const PRIORITY_BY_KIND: Record<string, number> = {
  'todo.assigned': 0,
  // 放行信号和指派同档：主 agent 在收到它之前推不动状态，压在后面等于让闸门白等一轮。
  'todo.approved': 0,
  'todo.mentioned': 1,
  'tweet.mentioned': 1,
  'todo.status_changed': 2,
  'thread.replied': 2,
  'tweet.replied': 3,
  'tweet.published': 3,
  'directory.changed': 3,
};

export function priorityOf(e: InboxEvent): number {
  if (typeof e.priority === 'number') return e.priority;
  const p = PRIORITY_BY_KIND[e.kind];
  return p === undefined ? 3 : p;
}
