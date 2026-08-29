import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { Journal } from '../core/journal.js';

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
  #sessions = new Map<string, string>();

  constructor(m: ClaudeCodeManifest, journal: Journal) {
    super({ command: [m.bin ?? 'claude'], ...m } as GenericShellManifest, 'claude-code');
    this.#journal = journal;
    const meta = journal.loadMeta();
    for (const [k, v] of Object.entries(meta)) if (k.startsWith('session:')) this.#sessions.set(k.slice(8), v);
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'claude-code', resumesSession: true };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const bin = this.m.command[0];
    const prior = p.threadId ? this.#sessions.get(p.threadId) : undefined;
    const argv = [bin, '-p', '--output-format', 'json', ...(this.m as ClaudeCodeManifest).args ?? []];
    if (prior) argv.push('--resume', prior);
    const prompt =
      `agent-hub 事件：${p.kind}（优先级 P${p.priority}${p.coalescedCount > 1 ? `，合并了 ${p.coalescedCount} 条` : ''}）\n` +
      `thread: ${p.threadId ?? '(无)'}  seq: ${p.seqs.join(',')}\n` +
      `去 hub 拉取 thread 全文后判断是否需要回复。原始事件：\n${JSON.stringify(p.event)}`;
    const outcome = await this.run(argv, prompt);
    if (outcome.ok && p.threadId) {
      const sid = extractSessionId(outcome.detail ?? '');
      if (sid) {
        this.#sessions.set(p.threadId, sid);
        this.#journal.setMeta(`session:${p.threadId}`, sid);
      }
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
