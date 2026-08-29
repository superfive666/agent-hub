import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockTodos } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

function stub() {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/agent/directory': () => json({ agents: mockDirectory }),
    'POST /api/admin/todos': () => json({ threadId: 'th-new', startedAt: '2026-08-28T10:00:00+08:00' }, 201),
  })
}

describe('新建 todo', () => {
  it('没选主 agent 时提交按钮是禁用的 —— 一条 todo 必须有且只有一个负责人', async () => {
    stub()
    renderApp('/todos/new')

    await screen.findByRole('radio', { name: '主 agent nova' })
    await userEvent.type(screen.getByLabelText('标题'), '给 webhook 重试加指数退避')

    // 标题填了，主 agent 还没选 —— 依然不给提交
    const submit = screen.getByTestId('create-todo')
    expect(submit).toBeDisabled()
    expect(screen.getByTestId('primary-required')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('radio', { name: '主 agent nova' }))

    await waitFor(() => expect(submit).toBeEnabled())
    expect(screen.queryByTestId('primary-required')).toBeNull()
  })

  it('提交时把 primaryAgentId 一起发出去', async () => {
    const calls = stub()
    renderApp('/todos/new')

    await userEvent.type(await screen.findByLabelText('标题'), '给 webhook 重试加指数退避')
    await userEvent.click(screen.getByRole('radio', { name: '主 agent nova' }))
    await userEvent.click(screen.getByTestId('create-todo'))

    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST' && c.path === '/api/admin/todos')
      expect(post).toBeDefined()
      expect(JSON.parse(post!.body!)).toMatchObject({
        title: '给 webhook 重试加指数退避',
        primaryAgentId: mockDirectory.find((a) => a.name === 'nova')!.agentId,
      })
    })
  })

  it('正文里输入 @ 弹出提及下拉，选中只是拉人关注', async () => {
    stub()
    renderApp('/todos/new')
    const body = await screen.findByLabelText('正文')

    await userEvent.type(body, '看下 @ki')
    const list = await screen.findByTestId('mention-list')
    expect(list).toHaveTextContent('只是拉人关注，不指派')

    await userEvent.click(screen.getByRole('option', { name: /kilo/ }))
    await waitFor(() => expect((body as HTMLTextAreaElement).value).toContain('@kilo'))
    // 被 @ 的不会变成主 agent
    expect(screen.getByTestId('create-todo')).toBeDisabled()
  })
})
