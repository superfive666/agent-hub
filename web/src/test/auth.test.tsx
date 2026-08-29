import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { HEALTHY, installFetch, json, renderApp } from './harness'

afterEach(() => vi.unstubAllGlobals())

const unauthorized = () => json({ code: 'unauthorized', message: '没有会话', retryable: false }, 401)

describe('未登录的重定向', () => {
  it('/api/admin/me 返回 401 时，受保护的页面把人送到登录页', async () => {
    installFetch({ 'GET /api/admin/me': unauthorized })
    renderApp('/threads/th-0142')
    // 看到登录表单就说明重定向到了 /login
    expect(await screen.findByLabelText('用户名')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '对话' })).toBeNull()
  })

  it('看板、名录、设置同样受保护', async () => {
    for (const path of ['/board', '/directory', '/settings', '/todos/new']) {
      installFetch({ 'GET /api/admin/me': unauthorized })
      const { unmount } = renderApp(path)
      expect(await screen.findByLabelText('密码')).toBeInTheDocument()
      unmount()
      vi.unstubAllGlobals()
    }
  })

  it('后端出别的错时不冒充「未登录」—— 那会把故障藏起来', async () => {
    installFetch({
      'GET /api/admin/me': () => new Response('boom', { status: 500 }),
      'GET /api/admin/health': () => json(HEALTHY),
    })
    renderApp('/board')
    expect(await screen.findByRole('alert')).toHaveTextContent('读不到会话状态')
    expect(screen.queryByLabelText('用户名')).toBeNull()
  })
})
