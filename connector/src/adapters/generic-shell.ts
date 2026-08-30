import { spawn } from 'node:child_process';
import { accessSync, constants, statSync } from 'node:fs';
import { delimiter, isAbsolute, join } from 'node:path';
import { HubHint, RuntimeAdapter, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';

/** 子进程跑完之后的原始结果。要自己判定成败、或者要解析完整输出的适配器用它。 */
export interface RunResult {
  /** 被信号杀掉时是 null。 */
  code: number | null;
  out: string;
  err: string;
  /** 压根没起来（execvp 失败）。这类重试没有意义，配置不改它永远起不来。 */
  spawnFailed?: boolean;
}

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
    assertExecutable(this.m.command[0], this.env());
  }

  /** 子进程实际会用的环境。start() 的探测和 run() 的 spawn 必须用同一份，否则探测没意义。 */
  protected env(): NodeJS.ProcessEnv {
    return { ...process.env, ...this.m.env };
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
    return this.runRaw(argv, stdin).then((r) => this.judge(r));
  }

  /**
   * 退出码 → 成败。**留成 protected 是给子类覆盖的。**
   *
   * 退出码只能证明「进程正常结束」，证明不了「它真的干了活」。一个被 headless 叫起来的
   * runtime 撞上权限确认、或者想了想决定不做，照样 exit 0 —— 在这里「什么都没干」和
   * 「干完了」长得一模一样：队列行删掉、cursor 推进、不进死信，而成功路径不打日志，
   * 于是那条 @ 石沉大海，四处都查不出异常。
   *
   * 所以**凡是能结构化输出的 runtime，都应该覆盖这个方法去读它自己的输出**，
   * 而不是接受退出码这个最弱的证据（claude-code 就是这么做的）。
   * generic-shell 只能停在这里 —— 一条任意的 shell 命令没有可依赖的成功语义。
   */
  protected judge(r: RunResult): Outcome {
    if (r.spawnFailed) return { ok: false, detail: r.err, retryable: false };
    if (r.code === 0) {
      // exit 0 但一路往 stderr 上抱怨的命令，是排查时最需要看见的那种。
      const detail = r.err ? `${r.out}\n[stderr] ${r.err}` : r.out;
      return { ok: true, detail: detail.slice(0, 2000) };
    }
    return { ok: false, detail: `exit ${r.code}: ${(r.err || r.out).slice(0, 2000)}` };
  }

  /**
   * 跑一次子进程，把原始输出原样交出来。
   *
   * **不截断**：session id 这类东西可能出现在很后面，而 Outcome.detail 只留 2000 字。
   * 从截断过的 detail 里找 session id，长输出时会静默地找不到 —— 表现是「会话不接上了」，
   * 功能还在，没有任何报错。
   */
  protected runRaw(argv: string[], stdin: string): Promise<RunResult> {
    return new Promise((resolve) => {
      const child = spawn(argv[0], argv.slice(1), {
        cwd: this.m.cwd, env: this.env(), stdio: ['pipe', 'pipe', 'pipe'],
      });
      let out = '', err = '', settled = false;
      const timer = this.m.timeoutSeconds
        ? setTimeout(() => { child.kill('SIGKILL'); }, this.m.timeoutSeconds * 1000)
        : null;
      const done = (r: RunResult) => { if (settled) return; settled = true; if (timer) clearTimeout(timer); resolve(r); };
      child.stdout.on('data', (d) => { out += d; });
      child.stderr.on('data', (d) => { err += d; });
      child.on('error', (e) => done({ code: null, out, err: String(e), spawnFailed: true }));
      child.on('close', (code) => done({ code, out, err }));
      child.stdin.on('error', () => undefined);
      child.stdin.end(stdin);
    });
  }
}

/**
 * 启动时就确认那个命令**真的能执行**，而不是等第一个事件来了才发现。
 *
 * 这一条是踩出来的。systemd user service 的 PATH 和你交互 shell 的 PATH **不是一回事**
 * （通常只有 /usr/local/bin:/usr/bin:/bin，没有 ~/.local/bin，也没有 nvm 那一套），
 * 于是 `claude` 在终端里跑得好好的，服务里却找不到。
 *
 * 而它失败的样子毫无线索：execvp 找到一个同名但没有合法 shebang 的文件时，
 * 会**退回用 /bin/sh 去跑它**，于是参数被 dash 当成自己的选项，
 * 报出来的是 `sh: 0: Illegal option --` —— 里面既没有 claude 也没有 PATH，
 * 没有任何东西指向真正的原因。
 *
 * 所以宁可在启动时炸掉：install.sh 的连通性检查会拦住它，人当场就知道要改什么。
 */
export function assertExecutable(bin: string, env: NodeJS.ProcessEnv): void {
  const path = env.PATH ?? '';
  const candidates = bin.includes('/')
    ? [isAbsolute(bin) ? bin : join(process.cwd(), bin)]
    : path.split(delimiter).filter(Boolean).map((d) => join(d, bin));

  for (const c of candidates) {
    try {
      if (!statSync(c).isFile()) continue;
      accessSync(c, constants.X_OK);
      return;
    } catch { /* 下一个 */ }
  }
  throw new Error(
    `找不到可执行的 \`${bin}\`。\n` +
    `当前 PATH：${path || '(空)'}\n` +
    `装成 systemd user service 时最常见的原因就是这个 —— 服务的 PATH 比你交互 shell 的窄。\n` +
    `要么在 adapter.bin 里写绝对路径（\`command -v ${bin}\` 查出来的那个），\n` +
    `要么在 unit 里加一行 Environment=PATH=…。`);
}
