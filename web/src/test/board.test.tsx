import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockBoardActivity, mockBoardStarted, mockDirectory, mockTodos } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

function stub() {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/board': ({ url }) =>
      json(url.searchParams.get('groupBy') === 'started' ? mockBoardStarted : mockBoardActivity),
  })
}

describe('看板的两种归档口径', () => {
  it('默认按活动：请求带 groupBy=activity，渲染的是那一天发生的事', async () => {
    const calls = stub()
    renderApp('/board')
    await within(await screen.findByTestId('board-stream')).findByText(mockBoardActivity.items![0].summary!)
    const first = calls.find((c) => c.path === '/api/admin/board')
    expect(first?.search).toContain('groupBy=activity')
    expect(first?.search).toMatch(/date=\d{4}-\d{2}-\d{2}/)
  })

  it('切到「按开始」会换一个 query，并换成 thread 维度的条目', async () => {
    const calls = stub()
    renderApp('/board')
    await within(await screen.findByTestId('board-stream')).findByText(mockBoardActivity.items![0].summary!)

    await userEvent.click(screen.getByRole('radio', { name: '按开始' }))

    await waitFor(() =>
      expect(calls.some((c) => c.path === '/api/admin/board' && c.search.includes('groupBy=started'))).toBe(
        true,
      ),
    )
    // 「按开始」带的是当前状态与累计统计，每条只出现一次
    const stream = screen.getByTestId('board-stream')
    expect(await within(stream).findByText(mockBoardStarted.items![0].title!)).toBeInTheDocument()
    expect(stream.dataset.groupby).toBe('started')
    // 活动条目不该再出现
    expect(within(stream).queryByText(mockBoardActivity.items![0].summary!)).toBeNull()
  })

  it('日期导航换一天就换一个 query', async () => {
    const calls = stub()
    renderApp('/board')
    await within(await screen.findByTestId('board-stream')).findByText(mockBoardActivity.items![0].summary!)
    const before = calls.filter((c) => c.path === '/api/admin/board').at(-1)!.search

    await userEvent.click(screen.getByRole('button', { name: '前一天' }))

    await waitFor(() => {
      const now = calls.filter((c) => c.path === '/api/admin/board').at(-1)!.search
      expect(now).not.toBe(before)
    })
  })
})
