import createClient from 'openapi-fetch'
import type { components, paths } from './schema'

/**
 * 类型全部从 docs/api/openapi.yaml 生成，一个都不手写。
 * 契约变了就 `npm run gen:api`，不要在这里补形状。
 */
export type InboxEvent = components['schemas']['InboxEvent']
export type AgentSummary = components['schemas']['AgentSummary']
export type ApiError = components['schemas']['Error']
export type Post = components['schemas']['Post']
export type ThreadDetail = components['schemas']['ThreadDetail']
export type ThreadWatcher = components['schemas']['ThreadWatcher']
export type TodoSummary = components['schemas']['TodoSummary']
export type Settings = components['schemas']['Settings']

type BoardResponse =
  paths['/api/admin/board']['get']['responses']['200']['content']['application/json']
/** groupBy=activity：这一天发生了什么，一条 thread 会跨多天反复出现 */
export type BoardActivity = Extract<BoardResponse, { groupBy?: 'activity' }>
/** groupBy=started：这一天开了哪些事、现在怎么样了，每条只出现一次 */
export type BoardStarted = Extract<BoardResponse, { groupBy?: 'started' }>
export type BoardActivityItem = NonNullable<BoardActivity['items']>[number]
export type BoardStartedItem = NonNullable<BoardStarted['items']>[number]

export type AdminMe = NonNullable<
  paths['/api/admin/me']['get']['responses']['200']['content']['application/json']
>
export type Health = NonNullable<
  paths['/api/admin/health']['get']['responses']['200']['content']['application/json']
>

/**
 * 管理台的会话是 HttpOnly Cookie，所以每一发都要带 credentials。
 * 少了它登录能成功，之后每个请求都 401 —— 而且看起来像"后端坏了"。
 */
/**
 * 默认同源。写成绝对地址而不是 `'/'`，因为 undici 的 Request 不接受相对 URL
 * （浏览器接受，Node/测试环境不接受）。
 */
const baseUrl =
  import.meta.env.VITE_API_BASE_URL ??
  (typeof location !== 'undefined' && location.origin ? location.origin : '/')

export const api = createClient<paths>({
  baseUrl,
  credentials: 'include',
  // 晚绑定：openapi-fetch 默认在 createClient 时就抓住 globalThis.fetch，
  // 那样测试里换掉全局 fetch 就不生效了。
  fetch: (...args) => globalThis.fetch(...args),
})

/**
 * 拼一个走 API 的绝对地址。给**整页跳转**用（OIDC 授权入口那种）——
 * 那条路必须让浏览器自己跟 302 并收 cookie，用 fetch 是拿不到的。
 */
export function apiUrl(path: string): string {
  return baseUrl === '/' ? path : `${baseUrl.replace(/\/$/, '')}${path}`
}

/**
 * Google OIDC 授权入口。口令模式的实例访问它会 401 —— 两种模式互斥。
 * `satisfies keyof paths` 让契约里改了路径时这里直接编译不过，而不是线上 404。
 */
export const OIDC_START_PATH = '/api/admin/auth/google/start' satisfies keyof paths

export class HttpError extends Error {
  status: number
  body?: ApiError

  constructor(status: number, body?: ApiError) {
    super(body?.message ?? `HTTP ${status}`)
    this.name = 'HttpError'
    this.status = status
    this.body = body
  }
}

export function isUnauthorized(err: unknown): boolean {
  return err instanceof HttpError && err.status === 401
}

/** openapi-fetch 的 {data,error,response} → 要么给数据，要么抛 HttpError。 */
export function unwrap<T>(r: { data?: T; error?: unknown; response: Response }): T {
  if (!r.response.ok) throw new HttpError(r.response.status, r.error as ApiError | undefined)
  return r.data as T
}
