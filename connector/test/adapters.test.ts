import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { GenericShellAdapter } from '../src/adapters/generic-shell.js';
import { HttpEndpointAdapter } from '../src/adapters/http-endpoint.js';
import { createAdapter } from '../src/adapters/registry.js';
import { openJournal } from '../src/core/journal.js';
import { Queue } from '../src/core/queue.js';
import { WakePayload } from '../src/core/types.js';
import { tmpDir, cleanup } from './helpers.js';
import { writeFileSync, chmodSync } from 'node:fs';
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
    // 判据是「跑的是不是 echo」：echo 会把 claude 的参数原样打回来。
    // 不能再用 ok 当判据 —— echo 不吐 `--output-format json` 的结果信封，
    // 而 claude-code 现在会读那份信封，所以这里的 ok 本来就该是 false。
    assert.match(r.detail!, /--output-format json/, 'bin 必须赢过配置里残留的 command');
    assert.doesNotMatch(r.detail!, /Illegal option/, '走到 dash 上就说明 bin 被顶掉了');
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

/**
 * 这一组也来自一次真实故障，而且是更难查的那一类：**成功路径上的静默**。
 *
 * 04:19 的那条 todo.assigned 一直没人回。查下来是 PATH 太窄找不到 claude，
 * 四次重试后进了死信 —— 那次还算幸运，至少有死信可查。真正危险的是它的近亲：
 * headless 的 claude 撞上权限确认、或者跑到 max turns，**照样 exit 0**。
 * 只看退出码的话，「什么都没干」和「干完了」一模一样：队列行删掉、cursor 推进、
 * 不进死信，而成功路径不打日志 —— 那条 @ 石沉大海，任何一个地方都查不出异常。
 */
describe('需求：退出码不是成功的证据 —— 能结构化输出的 runtime 要读它自己的输出', () => {
  /** 造一个假的 claude：把给定内容打到 stdout，然后按给定退出码退出。 */
  function fakeClaude(dir: string, stdout: string, code = 0): string {
    const p = join(dir, 'claude');
    writeFileSync(p, `#!/bin/sh\ncat > /dev/null\nprintf '%s' '${stdout}'\nexit ${code}\n`);
    chmodSync(p, 0o755);
    return p;
  }

  const mk = (bin: string) =>
    createAdapter({ type: 'claude-code', bin } as never, openJournal(tmpDir(), 'jsonl'));

  test('信封里 is_error=true 就是失败，哪怕 exit 0', async () => {
    const bin = fakeClaude(tmpDir(),
      '{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}');
    const r = await mk(bin).wake(payload());
    assert.equal(r.ok, false, 'runtime 自己说失败了，不能当成功');
    assert.match(r.detail!, /error_during_execution/, '报错里要带上 subtype，否则查的人只知道「失败了」');
  });

  test('subtype 不是 success 也是失败 —— max turns 是最常见的那个', async () => {
    const bin = fakeClaude(tmpDir(),
      '{"type":"result","subtype":"error_max_turns","is_error":false,"result":""}');
    const r = await mk(bin).wake(payload());
    assert.equal(r.ok, false);
    assert.match(r.detail!, /error_max_turns/);
  });

  test('要了 JSON 却没拿到信封：exit 0 也不算成功', async () => {
    // 进程被杀、参数不对、装的根本不是我们以为的那个 claude，都长这样。
    // 当成功处理等于把这条事件丢掉，而且不留痕迹。
    const bin = fakeClaude(tmpDir(), 'Usage: claude [options]');
    const r = await mk(bin).wake(payload());
    assert.equal(r.ok, false);
    assert.equal(r.retryable, true, '这类值得重试 —— 可能只是这一次没起来');
    assert.match(r.detail!, /Usage: claude/, '把它到底输出了什么带上，否则无从下手');
  });

  test('正常成功仍然是成功，session id 照常记下来', async () => {
    const dir = tmpDir();
    const bin = fakeClaude(dir,
      '{"type":"result","subtype":"success","is_error":false,"result":"回好了","session_id":"s-42"}');
    const j = openJournal(tmpDir(), 'jsonl');
    const a = createAdapter({ type: 'claude-code', bin } as never, j);
    const r = await a.wake(payload());
    assert.equal(r.ok, true);
    assert.equal(j.loadMeta()['session:t1'], 's-42', '同 thread 的下一次唤起要能 --resume');
  });

  test('输出很长时 session id 仍然找得到 —— 不能从截断过的 detail 里找', async () => {
    // detail 只留 2000 字。从截断过的 detail 里找 session id，长输出时会静默地
    // 找不到：功能还在，只是会话不再续接，而且没有任何报错。
    const pad = 'x'.repeat(4000);
    const bin = fakeClaude(tmpDir(),
      `{"type":"result","subtype":"success","is_error":false,"result":"${pad}","session_id":"s-long"}`);
    const j = openJournal(tmpDir(), 'jsonl');
    const a = createAdapter({ type: 'claude-code', bin } as never, j);
    const r = await a.wake(payload());
    assert.equal(r.ok, true);
    assert.equal(j.loadMeta()['session:t1'], 's-long');
  });

  test('generic-shell 只能停在退出码上，但 exit 0 时的 stderr 要留下来', async () => {
    // 一条任意的 shell 命令没有可依赖的成功语义，我们没有立场判它失败；
    // 但它一路往 stderr 上抱怨的内容，是排查时最需要看见的那种。
    const a = new GenericShellAdapter({ command: ['sh', '-c', 'echo 出事了 >&2; exit 0'] });
    const r = await a.wake(payload());
    assert.equal(r.ok, true);
    assert.match(r.detail!, /出事了/);
  });
});

describe('需求：病根修好之后，进了死信的那条事件要能重放', () => {
  test('revive 把死信放回 pending，attempts 归零', () => {
    const dir = tmpDir();
    const j = openJournal(dir, 'jsonl');
    const q = new Queue(j, {
      maxConcurrentWakes: 1, coalesceWindowMs: 0, coalesceKinds: [],
      maxAttempts: 2, backoffBaseMs: 0, backoffMaxMs: 0,
    });
    q.enqueue({ seq: 1, kind: 'todo.assigned', threadId: 't1' });
    const row = q.acquire()!;
    q.fail(row.id, 'sh: 0: Illegal option --');
    const again = q.acquire()!;
    assert.equal(q.fail(again.id, 'sh: 0: Illegal option --'), 'dead');
    assert.equal(q.deadLetters().length, 1);

    assert.equal(q.revive(), 1, '放回一条');
    assert.equal(q.deadLetters().length, 0);
    const back = q.acquire();
    assert.ok(back, '放回之后要能重新出队 —— 否则「重放」只是改了个状态字');
    assert.equal(back!.attempts, 0, '重新给一次完整的机会，不是接着上一轮的退避数');
    assert.deepEqual(back!.seqs, [1], '事件本身不能丢');
    j.close(); cleanup(dir);
  });

  test('没有死信时 revive 返回 0 —— 调用方要能区分「放回了」和「什么都没有」', () => {
    const dir = tmpDir();
    const j = openJournal(dir, 'jsonl');
    const q = new Queue(j, {
      maxConcurrentWakes: 1, coalesceWindowMs: 0, coalesceKinds: [],
      maxAttempts: 4, backoffBaseMs: 0, backoffMaxMs: 0,
    });
    assert.equal(q.revive(), 0);
    assert.equal(q.revive(99), 0, '指定一个不存在的 id 也是 0，不能报成放回了');
    j.close(); cleanup(dir);
  });
});

describe('需求：两个 journal 驱动的语义必须一致', () => {
  test('jsonl：setMeta 之后，同一个进程里就能读到', () => {
    // 只追加文件、不改内存那份的话，要等下次开进程重放才读得到 ——
    // sqlite 版没有这个时间差。session id 正是走这条路存的。
    for (const driver of ['jsonl', 'sqlite'] as const) {
      const dir = tmpDir();
      const j = openJournal(dir, driver);
      j.setMeta('session:t1', 's-1');
      assert.equal(j.loadMeta()['session:t1'], 's-1', `${driver}: 写完立刻要读得到`);
      j.close();
      // 重开一次：落盘的那份也得在。
      const again = openJournal(dir, driver);
      assert.equal(again.loadMeta()['session:t1'], 's-1', `${driver}: 重启后还要在`);
      again.close();
      cleanup(dir);
    }
  });
});
