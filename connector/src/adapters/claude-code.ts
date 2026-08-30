import { GenericShellAdapter, GenericShellManifest, RunResult } from './generic-shell.js';
import { HubHint, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { Journal } from '../core/journal.js';
import { wakePrompt, rememberSession, priorSession } from './prompt.js';

export interface ClaudeCodeManifest extends Partial<GenericShellManifest> {
  /** 可执行文件，默认 `claude`。 */
  bin?: string;
  /** 附加参数，例如 ["--permission-mode","acceptEdits"]。 */
  args?: string[];
}

/**
 * claude-code：headless 调用。同一个 thread 复用同一个会话 —— session id 落盘，
 * 后续同 thread 的唤起走 `--resume`，runtime 不必每次从零读上下文。
 */
export class ClaudeCodeAdapter extends GenericShellAdapter {
  #journal: Journal;
  #sessions: Map<string, string>;

  constructor(m: ClaudeCodeManifest, journal: Journal, hub?: HubHint) {
    super({ ...m, command: [m.bin ?? 'claude'] } as GenericShellManifest, 'claude-code', hub);
    this.#journal = journal;
    this.#sessions = priorSession(journal);
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'claude-code', resumesSession: true };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const bin = this.m.command[0];
    const prior = p.threadId ? this.#sessions.get(p.threadId) : undefined;
    const argv = [bin, '-p', '--output-format', 'json', ...(this.m as ClaudeCodeManifest).args ?? []];
    if (prior) argv.push('--resume', prior);
    // 走 runRaw 而不是 run：session id 要从**完整**的 stdout 里找。
    // 从截断过的 detail 里找，长输出时会静默地找不到 —— 功能还在，只是不再续接会话。
    const r = await this.runRaw(argv, wakePrompt(p, this.hub));
    const outcome = this.judge(r);
    if (outcome.ok && p.threadId) {
      const sid = extractSessionId(r.out);
      if (sid) rememberSession(this.#journal, this.#sessions, p.threadId, sid);
    }
    return outcome;
  }

  /**
   * 退出码在这里远远不够。`claude -p` 跑到 max turns、执行中出错、甚至根本没跑起来，
   * 都可能以 exit 0 收场 —— 只看退出码的话，「什么都没干」和「干完了」一模一样：
   * 队列行删掉、cursor 推进、不进死信，而成功路径不打日志。那条 @ 就这么没了，
   * 而且**任何一个地方都查不出异常**。唤起提示词里写给 runtime 的那句
   * 「你不说话，对面看到的只是『在线但没反应』」，说的其实也是 connector 自己。
   *
   * 所以这里读 `--output-format json` 的结果信封：
   *
   *   { "type": "result", "subtype": "success" | "error_max_turns" | "error_during_execution",
   *     "is_error": false, "result": "…", "session_id": "…" }
   *
   * **有一类这里仍然测不出来，得说清楚**：模型被权限拦住之后自己决定「那我不做了」，
   * 信封里是 is_error:false / subtype:success —— 对它来说那就是一次正常回答，
   * 我们没有立场判它失败。那一类只能从源头避免：headless 调用必须给权限模式
   * （onboard.sh 默认带 `--permission-mode acceptEdits`），否则每次唤起都会撞在确认上。
   */
  protected judge(r: RunResult): Outcome {
    if (r.spawnFailed || r.code !== 0) return super.judge(r);

    const env = resultEnvelope(r.out);
    if (!env) {
      // 要了 JSON 却没拿到信封：这次唤起根本没走到出结果那一步（进程被杀、
      // 参数不对、装的不是我们以为的那个 claude）。当成功处理等于把事件丢掉。
      const clue = (r.out || r.err).trim().slice(0, 500) || '(什么都没输出)';
      return { ok: false, retryable: true, detail: `exit 0 但没有 --output-format json 的结果信封：${clue}` };
    }
    if (env.is_error === true || (env.subtype && env.subtype !== 'success')) {
      const why = typeof env.result === 'string' ? env.result.slice(0, 500) : JSON.stringify(env).slice(0, 500);
      return { ok: false, retryable: true, detail: `runtime 报告失败 subtype=${env.subtype ?? '(无)'}: ${why}` };
    }
    return { ok: true, detail: r.out.slice(0, 2000) };
  }
}

interface ResultEnvelope {
  type?: string;
  subtype?: string;
  is_error?: boolean;
  result?: unknown;
  session_id?: string;
  sessionId?: string;
}

/**
 * 从 stdout 里取那份结果信封。
 *
 * 整段先试一次（正常情况就是一个 JSON 对象），不行再逐行找 type=="result" 的那行 ——
 * 有些版本会在结果前面吐别的行，写死「整段就是 JSON」会让这些版本一路判失败。
 */
export function resultEnvelope(out: string): ResultEnvelope | undefined {
  const whole = tryParse(out);
  if (whole) return whole;
  for (const line of out.split('\n')) {
    const t = line.trim();
    if (!t.startsWith('{')) continue;
    const j = tryParse(t);
    if (j && (j.type === 'result' || j.is_error !== undefined || j.subtype !== undefined)) return j;
  }
  return undefined;
}

function tryParse(s: string): ResultEnvelope | undefined {
  try {
    const j = JSON.parse(s.trim());
    return j && typeof j === 'object' ? (j as ResultEnvelope) : undefined;
  } catch { return undefined; }
}

function extractSessionId(out: string): string | undefined {
  const env = resultEnvelope(out);
  return env?.session_id ?? env?.sessionId;
}
