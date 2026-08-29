-- Todo 的「用户确认闸门」与「任务处理详情步骤」。
--
-- 要解决的问题：agent 拿到一条 todo 就闷头开工，等交付出来才发现理解偏了。
-- 所以在「收到指派」和「开始执行」之间插一道人的确认动作：
-- 主 agent 先在 thread 里把疑问问清楚，管理员点确认，agent 才允许往下推进。
-- 闸门的判据放在数据上（confirmed_at 是否为空），不放在状态机里 ——
-- 状态会被反复推来推去，「有没有被人确认过」这件事只该发生一次且不可回退。
--
-- 兼容性：这份脚本要能在已有数据的库上跑。**现存 todo 的 confirmed_at 一律为 NULL，
-- 也就是全部变成「待确认」。这是刻意的**：这些 todo 确实没有经过任何人的确认动作，
-- 假装它们确认过（比如按 status 回填）等于把闸门第一天就开了个后门。
-- 代价是升级后管理员要把在办的 todo 逐条 approve 一次，一次性的，可接受。

BEGIN;

-- ============ 确认闸门 ============

ALTER TABLE todo ADD COLUMN confirmed_at timestamptz;
-- 谁确认的。目前只有一个管理员，存的是它的 subject（用户名或预置 Google 邮箱），
-- 不是 uuid —— 管理员本来就不在 agent 表里，留 text 也给「将来有第二个管理员」留了位置。
ALTER TABLE todo ADD COLUMN confirmed_by text;
-- 两个字段要么都空要么都有：只有 confirmed_at 而不知道是谁确认的，审计上等于没记。
ALTER TABLE todo ADD CONSTRAINT todo_confirmed_pair
  CHECK ((confirmed_at IS NULL) = (confirmed_by IS NULL));

-- 控制台的「待确认需求」列表按它筛，只索引没确认的那一小撮。
CREATE INDEX todo_unconfirmed ON todo (updated_at DESC) WHERE confirmed_at IS NULL;

-- ============ 任务处理详情步骤 ============

-- 为什么要单独一张表而不是塞进 post：
-- post 是「说了什么」，按时间线读；todo_step 是「做到哪一步了」，是可以被回填、
-- 被改状态（pending → done）的结构化记录。把它们混在一张表里，
-- 要么 post 长出一堆只有一半行会用的字段，要么「第 3 步现在是什么状态」
-- 得靠扫一遍正文猜出来。post_id 把两者关联起来：这一步对应 thread 里哪条发言。
CREATE TABLE todo_step (
  id             uuid PRIMARY KEY,
  thread_id      uuid NOT NULL REFERENCES todo(thread_id) ON DELETE CASCADE,
  -- 每条 todo 内单调递增。全局自增序列做不到这件事：步骤要按「这是第几步」展示，
  -- 而不是「这是全库第几条记录」。分配在事务里做，见 store.AppendTodoStep。
  seq            int  NOT NULL,
  kind           text NOT NULL
                 CHECK (kind IN ('clarification','plan','progress','blocked',
                                 'deliverable','confirmation')),
  title          text NOT NULL,
  detail         text NOT NULL DEFAULT '',
  -- 步骤默认就是「已经发生了」，所以 done 是默认值；pending / in_progress
  -- 留给主 agent 预先铺计划（一次写出 5 步，做完一步改一步）。
  status         text NOT NULL DEFAULT 'done'
                 CHECK (status IN ('pending','in_progress','done','blocked')),
  actor_kind     text NOT NULL CHECK (actor_kind IN ('agent','admin')),
  actor_agent_id uuid REFERENCES agent(id),
  post_id        uuid REFERENCES post(id) ON DELETE SET NULL,  -- 可选：这一步对应哪条发言
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  -- 同一条 todo 里不能有两个「第 3 步」。并发追加时这条约束是最后一道防线，
  -- 但正确性不该依赖它报错 —— 分配 seq 之前先锁住 todo 行，见 store.AppendTodoStep。
  -- 它背后那个 (thread_id, seq) 索引同时也是「按 seq 升序读一条 todo 的步骤」的索引，
  -- 所以不需要再单独建一个。
  UNIQUE (thread_id, seq),
  -- 和 post 表同款：agent 发的必须有 id，admin 发的必须没有。
  CHECK ((actor_kind = 'agent') = (actor_agent_id IS NOT NULL))
);

COMMIT;
