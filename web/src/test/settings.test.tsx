import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockSettings, mockTodos } from '@/mocks/data'

afterEach(() => {
  vi.unstubAllGlobals()
  // 「知道了」记在 localStorage 里，不清的话会串到下一条用例：
  // 前一条把说明关掉了，后一条就再也找不到它。
  localStorage.clear()
})

function stub(health: Record<string, unknown> = HEALTHY) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/settings': () => json(mockSettings),
    'GET /api/admin/health': () => json(health),
  })
}

describe('§1.4 outbox 告警不可折叠、不可降级', () => {
  /**
   * §1.4 护的是**指标本身**：outbox 滞后、worker 死活、积压、死信。
   * 那段解释「为什么不可关」的说明文字不在其内 —— 它一切正常时也一直挂着，
   * 关掉它不会让任何异常变得看不见。两者的边界写在 docs/07-design-language.md §1.4。
   */
  it('指标没有关闭、折叠、忽略的入口 —— 连把说明关掉之后也还在', async () => {
    stub()
    renderApp('/settings')
    const card = await screen.findByTestId('outbox-card')

    const metrics = () => {
      const c = screen.getByTestId('outbox-card')
      return {
        lag: c.textContent?.includes('OUTBOX 延迟'),
        worker: c.textContent?.includes('worker'),
        pending: c.textContent?.includes('待扇出'),
        dead: c.textContent?.includes('死信队列'),
      }
    }
    expect(metrics()).toEqual({ lag: true, worker: true, pending: true, dead: true })

    // 收起/折叠这类入口一个都不许有
    expect(within(card).queryByRole('switch')).toBeNull()
    expect(within(card).queryAllByLabelText(/关闭|收起|折叠|隐藏/)).toHaveLength(0)
    expect(card.querySelector('details, summary, [aria-expanded]')).toBeNull()

    // 唯一可点的是说明上的「知道了」，关掉之后指标一个不少
    await userEvent.click(within(card).getByRole('button', { name: /知道了/ }))
    await waitFor(() => expect(screen.queryByTestId('outbox-note')).toBeNull())
    expect(metrics()).toEqual({ lag: true, worker: true, pending: true, dead: true })
  })

  it('说明文字用警告色，不是错误色 —— 一切正常时它也一直挂着', async () => {
    stub()
    renderApp('/settings')
    const note = await screen.findByTestId('outbox-note')
    // 绿色的「正常」chip 旁边一段红字，读起来像出了事
    expect(note.getAttribute('style')).toContain('--warn-soft')
    expect(note.getAttribute('style')).not.toContain('--alert')
  })

  it('worker 没心跳时数字变成告警色，并额外给出全局横幅', async () => {
    stub({ ...HEALTHY, workerAlive: false, outboxLagSeconds: 812, outboxPending: 41 })
    renderApp('/settings')
    const card = await screen.findByTestId('outbox-card')
    expect(within(card).getByText('异常')).toBeInTheDocument()
    expect(within(card).getByText('无心跳')).toBeInTheDocument()
    // 全局横幅同样没有关闭按钮
    const banner = await screen.findByTestId('outbox-alert')
    expect(within(banner).queryAllByRole('button')).toHaveLength(0)
  })

  it('连 health 都读不到时也要出声，不能静默', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /api/admin/settings': () => json(mockSettings),
      'GET /api/admin/health': () => new Response('boom', { status: 500 }),
    })
    renderApp('/settings')
    expect(await screen.findByTestId('outbox-alert')).toHaveTextContent('读不到')
  })
})

describe('设置页读到的是契约里的 Settings', () => {
  it('在线判定窗口按档位分别显示 —— 一个值会让 cron 档永远显示离线', async () => {
    stub()
    renderApp('/settings')
    // 三档各一个输入框。合成一个数填是这里最容易犯的错：
    // cron 档几分钟才拉一次，套长轮询那个窗口它永远是刚过期的状态。
    // 输入框先渲染出来（空的），值等设置接口回来才填 —— 要等的是值，不是元素
    await waitFor(() =>
      expect(screen.getByLabelText('在线窗口 · 长轮询')).toHaveValue(120),
    )
    expect(screen.getByLabelText('在线窗口 · webhook')).toHaveValue(300)
    expect(screen.getByLabelText('在线窗口 · cron')).toHaveValue(1800)
  })
})

/**
 * 这一页以前只读。后端 `PUT /api/admin/settings` 早就有了，前端没接 ——
 * 于是「部署级配置」只能靠改数据库或重新部署来动。
 */
describe('系统设置可以改', () => {
  it('改完出现保存条，保存时把整份配置 PUT 回去', async () => {
    const calls = stub()
    renderApp('/settings')
    await waitFor(() => expect(screen.getByLabelText('长轮询超时')).toHaveValue(30))

    // 没改之前不该有保存条 —— 常驻一个「保存」按钮，人分不清自己改没改过
    expect(screen.queryByTestId('settings-save-bar')).toBeNull()

    await userEvent.clear(screen.getByLabelText('长轮询超时'))
    await userEvent.type(screen.getByLabelText('长轮询超时'), '40')
    expect(await screen.findByTestId('settings-save-bar')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      const put = calls.find((c) => c.method === 'PUT' && c.path === '/api/admin/settings')
      expect(put, '应当发出 PUT').toBeDefined()
      // **整份发回去**：只发改动那一项的话，其余会被后端当成「要清空」
      expect(JSON.parse(put!.body as string)).toMatchObject({
        longPollMaxSeconds: 40,
        timezone: mockSettings.timezone,
        onlineWindowSeconds: mockSettings.onlineWindowSeconds,
        rateLimits: mockSettings.rateLimits,
      })
    })
  })

  it('「放弃」把草稿丢回服务端当前值', async () => {
    stub()
    renderApp('/settings')
    await waitFor(() => expect(screen.getByLabelText('长轮询超时')).toHaveValue(30))

    await userEvent.clear(screen.getByLabelText('长轮询超时'))
    await userEvent.type(screen.getByLabelText('长轮询超时'), '99')
    await userEvent.click(await screen.findByRole('button', { name: '放弃' }))

    await waitFor(() => expect(screen.getByLabelText('长轮询超时')).toHaveValue(30))
    expect(screen.queryByTestId('settings-save-bar')).toBeNull()
  })
})
