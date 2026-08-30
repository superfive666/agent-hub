import { createServer, Server } from 'node:http';
import { Config } from './config.js';
import { HubClient, ConnectionReplacedError } from './hub.js';
import { Journal } from './journal.js';
import { Queue } from './queue.js';
import { RuntimeAdapter, WakePayload } from './types.js';

export interface Logger { info(m: string, x?: unknown): void; warn(m: string, x?: unknown): void; error(m: string, x?: unknown): void }
export const consoleLogger: Logger = {
  info: (m, x) => console.log(`[info] ${m}`, x ?? ''),
  warn: (m, x) => console.warn(`[warn] ${m}`, x ?? ''),
  error: (m, x) => console.error(`[error] ${m}`, x ?? ''),
};

const CURSOR_KEY = 'cursor';

/**
 * Connector Core。runtime 无关：它只负责「该叫醒你了」和「一次别叫醒太多」，
 * 要不要回复、怎么回复全是 runtime 的事。
 */
export class Connector {
  readonly queue: Queue;
  #cfg: Config;
  #hub: HubClient;
  #adapter: RuntimeAdapter;
  #journal: Journal;
  #log: Logger;
  /** 拉到过的最大 seq，长轮询的 after 用它。与 ack cursor 是两回事。 */
  #fetchCursor: number;
  #ackedCursor = -1;
  #running = false;
  #pumping = false;
  #loop: Promise<void> | null = null;
  #webhook: Server | null = null;
  #kick: (() => void) | null = null;
  #inflight = new Set<Promise<void>>();

  constructor(opts: { config: Config; hub: HubClient; adapter: RuntimeAdapter; journal: Journal; logger?: Logger }) {
    this.#cfg = opts.config;
    this.#hub = opts.hub;
    this.#adapter = opts.adapter;
    this.#journal = opts.journal;
    this.#log = opts.logger ?? consoleLogger;
    this.queue = new Queue(this.#journal, this.#cfg.queue);
    const meta = this.#journal.loadMeta();
    this.#fetchCursor = meta[CURSOR_KEY] ? Number(meta[CURSOR_KEY]) : 0;
  }

  get cursor(): number { return this.#fetchCursor; }

  async start(): Promise<void> {
    await this.#adapter.start();
    this.#running = true;
    if (this.#cfg.tier === 'webhook') this.#startWebhook();
    this.#loop = this.#pullLoop();
    this.#pump();
  }

  async stop(): Promise<void> {
    this.#running = false;
    this.#kick?.();
    this.#webhook?.close();
    await Promise.allSettled([...this.#inflight]);
    if (this.#loop) await this.#loop.catch(() => undefined);
    await this.#adapter.stop().catch(() => undefined);
    this.#journal.close();
  }

  // ── 拉取：三档共用同一套 cursor 和同一个 inbox 端点 ────────────────────────

  /** 拉一轮 inbox 并落盘。返回本轮新入队的事件数。测试直接调它，不依赖定时器。 */
  async pullOnce(waitSeconds = 0): Promise<number> {
    const limit = this.#cfg.tier === 'cron' ? this.#cfg.cron.limit : this.#cfg.longpoll.limit;
    const page = await this.#hub.fetchInbox(this.#fetchCursor, limit, waitSeconds);
    let n = 0;
    for (const e of page.events) {
      // 先落盘再处理：进程被 kill / 机器重启，队列还在。
      if (this.queue.enqueue(e) !== 'duplicate') n++;
      if (e.seq > this.#fetchCursor) this.#fetchCursor = e.seq;
    }
    if (page.lastSeq > this.#fetchCursor) this.#fetchCursor = page.lastSeq;
    this.#journal.setMeta(CURSOR_KEY, String(this.#fetchCursor));
    if (n) this.#pump();
    await this.#maybeAck();
    return n;
  }

  async #pullLoop(): Promise<void> {
    while (this.#running) {
      try {
        // 背压是免费的：本地积压时放慢拉取，事件安全地堆在 hub 的持久 inbox 里。
        if (this.queue.pendingCount >= this.#cfg.queue.backpressureHighWater) {
          this.#log.warn('本地队列积压，放慢拉取', { pending: this.queue.pendingCount });
          await this.#sleep(this.#cfg.queue.backpressureSleepMs);
          continue;
        }
        if (this.#cfg.tier === 'longpoll') {
          const n = await this.pullOnce(this.#cfg.longpoll.waitSeconds);
          if (n === 0) await this.#sleep(this.#cfg.longpoll.idleBackoffMs);
        } else if (this.#cfg.tier === 'cron') {
          await this.pullOnce(0);
          await this.#sleep(this.#cfg.cron.intervalMs);
        } else {
          // webhook：收到 {agentId, seq} 信号才拉；信号可以丢，所以仍然定时兜底一次。
          await this.pullOnce(0);
          await this.#sleep(this.#cfg.cron.intervalMs);
        }
      } catch (e) {
        if (e instanceof ConnectionReplacedError) {
          this.#log.error('长轮询被另一实例顶替，退出（ADR-0005）');
          this.#running = false;
          break;
        }
        // 断线：cursor 在盘上，重连后按 cursor 从断点续拉，期间的事件一条不少。
        this.#log.warn('拉取失败，退避后重试', String(e));
        await this.#sleep(this.#cfg.longpoll.idleBackoffMs);
      }
    }
  }

  #startWebhook(): void {
    const { host, port, path, secretEnv } = this.#cfg.webhook;
    const secret = secretEnv ? process.env[secretEnv] : undefined;
    this.#webhook = createServer((req, res) => {
      if (req.method !== 'POST' || !req.url?.startsWith(path)) { res.writeHead(404).end(); return; }
      if (secret && req.headers['x-hub-secret'] !== secret) { res.writeHead(401).end(); return; }
      req.resume();
      req.on('end', () => {
        res.writeHead(204).end();
        // 信号里只有 {agentId, seq}，不带内容 —— 收到就去拉。
        this.pullOnce(0).catch((e) => this.#log.warn('webhook 触发的拉取失败', String(e)));
      });
    }).listen(port, host);
  }

  // ── 调度：优先级出队 + 并发租约 ───────────────────────────────────────────

  /** 把队列尽量排空（受租约限制）。测试用它同步等待所有唤起结束。 */
  async drain(): Promise<void> {
    for (;;) {
      this.#pump();
      if (this.#inflight.size === 0) {
        // 还有在退避等待的行时也算排空 —— 由调用方决定要不要推进时钟。
        if (!this.queue.acquireable()) return;
        continue;
      }
      await Promise.race([...this.#inflight]);
    }
  }

  #pump(): void {
    if (this.#pumping) return;
    this.#pumping = true;
    try {
      for (;;) {
        const row = this.queue.acquire();
        if (!row) break;
        const payload: WakePayload = {
          localId: row.id,
          kind: row.kind,
          priority: row.priority,
          threadId: row.threadId,
          seqs: [...row.seqs],
          seq: row.seqs[row.seqs.length - 1],
          coalescedCount: row.seqs.length,
          event: row.event,
          attempt: row.attempts + 1,
        };
        const p = this.#runWake(payload).finally(() => {
          this.#inflight.delete(p);
          // 一个租约释放出来，立刻看看有没有更高优先级的在等
          if (this.#running || this.#inflight.size >= 0) queueMicrotask(() => this.#pump());
        });
        this.#inflight.add(p);
      }
    } finally {
      this.#pumping = false;
    }
  }

  async #runWake(payload: WakePayload): Promise<void> {
    let ok = false, detail = '', retryable = true;
    try {
      const outcome = await withTimeout(this.#adapter.wake(payload), this.#cfg.queue.wakeTimeoutMs);
      ok = outcome.ok;
      detail = outcome.detail ?? '';
      retryable = outcome.retryable !== false;
    } catch (e) {
      detail = String(e);
    }
    if (ok) {
      this.queue.complete(payload.localId);
    } else {
      const state = this.queue.fail(payload.localId, detail, retryable);
      // kind 从老版本留下的队列行里可能读不到，退回事件本身 ——
      // 一条写着 `kind: undefined` 的告警等于少给了一半线索。
      const kind = payload.kind ?? payload.event?.kind ?? '(未知)';
      this.#log.warn(`唤起失败(${state})`, { localId: payload.localId, kind, detail });
      if (state === 'dead') await this.#reportDeadLetters();
    }
    await this.#maybeAck().catch(() => undefined);
  }

  /** cursor 只推进到「所有更早的事件都已终结」的位置，重启后不会跳过没处理完的。 */
  async #maybeAck(): Promise<void> {
    const c = this.queue.ackCursor(this.#fetchCursor);
    if (c <= this.#ackedCursor || c < 0) return;
    await this.#hub.ack(c);
    this.#ackedCursor = c;
  }

  /** 连续失败进死信并上报 hub：admin 要在控制台看得见，而不是静默地什么都没发生。 */
  async #reportDeadLetters(): Promise<void> {
    for (const r of this.queue.deadLetters()) {
      if (r.reported) continue;
      const result = await this.#hub.reportDeadLetter(this.#cfg.deadLetterReportPath, {
        localId: r.id, kind: r.kind, threadId: r.threadId, seqs: r.seqs,
        attempts: r.attempts, lastError: r.lastError, runtime: this.#adapter.capabilities().runtime,
      });
      if (result === 'ok' || result === 'unsupported') this.queue.markReported(r.id);
      if (result === 'unsupported') this.#log.error('hub 没有死信上报端点，死信只能留在本地', { localId: r.id });
    }
  }

  #sleep(ms: number): Promise<void> {
    return new Promise((resolve) => {
      const t = setTimeout(done, ms);
      this.#kick = done;
      function done() { clearTimeout(t); resolve(); }
    });
  }
}

function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  if (!Number.isFinite(ms) || ms <= 0) return p;
  return new Promise<T>((resolve, reject) => {
    const t = setTimeout(() => reject(new Error(`唤起超时 ${ms}ms`)), ms);
    p.then((v) => { clearTimeout(t); resolve(v); }, (e) => { clearTimeout(t); reject(e); });
  });
}
