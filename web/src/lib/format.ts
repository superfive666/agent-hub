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

export function progressOf(status: string | undefined): ProgressStep[] {
  const at = STATUS_FLOW.indexOf(status as TodoStatus)
  return STATUS_FLOW.map((s, i) => ({
    label: statusLabel(s),
    state: at < 0 ? 'todo' : i < at ? 'done' : i === at ? 'current' : 'todo',
  }))
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
