import { Journal } from './journal.js';
import { InboxEvent, priorityOf } from './types.js';

export type RowState = 'pending' | 'leased' | 'dead';

export interface QueueRow {
  id: number;
  state: RowState;
  priority: number;
  kind: string;
  threadId?: string;
  /** 合并键：非空表示这一行可以吸收同键的后续事件。 */
  coalesceKey?: string;
  seqs: number[];
  event: InboxEvent;
  attempts: number;
  /** 早于这个时刻不出队：合并窗口 + 重试退避共用一个字段。 */
  availableAt: number;
  leaseId?: string;
  leasedAt?: number;
  lastError?: string;
  /** 死信是否已上报 hub。上报失败下轮重来，不阻塞队列。 */
  reported?: boolean;
}

export interface QueueOptions {
  maxConcurrentWakes: number;
  coalesceWindowMs: number;
  coalesceKinds: string[];
  maxAttempts: number;
  backoffBaseMs: number;
  backoffMaxMs: number;
  now?: () => number;
}

/**
 * 本地持久队列。五个机制都在这里，adapter 里一行队列逻辑都没有：
 * 持久化 / 并发租约 / 合并 / 优先级出队 / 重试与死信。
 */
export class Queue {
  #j: Journal;
  #opt: QueueOptions;
  #rows = new Map<number, QueueRow>();
  #nextId = 1;
  #now: () => number;
  /** 进程内的租约计数。跨进程的互斥由单实例锁保证（ADR-0005 同样只允许一个实例）。 */
  #leases = 0;

  constructor(journal: Journal, opt: QueueOptions) {
    this.#j = journal;
    this.#opt = opt;
    this.#now = opt.now ?? (() => Date.now());
    for (const raw of journal.loadRows()) {
      const row = raw as unknown as QueueRow;
      // 上次进程被 kill 时正在处理的行，重启后回到 pending 重新排队 —— 至少一次投递。
      if (row.state === 'leased') { row.state = 'pending'; row.leaseId = undefined; }
      this.#rows.set(row.id, row);
      this.#nextId = Math.max(this.#nextId, row.id + 1);
    }
  }

  get leasesInUse(): number { return this.#leases; }
  get pendingCount(): number {
    let n = 0;
    for (const r of this.#rows.values()) if (r.state !== 'dead') n++;
    return n;
  }
  rows(): QueueRow[] { return [...this.#rows.values()]; }
  deadLetters(): QueueRow[] { return this.rows().filter((r) => r.state === 'dead'); }

  #persist(r: QueueRow) { this.#j.putRow(r as unknown as { id: number }); }

  /**
   * 入队。返回 'new' | 'coalesced' | 'duplicate'。
   * 合并只发生在**还没被租出去**的行上：已经在跑的那次唤起看不到新事件，
   * 新事件必须留下一次新的唤起，否则会被静默吞掉。
   */
  enqueue(e: InboxEvent): 'new' | 'coalesced' | 'duplicate' {
    for (const r of this.#rows.values()) {
      if (r.state !== 'dead' && r.seqs.includes(e.seq)) return 'duplicate';
    }
    const key = this.#coalesceKey(e);
    const now = this.#now();
    if (key) {
      for (const r of this.#rows.values()) {
        if (r.state !== 'pending' || r.coalesceKey !== key) continue;
        if (r.attempts > 0) continue; // 正在退避重试的行不再吸收新事件
        r.seqs.push(e.seq);
        r.seqs.sort((a, b) => a - b);
        r.event = e;                       // 保留最新一条作为线索
        r.priority = Math.min(r.priority, priorityOf(e));
        this.#persist(r);
        return 'coalesced';
      }
    }
    const row: QueueRow = {
      id: this.#nextId++,
      state: 'pending',
      priority: priorityOf(e),
      kind: e.kind,
      threadId: e.threadId,
      coalesceKey: key,
      seqs: [e.seq],
      event: e,
      attempts: 0,
      // 可合并事件先压住一个窗口，让同 thread 的后续回复折叠进来（hermes 的去抖窗口）。
      availableAt: key ? now + this.#opt.coalesceWindowMs : now,
    };
    this.#rows.set(row.id, row);
    this.#persist(row);
    return 'new';
  }

  #coalesceKey(e: InboxEvent): string | undefined {
    if (!this.#opt.coalesceKinds.includes(e.kind)) return undefined;
    if (!e.threadId) return undefined;
    return `${e.kind}:${e.threadId}`;
  }

  /** 现在是否有行可以被立刻取走（用于判断队列是否已经排空）。 */
  acquireable(): boolean {
    if (this.#leases >= this.#opt.maxConcurrentWakes) return false;
    const now = this.#now();
    for (const r of this.#rows.values()) if (r.state === 'pending' && r.availableAt <= now) return true;
    return false;
  }

  /** 优先级出队 + 并发租约。拿不到租约或没有到期的行就返回 null。 */
  acquire(): QueueRow | null {
    if (this.#leases >= this.#opt.maxConcurrentWakes) return null;
    const now = this.#now();
    let best: QueueRow | null = null;
    for (const r of this.#rows.values()) {
      if (r.state !== 'pending' || r.availableAt > now) continue;
      // P0 排在 P3 前面；同优先级按最早的 seq，保证同一 thread 的因果顺序。
      if (!best || r.priority < best.priority || (r.priority === best.priority && r.seqs[0] < best.seqs[0])) best = r;
    }
    if (!best) return null;
    best.state = 'leased';
    best.leaseId = `${process.pid}-${now}-${best.id}`;
    best.leasedAt = now;
    this.#leases++;
    this.#persist(best);
    return best;
  }

  complete(id: number): void {
    const r = this.#rows.get(id);
    if (!r) return;
    if (r.state === 'leased') this.#leases--;
    this.#rows.delete(id);
    this.#j.delRow(id);
  }

  /** 唤起失败。指数退避重排队；超过上限进死信。返回这一行的新状态。 */
  fail(id: number, error: string, retryable = true): 'pending' | 'dead' {
    const r = this.#rows.get(id);
    if (!r) return 'dead';
    if (r.state === 'leased') this.#leases--;
    r.attempts++;
    r.lastError = error;
    r.leaseId = undefined;
    if (!retryable || r.attempts >= this.#opt.maxAttempts) {
      r.state = 'dead';
      this.#persist(r);
      return 'dead';
    }
    r.state = 'pending';
    const backoff = Math.min(this.#opt.backoffBaseMs * 2 ** (r.attempts - 1), this.#opt.backoffMaxMs);
    r.availableAt = this.#now() + backoff;
    this.#persist(r);
    return 'pending';
  }

  markReported(id: number): void {
    const r = this.#rows.get(id);
    if (!r) return;
    r.reported = true;
    this.#persist(r);
  }

  /**
   * 可以 ack 到哪个 cursor：所有还没处理完的事件里最小 seq 减一。
   * 死信已经上报给 hub，视为终态，不阻塞 cursor —— 否则一条处理不了的事件会把 cursor 永久钉住。
   */
  ackCursor(maxFetchedSeq: number): number {
    let min = Infinity;
    for (const r of this.#rows.values()) {
      if (r.state === 'dead') continue;
      for (const s of r.seqs) if (s < min) min = s;
    }
    return min === Infinity ? maxFetchedSeq : min - 1;
  }

  /** 下一行到期的时刻，供调度器决定睡多久。 */
  nextAvailableAt(): number | null {
    let t: number | null = null;
    for (const r of this.#rows.values()) {
      if (r.state !== 'pending') continue;
      if (t === null || r.availableAt < t) t = r.availableAt;
    }
    return t;
  }
}
