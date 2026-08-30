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
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/threads/th-0142': () => json(thread),
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

  it('把 GET /api/admin/threads/{id} 的每条 post 渲染成一行，人和 agent 分列两侧', async () => {
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
    expect(calls.some((c) => c.path === '/api/admin/threads/th-0142')).toBe(true)
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

  // 控制台不是一个 agent，不该去打 agent 侧的 inbox 长轮询：
  // 那条路要 Bearer 凭证，带会话 cookie 过去只会一直 401 并空转重试。
  it('控制台不碰 agent 侧的 inbox 长轮询', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')
    await screen.findByRole('heading', { name: thread.title })
    expect(calls.some((c) => c.path.startsWith('/api/agent/'))).toBe(false)
  })
})

/**
 * 广播的右栏一度画的是 todo 那套：一张空的「主 AGENT · 必须响应」（名字是「—」、
 * 状态「离线」），底下再来 5 个永远点不亮的状态圆点。看的人只会以为出了故障。
 *
 * 广播没有主责人，但**有发起人**；广播也没有状态（契约里 status 就写着「仅 todo 有」）。
 */
describe('广播的右栏', () => {
  const tweet = {
    threadId: 'tw-0030',
    kind: 'tweet' as const,
    startedAt: '2026-08-30T13:10:00+08:00',
    authorAgentId: mockDirectory[0].agentId,
    authorName: mockDirectory[0].name,
    tags: [],
    watchers: [
      { agentId: 'w1', name: 'ubuntu-warrior', reason: 'mentioned' as const, online: true },
    ],
    posts: [],
  }

  function stubTweet() {
    return installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /api/admin/threads/tw-0030': () => json(tweet),
    })
  }

  it('第一张卡画的是发起人，不是「主 AGENT · 必须响应」', async () => {
    stubTweet()
    renderApp('/threads/tw-0030')

    const card = await screen.findByTestId('owner-card')
    expect(card).toHaveTextContent('发起人')
    expect(card).toHaveTextContent(mockDirectory[0].name!)
    expect(card).not.toHaveTextContent('必须响应')
    // 流光表达的是「这个人必须响应」，广播里没人必须响应
    expect(card.className).not.toMatch(/\bglow\b/)
  })

  it('不画状态推进 —— 广播没有状态，画出来就是 5 个永远点不亮的圆点', async () => {
    stubTweet()
    renderApp('/threads/tw-0030')

    const card = await screen.findByTestId('progress-card')
    expect(card).not.toHaveTextContent('状态推进')
    expect(card).not.toHaveTextContent('待响应')
    expect(card).not.toHaveTextContent('已完成')
    // 只留「开始于」
    expect(card).toHaveTextContent('开始于')
  })

  it('todo 照旧画主 agent 和状态推进 —— 收窄只针对广播', async () => {
    stub()
    renderApp('/threads/th-0142')

    const owner = await screen.findByTestId('owner-card')
    expect(owner).toHaveTextContent('必须响应')
    expect(owner.className).toMatch(/\bglow\b/)
    expect(await screen.findByTestId('progress-card')).toHaveTextContent('状态推进')
  })
})
