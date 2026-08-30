import { mkdirSync, appendFileSync, readFileSync, existsSync, writeFileSync, renameSync } from 'node:fs';
import { join } from 'node:path';
import { createRequire } from 'node:module';

const require_ = createRequire(import.meta.url);

/**
 * 持久化底座。只做「一堆带 id 的行 + 一张 meta 表」，查询逻辑在 QueueStore 里，
 * 两种实现才不会各写一遍 SQL / 各错一遍。
 */
export interface Journal {
  loadRows(): Record<string, unknown>[];
  loadMeta(): Record<string, string>;
  putRow(row: { id: number } & Record<string, unknown>): void;
  delRow(id: number): void;
  setMeta(k: string, v: string): void;
  close(): void;
  readonly driver: 'sqlite' | 'jsonl';
}

class SqliteJournal implements Journal {
  readonly driver = 'sqlite' as const;
  #db: any;
  #closed = false;
  constructor(file: string, DatabaseSync: any) {
    this.#db = new DatabaseSync(file);
    this.#db.exec(`
      PRAGMA journal_mode = WAL;
      CREATE TABLE IF NOT EXISTS rows_ (id INTEGER PRIMARY KEY, body TEXT NOT NULL);
      CREATE TABLE IF NOT EXISTS meta_ (k TEXT PRIMARY KEY, v TEXT NOT NULL);
    `);
  }
  loadRows() {
    return this.#db.prepare('SELECT body FROM rows_ ORDER BY id').all().map((r: any) => JSON.parse(r.body));
  }
  loadMeta() {
    const out: Record<string, string> = {};
    for (const r of this.#db.prepare('SELECT k, v FROM meta_').all()) out[(r as any).k] = (r as any).v;
    return out;
  }
  putRow(row: { id: number }) {
    if (this.#closed) return;
    this.#db.prepare('INSERT INTO rows_(id, body) VALUES(?, ?) ON CONFLICT(id) DO UPDATE SET body=excluded.body')
      .run(row.id, JSON.stringify(row));
  }
  delRow(id: number) { if (this.#closed) return; this.#db.prepare('DELETE FROM rows_ WHERE id=?').run(id); }
  setMeta(k: string, v: string) {
    if (this.#closed) return;
    this.#db.prepare('INSERT INTO meta_(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v').run(k, v);
  }
  close() { this.#closed = true; try { this.#db.close(); } catch { /* 已关闭 */ } }
}

/** node:sqlite 不可用时的兜底：追加写 JSONL，启动时重放 + 压缩。语义与 sqlite 版完全一致。 */
class JsonlJournal implements Journal {
  readonly driver = 'jsonl' as const;
  #file: string;
  #rows = new Map<number, Record<string, unknown>>();
  #meta: Record<string, string> = {};
  #closed = false;
  constructor(file: string) {
    this.#file = file;
    if (existsSync(file)) {
      for (const line of readFileSync(file, 'utf8').split('\n')) {
        if (!line.trim()) continue;
        let rec: any;
        try { rec = JSON.parse(line); } catch { continue; } // 崩溃时可能留下半行，丢掉它
        if (rec.op === 'put') this.#rows.set(rec.row.id, rec.row);
        else if (rec.op === 'del') this.#rows.delete(rec.id);
        else if (rec.op === 'meta') this.#meta[rec.k] = rec.v;
      }
    }
    this.#compact();
  }
  #compact() {
    const tmp = this.#file + '.tmp';
    const lines: string[] = [];
    for (const [k, v] of Object.entries(this.#meta)) lines.push(JSON.stringify({ op: 'meta', k, v }));
    for (const row of this.#rows.values()) lines.push(JSON.stringify({ op: 'put', row }));
    writeFileSync(tmp, lines.length ? lines.join('\n') + '\n' : '', { mode: 0o600 });
    renameSync(tmp, this.#file);
  }
  loadRows() { return [...this.#rows.values()]; }
  loadMeta() { return { ...this.#meta }; }
  putRow(row: { id: number }) { if (this.#closed) return; appendFileSync(this.#file, JSON.stringify({ op: 'put', row }) + '\n'); }
  delRow(id: number) { if (this.#closed) return; appendFileSync(this.#file, JSON.stringify({ op: 'del', id }) + '\n'); }
  setMeta(k: string, v: string) {
    if (this.#closed) return;
    // **内存里那份也要跟着改。** 只追加文件的话，同一个进程内 loadMeta() 读不到刚写的值，
    // 要等下次开进程重放才出现 —— 与 sqlite 版的语义就不一致了，而这个类的存在前提
    // 正是「语义与 sqlite 版完全一致」。
    this.#meta[k] = v;
    appendFileSync(this.#file, JSON.stringify({ op: 'meta', k, v }) + '\n');
  }
  close() { this.#closed = true; }
}

export function openJournal(dir: string, driver: 'auto' | 'sqlite' | 'jsonl'): Journal {
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  if (driver !== 'jsonl') {
    try {
      // node:sqlite 是 Node 22 内置模块，用它就不需要 better-sqlite3 那种原生编译依赖。
      const mod = require_('node:sqlite');
      return new SqliteJournal(join(dir, 'connector.db'), mod.DatabaseSync);
    } catch (e) {
      if (driver === 'sqlite') throw e;
      // auto：这个 Node 上没有 node:sqlite，退到 JSONL。
    }
  }
  return new JsonlJournal(join(dir, 'connector.jsonl'));
}
