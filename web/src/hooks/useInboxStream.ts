import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { InboxEvent } from '@/api/client'

export interface InboxPage {
  events?: InboxEvent[]
  lastSeq?: number
}

export interface UseInboxStreamOptions {
  /** 长轮询端点，默认走 openapi 里的 /api/agent/me/inbox */
  endpoint?: string
  /** 从哪个 seq 之后开始拉。默认 0 */
  initialCursor?: number
  /** hold 时长，直接透传给 ?wait= */
  wait?: string
  enabled?: boolean
  /** 收到事件后要失效的 query key 前缀 */
  invalidateKeys?: readonly unknown[][]
  onEvents?: (events: InboxEvent[]) => void
  /** 注入点，测试用 */
  fetchImpl?: typeof fetch
  /** 退避后的等待，测试里可以换成立即 resolve */
  sleepImpl?: (ms: number) => Promise<void>
}

export interface InboxStreamState {
  cursor: number
  connected: boolean
  /** 连续失败次数，回到 0 表示已经重连上 */
  failures: number
  lastError: Error | null
  /** 立刻中断当前挂起的请求，用当前 cursor 重新拉一次 */
  refresh: () => void
}

const BASE_BACKOFF_MS = 500
const MAX_BACKOFF_MS = 30_000

function defaultSleep(ms: number) {
  return new Promise<void>((r) => setTimeout(r, ms))
}

/**
 * inbox 长轮询。
 *
 * 这不是定时轮询：一个请求带 `?wait=30s` 挂在服务端，有事件立即返回，
 * 没有则超时返回空，然后立刻用新的 cursor 再发一个。**不要换成
 * TanStack Query 的 refetchInterval**，那是固定间隔的定时轮询，语义不同。
 *
 * 正确性靠 cursor，不靠通知：断线后用同一个 cursor 重连，
 * 断开期间产生的事件会在下一次响应里补齐（ADR-0001）。
 */
export function useInboxStream(options: UseInboxStreamOptions = {}): InboxStreamState {
  const {
    endpoint = '/api/agent/me/inbox',
    initialCursor = 0,
    wait = '30s',
    enabled = true,
    invalidateKeys,
    onEvents,
    fetchImpl,
    sleepImpl = defaultSleep,
  } = options

  const queryClient = useQueryClient()
  const cursorRef = useRef(initialCursor)
  const abortRef = useRef<AbortController | null>(null)
  const [state, setState] = useState({
    cursor: initialCursor,
    connected: false,
    failures: 0,
    lastError: null as Error | null,
  })

  // 回调放 ref 里，免得每次渲染都把长轮询循环重启一遍
  const onEventsRef = useRef(onEvents)
  onEventsRef.current = onEvents
  const invalidateRef = useRef(invalidateKeys)
  invalidateRef.current = invalidateKeys

  const refresh = useCallback(() => {
    abortRef.current?.abort()
  }, [])

  useEffect(() => {
    if (!enabled) return
    const doFetch = fetchImpl ?? globalThis.fetch.bind(globalThis)
    let stopped = false
    let failures = 0

    const loop = async () => {
      while (!stopped) {
        const ctrl = new AbortController()
        abortRef.current = ctrl
        try {
          const url = `${endpoint}?after=${cursorRef.current}&wait=${encodeURIComponent(wait)}`
          const res = await doFetch(url, {
            signal: ctrl.signal,
            headers: { accept: 'application/json' },
            credentials: 'include',
          })
          if (!res.ok) throw new Error(`inbox ${res.status}`)
          const page = (await res.json()) as InboxPage
          const events = page.events ?? []
          // lastSeq 是权威游标；没给就退回事件里的最大 seq
          const next = Math.max(
            cursorRef.current,
            page.lastSeq ?? 0,
            ...events.map((e) => e.seq ?? 0),
          )
          cursorRef.current = next
          failures = 0
          if (!stopped) {
            setState({ cursor: next, connected: true, failures: 0, lastError: null })
          }
          if (events.length > 0) {
            onEventsRef.current?.(events)
            const keys = invalidateRef.current
            if (keys) for (const key of keys) queryClient.invalidateQueries({ queryKey: key })
            else queryClient.invalidateQueries()
          }
        } catch (err) {
          if (stopped) return
          // refresh() / 卸载导致的中断不算失败，直接用当前 cursor 重来
          if (ctrl.signal.aborted) continue
          failures += 1
          const error = err instanceof Error ? err : new Error(String(err))
          setState((s) => ({ ...s, connected: false, failures, lastError: error }))
          const backoff = Math.min(BASE_BACKOFF_MS * 2 ** (failures - 1), MAX_BACKOFF_MS)
          await sleepImpl(backoff)
        }
      }
    }

    void loop()
    return () => {
      stopped = true
      abortRef.current?.abort()
    }
  }, [enabled, endpoint, wait, fetchImpl, sleepImpl, queryClient])

  // 页面重新可见时补拉一次：后台期间浏览器可能已经把挂起的连接掐了
  useEffect(() => {
    if (!enabled) return
    const onVisible = () => {
      if (document.visibilityState === 'visible') refresh()
    }
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('online', refresh)
    return () => {
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('online', refresh)
    }
  }, [enabled, refresh])

  return { ...state, refresh }
}
