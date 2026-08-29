// 从 JSON Schema 造示例值，再由示例值拼 curl。
// 目标不是「schema 的完整投影」，是「贴上去就能跑的一条命令」。
import { resolveRef } from './spec.mjs';

const UUIDS = [
  '9f1c0b6e-6b1a-4c8e-9d2f-7a3b5c1e0d44',
  '2c7d81a3-5e40-4bb1-8a6f-1d9e3f705b12',
  '4e58a0d2-91cf-4a17-b3c8-6f2b0e5da983',
];

const SAMPLES = {
  registrationToken: 'rt_9Qk2xR7fLp0aZ3nV',
  credential: 'ah_live_7bE2sK9wQ1mX4tR8',
  cursor: 1042,
  // 数值给贴近真实部署的值，不然满屏都是 42，读者反而看不出哪个字段是什么量级
  priority: 0,
  limit: 50,
  longPollMaxSeconds: 30,
  inboxRetentionDays: 30,
  longpoll: 90,
  webhook: 300,
  cron: 5400,
  tweetsPerHour: 20,
  inboxWritesPerMinute: 60,
  apiRequestsPerMinute: 120,
  typicalLatencySeconds: 45,
  replyCount: 4,
  outboxLagSeconds: 0.9,
  outboxPending: 3,
  outboxDead: 0,
  pendingLongPolls: 2,
  workerAlive: true,
  seq: 1043,
  lastSeq: 1043,
  attempts: 3,
  error: 'runtime 连续 3 次唤起失败：exit 127',
  kind: 'todo.assigned',
  body: '收到，我先看一下日志。@log-digger 帮忙拉一下昨晚的 outbox 指标',
  note: '接口已联调通过，等你确认',
  title: '把 outbox_lag 告警接到值班群',
  username: 'admin',
  password: '••••••••',
  timezone: 'Asia/Shanghai',
  message: '发布频率超过上限，等一会儿再来',
  code: 'rate_limited',
  name: 'log-digger',
  summary: '@log-digger 提交了交付物',
  status: 'in_progress',
  tags: ['ops', 'alerting'],
  mentions: [UUIDS[1]],
  watchers: ['log-digger', 'doc-writer'],
  skills: [{ id: 'log-analysis', name: '日志分析' }],
  limitations: ['不能访问生产数据库', '单次分析上限 200MB 日志'],
  runtime: 'claude-code',
  value: 'ops',
  purpose: '值班时翻日志、定位异常、给出复现步骤',
  openTodos: 2,
  hasCard: true,
  description: '翻日志、定位异常、给出复现步骤',
  authorName: 'log-digger',
};

const NOW = '2026-08-28T09:12:44Z';

export function exampleFor(spec, node, key = '', depth = 0, seed = { i: 0 }) {
  const s = resolveRef(spec, node);
  if (!s || depth > 6) return null;

  if (s.examples?.length) return s.examples[0];
  if (s.example !== undefined) return s.example;
  if (s.const !== undefined) return s.const;
  if (s.default !== undefined && s.type !== 'object') return s.default;
  if (s.oneOf) return exampleFor(spec, s.oneOf[0], key, depth + 1, seed);
  if (s.anyOf) return exampleFor(spec, s.anyOf[0], key, depth + 1, seed);
  if (s.allOf) {
    return Object.assign({}, ...s.allOf.map((x) => exampleFor(spec, x, key, depth + 1, seed) ?? {}));
  }
  if (s.enum?.length) return s.enum[0];

  if (key in SAMPLES) return structuredClone(SAMPLES[key]);

  switch (s.type) {
    case 'object': {
      if (!s.properties) return { note: '任意 JSON 对象' };
      const out = {};
      for (const [k, v] of Object.entries(s.properties)) out[k] = exampleFor(spec, v, k, depth + 1, seed);
      return out;
    }
    case 'array': {
      const item = exampleFor(spec, s.items ?? {}, key, depth + 1, seed);
      return item === null ? [] : [item];
    }
    case 'integer':
      return /seconds|Seconds/.test(key) ? 30 : /count|Count|priority/.test(key) ? 1 : 42;
    case 'number':
      return 0.8;
    case 'boolean':
      return true;
    case 'string':
    default:
      if (s.format === 'uuid') return UUIDS[seed.i++ % UUIDS.length];
      if (s.format === 'date-time') return NOW;
      if (s.format === 'date') return NOW.slice(0, 10);
      if (/Id$/.test(key)) return UUIDS[seed.i++ % UUIDS.length];
      if (/At$/.test(key)) return NOW;
      return key ? `<${key}>` : 'string';
  }
}

export function paramExample(spec, p) {
  const s = resolveRef(spec, p.schema) ?? {};
  if (s.examples?.length) return String(s.examples[0]);
  if (s.default !== undefined) return String(s.default);
  if (s.enum?.length) return String(s.enum[0]);
  if (s.format === 'uuid') return UUIDS[0];
  if (s.format === 'date') return NOW.slice(0, 10);
  if (p.name === 'Idempotency-Key') return UUIDS[0];
  if (p.name === 'status') return 'in_progress';
  if (p.name === 'skill') return 'log-analysis';
  if (p.name === 'tag') return 'ops';
  if (p.name === 'after') return '1042';
  if (p.name === 'limit') return '50';
  if (s.type === 'integer') return '10';
  if (s.type === 'boolean') return 'true';
  return `<${p.name}>`;
}

const AUTH_HEADER = {
  agentToken: `-H "Authorization: Bearer $AGENT_TOKEN"`,
  adminSession: `-b "hub_session=$HUB_SESSION"`,
};

export function buildCurl(spec, op, server) {
  let url = server + op.path;
  for (const p of op.params.filter((p) => p.in === 'path')) {
    url = url.replace(`{${p.name}}`, paramExample(spec, p));
  }
  const query = op.params.filter((p) => p.in === 'query').map((p) => {
    const v = paramExample(spec, p);
    // `<code>` 这种占位符别编码成 %3C，贴出来是给人替换的
    return `${p.name}=${v.startsWith('<') ? v : encodeURIComponent(v)}`;
  });
  if (query.length) url += '?' + query.join('&');

  const lines = [];
  const verb = op.method === 'GET' ? '' : ` -X ${op.method}`;
  // 3xx 的端点看的是响应头，不看响应体
  const redirects = op.responses.some((r) => r.status.startsWith('3'));
  lines.push(`curl${redirects ? ' -i' : ''}${verb} '${url}'`);

  for (const scheme of op.security) if (AUTH_HEADER[scheme]) lines.push(AUTH_HEADER[scheme]);
  for (const p of op.params.filter((p) => p.in === 'header')) {
    lines.push(`-H "${p.name}: ${p.name === 'Idempotency-Key' ? '$(uuidgen)' : paramExample(spec, p)}"`);
  }

  if (op.body) {
    const payload = exampleFor(spec, op.body.schema);
    lines.push(`-H "Content-Type: application/json"`);
    lines.push(`-d '${JSON.stringify(payload, null, 2)}'`);
  }

  return lines.join(' \\\n  ');
}

export function responseExample(spec, op) {
  const ok = op.responses.find((r) => r.status.startsWith('2') && r.schema);
  if (!ok) return null;
  return { status: ok.status, json: exampleFor(spec, ok.schema) };
}
