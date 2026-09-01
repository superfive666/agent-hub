import type { Post, ThreadDetail, ThreadWatcher } from '@/api/client'

/** 头像缩写：拉丁名取前两位，CJK 取第一个字。 */
export function initialsOf(name: string | undefined): string {
  const n = (name ?? '').trim()
  if (!n) return '??'
  if (/[㐀-鿿぀-ヿ]/.test(n[0])) return n[0]
  return n.slice(0, 2).toUpperCase()
}

export type TodoStatus = NonNullable<ThreadDetail['status']>

/** 契约里的状态枚举 → 界面用语。顺序就是推进顺序。 */
export const STATUS_FLOW: TodoStatus[] = [
  'awaiting_response',
  'clarifying',
  'in_progress',
  'awaiting_review',
  'done',
]

const STATUS_LABEL: Record<string, string> = {
  awaiting_response: '待响应',
  clarifying: '澄清中',
  in_progress: '进行中',
  awaiting_review: '待确认',
  done: '已完成',
  cancelled: '已取消',
}

export function statusLabel(status: string | undefined): string {
  return status ? (STATUS_LABEL[status] ?? status) : '—'
}

export interface ProgressStep {
  label: string
  state: 'done' | 'current' | 'todo'
}

/**
 * 进度条。第二个参数是 todo 的 `confirmedAt`（可空）。
 *
 * **为什么确认是画出来的、而不是加进 `STATUS_FLOW`。** 确认不是一个 status，
 * 它是 `confirmedAt` 这一位数据（ADR-0008 明确拒绝了「加一个 awaiting_approval 状态」）。
 * `STATUS_FLOW` 还兼着待办页筛选器的值域（`todos.tsx` 拿它和 `t.status` 逐个比），
 * 往里塞一个不存在的状态，会平白多出一个永远筛不到东西的筛选项。
 *
 * 所以流程本身不动，只在「澄清中」和「进行中」之间**插一个由 confirmedAt 推出来的
 * 节点**：确认过是 ✓，没确认就是灰的下一步。它必须出现在进度里 —— 未确认的 todo
 * 卡在这儿推不动，进度条上却完全看不出原因，人只会以为 agent 在偷懒。
 *
 * 不传 `confirmedAt` 时行为和以前完全一样（tweet、以及不关心闸门的调用方）。
 */
export function progressOf(
  status: string | undefined,
  confirmedAt?: string | null | undefined,
  options: { withGate?: boolean } = {},
): ProgressStep[] {
  const withGate = options.withGate ?? confirmedAt !== undefined
  const at = STATUS_FLOW.indexOf(status as TodoStatus)
  const steps: ProgressStep[] = STATUS_FLOW.map((s, i) => ({
    label: statusLabel(s),
    state: at < 0 ? 'todo' : i < at ? 'done' : i === at ? 'current' : 'todo',
  }))
  if (!withGate) return steps
  const gateAt = STATUS_FLOW.indexOf('in_progress')
  const gate: ProgressStep = { label: GATE_LABEL, state: confirmedAt ? 'done' : 'todo' }
  return [...steps.slice(0, gateAt), gate, ...steps.slice(gateAt)]
}

/** 闸门节点在进度条上的名字。卡片标题也用它，两处叫法必须一致。 */
export const GATE_LABEL = '需求确认'

const STEP_KIND_LABEL: Record<string, string> = {
  clarification: '澄清',
  plan: '计划',
  progress: '进展',
  blocked: '受阻',
  deliverable: '交付物',
  confirmation: '确认放行',
}

/** 步骤类型 → 中文。英文枚举直接甩给用户等于没写。 */
export function stepKindLabel(kind: string | undefined): string {
  return kind ? (STEP_KIND_LABEL[kind] ?? kind) : '—'
}

const STEP_STATUS_LABEL: Record<string, string> = {
  pending: '待开始',
  in_progress: '进行中',
  done: '已完成',
  // 和 kind 的「受阻」区分开：这里说的是这一步现在被卡住了
  blocked: '卡住了',
}

export function stepStatusLabel(status: string | undefined): string {
  return status ? (STEP_STATUS_LABEL[status] ?? status) : '—'
}

const AGENT_STATUS_LABEL: Record<string, string> = {
  // 只是一条占位记录，还没拿注册 token 换过长期凭证
  pending_registration: '未接入',
  active: '已接入',
  disabled: '已停用',
}

export function agentStatusLabel(status: string | undefined): string {
  return status ? (AGENT_STATUS_LABEL[status] ?? status) : '—'
}

function fmt(iso: string, opts: Intl.DateTimeFormatOptions, timeZone?: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return new Intl.DateTimeFormat('zh-CN', { ...opts, ...(timeZone ? { timeZone } : {}) }).format(d)
}

/** 列表/气泡上的短时刻：今天给 HH:MM，否则给月日。 */
export function timeLabel(iso: string | undefined, now = new Date(), timeZone?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const sameDay = fmt(iso, { dateStyle: 'short' }, timeZone) === fmt(now.toISOString(), { dateStyle: 'short' }, timeZone)
  if (sameDay) return fmt(iso, { hour: '2-digit', minute: '2-digit', hour12: false }, timeZone)
  return fmt(iso, { month: 'numeric', day: 'numeric' }, timeZone)
}

/** 详情页的完整时刻：8月28日 09:14 */
export function dateTimeLabel(iso: string | undefined, timeZone?: string): string {
  if (!iso) return '—'
  return fmt(iso, { month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }, timeZone)
}

export function dayLabel(iso: string | undefined, timeZone?: string): string {
  if (!iso) return '—'
  return fmt(iso, { month: 'long', day: 'numeric', weekday: 'long' }, timeZone)
}

/** YYYY-MM-DD，看板的 date 参数用它。按本地（或平台）时区切分，不用 toISOString。 */
export function isoDate(d: Date, timeZone?: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    ...(timeZone ? { timeZone } : {}),
  }).format(d)
  return parts
}

export function shiftDate(date: string, days: number): string {
  const [y, m, d] = date.split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  dt.setUTCDate(dt.getUTCDate() + days)
  return dt.toISOString().slice(0, 10)
}

export type Participation = 'primary' | 'watcher'

/** §1.1 的四重信号需要的全部展示信息，从契约类型推出来。 */
export interface AuthorView {
  name: string
  initials: string
  /** 位置 / 气泡底色 / @ 前缀 /「人类」chip 都挂在这一位上 */
  isHuman: boolean
  participation?: Participation
  online?: boolean
}

export function authorOf(post: Post, thread?: Pick<ThreadDetail, 'primaryAgentId' | 'watchers'>): AuthorView {
  const isHuman = post.authorKind === 'admin'
  const name = post.authorName ?? (isHuman ? '管理员' : 'agent')
  if (isHuman) return { name, initials: initialsOf(name), isHuman: true }
  const watcher = thread?.watchers?.find((w: ThreadWatcher) => w.agentId === post.authorId)
  const primary = !!post.authorId && post.authorId === thread?.primaryAgentId
  return {
    name,
    initials: initialsOf(name),
    isHuman: false,
    participation: primary ? 'primary' : 'watcher',
    online: watcher?.online,
  }
}

export function tierLabel(tier: string | undefined): string {
  if (tier === 'longpoll') return '长轮询'
  if (tier === 'webhook') return 'webhook'
  if (tier === 'cron') return 'cron'
  return tier ?? '—'
}

export function latencyLabel(seconds: number | undefined): string {
  if (seconds === undefined) return '—'
  if (seconds < 90) return `~${seconds} 秒`
  if (seconds < 3600) return `~${Math.round(seconds / 60)} 分钟`
  return `~${Math.round(seconds / 3600)} 小时`
}

/**
 * 邮箱局部打码：`wuchao900726@gmail.com` → `wuchao**@gmail.com`。
 *
 * OIDC 模式下 `me.username` 就是那个 Google 邮箱（后端 `adminSubject()` 直接给邮箱），
 * 整串画在侧栏底部会把那一栏撑破。三条规则一条都不能少：
 * - **域名完整保留** —— 用户要靠它认出自己登录的是哪个账号，打掉域名等于没法确认；
 * - 本地部分不超过 `keep` 就原样返回，短邮箱不该被无谓地遮起来；
 * - **不是邮箱就原样返回** —— 口令模式下 `username` 是普通用户名（`superfive`），
 *   把用户名也打码只会让人以为自己登错了。
 *
 * 这只是缩短，**不是布局保证** —— 用它的地方仍然要 `truncate` / `min-w-0` 兜底。
 */
export function maskEmail(v: string | undefined, keep = 6): string {
  const s = (v ?? '').trim()
  const at = s.lastIndexOf('@')
  // 没有 @、@ 开头、@ 结尾：都不是邮箱，原样交回去
  if (at <= 0 || at === s.length - 1) return s
  const local = s.slice(0, at)
  if (local.length <= keep) return s
  return `${local.slice(0, keep)}**${s.slice(at)}`
}

/**
 * 字节数写成人读的样子。
 *
 * 用 1024 进制并写成 KB/MB（不是 KiB/MiB）—— 界面上跟着操作系统的习惯走。
 * 后端错误消息里那份用的是 MiB，因为那是给 agent 读的、要和配置里的
 * 字节数严格对得上，两处口径不同是有意的。
 */
export function byteLabel(n: number | undefined): string {
  if (n === undefined || n < 0) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  // 小于 10 时留一位小数：1.4 MB 和 1 MB 差着 40%，取整会把这个差抹掉
  return `${v < 10 ? Math.round(v * 10) / 10 : Math.round(v)} ${units[i]}`
}

/**
 * 界面上能不能给这个附件画缩略图。
 *
 * 判据是**后端归一化之后**的 contentType，不是文件扩展名 —— 后端只回显
 * 白名单里的类型，所以这里看到 image/* 就意味着它已经过了那道白名单。
 * （`image/svg+xml` 不在白名单里，所以永远走不到这里。）
 */
export function isPreviewableImage(contentType: string | undefined): boolean {
  return !!contentType && contentType.startsWith('image/')
}
