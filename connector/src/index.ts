#!/usr/bin/env node
import { loadConfig, readToken, Config } from './core/config.js';
import { openJournal } from './core/journal.js';
import { HubClient } from './core/hub.js';
import { Connector, consoleLogger } from './core/connector.js';
import { InstanceLock, DuplicateInstanceError } from './core/singleton.js';
import { createAdapter } from './adapters/registry.js';

function usage(): never {
  console.log(`用法:
  agent-hub-connector run     --config <file>   前台常驻（systemd 用这个）
  agent-hub-connector check   --config <file>   校验配置、凭证、hub 连通性与 runtime 可用性
  agent-hub-connector status  --config <file>   打印本地队列与 cursor
`);
  process.exit(1);
}

function parse(argv: string[]) {
  const cmd = argv[2];
  const i = argv.indexOf('--config');
  return { cmd, configPath: i > 0 ? argv[i + 1] : process.env.CONNECTOR_CONFIG };
}

function build(cfg: Config) {
  const journal = openJournal(cfg.storage.dir, cfg.storage.driver);
  const hub = new HubClient(cfg.hub.baseUrl, readToken(cfg), cfg.hub.requestTimeoutMs);
  // 唤起 runtime 时把 hub 地址与凭证**路径**一起递过去 —— 只有路径，没有凭证本身。
  // 少了这几行，被 headless 叫起来的 runtime 手上什么都没有，想回也回不了：
  // 它想了想就退出，退出码 0，connector 当成处理成功，那条 @ 石沉大海且无处可查。
  const adapter = createAdapter(cfg.adapter, journal, {
    baseUrl: cfg.hub.baseUrl,
    agentId: cfg.hub.agentId,
    tokenFile: cfg.hub.tokenFile,
    tokenEnv: cfg.hub.tokenEnv,
  });
  return { journal, hub, adapter };
}

async function main() {
  const { cmd, configPath } = parse(process.argv);
  if (!cmd || !['run', 'check', 'status'].includes(cmd)) usage();
  const cfg = loadConfig(configPath);

  if (cmd === 'status') {
    const { journal, hub, adapter } = build(cfg);
    const c = new Connector({ config: cfg, hub, adapter, journal, logger: consoleLogger });
    console.log(JSON.stringify({
      cursor: c.cursor, driver: journal.driver, tier: cfg.tier,
      pending: c.queue.pendingCount, dead: c.queue.deadLetters().length,
      capabilities: adapter.capabilities(),
    }, null, 2));
    journal.close();
    return;
  }

  // ADR-0005：一个身份一条连接。两个实例共用一个 cursor 会互相吞事件且不报错。
  const lock = new InstanceLock(cfg.storage.dir, cfg.hub.agentId ?? 'default');
  lock.acquire();
  // check 失败也要把锁还回去，否则下次启动会被自己的陈旧锁挡住
  process.on('exit', () => lock.release());

  const { journal, hub, adapter } = build(cfg);
  const connector = new Connector({ config: cfg, hub, adapter, journal, logger: consoleLogger });

  if (cmd === 'check') {
    await adapter.start();
    const page = await hub.fetchInbox(connector.cursor, 1, 0);
    console.log(`OK  hub=${cfg.hub.baseUrl} tier=${cfg.tier} lastSeq=${page.lastSeq} ` +
      `runtime=${adapter.capabilities().runtime} storage=${journal.driver}`);
    await adapter.stop();
    journal.close();
    lock.release();
    return;
  }

  const shutdown = async () => { await connector.stop(); lock.release(); process.exit(0); };
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
  consoleLogger.info('connector 启动', { tier: cfg.tier, runtime: adapter.capabilities().runtime, storage: journal.driver });
  await connector.start();
}

main().catch((e) => {
  if (e instanceof DuplicateInstanceError) { console.error(String(e.message)); process.exit(2); }
  console.error(e);
  process.exit(1);
});
