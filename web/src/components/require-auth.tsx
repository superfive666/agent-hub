import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { useMe } from '@/api/queries'
import { isUnauthorized } from '@/api/client'

/**
 * 会话是 HttpOnly Cookie，前端读不到它，只能问 `/api/admin/me`。
 * 401 = 没登录 → 去登录页；其它错误是"后端有问题"，不能伪装成未登录。
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation()
  const { data, isPending, isError, error } = useMe()

  if (isPending) {
    return (
      <div className="app min-h-dvh items-center justify-center">
        <span className="lbl" role="status">
          正在确认会话…
        </span>
      </div>
    )
  }
  if (isError && isUnauthorized(error)) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  if (isError || !data) {
    return (
      <div className="app min-h-dvh items-center justify-center">
        <div role="alert" className="lbl" style={{ color: 'var(--alert)' }}>
          读不到会话状态：{(error as Error | null)?.message ?? '未知错误'}
        </div>
      </div>
    )
  }
  return <>{children}</>
}
