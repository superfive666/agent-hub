import type { ReactNode } from 'react'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { vi } from 'vitest'
import { AppRoutes } from '@/app'

export interface Call {
  method: string
  path: string
  search: string
  body?: string
  /** 会话是 HttpOnly Cookie，所以每一发都必须是 'include' */
  credentials?: RequestCredentials
}

export type Handler = (ctx: { url: URL; init?: RequestInit }) => Response | Promise<Response>

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

export function noContent(status = 204): Response {
  return new Response(null, { status })
}

/**
 * 用 `'GET /api/admin/me'` 这样的 key 打桩。没打桩的路径一律 404，
 * 免得测试悄悄依赖真网络。inbox 长轮询默认挂着，不打扰断言。
 */
export function installFetch(routes: Record<string, Handler>): Call[] {
  const calls: Call[] = []
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    // openapi-fetch 传的是 Request 对象，裸 fetch 传的是字符串 —— 两种都要认
    const isRequest = typeof Request !== 'undefined' && input instanceof Request
    const href = isRequest ? input.url : String(input)
    const url = new URL(href, 'http://localhost')
    const method = (isRequest ? input.method : (init?.method ?? 'GET')).toUpperCase()
    const body = isRequest ? await input.clone().text() : (init?.body as string | undefined)
    const credentials = isRequest ? input.credentials : init?.credentials
    calls.push({ method, path: url.pathname, search: url.search, body, credentials })
    const handler = routes[`${method} ${url.pathname}`] ?? routes[url.pathname]
    if (handler) return handler({ url, init: { ...init, method, body, credentials } })
    return new Response(`no stub for ${method} ${url.pathname}`, { status: 404 })
  })
  vi.stubGlobal('fetch', impl)
  return calls
}

export function newQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
}

export function Providers({ children, path = '/' }: { children: ReactNode; path?: string }) {
  return (
    <QueryClientProvider client={newQueryClient()}>
      <MemoryRouter initialEntries={[path]}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

/** 挂整棵路由树 —— 重定向、守卫这类行为只有整棵树上才测得到。 */
export function renderApp(path = '/threads') {
  return render(
    <QueryClientProvider client={newQueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <AppRoutes />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

export const ADMIN = { username: 'superfive', authMode: 'password', timezone: 'Asia/Singapore' }
export const HEALTHY = {
  outboxLagSeconds: 0.4,
  outboxPending: 0,
  outboxDead: 0,
  workerAlive: true,
  pendingLongPolls: 2,
}
