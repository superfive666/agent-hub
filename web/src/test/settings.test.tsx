import { screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockSettings, mockTodos } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

function stub(health: Record<string, unknown> = HEALTHY) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/agent/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/settings': () => json(mockSettings),
    'GET /api/admin/health': () => json(health),
  })
}

describe('§1.4 outbox 告警不可折叠、不可降级', () => {
  it('一切正常时卡片照样在，没有关闭按钮也没有折叠开关', async () => {
    stub()
    renderApp('/settings')
    const card = await screen.findByTestId('outbox-card')

    // 卡片里一个可点的控件都不该有 —— 关掉它、收起它的入口都不存在
    expect(within(card).queryAllByRole('button')).toHaveLength(0)
    expect(within(card).queryByRole('switch')).toBeNull()
    expect(within(card).queryAllByLabelText(/关闭|收起|忽略/)).toHaveLength(0)
    // details/summary 这类原生折叠也不许有
    expect(card.querySelector('details, summary, [aria-expanded]')).toBeNull()
    expect(card).toHaveTextContent('告警不可关闭')
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
      'GET /api/agent/directory': () => json({ agents: mockDirectory }),
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
    expect(await screen.findByText('在线判定窗口')).toBeInTheDocument()
    expect(await screen.findByText('120 / 300 / 1800 秒')).toBeInTheDocument()
  })
})
