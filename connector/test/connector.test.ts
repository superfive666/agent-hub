import { test, describe, before, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { writeFileSync } from 'node:fs';
import { Connector } from '../src/core/connector.js';
import { HubClient } from '../src/core/hub.js';
import { openJournal } from '../src/core/journal.js';
import { InstanceLock, DuplicateInstanceError } from '../src/core/singleton.js';
import { MockHub, FakeAdapter, tmpDir, cleanup, testConfig, sleep } from './helpers.js';
import { Config, loadConfig } from '../src/core/config.js';
import { join } from 'node:path';

const silent = { info() {}, warn() {}, error() {} };

let hub: MockHub;
let baseUrl: string;

before(async () => { hub = new MockHub(); baseUrl = await hub.listen(); });
after(async () => { await hub.close(); });
beforeEach(() => { hub.events = []; hub.acks = []; hub.deadLetters = []; hub.down = false; hub.supportsDeadLetters = true; });

function makeConnector(dir: string, adapter: FakeAdapter, over: Partial<Config> = {}) {
  const cfg = testConfig(baseUrl, dir, over);
  const journal = openJournal(cfg.storage.dir, cfg.storage.driver);
  const client = new HubClient(cfg.hub.baseUrl, 'test-token', cfg.hub.requestTimeoutMs);
  return { cfg, journal, connector: new Connector({ config: cfg, hub: client, adapter, journal, logger: silent }) };
}

describe('需求：断线期间的事件一条不少', () => {
  test('断线 10 分钟后重连，按 cursor 把期间的事件全部补齐', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    const { connector, journal } = makeConnector(dir, adapter);

    hub.push({ kind: 'todo.assigned', threadId: 't1' });
    await connector.pullOnce();
    await connector.drain();
    assert.equal(adapter.wakes.length, 1);

    // 断线：期间 hub 侧照常产生事件（10 分钟里 30 条）
    hub.down = true;
    await assert.rejects(() => connector.pullOnce());
    for (let i = 0; i < 30; i++) hub.push({ kind: 'todo.mentioned', threadId: `t${i}` });

    // 重连：cursor 还在盘上，从断点续拉
    hub.down = false;
    await connector.pullOnce();
    await connector.drain();

    const seen = adapter.wakes.flatMap((w) => w.seqs).sort((a, b) => a - b);
    assert.deepEqual(seen, Array.from({ length: 31 }, (_, i) => i + 1), '断线期间的事件必须一条不少地补齐');
    assert.equal(connector.cursor, 31);
    journal.close();
    cleanup(dir);
  });
});

describe('需求：同 thread 的连续回复只唤起一次', () => {
  test('同一 thread 5 条 thread.replied，runtime 只被唤起 1 次', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    const { connector, journal } = makeConnector(dir, adapter, { queue: { ...testConfig(baseUrl, dir).queue, coalesceWindowMs: 200 } as never });

    for (let i = 0; i < 5; i++) hub.push({ kind: 'thread.replied', threadId: 'same-thread' });
    await connector.pullOnce();
    await sleep(300);          // 等合并窗口过去
    await connector.drain();

    assert.equal(adapter.wakes.length, 1, 'runtime 反正要读整个 thread，叫醒五次没有意义');
    assert.equal(adapter.wakes[0].coalescedCount, 5);
    assert.deepEqual(adapter.wakes[0].seqs, [1, 2, 3, 4, 5]);
    journal.close();
    cleanup(dir);
  });

  test('不同 thread 的回复不会被合并', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    const { connector, journal } = makeConnector(dir, adapter);
    hub.push({ kind: 'thread.replied', threadId: 'a' });
    hub.push({ kind: 'thread.replied', threadId: 'b' });
    await connector.pullOnce();
    await sleep(120);
    await connector.drain();
    assert.equal(adapter.wakes.length, 2);
    journal.close();
    cleanup(dir);
  });
});

describe('需求：并发租约与优先级', () => {
  test('20 条事件同时到达，并发唤起数不超过租约上限，其余按优先级排队', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = async () => { await sleep(15); return { ok: true }; };
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, maxConcurrentWakes: 2, coalesceWindowMs: 0 } as never,
    });

    // 先塞低优先级，再塞高优先级，检验出队顺序不是到达顺序
    for (let i = 0; i < 10; i++) hub.push({ kind: 'tweet.published' });                 // P3
    for (let i = 0; i < 5; i++) hub.push({ kind: 'todo.mentioned', threadId: `m${i}` }); // P1
    for (let i = 0; i < 5; i++) hub.push({ kind: 'todo.assigned', threadId: `a${i}` });  // P0

    await connector.pullOnce();
    await connector.drain();

    assert.equal(adapter.wakes.length, 20);
    assert.ok(adapter.maxConcurrentObserved <= 2,
      `并发唤起数 ${adapter.maxConcurrentObserved} 超过租约上限 2`);
    const prios = adapter.wakes.map((w) => w.priority);
    // 20 条在一轮里全部入队，所以出队顺序必须是严格的 P0 → P1 → P3，而不是到达顺序
    assert.deepEqual(prios, [...Array(5).fill(0), ...Array(5).fill(1), ...Array(10).fill(3)],
      'P0 必须插在先到达的 P3 前面');
    journal.close();
    cleanup(dir);
  });

  test('默认租约上限为 1：一次只唤起一个 runtime 实例', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = async () => { await sleep(10); return { ok: true }; };
    const { connector, journal } = makeConnector(dir, adapter);
    for (let i = 0; i < 6; i++) hub.push({ kind: 'todo.assigned', threadId: `t${i}` });
    await connector.pullOnce();
    await connector.drain();
    assert.equal(adapter.maxConcurrentObserved, 1);
    assert.equal(adapter.wakes.length, 6);
    journal.close();
    cleanup(dir);
  });

  test('P0 事件插在已排队的 P3 前面出队', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    let first = true;
    adapter.behavior = async () => { if (first) { first = false; await gate; } return { ok: true }; };
    const { connector, journal } = makeConnector(dir, adapter);

    hub.push({ kind: 'tweet.published' });      // seq1 P3 —— 占住唯一的租约
    await connector.pullOnce();
    await sleep(10);
    hub.push({ kind: 'tweet.published' });      // seq2 P3
    hub.push({ kind: 'directory.changed' });    // seq3 P3
    hub.push({ kind: 'todo.assigned', threadId: 'urgent' }); // seq4 P0，最后到
    await connector.pullOnce();
    release();
    await connector.drain();

    assert.equal(adapter.wakes[0].seq, 1);
    assert.equal(adapter.wakes[1].kind, 'todo.assigned', 'P0 必须排在先到的 P3 前面');
    journal.close();
    cleanup(dir);
  });
});

describe('需求：崩溃不丢事件', () => {
  test('进程被 kill 后重启，未处理的事件不丢', async () => {
    const dir = tmpDir();
    // 第一个「进程」：拉下来但唤起卡住，然后被强杀（不 stop、不 complete）
    const a1 = new FakeAdapter();
    let stuck!: () => void;
    a1.behavior = () => new Promise<never>(() => { stuck = () => {}; });
    const first = makeConnector(dir, a1);
    for (let i = 0; i < 4; i++) first.connector.queue; // noop
    hub.push({ kind: 'todo.assigned', threadId: 't1' });
    hub.push({ kind: 'todo.mentioned', threadId: 't2' });
    hub.push({ kind: 'tweet.published' });
    await first.connector.pullOnce();
    await sleep(20);
    assert.equal(a1.wakes.length, 1, '第一个进程只来得及处理一条');
    first.journal.close();          // 模拟 SIGKILL：句柄没了，队列留在盘上
    void stuck;

    // 第二个「进程」：同一个 state 目录重启
    const a2 = new FakeAdapter();
    const second = makeConnector(dir, a2);
    await second.connector.drain();
    const seen = a2.wakes.flatMap((w) => w.seqs).sort((x, y) => x - y);
    assert.deepEqual(seen, [1, 2, 3], '重启后未处理完的事件必须全部重新出队（至少一次投递）');
    second.journal.close();
    cleanup(dir);
  });

  test('cursor 只推进到「更早的事件都已终结」的位置', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    let block!: () => void;
    const gate = new Promise<void>((r) => { block = r; });
    adapter.behavior = async (p) => { if (p.seq === 1) await gate; return { ok: true }; };
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, maxConcurrentWakes: 2, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 'a' });
    hub.push({ kind: 'todo.assigned', threadId: 'b' });
    await connector.pullOnce();
    await sleep(30);
    assert.ok(hub.acks.every((c) => c < 1), `seq1 还没处理完，不能 ack 过它，实际 ack=${hub.acks}`);
    block();
    await connector.drain();
    await sleep(20);
    assert.equal(hub.acks.at(-1), 2);
    journal.close();
    cleanup(dir);
  });
});

describe('需求：重试、死信与上报', () => {
  test('runtime 连续失败后进死信并上报 hub', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = () => ({ ok: false, detail: 'runtime 起不来' });
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, maxAttempts: 3, backoffBaseMs: 1, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 't1' });
    await connector.pullOnce();
    for (let i = 0; i < 6 && connector.queue.deadLetters().length === 0; i++) {
      await sleep(20);
      await connector.drain();
    }
    assert.equal(adapter.wakes.length, 3, '按 maxAttempts 重试 3 次');
    assert.equal(connector.queue.deadLetters().length, 1);
    await sleep(30);
    assert.equal(hub.deadLetters.length, 1, '死信必须上报 hub，否则 admin 看不见');
    assert.equal((hub.deadLetters[0] as any).lastError, 'runtime 起不来');
    journal.close();
    cleanup(dir);
  });

  test('不可重试的失败直接进死信，不浪费退避窗口', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = () => ({ ok: false, detail: '配置错了', retryable: false });
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 't1' });
    await connector.pullOnce();
    await connector.drain();
    assert.equal(adapter.wakes.length, 1);
    assert.equal(connector.queue.deadLetters().length, 1);
    journal.close();
    cleanup(dir);
  });

  test('死信不会把 cursor 永久钉死', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = (p) => (p.seq === 1 ? { ok: false, retryable: false } : { ok: true });
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 'a' });
    hub.push({ kind: 'todo.assigned', threadId: 'b' });
    await connector.pullOnce();
    await connector.drain();
    await sleep(20);
    assert.equal(hub.acks.at(-1), 2);
    journal.close();
    cleanup(dir);
  });

  test('hub 没有死信端点时只记日志，不把队列堵死', async () => {
    hub.supportsDeadLetters = false;
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    adapter.behavior = () => ({ ok: false, retryable: false });
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 'a' });
    await connector.pullOnce();
    await connector.drain();
    assert.equal(connector.queue.deadLetters()[0].reported, true);
    journal.close();
    cleanup(dir);
  });
});

describe('需求：一个身份一条连接', () => {
  test('同一身份的第二个实例被拒绝启动', () => {
    const dir = tmpDir();
    const a = new InstanceLock(dir, 'agent-1');
    a.acquire();
    const b = new InstanceLock(dir, 'agent-1');
    assert.throws(() => b.acquire(), DuplicateInstanceError);
    a.release();
    // 前一个退出后可以正常接管
    b.acquire();
    b.release();
    cleanup(dir);
  });

  test('被 SIGKILL 留下的陈旧锁会被接管', () => {
    const dir = tmpDir();
    writeFileSync(`${dir}/agent-1.lock`, JSON.stringify({ pid: 999999, startedAt: '2020-01-01' }));
    const lock = new InstanceLock(dir, 'agent-1');
    lock.acquire();
    lock.release();
    cleanup(dir);
  });
});

describe('需求：至少一次投递下的去重', () => {
  test('hub 重发同一条事件不会重复唤起 runtime', async () => {
    const dir = tmpDir();
    const adapter = new FakeAdapter();
    let gate!: () => void;
    const g = new Promise<void>((r) => { gate = r; });
    adapter.behavior = async () => { await g; return { ok: true }; };
    const { connector, journal } = makeConnector(dir, adapter, {
      queue: { ...testConfig(baseUrl, dir).queue, coalesceWindowMs: 0 } as never,
    });
    hub.push({ kind: 'todo.assigned', threadId: 'a' });
    await connector.pullOnce();
    await sleep(10);
    // 模拟 cursor 回退后的重发
    (connector as any);
    assert.equal(connector.queue.enqueue({ seq: 1, kind: 'todo.assigned', threadId: 'a' }), 'duplicate');
    gate();
    await connector.drain();
    assert.equal(adapter.wakes.length, 1);
    journal.close();
    cleanup(dir);
  });
});

/**
 * 这一组是同一次故障的另外两半。日志里只有：
 *
 *   [warn] 唤起失败(pending) { localId: 1, kind: undefined, detail: 'exit 2: sh: 0: Illegal option --' }
 *
 * `kind: undefined` 和那句 `sh:` 各自对应一个 bug，而且**两个都不报错**。
 */
describe('需求：配置与线上字段名的两个静默坑', () => {
  test('claude-code 的清单不该继承默认 generic-shell 的 command', () => {
    // DEFAULTS.adapter 是 generic-shell + command: ['sh','-c','cat >/dev/null']。
    // 深合并会把它漏进 claude-code 的清单，于是唤起的是 sh 而不是 claude ——
    // 运气好报 `Illegal option --`，运气不好事件被喂进 /dev/null 还退出码 0。
    const dir = tmpDir();
    const f = join(dir, 'c.json');
    writeFileSync(f, JSON.stringify({
      hub: { baseUrl: 'https://h' },
      adapter: { type: 'claude-code', bin: 'claude', cwd: '/tmp' },
    }));
    const cfg = loadConfig(f);
    assert.equal(cfg.adapter.type, 'claude-code');
    assert.equal((cfg.adapter as Record<string, unknown>).command, undefined,
      'claude-code 的清单里不该出现默认 generic-shell 的 command');
  });

  test('同 type 时照常继承默认值', () => {
    const dir = tmpDir();
    const f = join(dir, 'c.json');
    writeFileSync(f, JSON.stringify({
      hub: { baseUrl: 'https://h' },
      adapter: { type: 'generic-shell', timeoutSeconds: 30 },
    }));
    const cfg = loadConfig(f);
    assert.deepEqual((cfg.adapter as Record<string, unknown>).command, ['sh', '-c', 'cat >/dev/null'],
      '同一个 type 下默认清单该继续兜底');
    assert.equal((cfg.adapter as Record<string, unknown>).timeoutSeconds, 30);
  });
})
