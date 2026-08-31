import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { HubHint, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { wakePrompt } from './prompt.js';

export interface DeepseekManifest extends Partial<GenericShellManifest> {
  /** 可执行文件，默认 `dsh`。用 npx 跑的话填 `npx` 并把包名放进 args。 */
  bin?: string;
  /**
   * `--profile`，默认 `headless`。
   *
   * headless 是唯一适合被 connector 唤起的档：它跑一次、打印结果、退出。
   * 其余 profile（tui / web）要么等人交互、要么起一个服务，
   * 被 headless 拉起来只会挂到超时。
   */
  profile?: string;
  args?: string[];
}

/**
 * deepseek-harness（`dsh`）。非交互入口是 headless profile：
 *
 *   dsh --profile headless "<prompt>"     # 跑一次，打印最终答案，退出
 *
 * **不接会话。** `--resume <id>` 这个参数确实存在，但官方 CLI 文档里
 * 只在 tui profile 的例子里出现过，headless 下的行为没有文档；
 * 而且 headless 也没有文档化的 JSON 输出，拿不到 session id。
 * 两头都缺的情况下猜一个出来，表现是「每次唤起都接错会话」——
 * 比不接会话糟得多。所以 `resumesSession: false`，
 * 让它每次从 hub 拉 thread 全文重建上下文（那本来就是唤起 prompt 教它做的事）。
 *
 * 核实状态见 RUNTIMES.md：命令名、`--profile headless`、prompt 走位置参数
 * 都来自官方仓库 `apps/cli` 的文档；`--resume` 的 headless 语义未核实。
 *
 * 多数部署要 `DEEPSEEK_API_KEY`。**没有写死进 requiresEnv**：
 * dsh 支持自定义 provider，那种部署下这个变量根本不存在，
 * 写死会让它启动就失败。要强制就在清单里自己填 requiresEnv。
 */
export class DeepseekAdapter extends GenericShellAdapter {
  constructor(m: DeepseekManifest, hub?: HubHint) {
    super({ ...m, command: [m.bin ?? 'dsh'] } as GenericShellManifest, 'deepseek', hub);
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'deepseek', resumesSession: false };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const m = this.m as DeepseekManifest;
    const argv = [this.m.command[0], '--profile', m.profile ?? 'headless'];
    argv.push(...(m.args ?? []));
    // prompt 走位置参数，和 codex / opencode 一样。
    // **必须放在最后**：dsh 的 launcher 把第一个非选项参数当任务描述，
    // 夹在选项中间会被后面的选项吃掉。
    argv.push(wakePrompt(p, this.hub));
    return this.judge(await this.runRaw(argv, ''));
  }
}
