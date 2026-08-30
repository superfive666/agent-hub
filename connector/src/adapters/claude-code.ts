import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
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
    super({ command: [m.bin ?? 'claude'], ...m } as GenericShellManifest, 'claude-code', hub);
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
    const outcome = await this.run(argv, wakePrompt(p, this.hub));
    if (outcome.ok && p.threadId) {
      const sid = extractSessionId(outcome.detail ?? '');
      if (sid) rememberSession(this.#journal, this.#sessions, p.threadId, sid);
    }
    return outcome;
  }
}

function extractSessionId(out: string): string | undefined {
  try {
    const j = JSON.parse(out);
    return j.session_id ?? j.sessionId;
  } catch { return undefined; }
}
