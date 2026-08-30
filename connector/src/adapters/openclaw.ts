import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { HubHint, RuntimeCapabilities, Outcome, WakePayload } from '../core/types.js';
import { wakePrompt } from './prompt.js';

export interface OpenclawManifest extends Partial<GenericShellManifest> {
  /** 可执行文件，默认 `openclaw`。 */
  bin?: string;
  /**
   * 一次性投递消息的子命令，argv 片段，**必填**。
   *
   * 没有默认值是有意的：openclaw 的子命令随版本变化，写死一个猜来的默认
   * 会让 connector「跑起来了但每次唤起都失败」，而这种失败在队列里表现为
   * 一直重试到死信，很难第一时间联想到是命令写错了。宁可启动就报错。
   *
   * 用 `openclaw --help` 查你这个版本的实际子命令，例如：
   *   subcommand: ["message", "send"]
   * 提示词会作为最后一个位置参数追加上去。
   */
  subcommand?: string[];
  args?: string[];
}

/**
 * OpenClaw。它自带 Gateway（本地控制平面，管会话、工具、通道），
 * 所以**如果 Gateway 暴露了 HTTP 接口，用 `http-endpoint` 适配器通常比拉起子进程更合适** ——
 * 常驻进程不用每次冷启动，也能保住会话。
 *
 * 这个适配器是给「就想用命令行接进来」的场景兜底的。
 */
export class OpenclawAdapter extends GenericShellAdapter {
  constructor(m: OpenclawManifest, hub?: HubHint) {
    super({ command: [m.bin ?? 'openclaw'], ...m } as GenericShellManifest, 'openclaw', hub);
  }

  async start(): Promise<void> {
    await super.start();
    const m = this.m as OpenclawManifest;
    if (!m.subcommand?.length) {
      throw new Error(
        'openclaw 适配器需要显式配置 adapter.subcommand（argv 片段）。\n' +
        '本项目没有替你猜一个默认值：子命令随 openclaw 版本变化，猜错的表现是' +
        '每次唤起都失败、事件一路重试进死信，排查成本远高于现在直接报错。\n' +
        '用 `openclaw --help` 查你这个版本的一次性发消息命令，例如 subcommand: ["message","send"]。\n' +
        '另：openclaw 的 Gateway 若暴露了 HTTP 接口，改用 http-endpoint 适配器通常更划算。');
    }
  }

  capabilities(): RuntimeCapabilities {
    return { ...super.capabilities(), runtime: 'openclaw', resumesSession: false };
  }

  async wake(p: WakePayload): Promise<Outcome> {
    const m = this.m as OpenclawManifest;
    const argv = [this.m.command[0], ...(m.subcommand ?? []), ...(m.args ?? []), wakePrompt(p, this.hub)];
    return this.run(argv, '');
  }
}
