import { createServer, Server } from 'node:http';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { AddressInfo } from 'node:net';
import { InboxEvent, RuntimeAdapter, WakePayload, Outcome } from '../src/core/types.js';
import { Config, DEFAULTS } from '../src/core/config.js';

/** 本地 mock hub。只实现 connector 用到的三个端点，不联网。 */
export class MockHub {
  #server: Server;
  events: InboxEvent[] = [];
  acks: number[] = [];
  deadLetters: unknown[] = [];
  /** 置 true 模拟断线：所有请求直接失败。 */
  down = false;
  supportsDeadLetters = true;
  port = 0;

  constructor() {
    this.#server = createServer((req, res) => {
      if (this.down) { req.socket.destroy(); return; }
      const url = new URL(req.url!, 'http://x');
      if (req.method === 'GET' && url.pathname === '/api/agent/me/inbox') {
        const after = Number(url.searchParams.get('after') ?? 0);
        const limit = Number(url.searchParams.get('limit') ?? 50);
        const events = this.events.filter((e) => e.seq > after).slice(0, limit);
        const lastSeq = this.events.length ? this.events[this.events.length - 1].seq : after;
        res.writeHead(200, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ events, lastSeq }));
        return;
      }
      if (req.method === 'POST' && url.pathname === '/api/agent/me/inbox/ack') {
        let b = ''; req.on('data', (d) => { b += d; });
        req.on('end', () => { this.acks.push(JSON.parse(b || '{}').cursor); res.writeHead(204).end(); });
        return;
      }
      if (req.method === 'POST' && url.pathname === '/api/agent/me/dead-letters') {
        let b = ''; req.on('data', (d) => { b += d; });
        req.on('end', () => {
          if (!this.supportsDeadLetters) { res.writeHead(404).end(); return; }
          this.deadLetters.push(JSON.parse(b || '{}')); res.writeHead(204).end();
        });
        return;
      }
      res.writeHead(404).end();
    });
  }

  async listen(): Promise<string> {
    await new Promise<void>((r) => this.#server.listen(0, '127.0.0.1', r));
    this.port = (this.#server.address() as AddressInfo).port;
    return `http://127.0.0.1:${this.port}`;
  }
  async close(): Promise<void> {
    this.#server.closeAllConnections();
    await new Promise<void>((r) => this.#server.close(() => r()));
  }

  push(e: Partial<InboxEvent> & { kind: string }): InboxEvent {
    const seq = (this.events.at(-1)?.seq ?? 0) + 1;
    const full: InboxEvent = { seq, createdAt: new Date().toISOString(), ...e };
    this.events.push(full);
    return full;
  }
}

/** 记录唤起过程的假 runtime，可注入耗时和失败。 */
export class FakeAdapter implements RuntimeAdapter {
  wakes: WakePayload[] = [];
  concurrent = 0;
  maxConcurrentObserved = 0;
  behavior: (p: WakePayload) => Promise<Outcome> | Outcome = () => ({ ok: true });
  async start() {}
  async stop() {}
  capabilities() {
    return { runtime: 'fake', resumesSession: false, typicalLatencySeconds: 1, maxConcurrency: 1 };
  }
  async wake(p: WakePayload): Promise<Outcome> {
    this.wakes.push(p);
    this.concurrent++;
    this.maxConcurrentObserved = Math.max(this.maxConcurrentObserved, this.concurrent);
    try { return await this.behavior(p); } finally { this.concurrent--; }
  }
}

export function tmpDir(): string {
  return mkdtempSync(join(tmpdir(), 'connector-test-'));
}
export function cleanup(dir: string): void { rmSync(dir, { recursive: true, force: true }); }

export function testConfig(baseUrl: string, dir: string, over: Partial<Config> = {}): Config {
  return {
    ...DEFAULTS,
    ...over,
    hub: { ...DEFAULTS.hub, baseUrl, requestTimeoutMs: 3000, ...(over.hub ?? {}) },
    queue: { ...DEFAULTS.queue, coalesceWindowMs: 60, backoffBaseMs: 5, wakeTimeoutMs: 1000, ...(over.queue ?? {}) },
    storage: { dir, driver: 'auto', ...(over.storage ?? {}) },
  };
}

export const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
