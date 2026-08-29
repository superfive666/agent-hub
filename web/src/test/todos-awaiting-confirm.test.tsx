import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TodoSummary } from '@/api/client'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'

afterEach(() => vi.unstubAllGlobals())

const base = {
  primaryAgentId: 'a1',
  primaryAgentName: 'kilo',
  primaryAgentOnline: true,
  startedAt: '2026-08-28T09:00:00+08:00',
  updatedAt: '2026-08-28T10:00:00+08:00',
  replyCount: 2,
  watchers: [],
}

/**
 * 三条 todo 覆盖三种情况：等确认的、已确认的、已完成但从没确认过的
 * （迁移后的历史数据正是最后这种 —— 它不该被催）。
 */
const TODOS: TodoSummary[] = [
  { ...base, threadId: 'th-1', title: '等你确认的那条', status: 'awaiting_response' },
  {
    ...base,
    threadId: 'th-2',
    title: '已经放行的那条',
    status: 'in_progress',
    confirmedAt: '2026-08-28T09:40:00+08:00',
  },
  { ...base, threadId: 'th-3', title: '早就做完的老数据', status: 'done' },
]

function stub(todos: TodoSummary[] = TODOS) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: [] }),
  })
}

describe('待办列表上的「等你确认」', () => {
  /**
   * 闸门挡着的 todo 在列表里和别的长得一样的话，人只会以为 agent 在偷懒，
   * 而实际上是在等自己点一下 —— 这正是闸门最容易变成「卡住不动」的地方。
   */
  it('未确认的行挂「等你确认」，已确认的不挂', async () => {
    stub()
    renderApp('/todos')

    const rows = await screen.findAllByTestId('todo-row')
    expect(within(rows[0]).getByTestId('awaiting-confirm')).toBeInTheDocument()
    expect(within(rows[1]).queryByTestId('awaiting-confirm')).toBeNull()
  })

  it('已完成 / 已取消的老数据不被催 —— 迁移后它们的 confirmedAt 也是空的', async () => {
    stub()
    renderApp('/todos')

    const rows = await screen.findAllByTestId('todo-row')
    // th-3 是 done，虽然 confirmedAt 为空也不该挂标记
    expect(within(rows[2]).queryByTestId('awaiting-confirm')).toBeNull()
  })

  it('页头直接给出「几条在等你确认」，不用自己数', async () => {
    stub()
    renderApp('/todos')
    expect(await screen.findByText(/1 条在等你确认/)).toBeInTheDocument()
  })

  it('一条都不欠时页头回到普通说明，不显示「0 条在等你确认」', async () => {
    stub([TODOS[1], TODOS[2]])
    renderApp('/todos')
    expect(await screen.findByText(/每条 todo 有且只有一个主 agent/)).toBeInTheDocument()
    expect(screen.queryByText(/在等你确认/)).toBeNull()
  })

  it('能按「等你确认」筛 —— 它不是 status，但必须筛得出来', async () => {
    stub()
    renderApp('/todos')
    await screen.findAllByTestId('todo-row')

    await userEvent.click(screen.getByRole('radio', { name: '等你确认' }))
    const rows = await screen.findAllByTestId('todo-row')
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('等你确认的那条')
  })
})
