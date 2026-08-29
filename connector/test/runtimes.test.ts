import { test, describe, after } from 'node:test';
import assert from 'node:assert/strict';
import { writeFileSync, chmodSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createServer } from 'node:http';
import { AddressInfo } from 'node:net';
import { CodexAdapter, sessionFromJsonl } from '../src/adapters/codex.js';
import { OpencodeAdapter, sessionFromJson } from '../src/adapters/opencode.js';
import { OpenclawAdapter } from '../src/adapters/openclaw.js';
import { createAdapter } from '../src/adapters/registry.js';
import { openJournal } from '../src/core/journal.js';
import { WakePayload } from '../src/core/types.js';
import { tmpDir, cleanup } from './helpers.js';

const dirs: string[] = [];
after(() => dirs.forEach(cleanup));
function scratch(): string { const d = tmpDir(); dirs.push(d); return d; }

const payload = (over: Partial<WakePayload> = {}): WakePayload => ({
  localId: 1, kind: 'todo.assigned', priority: 0, threadId: 't1',
  seqs: [1], seq: 1, coalescedCount: 1, attempt: 1,
  event: { seq: 1, kind: 'todo.assigned', threadId: 't1' }, ...over,
});

/**
 * 造一个假的 runtime 可执行文件：把自己收到的 argv 原样打回来，
 * 顺便吐一个 session id。这样就能断言「我们到底拼了什么命令」——
 * 适配器的全部价值就在这条命令上，拼错了功能就是废的。
 */
function stubBin(dir: string, name: string, sessionJson: string): string {
  const p = join(dir, name);
  // argv 逐行写进旁边的文件，不要拼进 stdout 的 JSON —— 提示词里带换行和引号，
  // 拼进去会把 JSON 打断，测试就变成在验证 shell 引号而不是在验证命令。
  writeFileSync(
    p,
    `#!/bin/sh\n: > "$0.argv"\nfor a in "$@"; do printf '%s\\n' "$a" >> "$0.argv"; done\nprintf '%s\\n' '${sessionJson}'\n`,
  );
  chmodSync(p, 0o755);
  return p;
}

/** 读回假 runtime 收到的参数。最后一个是提示词，断言命令时不看它。 */
function argvOf(bin: string): string {
  const lines = readFileSync(`${bin}.argv`, 'utf8').split('\n').filter(Boolean);
  return lines.slice(0, -1).join(' ');
}

describe('需求：codex 以非交互方式唤起，并按 thread 续接会话', () => {
  test('第一次唤起走 codex exec --json，带沙箱级别，不带 resume', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    const bin = stubBin(dir, 'codex', '{"session_id":"S-1"}');
    const a = new CodexAdapter({ bin }, j);
    const out = await a.wake(payload());
    assert.equal(out.ok, true);
    const argv = argvOf(bin);
    assert.match(argv, /^exec /);
    assert.match(argv, /--json/);
    assert.match(argv, /--sandbox workspace-write/);
    assert.doesNotMatch(argv, /resume/);
  });

  test('同一个 thread 第二次唤起接上第一次的会话', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    const bin = stubBin(dir, 'codex', '{"session_id":"S-42"}');
    const a = new CodexAdapter({ bin }, j);
    await a.wake(payload());
    const out = await a.wake(payload({ localId: 2, seq: 2 }));
    const argv = argvOf(bin);
    assert.match(argv, /exec resume S-42/);
  });

  test('会话 id 落盘：换一个适配器实例（等同进程重启）仍然接得上', async () => {
    const dir = scratch();
    const bin = stubBin(dir, 'codex', '{"session_id":"S-7"}');
    const j1 = openJournal(dir, 'auto');
    await new CodexAdapter({ bin }, j1).wake(payload());
    const j2 = openJournal(dir, 'auto');
    await new CodexAdapter({ bin }, j2).wake(payload({ localId: 9 }));
    assert.match(argvOf(bin), /resume S-7/);
  });

  test('拿不到 session id 就退回无状态，绝不 resume 到别人的会话上', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    // 这个假 runtime 不吐 session id
    const bin = stubBin(dir, 'codex', '{"noise":"没有会话字段"}');
    const a = new CodexAdapter({ bin }, j);
    await a.wake(payload());
    const out = await a.wake(payload({ localId: 2 }));
    const argv = argvOf(bin);
    assert.doesNotMatch(argv, /resume/);
  });

  test('JSONL 里字段藏在嵌套结构里也能找到', () => {
    assert.equal(sessionFromJsonl('{"a":1}\n{"msg":{"conversation_id":"C-9"}}'), 'C-9');
    assert.equal(sessionFromJsonl('not json at all'), undefined);
  });
});

describe('需求：opencode 以非交互方式唤起，并按 thread 续接会话', () => {
  test('走 opencode run --format json', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    const bin = stubBin(dir, 'opencode', '{"sessionID":"ses_1"}');
    const a = new OpencodeAdapter({ bin }, j);
    await a.wake(payload());
    const argv = argvOf(bin);
    assert.match(argv, /^run /);
    assert.match(argv, /--format json/);
  });

  test('续接用 -s <id>，不是 --continue', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    const bin = stubBin(dir, 'opencode', '{"sessionID":"ses_77"}');
    const a = new OpencodeAdapter({ bin }, j);
    await a.wake(payload());
    const out = await a.wake(payload({ localId: 2 }));
    const argv = argvOf(bin);
    // --continue 接的是「上一个会话」，并发处理多个 thread 时会接错
    assert.match(argv, /-s ses_77/);
    assert.doesNotMatch(argv, /--continue/);
  });

  test('model / agent / attach 都会传下去', async () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    const bin = stubBin(dir, 'opencode', '{}');
    const a = new OpencodeAdapter({
      bin, model: 'anthropic/claude-sonnet-4-5', agent: 'build', attach: 'http://127.0.0.1:4096',
    }, j);
    const out = await a.wake(payload());
    const argv = argvOf(bin);
    assert.match(argv, /--model anthropic\/claude-sonnet-4-5/);
    assert.match(argv, /--agent build/);
    assert.match(argv, /--attach http:\/\/127\.0\.0\.1:4096/);
  });

  test('不会把消息 id 误当成会话 id', () => {
    assert.equal(sessionFromJson('{"id":"msg_1"}'), undefined);
    assert.equal(sessionFromJson('{"sessionID":"ses_2"}'), 'ses_2');
  });
});

describe('需求：openclaw 的子命令必须显式配置，不猜', () => {
  test('没配 subcommand 时 start() 就报错，并指出怎么查', async () => {
    const a = new OpenclawAdapter({ bin: 'openclaw' });
    await assert.rejects(() => a.start(), /subcommand[\s\S]*openclaw --help/);
  });

  test('配了就按 argv 片段拼命令', async () => {
    const dir = scratch();
    const bin = stubBin(dir, 'openclaw', '{}');
    const a = new OpenclawAdapter({ bin, subcommand: ['message', 'send'] });
    await a.start();
    const out = await a.wake(payload());
    const argv = argvOf(bin);
    assert.match(argv, /^message send /);
  });
});

describe('需求：常驻型 runtime 走 webhook，body 要塑成对方认得的形状', () => {
  async function captureBody(type: string, over: Record<string, unknown> = {}) {
    const bodies: unknown[] = [];
    const srv = createServer((req, res) => {
      let b = ''; req.on('data', (d) => { b += d; });
      req.on('end', () => { bodies.push(JSON.parse(b)); res.writeHead(200).end('ok'); });
    });
    await new Promise<void>((r) => srv.listen(0, '127.0.0.1', r));
    const url = `http://127.0.0.1:${(srv.address() as AddressInfo).port}/hook`;
    const dir = scratch();
    const a = createAdapter({ type, url, ...over } as never, openJournal(dir, 'auto'));
    const out = await a.wake(payload());
    await new Promise<void>((r) => srv.close(() => r()));
    return { out, body: bodies[0] as Record<string, unknown>, caps: a.capabilities() };
  }

  test('hermes 收到的是 text 字段，原始负载挂在 agentHub 下', async () => {
    const { out, body, caps } = await captureBody('hermes');
    assert.equal(out.ok, true);
    assert.equal(caps.runtime, 'hermes');
    assert.match(String(body.text), /agent-hub 事件：todo.assigned/);
    assert.equal((body.agentHub as WakePayload).threadId, 't1');
  });

  test('openhuman 收到的是 message 字段', async () => {
    const { body, caps } = await captureBody('openhuman');
    assert.equal(caps.runtime, 'openhuman');
    assert.match(String(body.message), /thread: t1/);
  });

  test('extraBody 会被合进去，用来带 chat id 这类固定字段', async () => {
    const { body } = await captureBody('hermes', { extraBody: { chatId: 'C-1' } });
    assert.equal(body.chatId, 'C-1');
  });

  test('不填 messageField 时原样 POST WakePayload —— 自己写的服务认得我们的字段', async () => {
    const { body } = await captureBody('http-endpoint');
    assert.equal((body as unknown as WakePayload).kind, 'todo.assigned');
    assert.equal(body.text, undefined);
  });
});

describe('需求：注册表覆盖五个新 runtime', () => {
  test('都能造出来，且 codex-cli 这个老名字还认', () => {
    const dir = scratch();
    const j = openJournal(dir, 'auto');
    for (const type of ['codex', 'codex-cli', 'opencode', 'openclaw']) {
      assert.ok(createAdapter({ type } as never, j), type);
    }
    for (const type of ['hermes', 'openhuman']) {
      assert.ok(createAdapter({ type, url: 'http://127.0.0.1:1/x' } as never, j), type);
    }
  });
});
