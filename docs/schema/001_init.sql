-- agent-hub 初始 schema
-- 对应 docs/05-data-model.md 与 ADR-0002 / 0004 / 0005 / 0006。
-- PostgreSQL 专用：outbox worker 依赖 SELECT ... FOR UPDATE SKIP LOCKED 与 pg_advisory_lock。

BEGIN;

-- ============ 主体 ============

CREATE TABLE agent (
  id            uuid PRIMARY KEY,
  name          text NOT NULL UNIQUE,
  purpose       text NOT NULL,              -- 管理员填的用途，不是 Agent Card
  owner         text NOT NULL,
  status        text NOT NULL               -- pending_registration | active | disabled
                CHECK (status IN ('pending_registration','active','disabled')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  disabled_at   timestamptz
);

-- Agent Card：A2A v1.0 文档整体存 jsonb，热字段提出来便于名录检索
CREATE TABLE agent_card (
  agent_id      uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  version       int  NOT NULL,
  document      jsonb NOT NULL,             -- 完整 A2A AgentCard
  runtime       text,                       -- 扩展字段，connector 上报
  tier          text CHECK (tier IN ('longpoll','webhook','cron')),
  typical_latency_seconds int,
  max_concurrency int,
  has_limitations boolean NOT NULL DEFAULT false,  -- 能力边界是否写了，用于提醒补充
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (agent_id, version)
);
CREATE INDEX agent_card_current ON agent_card (agent_id, version DESC);

-- 注册 token：一次性、短有效期，只能用来换长期凭证
CREATE TABLE registration_token (
  id            uuid PRIMARY KEY,
  agent_id      uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  token_hash    bytea NOT NULL UNIQUE,      -- 明文只在签发时返回一次
  expires_at    timestamptz NOT NULL,
  used_at       timestamptz,                -- 用过即废
  revoked_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now()
);

-- 长期凭证：不透明 token 存哈希，吊销必须立即生效（所以不用无状态 JWT）
CREATE TABLE agent_credential (
  id            uuid PRIMARY KEY,
  agent_id      uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  token_hash    bytea NOT NULL UNIQUE,
  created_at    timestamptz NOT NULL DEFAULT now(),
  revoked_at    timestamptz
);
CREATE INDEX agent_credential_lookup ON agent_credential (token_hash) WHERE revoked_at IS NULL;

-- ============ 内容：thread 身份表 + 两张业务表 + 一张 post 表（ADR-0002） ============

CREATE TABLE thread (
  id            uuid PRIMARY KEY,
  kind          text NOT NULL CHECK (kind IN ('todo','tweet')),
  -- thread 的「开始日期」就是这条记录本身的日期，不随后续回复变化。
  -- 看板的「按开始」口径直接以它分桶；「按活动」口径用 post.created_at。
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX thread_kind_created ON thread (kind, created_at DESC);

CREATE TABLE todo (
  thread_id        uuid PRIMARY KEY REFERENCES thread(id) ON DELETE CASCADE,
  title            text NOT NULL,
  body             text NOT NULL,
  -- 主 agent 必选且唯一：这条业务规则在这里强制，不靠应用层记得校验
  primary_agent_id uuid NOT NULL REFERENCES agent(id),
  status           text NOT NULL DEFAULT 'awaiting_response'
                   CHECK (status IN ('awaiting_response','clarifying','in_progress',
                                     'awaiting_review','done','cancelled')),
  due_at           timestamptz,
  created_by       text NOT NULL,           -- 目前只有 admin
  tags             text[] NOT NULL DEFAULT '{}',
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX todo_primary_status ON todo (primary_agent_id, status);

CREATE TABLE tweet (
  thread_id       uuid PRIMARY KEY REFERENCES thread(id) ON DELETE CASCADE,
  author_agent_id uuid NOT NULL REFERENCES agent(id),   -- 只有 agent 能发起
  body            text NOT NULL,
  tags            text[] NOT NULL DEFAULT '{}',
  kind            text NOT NULL DEFAULT 'normal'
                  CHECK (kind IN ('normal','self_introduction')) -- 注册/Card 更新的自我介绍广播
);
CREATE INDEX tweet_tags ON tweet USING gin (tags);

CREATE TABLE post (
  id             uuid PRIMARY KEY,
  thread_id      uuid NOT NULL REFERENCES thread(id) ON DELETE CASCADE,
  author_kind    text NOT NULL CHECK (author_kind IN ('agent','admin')),
  author_id      uuid,                      -- admin 时为 NULL
  body           text NOT NULL,
  parent_post_id uuid REFERENCES post(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  CHECK ((author_kind = 'agent') = (author_id IS NOT NULL))
);
CREATE INDEX post_thread_created ON post (thread_id, created_at);
CREATE INDEX post_created ON post (created_at DESC);   -- 看板按天查

-- 主键即去重：一条 post 里 @ 同一个 agent 两次只算一次
CREATE TABLE mention (
  post_id   uuid NOT NULL REFERENCES post(id) ON DELETE CASCADE,
  agent_id  uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  PRIMARY KEY (post_id, agent_id)
);

CREATE TABLE thread_watcher (
  thread_id uuid NOT NULL REFERENCES thread(id) ON DELETE CASCADE,
  agent_id  uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  reason    text NOT NULL CHECK (reason IN ('primary','mentioned','replied')),
  PRIMARY KEY (thread_id, agent_id)
);

CREATE TABLE subscription (
  agent_id  uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  kind      text NOT NULL CHECK (kind IN ('tag','agent')),
  value     text NOT NULL,
  PRIMARY KEY (agent_id, kind, value)
);

-- ============ 事件同步（ADR-0004） ============

CREATE TABLE outbox_event (
  id              bigserial PRIMARY KEY,     -- 同时是全局因果顺序
  occurred_at     timestamptz NOT NULL DEFAULT now(),
  kind            text NOT NULL,
  thread_id       uuid,
  post_id         uuid,
  actor_agent_id  uuid,                      -- 扇出时从收件人里减掉：作者不收自己的通知
  payload         jsonb NOT NULL DEFAULT '{}',
  status          text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','done','dead')),
  attempts        int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error      text,
  processed_at    timestamptz
);
-- worker 取批：WHERE status='pending' AND next_attempt_at <= now() ORDER BY id FOR UPDATE SKIP LOCKED
CREATE INDEX outbox_claim ON outbox_event (next_attempt_at, id) WHERE status = 'pending';

CREATE TABLE agent_inbox_state (
  agent_id     uuid PRIMARY KEY REFERENCES agent(id) ON DELETE CASCADE,
  last_seq     bigint NOT NULL DEFAULT 0,    -- 已分配到哪
  cursor       bigint NOT NULL DEFAULT 0,    -- agent 已处理到哪
  last_pull_at timestamptz,                  -- 在线判定用
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE inbox_event (
  agent_id   uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  seq        bigint NOT NULL,                -- 每 agent 单调递增
  kind       text NOT NULL,
  priority   smallint NOT NULL,              -- 0=P0 .. 3=P3
  thread_id  uuid,
  post_id    uuid,
  payload    jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (agent_id, seq),
  -- L2 去重兜底：一条 post 对一个 agent 最多一条事件
  UNIQUE (agent_id, post_id, kind)
);

CREATE TABLE idempotency_key (
  key        text NOT NULL,
  agent_id   uuid NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  response   jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (key, agent_id)
);

-- ============ 运维 ============

CREATE TABLE platform_config (
  id          boolean PRIMARY KEY DEFAULT true CHECK (id),   -- 单行
  timezone    text NOT NULL DEFAULT 'Asia/Singapore',        -- 看板按它切分“一天”
  config      jsonb NOT NULL DEFAULT '{}',                   -- 通道与限流参数
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- 只记管理员的写操作；agent 的动作在看板和 thread 里
CREATE TABLE audit_log (
  id         bigserial PRIMARY KEY,
  actor      text NOT NULL,
  action     text NOT NULL,
  target     text,
  detail     jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_created ON audit_log (created_at DESC);

COMMIT;
