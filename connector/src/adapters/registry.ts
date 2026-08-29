import { AdapterConfig } from '../core/config.js';
import { Journal } from '../core/journal.js';
import { RuntimeAdapter } from '../core/types.js';
import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { ClaudeCodeAdapter, ClaudeCodeManifest } from './claude-code.js';
import { HttpEndpointAdapter, HttpEndpointManifest } from './http-endpoint.js';
import { CodexAdapter, CodexManifest } from './codex.js';
import { OpencodeAdapter, OpencodeManifest } from './opencode.js';
import { OpenclawAdapter, OpenclawManifest } from './openclaw.js';

export type AdapterFactory = (m: AdapterConfig, journal: Journal) => RuntimeAdapter;

/**
 * 适配器注册表。新增一个 runtime = 加一份清单（命令模板、环境要求、并发上限、超时），
 * 走 generic-shell 就够，不用改 Core、不用 fork。
 *
 * 分两类，选哪一类取决于 runtime 本身的形态，不是偏好：
 *
 * - **拉起子进程**（claude-code / codex / opencode / openclaw）：runtime 是个 CLI，
 *   每次唤起执行一次。前三个支持按 thread 续接会话。
 * - **推给常驻服务**（http-endpoint 及其两个预设 hermes / openhuman）：runtime 本身
 *   是长期跑着的、带自己的消息通道。这时拉子进程既慢又会丢掉它自己的会话状态。
 */
export const builtinAdapters: Record<string, AdapterFactory> = {
  'generic-shell': (m) => new GenericShellAdapter(m as unknown as GenericShellManifest),
  'http-endpoint': (m) => new HttpEndpointAdapter(m as unknown as HttpEndpointManifest),

  'claude-code': (m, j) => new ClaudeCodeAdapter(m as unknown as ClaudeCodeManifest, j),
  'codex': (m, j) => new CodexAdapter(m as unknown as CodexManifest, j),
  'opencode': (m, j) => new OpencodeAdapter(m as unknown as OpencodeManifest, j),
  'openclaw': (m) => new OpenclawAdapter(m as unknown as OpenclawManifest),

  // 老名字，别断掉已经写好的配置。
  'codex-cli': (m, j) => new CodexAdapter(m as unknown as CodexManifest, j),

  /**
   * hermes：Nous Research 的 hermes-agent。它是常驻的 messaging gateway，
   * 通道里包含 Webhook —— 那就是我们的入口。先 `hermes gateway setup` 配好
   * Webhook 通道拿到 URL，填进 adapter.url。
   *
   * 不走 CLI 是有原因的：`hermes` 本体是交互式 TUI，没有文档化的一次性执行参数，
   * 硬拿 shell 拉起来等于每次都开一个交互进程，还丢掉它引以为卖点的持久记忆。
   */
  'hermes': (m) => new HttpEndpointAdapter({
    messageField: 'text',
    runtimeName: 'hermes',
    resumesSession: true,   // 会话状态在 hermes 自己那边，跨唤起是连着的
    typicalLatencySeconds: 60,
    ...(m as unknown as HttpEndpointManifest),
  }),

  /**
   * openhuman：tinyhumans 的本地优先 agent 平台。它是 GUI 优先的桌面程序，
   * 没有文档化的 CLI，但工作流可以由 webhook 触发 —— 在 openhuman 里建一个
   * webhook 触发的工作流，把它的 URL 填进 adapter.url。
   */
  'openhuman': (m) => new HttpEndpointAdapter({
    messageField: 'message',
    runtimeName: 'openhuman',
    resumesSession: true,
    typicalLatencySeconds: 90,
    ...(m as unknown as HttpEndpointManifest),
  }),
};

export function createAdapter(cfg: AdapterConfig, journal: Journal): RuntimeAdapter {
  const f = builtinAdapters[cfg.type];
  if (!f) {
    throw new Error(
      `未知 runtime "${cfg.type}"。内置：${Object.keys(builtinAdapters).join(', ')}；` +
      `任何 runtime 都可以用 generic-shell（命令行）或 http-endpoint（常驻服务）接进来。`);
  }
  return f(cfg, journal);
}
