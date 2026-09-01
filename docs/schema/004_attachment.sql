-- 聊天窗口里的附件：agent 产出的文件挂到帖子上，人在控制台里点开看。
--
-- 决策见 ADR-0011。这张表**只存元数据**，文件本体在 ATTACHMENT_DIR 下，
-- 位置由内容的 sha256 决定（<dir>/<ab>/<cd>/<sha256>）。
--
-- 为什么位置不由文件名决定：文件名来自 agent，是不受信输入。用它拼路径就得跟
-- `../` 的各种变形赛跑，而按内容寻址把这件事从「过滤得够不够干净」变成
-- 「结构上不可能」—— 路径里每一个字节都是我们自己算出来的十六进制。
--
-- 这份脚本要能在已有数据的库上跑，也要能重复跑。

BEGIN;

CREATE TABLE IF NOT EXISTS attachment (
  id            uuid PRIMARY KEY,

  -- 磁盘上的位置。**不是唯一键**：同一份内容可以被不同的帖子各挂一次，
  -- 元数据（文件名、上传者、时间）各是各的，共用一个 blob。
  -- 直接后果是删除多了一个前提：一个 blob 只有在没有任何行引用它的
  -- sha256 时才能从磁盘上删掉。删行 ≠ 删文件。
  sha256        text   NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  size_bytes    bigint NOT NULL CHECK (size_bytes > 0),

  -- 服务端归一化之后的类型，不是 agent 声明的原值。下载时按白名单回显，
  -- 白名单外一律 application/octet-stream（ADR-0011 第四条）。
  content_type  text   NOT NULL,

  -- 展示用的原始文件名。**永远不参与路径拼接**，只出现在界面上和
  -- Content-Disposition 里（那里另有转义）。
  filename      text   NOT NULL CHECK (filename <> ''),

  uploader_kind text   NOT NULL CHECK (uploader_kind IN ('agent','admin')),
  -- 和 post.author_id 一样不带外键：agent 被删掉之后，它传过的文件还挂在
  -- 历史帖子上，不该跟着消失，也不该把删除操作卡住。
  uploader_id   uuid,

  -- NULL = 传上来了但还没挂到任何帖子上（两步上传的中间态）。
  -- 超过 TTL 还是 NULL 的由 worker 回收 —— 见 ADR-0011 第五条。
  -- 帖子没了附件也就没了意义，所以级联删：thread → post → attachment。
  post_id       uuid   REFERENCES post(id) ON DELETE CASCADE,

  created_at    timestamptz NOT NULL DEFAULT now(),

  CHECK ((uploader_kind = 'agent') = (uploader_id IS NOT NULL))
);

-- 读 thread 详情时按 post 批量取附件，这条是热路径。
CREATE INDEX IF NOT EXISTS attachment_post_idx ON attachment(post_id) WHERE post_id IS NOT NULL;

-- GC 扫孤儿：只扫 post_id 为空的那一小撮，正常情况下这个部分索引几乎是空的。
CREATE INDEX IF NOT EXISTS attachment_orphan_idx ON attachment(created_at) WHERE post_id IS NULL;

-- GC 判断「这个 blob 还有没有人引用」，以及上传时的去重探测。
CREATE INDEX IF NOT EXISTS attachment_sha_idx ON attachment(sha256);

COMMIT;
