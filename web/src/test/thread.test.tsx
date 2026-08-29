import { screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockThread, mockTodos } from '@/mocks/data'

const thread = mockThread('th-0142')

function stub() {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/agent/directory': () => json({ agents: mockDirectory }),
    'GET /api/agent/threads/th-0142': () => json(thread),
  })
}

afterEach(() => vi.unstubAllGlobals())

describe('对话页（真数据）', () => {
  it('渲染出舞台 → 玻璃板 → 嵌套内板三层构图', async () => {
    stub()
    const { container } = renderApp('/threads/th-0142')
    await screen.findByRole('heading', { name: thread.title })
    expect(container.querySelector('.app')).toBeInTheDocument()
    // 两块玻璃板：左会话栏 + 主区
    expect(container.querySelectorAll('.pane').length).toBeGreaterThanOrEqual(2)
    // 板中有板：消息流 + 右详情
    expect(container.querySelectorAll('.inset').length).toBeGreaterThanOrEqual(2)
  })

  it('把 GET /api/agent/threads/{id} 的每条 post 渲染成一行，人和 agent 分列两侧', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')
    const rows = await screen.findAllByTestId('message-row')
    expect(rows).toHaveLength(thread.posts.length)
    // authorKind=admin 的两条靠右，其余靠左
    expect(rows.filter((r) => r.dataset.human === 'true').length).toBe(
      thread.posts.filter((p) => p.authorKind === 'admin').length,
    )
    expect(rows.every((r) => (r.dataset.human === 'true') === r.classList.contains('msg-me'))).toBe(
      true,
    )
    expect(calls.some((c) => c.path === '/api/agent/threads/th-0142')).toBe(true)
  })

  it('流光只出现在主 agent 卡片与当前会话上（§1.3）', async () => {
    stub()
    const { container } = renderApp('/threads/th-0142')
    await screen.findByRole('heading', { name: thread.title })
    await waitFor(() => expect(container.querySelectorAll('.glow').length).toBe(2))
    const glowing = Array.from(container.querySelectorAll('.glow'))
    expect(glowing.some((el) => el.classList.contains('convo'))).toBe(true)
    expect(glowing.some((el) => el.classList.contains('card'))).toBe(true)
    // §3 的坑：玻璃板的两个伪元素已经被棱镜边和高光占了，流光不许挂上去
    expect(container.querySelectorAll('.pane.glow, .pane.runner').length).toBe(0)
  })

  it('长轮询接上了：页面挂起一个带 ?after= 的 inbox 请求', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')
    await screen.findByRole('heading', { name: thread.title })
    await waitFor(() =>
      expect(calls.some((c) => c.path === '/api/agent/me/inbox' && c.search.includes('after='))).toBe(
        true,
      ),
    )
  })
})
