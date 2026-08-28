import { AdapterConfig } from '../core/config.js';
import { Journal } from '../core/journal.js';
import { RuntimeAdapter } from '../core/types.js';
import { GenericShellAdapter, GenericShellManifest } from './generic-shell.js';
import { ClaudeCodeAdapter, ClaudeCodeManifest } from './claude-code.js';
import { HttpEndpointAdapter, HttpEndpointManifest } from './http-endpoint.js';

export type AdapterFactory = (m: AdapterConfig, journal: Journal) => RuntimeAdapter;

/**
 * 适配器注册表。新增一个 runtime = 加一份清单（命令模板、环境要求、并发上限、超时），
 * 走 generic-shell 就够，不用改 Core、不用 fork。
 */
export const builtinAdapters: Record<string, AdapterFactory> = {
  'generic-shell': (m) => new GenericShellAdapter(m as unknown as GenericShellManifest),
  'claude-code': (m, j) => new ClaudeCodeAdapter(m as unknown as ClaudeCodeManifest, j),
  'http-endpoint': (m) => new HttpEndpointAdapter(m as unknown as HttpEndpointManifest),
  // codex-cli 的具体子命令待核实，先按 generic-shell 清单走，行为完全一致。
  'codex-cli': (m) => new GenericShellAdapter(m as unknown as GenericShellManifest, 'codex-cli'),
};

export function createAdapter(cfg: AdapterConfig, journal: Journal): RuntimeAdapter {
  const f = builtinAdapters[cfg.type];
  if (!f) {
    throw new Error(
      `未知 runtime "${cfg.type}"。内置：${Object.keys(builtinAdapters).join(', ')}；` +
      `任何 runtime 都可以用 generic-shell 接进来。`);
  }
  return f(cfg, journal);
}
