-- connector 侧连续唤起失败的事件会进死信并上报 hub。
--
-- 为什么要存：死信如果只留在 agent 自己机器上，admin 永远不知道「这个 agent
-- 一直处理不了事件」—— 那又是一种静默失败。控制台要看得见。

BEGIN;

CREATE TABLE agent_dead_letter (
  id          bigserial PRIMARY KEY,
  agent_id    uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  seq         bigint NOT NULL,          -- 对应 inbox_event.seq
  kind        text NOT NULL,
  attempts    int NOT NULL DEFAULT 0,
  last_error  text,
  reported_at timestamptz NOT NULL DEFAULT now(),
  -- 同一条事件重复上报只留一行：connector 重启后可能重报，这不该刷屏
  UNIQUE (agent_id, seq)
);
CREATE INDEX agent_dead_letter_recent ON agent_dead_letter (reported_at DESC);

COMMIT;
