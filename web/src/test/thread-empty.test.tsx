import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

/** 平台刚起来：一条 todo 都没有，`agents` 由各条用例自己决定有没有 */
function stub(agents: unknown[] = []) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents }),
  })
}

/**
 * 缺陷 1：react-query v5 里被 disabled 的 query 永远停在 `pending`。
 * 没有 todo 时 threadId 是 undefined、`useThread` 是 disabled 的，
 * 拿 `isPending` 当"正在加载"用，标题就永远卡在「加载中…」——
 * 而实际上一个请求都没发出去过。
 */
describe('平台还空着的时候落到 /threads', () => {
  it('一个 agent 都没有时给出「去名录」的引导，而不是永远显示「加载中」', async () => {
    stub([])
    renderApp('/threads')

    const empty = await screen.findByTestId('thread-empty')
    expect(empty).toHaveTextContent('先让第一个 agent 进来')
    expect(screen.getByRole('link', { name: '去名录添加第一个 agent' })).toHaveAttribute(
      'href',
      '/directory',
    )
    // 空转的「加载中…」一个字都不许留下
    expect(screen.queryByText('加载中…')).toBeNull()
    expect(screen.queryByText('正在拉取…')).toBeNull()
  })

  it('有 agent 但没有 todo 时改成引导去新建待办', async () => {
    stub(mockDirectory)
    renderApp('/threads')

    const empty = await screen.findByTestId('thread-empty')
    expect(empty).toHaveTextContent('把第一件事交给其中一个 agent')
    expect(screen.getByRole('link', { name: '新建一条待办' })).toHaveAttribute('href', '/todos/new')
    expect(screen.getByRole('link', { name: '先看看名录里有谁' })).toHaveAttribute(
      'href',
      '/directory',
    )
    expect(screen.queryByText('加载中…')).toBeNull()
  })

  it('没有 thread 可回时不画输入框 —— 对着不存在的 thread 打字只会在发送时失败', async () => {
    stub(mockDirectory)
    renderApp('/threads')

    await screen.findByTestId('thread-empty')
    expect(screen.queryByLabelText('回复这条 thread')).toBeNull()
    expect(screen.queryByRole('button', { name: '发送' })).toBeNull()
  })

  it('todo 列表还在拉的时候仍然显示加载中 —— 那是真的在加载', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      // 一直挂着不返回：模拟 todos 还在路上
      'GET /api/admin/todos': () => new Promise<Response>(() => {}),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    })
    renderApp('/threads')

    expect(await screen.findByRole('heading', { name: '加载中…' })).toBeInTheDocument()
    expect(screen.queryByTestId('thread-empty')).toBeNull()
  })
})
