import { openSync, closeSync, writeSync, readFileSync, unlinkSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';

export class DuplicateInstanceError extends Error {}

/** 本进程已持有的锁路径。同一个进程里起第二个实例同样是重复实例。 */
const heldInProcess = new Set<string>();

/**
 * 单实例锁。ADR-0005：同一 agent 身份只允许一条挂起的长轮询——
 * 两个实例共用一个 cursor 会互相吞事件且没有任何报错，所以启动时必须显式拒绝。
 * 锁里记 pid，进程被 SIGKILL 留下的陈旧锁会被自动接管。
 */
export class InstanceLock {
  #path: string;
  #fd: number | null = null;
  constructor(dir: string, identity: string) {
    mkdirSync(dir, { recursive: true, mode: 0o700 });
    this.#path = join(dir, `${identity.replace(/[^\w.-]/g, '_')}.lock`);
  }
  get path() { return this.#path; }

  acquire(): void {
    if (heldInProcess.has(this.#path)) {
      throw new DuplicateInstanceError(`同一 agent 身份在本进程内已有实例（锁 ${this.#path}）`);
    }
    if (existsSync(this.#path)) {
      let holder: { pid?: number; startedAt?: string } = {};
      try { holder = JSON.parse(readFileSync(this.#path, 'utf8')); } catch { /* 锁文件损坏，按陈旧处理 */ }
      if (holder.pid && holder.pid !== process.pid && alive(holder.pid)) {
        throw new DuplicateInstanceError(
          `同一 agent 身份已有实例在跑（pid ${holder.pid}，锁 ${this.#path}）。` +
          `两个 connector 共用一个 cursor 会互相吞事件，拒绝启动。`);
      }
      try { unlinkSync(this.#path); } catch { /* 竞态：别人刚清掉 */ }
    }
    // O_EXCL：两个实例同时起来时只有一个能建成功。
    this.#fd = openSync(this.#path, 'wx', 0o600);
    writeSync(this.#fd, JSON.stringify({ pid: process.pid, startedAt: new Date().toISOString() }));
    heldInProcess.add(this.#path);
  }

  release(): void {
    heldInProcess.delete(this.#path);
    if (this.#fd !== null) { try { closeSync(this.#fd); } catch { /* 已关闭 */ } this.#fd = null; }
    try { unlinkSync(this.#path); } catch { /* 已经没了 */ }
  }
}

function alive(pid: number): boolean {
  try { process.kill(pid, 0); return true; } catch (e: any) { return e?.code === 'EPERM'; }
}
