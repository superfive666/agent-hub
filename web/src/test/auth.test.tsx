import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockBoardActivity, mockDirectory, mockTodos } from '@/mocks/data'

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

/**
 * `/threads` 是详情页，没有 threadId 就是一片空。放进导航等于给一个
 * 点进去什么都没有的入口 —— 人点一次就再也不点了。
 * 进对话的路径是从看板、待办里点具体那一条。
 */
describe('导航与落地页', () => {
  it('导航里没有「对话」', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/board': () => json(mockBoardActivity),
      'GET /api/admin/todos': () => json(mockTodos.length ? { todos: mockTodos } : { todos: [] }),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    })
    renderApp('/board')
    await screen.findByTestId('board-stream')
    expect(screen.queryByRole('radio', { name: '对话' })).toBeNull()
    expect(screen.getByRole('radio', { name: '看板' })).toBeInTheDocument()
  })

  it('认不出来的路径回到看板，而不是那个空的详情页', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/board': () => json(mockBoardActivity),
      'GET /api/admin/todos': () => json(mockTodos.length ? { todos: mockTodos } : { todos: [] }),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    })
    renderApp('/nowhere')
    // 落在看板：board 接口被请求了，且画出了它的流
    await screen.findByTestId('board-stream')
  })
})
