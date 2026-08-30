import { spawn } from 'node:child_process';
import { HubHint, RuntimeAdapter, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';

export interface GenericShellManifest {
  /** 命令模板，argv 数组。支持 {{kind}} {{threadId}} {{seq}} {{coalescedCount}} 占位符。 */
  command: string[];
  cwd?: string;
  env?: Record<string, string>;
  timeoutSeconds?: number;
  maxConcurrency?: number;
  typicalLatencySeconds?: number;
  /** 启动时校验这些环境变量存在。 */
  requiresEnv?: string[];
}

/**
 * 兜底适配器：用户给一条命令模板，事件 JSON 走 stdin。
 * 有它就不存在「不支持的 runtime」—— 能写 shell 就能接进来。
 */
export class GenericShellAdapter implements RuntimeAdapter {
  constructor(
    protected m: GenericShellManifest,
    protected name = 'generic-shell',
    protected hub?: HubHint,
  ) {
    if (!m.command?.length) throw new Error('generic-shell 需要 command 模板');
  }

  async start(): Promise<void> {
    for (const k of this.m.requiresEnv ?? []) {
      if (!process.env[k]) throw new Error(`runtime 需要环境变量 ${k}，未设置`);
    }
  }
  async stop(): Promise<void> {}

  capabilities(): RuntimeCapabilities {
    return {
      runtime: this.name,
      resumesSession: false,
      typicalLatencySeconds: this.m.typicalLatencySeconds ?? 60,
      maxConcurrency: this.m.maxConcurrency ?? 1,
    };
  }

  protected argv(p: WakePayload): string[] {
    const vars: Record<string, string> = {
      kind: p.kind, threadId: p.threadId ?? '', seq: String(p.seq),
      coalescedCount: String(p.coalescedCount), priority: String(p.priority),
    };
    return this.m.command.map((a) => a.replace(/\{\{(\w+)\}\}/g, (_, k) => vars[k] ?? ''));
  }

  async wake(p: WakePayload): Promise<Outcome> {
    // stdin 里带上 hub 地址与凭证**路径**（不是凭证本身）。
    // 写脚本的人不该被迫在两个地方各配一遍 hub 地址 —— 而且少了这一段，
    // 脚本拿到一条「你有新事件」却不知道去哪读、往哪回，只能干瞪眼。
    return this.run(this.argv(p), JSON.stringify(this.hub ? { ...p, hub: this.hub } : p));
  }

  protected run(argv: string[], stdin: string): Promise<Outcome> {
    return new Promise((resolve) => {
      const child = spawn(argv[0], argv.slice(1), {
        cwd: this.m.cwd, env: { ...process.env, ...this.m.env }, stdio: ['pipe', 'pipe', 'pipe'],
      });
      let out = '', err = '', settled = false;
      const timer = this.m.timeoutSeconds
        ? setTimeout(() => { child.kill('SIGKILL'); }, this.m.timeoutSeconds * 1000)
        : null;
      const done = (o: Outcome) => { if (settled) return; settled = true; if (timer) clearTimeout(timer); resolve(o); };
      child.stdout.on('data', (d) => { out += d; });
      child.stderr.on('data', (d) => { err += d; });
      child.on('error', (e) => done({ ok: false, detail: String(e), retryable: false }));
      child.on('close', (code) =>
        done(code === 0
          ? { ok: true, detail: out.slice(0, 2000) }
          : { ok: false, detail: `exit ${code}: ${(err || out).slice(0, 2000)}` }));
      child.stdin.on('error', () => undefined);
      child.stdin.end(stdin);
    });
  }
}
