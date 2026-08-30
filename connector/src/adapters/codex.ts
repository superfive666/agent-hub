import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { HubHint, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { Journal } from '../core/journal.js';
import { wakePrompt, rememberSession, priorSession } from './prompt.js';

export interface CodexManifest extends Partial<GenericShellManifest> {
  /** 可执行文件，默认 `codex`。 */
  bin?: string;
  /** 传给 codex 的模型，对应 `--model`。不填走 codex 自己的默认。 */
  model?: string;
  /**
   * 沙箱级别，对应 `--sandbox`。默认 `workspace-write` ——
   * agent 要能改工作区文件才谈得上干活，但默认不放开网络与工作区之外的写入。
   */
  sandbox?: string;
  /** 附加参数，插在子命令之后。 */
  args?: string[];
}

/**
 * OpenAI Codex CLI。非交互入口是 `codex exec`：
 *
 *   codex exec --json "<prompt>"            # 一次性执行，stdout 是 JSONL 事件流
 *   codex exec resume <SESSION_ID> "<...>"  # 接着上一轮的会话
 *
 * 同一个 thread 复用同一个 codex 会话：session id 落盘，后续唤起走 resume。
 * 拿不到 session id 就退回无状态执行 —— **宁可重头讲一遍，也不能 resume 到别人的会话上**。
 */
export class CodexAdapter extends GenericShellAdapter {
  #journal: Journal;
  #sessions: Map<string, string>;

  constructor(m: CodexManifest, journal: Journal, hub?: HubHint) {
    super({ ...m, command: [m.bin ?? 'codex'] } as GenericShellManifest, 'codex', hub);
    this.#journal = journal;
    this.#sessions = priorSession(journal);
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'codex', resumesSession: true };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const m = this.m as CodexManifest;
    const bin = this.m.command[0];
    const prior = p.threadId ? this.#sessions.get(p.threadId) : undefined;

    const argv = [bin, 'exec'];
    if (prior) argv.push('resume', prior);
    argv.push('--json');
    if (m.model) argv.push('--model', m.model);
    argv.push('--sandbox', m.sandbox ?? 'workspace-write');
    argv.push(...(m.args ?? []));
    argv.push(wakePrompt(p, this.hub));

    // runRaw 而不是 run：session id 要从**完整**的 stdout 里找。JSONL 流里那一行
    // 可能排在很后面，而 Outcome.detail 只留 2000 字 —— 从截断过的 detail 里找，
    // 长输出时会静默地找不到，表现是「会话不接上了」，而且没有任何报错。
    const r = await this.runRaw(argv, '');
    const outcome = this.judge(r);
    if (outcome.ok && p.threadId) {
      const sid = sessionFromJsonl(r.out);
      if (sid) rememberSession(this.#journal, this.#sessions, p.threadId, sid);
    }
    return outcome;
  }
}

/**
 * 从 `--json` 的 JSONL 流里找 session id。
 *
 * 逐行扫、字段名多试几个：codex 的事件 schema 会随版本变，写死一个字段名等于
 * 「升级之后静默退化成无状态」，而那是最难发现的一类问题 —— 功能还在，只是会话不接上了。
 */
export function sessionFromJsonl(out: string): string | undefined {
  const keys = ['session_id', 'sessionId', 'conversation_id', 'conversationId', 'thread_id', 'threadId'];
  for (const line of out.split('\n')) {
    const t = line.trim();
    if (!t.startsWith('{')) continue;
    let j: unknown;
    try { j = JSON.parse(t); } catch { continue; }
    const found = digForKeys(j, keys);
    if (found) return found;
  }
  return undefined;
}

function digForKeys(v: unknown, keys: string[], depth = 0): string | undefined {
  if (depth > 4 || v === null || typeof v !== 'object') return undefined;
  const o = v as Record<string, unknown>;
  for (const k of keys) {
    const hit = o[k];
    if (typeof hit === 'string' && hit) return hit;
  }
  for (const nested of Object.values(o)) {
    const hit = digForKeys(nested, keys, depth + 1);
    if (hit) return hit;
  }
  return undefined;
}
