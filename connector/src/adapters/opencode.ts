import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { HubHint, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { Journal } from '../core/journal.js';
import { wakePrompt, rememberSession, priorSession } from './prompt.js';

export interface OpencodeManifest extends Partial<GenericShellManifest> {
  /** 可执行文件，默认 `opencode`。 */
  bin?: string;
  /** `--model`，形如 `anthropic/claude-sonnet-4-5`。不填走 opencode 的默认。 */
  model?: string;
  /** `--agent`，opencode 自己的 agent 预设（如 build / plan）。 */
  agent?: string;
  /**
   * `--attach`，指向一个已经在跑的 `opencode serve`。
   * 挂上去可以省掉每次唤起时 MCP server 的冷启动，事件密集时值得开。
   */
  attach?: string;
  args?: string[];
}

/**
 * opencode。非交互入口是 `opencode run`：
 *
 *   opencode run --format json "<prompt>"        # 一次性执行
 *   opencode run -s <SESSION_ID> "<prompt>"      # 接着指定会话
 *
 * 用 `-s` 而不是 `-c/--continue`：`--continue` 接的是「上一个会话」，
 * 而 connector 会并发处理多个 thread，接错会话比不接更糟。
 */
export class OpencodeAdapter extends GenericShellAdapter {
  #journal: Journal;
  #sessions: Map<string, string>;

  constructor(m: OpencodeManifest, journal: Journal, hub?: HubHint) {
    super({ ...m, command: [m.bin ?? 'opencode'] } as GenericShellManifest, 'opencode', hub);
    this.#journal = journal;
    this.#sessions = priorSession(journal);
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'opencode', resumesSession: true };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const m = this.m as OpencodeManifest;
    const prior = p.threadId ? this.#sessions.get(p.threadId) : undefined;

    const argv = [this.m.command[0], 'run', '--format', 'json'];
    if (prior) argv.push('-s', prior);
    if (m.model) argv.push('--model', m.model);
    if (m.agent) argv.push('--agent', m.agent);
    if (m.attach) argv.push('--attach', m.attach);
    argv.push(...(m.args ?? []));
    argv.push(wakePrompt(p, this.hub));

    const outcome = await this.run(argv, '');
    if (outcome.ok && p.threadId) {
      const sid = sessionFromJson(outcome.detail ?? '');
      if (sid) rememberSession(this.#journal, this.#sessions, p.threadId, sid);
    }
    return outcome;
  }
}

/** `--format json` 的输出里找 session id。字段名多试几个，理由同 codex 适配器。 */
export function sessionFromJson(out: string): string | undefined {
  const keys = ['sessionID', 'sessionId', 'session_id', 'id'];
  const trimmed = out.trim();
  if (!trimmed) return undefined;
  const candidates = trimmed.startsWith('{') || trimmed.startsWith('[') ? [trimmed] : trimmed.split('\n');
  for (const c of candidates) {
    const t = c.trim();
    if (!t.startsWith('{') && !t.startsWith('[')) continue;
    try {
      const j = JSON.parse(t);
      const hit = dig(j, keys);
      if (hit) return hit;
    } catch { /* 不是 JSON 就跳过，run 的输出里可能混着普通日志 */ }
  }
  return undefined;
}

function dig(v: unknown, keys: string[], depth = 0): string | undefined {
  if (depth > 4 || v === null || typeof v !== 'object') return undefined;
  if (Array.isArray(v)) {
    for (const it of v) { const h = dig(it, keys, depth + 1); if (h) return h; }
    return undefined;
  }
  const o = v as Record<string, unknown>;
  // session 字段优先，避免把消息 id 当成会话 id
  for (const k of keys) {
    const hit = o[k];
    if (typeof hit === 'string' && hit && (k !== 'id' || typeof o.parentID !== 'undefined')) return hit;
  }
  for (const nested of Object.values(o)) {
    const h = dig(nested, keys, depth + 1);
    if (h) return h;
  }
  return undefined;
}
