import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { useInboxStream, type InboxPage } from '@/hooks/useInboxStream'

type Step = InboxPage | Error

/**
 * 按脚本依次应答；脚本走完之后就一直挂着（模拟长轮询 hold 住不返回），
 * 免得循环空转。
 */
function hang(signal?: AbortSignal | null) {
  return new Promise<Response>((_, reject) => {
    signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
  })
}

function scriptedFetch(steps: Step[]) {
  const calls: string[] = []
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push(url)
    const step = steps.shift()
    // 脚本走完就挂着，像真的长轮询一样 hold 住；被 abort 时按 fetch 的语义抛出
    if (step === undefined) return hang(init?.signal)
    if (step instanceof Error) throw step
    return new Response(JSON.stringify(step), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  })
  return { impl: impl as unknown as typeof fetch, calls }
}

function wrapper(queryClient: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

const noSleep = () => Promise.resolve()

describe('useInboxStream', () => {
  it('用 ?wait= 长轮询，并按 lastSeq 推进 cursor', async () => {
    const qc = new QueryClient()
    const { impl, calls } = scriptedFetch([
      { events: [{ seq: 1, kind: 'todo.assigned', priority: 0, createdAt: 'x' }], lastSeq: 1 },
      { events: [{ seq: 2, kind: 'thread.replied', priority: 2, createdAt: 'x' }], lastSeq: 2 },
    ])

    const { result } = renderHook(
      () => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep, wait: '30s' }),
      { wrapper: wrapper(qc) },
    )

    await waitFor(() => expect(result.current.cursor).toBe(2))
    // 第一发从 after=0 起，之后每一发都带上一次的 lastSeq —— 这才是长轮询，不是定时轮询
    expect(calls[0]).toBe('/api/agent/me/inbox?after=0&wait=30s')
    expect(calls[1]).toBe('/api/agent/me/inbox?after=1&wait=30s')
    await waitFor(() => expect(calls[2]).toBe('/api/agent/me/inbox?after=2&wait=30s'))
    expect(result.current.connected).toBe(true)
  })

  it('响应为空时 cursor 不倒退，也不乱推进', async () => {
    const qc = new QueryClient()
    const { impl, calls } = scriptedFetch([
      { events: [{ seq: 5, kind: 'tweet.published', priority: 3, createdAt: 'x' }], lastSeq: 5 },
      { events: [], lastSeq: 5 },
    ])
    const { result } = renderHook(
      () => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep }),
      { wrapper: wrapper(qc) },
    )
    await waitFor(() => expect(calls.length).toBeGreaterThanOrEqual(3))
    expect(result.current.cursor).toBe(5)
    expect(calls[2]).toContain('after=5')
  })

  it('收到事件才 invalidate，空响应不打扰缓存', async () => {
    const qc = new QueryClient()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { impl } = scriptedFetch([
      { events: [], lastSeq: 0 },
      { events: [{ seq: 1, kind: 'todo.mentioned', priority: 1, createdAt: 'x' }], lastSeq: 1 },
    ])
    const key = [['thread', 'th-0142']] as const
    const { result } = renderHook(
      () =>
        useInboxStream({
          fetchImpl: impl,
          sleepImpl: noSleep,
          invalidateKeys: key as unknown as readonly unknown[][],
        }),
      { wrapper: wrapper(qc) },
    )
    await waitFor(() => expect(result.current.cursor).toBe(1))
    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith({ queryKey: ['thread', 'th-0142'] })
  })

  it('断线后按同一个 cursor 重连，断开期间的事件被补齐', async () => {
    const qc = new QueryClient()
    const onEvents = vi.fn()
    const { impl, calls } = scriptedFetch([
      { events: [{ seq: 7, kind: 'thread.replied', priority: 2, createdAt: 'x' }], lastSeq: 7 },
      new Error('network down'),
      new Error('network down'),
      // 重连：断开期间攒下的 8、9 一起回来
      {
        events: [
          { seq: 8, kind: 'thread.replied', priority: 2, createdAt: 'x' },
          { seq: 9, kind: 'todo.status_changed', priority: 1, createdAt: 'x' },
        ],
        lastSeq: 9,
      },
    ])

    const { result } = renderHook(
      () => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep, onEvents }),
      { wrapper: wrapper(qc) },
    )

    await waitFor(() => expect(result.current.cursor).toBe(9))
    // 两次失败都用 after=7 重发，没有把 cursor 丢掉，也没跳过 8
    expect(calls[1]).toContain('after=7')
    expect(calls[2]).toContain('after=7')
    expect(calls[3]).toContain('after=7')
    expect(onEvents.mock.calls.at(-1)?.[0].map((e: { seq: number }) => e.seq)).toEqual([8, 9])
    // 重连成功后失败计数归零
    await waitFor(() => expect(result.current.failures).toBe(0))
    expect(result.current.connected).toBe(true)
  })

  it('HTTP 错误码也走重试，cursor 不动', async () => {
    const qc = new QueryClient()
    const calls: string[] = []
    let n = 0
    const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(String(input))
      n += 1
      if (n === 1) return new Response('boom', { status: 503 })
      if (n === 2)
        return new Response(
          JSON.stringify({ events: [{ seq: 3, kind: 'tweet.replied', priority: 2, createdAt: 'x' }], lastSeq: 3 }),
          { status: 200 },
        )
      return hang(init?.signal)
    }) as unknown as typeof fetch

    const { result } = renderHook(
      () => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep }),
      { wrapper: wrapper(qc) },
    )
    await waitFor(() => expect(result.current.cursor).toBe(3))
    expect(calls[0]).toContain('after=0')
    expect(calls[1]).toContain('after=0')
  })

  it('页面重新可见时用当前 cursor 补拉一次', async () => {
    const qc = new QueryClient()
    const { impl, calls } = scriptedFetch([
      { events: [{ seq: 4, kind: 'thread.replied', priority: 2, createdAt: 'x' }], lastSeq: 4 },
    ])
    const { result } = renderHook(
      () => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep }),
      { wrapper: wrapper(qc) },
    )
    await waitFor(() => expect(result.current.cursor).toBe(4))
    const before = calls.length

    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))

    // 中断挂起的请求 → 立刻用 after=4 再发一次，不是等 30s 超时
    await waitFor(() => expect(calls.length).toBeGreaterThan(before))
    expect(calls.at(-1)).toContain('after=4')
  })

  it('enabled=false 时一个请求都不发', () => {
    const qc = new QueryClient()
    const { impl, calls } = scriptedFetch([])
    renderHook(() => useInboxStream({ fetchImpl: impl, sleepImpl: noSleep, enabled: false }), {
      wrapper: wrapper(qc),
    })
    expect(calls).toEqual([])
  })
})
