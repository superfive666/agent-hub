import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AdminAgent } from '@/api/client'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'

afterEach(() => vi.unstubAllGlobals())

const ID = '8c9d0e1f-2a3b-4c5d-9e6f-708192a3b4c5'

const ROW: AdminAgent = {
  agentId: ID,
  name: 'orin',
  purpose: '接手夜间巡检',
  status: 'pending_registration',
  online: false,
  hasCard: false,
  openTodos: 0,
  createdAt: '2026-08-28T11:30:00+08:00',
}

function stub(over: {
  row?: Partial<AdminAgent>
  patch?: () => Response
  del?: () => Response
} = {}) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: [] }),
    'GET /api/admin/agents': () => json({ agents: [{ ...ROW, ...over.row }] }),
    [`PATCH /api/admin/agents/${ID}`]: over.patch ?? (() => json({ agentId: ID })),
    [`DELETE /api/admin/agents/${ID}`]: over.del ?? (() => new Response(null, { status: 204 })),
  })
}

/** 找出打到这个 agent 上的某个方法的调用 */
const callsTo = (calls: ReturnType<typeof installFetch>, method: string) =>
  calls.filter((c) => c.method === method && c.path === `/api/admin/agents/${ID}`)

/** 「还没进名录」那一栏里的那张卡，管理按钮都挂在它上面 */
async function row() {
  return within(await screen.findByTestId('offstage-row'))
}

describe('管理已有的 agent', () => {
  it('能改简介 —— 保存时只发 purpose，不带 enabled', async () => {
    const calls = stub({ patch: () => json({ agentId: ID }) })
    renderApp('/directory')

    const r = await row()
    await userEvent.click(r.getByTestId('edit-purpose'))
    const input = r.getByTestId('purpose-input')
    await userEvent.clear(input)
    await userEvent.type(input, '改成白天巡检')
    await userEvent.click(r.getByRole('button', { name: /保存/ }))

    await waitFor(() => expect(callsTo(calls, 'PATCH')).toHaveLength(1))
    const sent = JSON.parse(String(callsTo(calls, 'PATCH')[0].body))
    expect(sent).toEqual({ purpose: '改成白天巡检' })
    // 只改简介不能顺手把 agent 停掉 —— enabled 必须整个不出现，
    // 带一个 false 上去后端就会真的把它停了
    expect(sent).not.toHaveProperty('enabled')
  })

  it('界面上没有改名的入口 —— 改名会让历史正文里的 @old-name 静默失效', async () => {
    stub()
    renderApp('/directory')

    const r = await row()
    await userEvent.click(r.getByTestId('edit-purpose'))
    // 编辑态下只有简介一个输入框，没有名字输入框
    expect(r.queryByLabelText('名称')).toBeNull()
    expect(r.queryByRole('textbox', { name: /名称|名字/ })).toBeNull()
    expect(r.getByTestId('purpose-input')).toBeInTheDocument()
  })

  it('停用发的是 enabled:false，并说清「凭证此刻就认证不过」但可逆', async () => {
    const calls = stub({ patch: () => json({ agentId: ID, status: 'disabled' }) })
    renderApp('/directory')

    const r = await row()
    await userEvent.click(r.getByTestId('toggle-enabled'))

    await waitFor(() => expect(callsTo(calls, 'PATCH')).toHaveLength(1))
    expect(JSON.parse(String(callsTo(calls, 'PATCH')[0].body))).toEqual({ enabled: false })
  })

  it('已停用的 agent 上按钮变成「重新启用」，并解释它此刻是下线的', async () => {
    stub({ row: { status: 'disabled' } })
    renderApp('/directory')

    const r = await row()
    expect(r.getByTestId('toggle-enabled')).toHaveTextContent('重新启用')
    expect(r.getByText(/此刻就认证不过/)).toBeInTheDocument()
    expect(r.getByText(/凭证还留着/)).toBeInTheDocument()
  })

  it('删除要按两下 —— 第一下只是确认，不发请求', async () => {
    const calls = stub()
    renderApp('/directory')

    const r = await row()
    await userEvent.click(r.getByTestId('delete-agent'))
    expect(r.getByTestId('confirm-delete')).toBeInTheDocument()
    expect(callsTo(calls, 'DELETE')).toHaveLength(0)

    await userEvent.click(r.getByTestId('confirm-delete'))
    await waitFor(() => expect(callsTo(calls, 'DELETE')).toHaveLength(1))
  })

  /**
   * 409 agent_in_use 不是「出错了」，是这个操作的正常结果之一。
   * 界面必须把它翻译成「改用停用」，并说清卡在哪 —— 只说「删除失败」
   * 等于让用户自己去库里猜为什么。
   */
  it('有历史的 agent 删不掉时，给出计数并指向「停用」，而不是一句删除失败', async () => {
    stub({
      del: () =>
        json(
          {
            code: 'agent_in_use',
            message: '这个 agent 已经在内容里留下痕迹',
            retryable: false,
            refs: { todos: 3, tweets: 1, steps: 7 },
          },
          409,
        ),
    })
    renderApp('/directory')

    const r = await row()
    await userEvent.click(r.getByTestId('delete-agent'))
    await userEvent.click(r.getByTestId('confirm-delete'))

    const box = await screen.findByTestId('agent-in-use')
    expect(box).toHaveTextContent('3 条 todo')
    expect(box).toHaveTextContent('1 条广播')
    expect(box).toHaveTextContent('7 条处理步骤')
    expect(box).toHaveTextContent('改用「停用」')
    // 不要再叠一句干巴巴的「删除失败」，上面那块已经解释清楚了
    expect(screen.queryByText(/没能删除/)).toBeNull()
  })
})
