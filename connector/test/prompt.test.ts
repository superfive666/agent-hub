import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { wakePrompt } from '../src/adapters/prompt.js';
import { HubHint, WakePayload } from '../src/core/types.js';

const payload = (over: Partial<WakePayload> = {}): WakePayload => ({
  localId: 1, kind: 'tweet.mentioned', priority: 1, threadId: 'th-1',
  seqs: [7], seq: 7, coalescedCount: 1, attempt: 1,
  event: { seq: 7, kind: 'tweet.mentioned', threadId: 'th-1' }, ...over,
});

const hub: HubHint = {
  baseUrl: 'https://hub.example.com',
  agentId: 'a-1',
  tokenFile: '/home/me/.config/agent-hub-connector/token',
  tokenEnv: 'AGENT_HUB_TOKEN',
};

/**
 * 这一组用例来自一次真实故障：hub 侧全绿 —— mention 记下了、outbox done、
 * inbox 有 tweet.mentioned、cursor 推进到位、没有死信 —— 但 agent 就是没回话。
 *
 * 原因是唤起 prompt 只说了「去 hub 拉取 thread 全文」，没说 hub 在哪、凭证在哪、
 * 回帖打哪个接口。被 headless 叫起来的 runtime 手上什么都没有，想回也回不了，
 * 于是它想了想就退出，退出码 0 —— connector 当成处理成功，那条 @ 石沉大海，
 * 而且**任何一个地方都查不到异常**。
 */
describe('需求：唤起 prompt 必须够 runtime 自己走完一轮', () => {
  test('带上 hub 地址、读 thread 和回帖两个接口', () => {
    const s = wakePrompt(payload(), hub);
    assert.match(s, /hub\.example\.com/, '没有 hub 地址，runtime 不知道去哪');
    assert.match(s, /\/api\/agent\/threads\/th-1/, '没给读 thread 的接口');
    assert.match(s, /POST[\s\S]*\/api\/agent\/threads\/th-1\/posts/, '没给回帖的接口');
    assert.match(s, /th-1/, 'threadId 要拼进命令里，不能让它自己填');
  });

  test('凭证只给路径，绝不给值', () => {
    const s = wakePrompt(payload(), hub);
    assert.match(s, /\.config\/agent-hub-connector\/token/, '要告诉它凭证在哪');
    // prompt 会进 runtime 的日志和会话记录，凭证进去了就收不回来
    assert.ok(!s.includes('Bearer ah_live'), 'prompt 里不许出现凭证明文');
    assert.match(s, /cat \/home\/me/, '应当让它自己去读文件，而不是我们替它读出来');
  });

  test('没有 tokenFile 时退回环境变量名，同样不给值', () => {
    const s = wakePrompt(payload(), { baseUrl: 'https://h', tokenEnv: 'MY_TOKEN' });
    assert.match(s, /\$MY_TOKEN/);
    assert.ok(!s.includes('/token'), '没有 tokenFile 就不该编一个路径出来');
  });

  test('说清楚什么时候必须回 —— 沉默在对面看来和「没收到」一模一样', () => {
    const s = wakePrompt(payload(), hub);
    assert.match(s, /被 @|必须回/);
  });

  test('不给 hub 线索时仍然能出一段话，不炸', () => {
    const s = wakePrompt(payload());
    assert.match(s, /tweet\.mentioned/);
    assert.ok(!s.includes('undefined'), '缺配置时不该把 undefined 拼进 prompt');
  });

  test('合并过的唤起要说明合并了几条，否则 runtime 只会处理最后一条', () => {
    const s = wakePrompt(payload({ coalescedCount: 3, seqs: [5, 6, 7] }), hub);
    assert.match(s, /合并了 3 条/);
    assert.match(s, /5,6,7/);
  });
});
