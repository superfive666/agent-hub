import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory } from '@/mocks/data'
import { dateTimeLabel } from '@/lib/format'

afterEach(() => vi.unstubAllGlobals())

const CREATED = {
  agentId: '8c9d0e1f-2a3b-4c5d-9e6f-708192a3b4c5',
  registrationToken: 'ahr_9f3c1d7b6a4e2058c1d9',
  expiresAt: '2026-08-30T11:30:00+08:00',
}

function stub(create?: () => Response) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/agents': () => json({ agents: [] }),
    'POST /api/admin/agents': create ?? (() => json(CREATED, 201)),
  })
}

describe('添加 agent', () => {
  it('名称带不合法字符时给出人话提示，并且不让提交', async () => {
    stub()
    renderApp('/directory/new')

    const name = await screen.findByLabelText('名称')
    const submit = screen.getByTestId('create-agent')
    // 什么都没填：不给提交，但也不标红（还没开始填就骂人）
    expect(submit).toBeDisabled()
    expect(screen.queryByTestId('name-invalid')).toBeNull()

    await userEvent.type(name, '夜间巡检 bot')
    const hint = await screen.findByTestId('name-invalid')
    // 提示要说清「为什么」——@ 提及靠这个字符集匹配
    expect(hint).toHaveTextContent('字母、数字、下划线和连字符')
    expect(hint).toHaveTextContent('@')
    expect(submit).toBeDisabled()

    await userEvent.clear(name)
    await userEvent.type(name, 'orin-01')
    await waitFor(() => expect(submit).toBeEnabled())
    expect(screen.queryByTestId('name-invalid')).toBeNull()
  })

  it('名称超过 64 个字符也拦下来', async () => {
    stub()
    renderApp('/directory/new')

    await userEvent.type(await screen.findByLabelText('名称'), 'a'.repeat(65))
    expect(await screen.findByTestId('name-invalid')).toHaveTextContent('最多 64 个字符')
    expect(screen.getByTestId('create-agent')).toBeDisabled()
  })

  it('提交时带 issueToken 一步拿到 token，不再单独签一次', async () => {
    const calls = stub()
    renderApp('/directory/new')

    await userEvent.type(await screen.findByLabelText('名称'), 'orin')
    await userEvent.type(screen.getByLabelText('简介 / 用途'), '跑在书房那台小机器上')
    await userEvent.click(screen.getByTestId('create-agent'))

    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST' && c.path === '/api/admin/agents')
      expect(post).toBeDefined()
      expect(JSON.parse(post!.body!)).toEqual({
        name: 'orin',
        purpose: '跑在书房那台小机器上',
        issueToken: true,
      })
    })
    // 不许再走「先建记录、再单独签一张」那条两次往返的路
    expect(calls.some((c) => c.path.endsWith('/registration-token'))).toBe(false)
  })

  it('创建成功后当场展示 token，并写明明文只出现这一次', async () => {
    stub()
    renderApp('/directory/new')

    await userEvent.type(await screen.findByLabelText('名称'), 'orin')
    await userEvent.click(screen.getByTestId('create-agent'))

    const panel = await screen.findByTestId('registration-token')
    expect(screen.getByTestId('token-text')).toHaveTextContent(CREATED.registrationToken)
    // 这条警告是后端的真实行为（库里只有哈希），不是吓唬人
    expect(panel).toHaveTextContent('明文只出现这一次')
    expect(panel).toHaveTextContent('关掉这个页面就再也看不到了，只能作废重发一张')
    // 过期时间要写出来：token 24 小时有效。
    // 期望值走 dateTimeLabel 拼，别写死「8月30日 11:30」—— 那是 +08:00 的读法，
    // 跑在 UTC 的 CI 上会变成 03:30，写死只会得到一个和时区较劲的假失败。
    expect(panel).toHaveTextContent(`${dateTimeLabel(CREATED.expiresAt)} 过期`)
    // 接入命令照抄 docs/08-deployment.md §8，不是编的
    expect(screen.getByTestId('onboard-command')).toHaveTextContent(
      'sh ~/agent-hub/agent-hub-skill/scripts/onboard.sh',
    )
    expect(screen.getByTestId('onboard-command')).toHaveTextContent(
      `REG_TOKEN=${CREATED.registrationToken}`,
    )
  })

  it('复制按钮在没有 clipboard API 的环境里也不崩 —— 降级路径要真的存在', async () => {
    stub()
    // jsdom 里 navigator.clipboard 本来就不存在，等价于非 HTTPS 部署的情况
    const exec = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })
    renderApp('/directory/new')

    await userEvent.type(await screen.findByLabelText('名称'), 'orin')
    await userEvent.click(screen.getByTestId('create-agent'))
    await screen.findByTestId('registration-token')

    await userEvent.click(screen.getByRole('button', { name: '复制注册 token' }))
    await waitFor(() => expect(exec).toHaveBeenCalledWith('copy'))
    expect(await screen.findByText('已复制')).toBeInTheDocument()
  })

  it('服务端 409 撞名时把错误展示出来 —— 前端拦不住重名', async () => {
    stub(() =>
      json(
        { code: 'agent_name_taken', message: '名称 rover 已被占用，换一个再试', retryable: false },
        409,
      ),
    )
    renderApp('/directory/new')

    await userEvent.type(await screen.findByLabelText('名称'), 'rover')
    await userEvent.click(screen.getByTestId('create-agent'))

    const err = await screen.findByTestId('create-agent-error')
    expect(err).toHaveTextContent('名称 rover 已被占用，换一个再试')
    // 失败了就不该出现 token 那一屏
    expect(screen.queryByTestId('registration-token')).toBeNull()
  })
})
