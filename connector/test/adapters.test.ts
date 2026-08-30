import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { GenericShellAdapter } from '../src/adapters/generic-shell.js';
import { HttpEndpointAdapter } from '../src/adapters/http-endpoint.js';
import { createAdapter } from '../src/adapters/registry.js';
import { openJournal } from '../src/core/journal.js';
import { Queue } from '../src/core/queue.js';
import { WakePayload } from '../src/core/types.js';
import { tmpDir, cleanup } from './helpers.js';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';

const payload = (over: Partial<WakePayload> = {}): WakePayload => ({
  localId: 1, kind: 'todo.assigned', priority: 0, threadId: 't1',
  seqs: [1], seq: 1, coalescedCount: 1, attempt: 1,
  event: { seq: 1, kind: 'todo.assigned', threadId: 't1' }, ...over,
});

describe('需求：兜底适配器保证不存在「不支持的 runtime」', () => {
  test('generic-shell 把事件 JSON 从 stdin 交给命令模板', async () => {
    const a = new GenericShellAdapter({ command: ['sh', '-c', 'cat'] });
    await a.start();
    const out = await a.wake(payload());
    assert.equal(out.ok, true);
    assert.equal(JSON.parse(out.detail!).seq, 1);
  });

  test('命令模板支持占位符', async () => {
    const a = new GenericShellAdapter({ command: ['sh', '-c', 'printf %s "$0"', '{{kind}}/{{threadId}}/{{seq}}'] });
    const out = await a.wake(payload());
    assert.equal(out.detail, 'todo.assigned/t1/1');
  });

  test('runtime 退出码非 0 算失败并带上 stderr', async () => {
    const a = new GenericShellAdapter({ command: ['sh', '-c', 'echo boom >&2; exit 3'] });
    const out = await a.wake(payload());
    assert.equal(out.ok, false);
    assert.match(out.detail!, /exit 3.*boom/s);
  });

  test('缺少声明的环境变量时 start() 就失败，不等到唤起才炸', async () => {
    const a = new GenericShellAdapter({ command: ['true'], requiresEnv: ['DEFINITELY_NOT_SET_XYZ'] });
    await assert.rejects(() => a.start(), /DEFINITELY_NOT_SET_XYZ/);
  });

  test('未知 runtime 报错时指出可以用 generic-shell 接进来', () => {
    const dir = tmpDir();
    const j = openJournal(dir, 'auto');
    assert.throws(() => createAdapter({ type: 'no-such-runtime' }, j), /generic-shell/);
    j.close(); cleanup(dir);
  });

  test('http-endpoint 的 4xx 不重试、5xx 重试', async () => {
    const { createServer } = await import('node:http');
    let status = 400;
    const srv = createServer((_, res) => { res.writeHead(status).end('nope'); });
    await new Promise<void>((r) => srv.listen(0, '127.0.0.1', r));
    const url = `http://127.0.0.1:${(srv.address() as any).port}/wake`;
    const a = new HttpEndpointAdapter({ url });
    assert.equal((await a.wake(payload())).retryable, false);
    status = 503;
    assert.equal((await a.wake(payload())).retryable, true);
    srv.closeAllConnections(); await new Promise<void>((r) => srv.close(() => r()));
  });
});

describe('需求：node:sqlite 不可用时 JSONL 兜底语义一致', () => {
  test('JSONL 驱动下队列同样跨进程存活', () => {
    const dir = tmpDir();
    const opt = { maxConcurrentWakes: 1, coalesceWindowMs: 0, coalesceKinds: [], maxAttempts: 3, backoffBaseMs: 1, backoffMaxMs: 10 };
    const j1 = openJournal(dir, 'jsonl');
    assert.equal(j1.driver, 'jsonl');
    const q1 = new Queue(j1, opt);
    q1.enqueue({ seq: 1, kind: 'todo.assigned', threadId: 'a' });
    q1.enqueue({ seq: 2, kind: 'tweet.published' });
    const leased = q1.acquire()!;
    assert.equal(leased.kind, 'todo.assigned');
    j1.close(); // 模拟 SIGKILL

    const j2 = openJournal(dir, 'jsonl');
    const q2 = new Queue(j2, opt);
    assert.equal(q2.pendingCount, 2, '被强杀时正在处理的那条要回到队列，不能丢');
    assert.equal(q2.acquire()!.kind, 'todo.assigned');
    j2.close(); cleanup(dir);
  });

  test('两种驱动的合并与优先级行为一致', () => {
    for (const driver of ['sqlite', 'jsonl'] as const) {
      const dir = tmpDir();
      const j = openJournal(dir, driver);
      const q = new Queue(j, {
        maxConcurrentWakes: 1, coalesceWindowMs: 0, coalesceKinds: ['thread.replied'],
        maxAttempts: 3, backoffBaseMs: 1, backoffMaxMs: 10,
      });
      for (let i = 1; i <= 3; i++) q.enqueue({ seq: i, kind: 'thread.replied', threadId: 'x' });
      q.enqueue({ seq: 4, kind: 'todo.assigned', threadId: 'y' });
      const first = q.acquire()!;
      assert.equal(first.kind, 'todo.assigned', `${driver}: P0 先出队`);
      q.complete(first.id);
      const second = q.acquire()!;
      assert.deepEqual(second.seqs, [1, 2, 3], `${driver}: 同 thread 的三条折叠成一次唤起`);
      j.close(); cleanup(dir);
    }
  });
});

/**
 * 这一组来自一次真实故障。journalctl 里只有这么一行：
 *
 *   [warn] 唤起失败(pending) { localId: 1, kind: undefined, detail: 'exit 2: sh: 0: Illegal option --\n' }
 *
 * 里面既没有 `claude` 也没有 PATH，没有任何东西指向真正的原因。
 * 两个独立的坑都会走到这个报错上，下面各钉一条。
 */
describe('需求：命令跑不起来时，报错要指得出是哪个命令', () => {
  test('配置里多一个 command 不该把 bin 顶掉', async () => {
    // 坑一：`{ command: [bin], ...m }` 的展开顺序反了，配置里若有 command 就会赢。
    // 于是 adapter.bin 写着 claude，实际执行的却是别的东西，
    // 参数被 dash 当成自己的选项 —— 就是那句 `sh: 0: Illegal option --`。
    const a = createAdapter(
      { type: 'claude-code', bin: '/bin/echo', command: ['sh'] } as never,
      openJournal(tmpDir(), 'jsonl'),
    );
    await a.start();
    const r = await a.wake(payload());
    assert.equal(r.ok, true, 'bin 必须赢过配置里残留的 command');
  });

  test('命令不在 PATH 里时，start() 就报错，并且把命令名和 PATH 都说出来', async () => {
    // 坑二：systemd user service 的 PATH 比交互 shell 窄得多。
    // execvp 找到同名但没有合法 shebang 的文件时会退回 /bin/sh 去跑，
    // 于是错误里连命令名都没有 —— 必须在启动时就拦住。
    const a = new GenericShellAdapter({ command: ['绝对不存在的命令-zzz'] });
    await assert.rejects(() => a.start(), (e: Error) => {
      assert.match(e.message, /绝对不存在的命令-zzz/, '报错里要有命令名');
      assert.match(e.message, /PATH/, '报错里要有 PATH —— 否则看的人不知道去哪找');
      assert.match(e.message, /systemd|绝对路径/, '要指出最常见的原因和怎么改');
      return true;
    });
  });

  test('找得到但没有执行位，同样在 start() 拦住', async () => {
    const dir = tmpDir();
    const f = join(dir, 'noexec');
    writeFileSync(f, '#!/bin/sh\ntrue\n', { mode: 0o644 });
    const a = new GenericShellAdapter({ command: [f] });
    await assert.rejects(() => a.start(), /找不到可执行的/);
  });

  test('绝对路径的命令能通过检查', async () => {
    const a = new GenericShellAdapter({ command: ['/bin/echo'] });
    await a.start();
  });
})
