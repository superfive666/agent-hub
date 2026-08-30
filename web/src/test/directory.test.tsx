import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AdminAgent, AgentSummary } from '@/api/client'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockAdminAgents, mockDirectory } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

const ORIN_ID = '8c9d0e1f-2a3b-4c5d-9e6f-708192a3b4c5'

/** 只建了记录、还没接入的 agent —— 名录（Card 摘要）里查不到它 */
const ORIN: AdminAgent = {
  agentId: ORIN_ID,
  name: 'orin',
  purpose: '接手夜间巡检',
  status: 'pending_registration',
  online: false,
  hasCard: false,
  openTodos: 0,
  createdAt: '2026-08-28T11:30:00+08:00',
}

function stub(opts: {
  directory?: AgentSummary[]
  agents?: AdminAgent[]
  token?: () => Response
} = {}) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: opts.directory ?? [] }),
    'GET /api/admin/agents': () => json({ agents: opts.agents ?? [] }),
    [`POST /api/admin/agents/${ORIN_ID}/registration-token`]:
      opts.token ??
      (() =>
        json(
          { registrationToken: 'ahr_reissued_5b7d0e2f', expiresAt: '2026-08-30T11:30:00+08:00' },
          201,
        )),
  })
}

describe('名录页的空态', () => {
  it('一个 agent 都没有时引导去添加，而不是渲染一片空白', async () => {
    stub()
    renderApp('/directory')

    const empty = await screen.findByTestId('directory-empty')
    expect(empty).toHaveTextContent('平台上还没有任何 agent')
    expect(screen.getByRole('link', { name: '添加第一个 agent' })).toHaveAttribute(
      'href',
      '/directory/new',
    )
    // 被「只看在线」筛没了是另一回事，这时候不该出现
    expect(screen.queryByTestId('directory-filtered-empty')).toBeNull()
  })

  it('有 agent 但被「只看在线」筛没了时，提示切回全部 —— 不能让人以为 agent 消失了', async () => {
    // **在线与否以 /api/admin/agents 为准**（它才知道 status；名录看不见停用的），
    // 所以造「全都离线」要造在运维那份上。
    const offline = mockAdminAgents.map((a) => ({ ...a, online: false }))
    stub({ directory: mockDirectory, agents: offline })
    renderApp('/directory')

    await screen.findAllByTestId('agent-card')
    await userEvent.click(screen.getByRole('button', { name: '全部' }))

    const filtered = await screen.findByTestId('directory-filtered-empty')
    expect(filtered).toHaveTextContent('只看在线')
    expect(filtered).toHaveTextContent(`${offline.length} 个 agent 全筛掉了`)
    // 「一个 agent 都没有」的引导不能在这里冒出来
    expect(screen.queryByTestId('directory-empty')).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: '切回全部' }))
    await waitFor(() => expect(screen.queryByTestId('directory-filtered-empty')).toBeNull())
    expect(screen.getAllByTestId('agent-card').length).toBe(
      offline.filter((a) => a.hasCard).length,
    )
  })
})

describe('名录页上的两份数据', () => {
  it('刚建出来的 agent 名录里查不到，但页面上必须看得见（未接入那一栏）', async () => {
    // 名录（Card 摘要）里没有 orin，运维视角里有 —— 这正是「我明明加了却找不到」的来源
    stub({ directory: mockDirectory, agents: [...mockAdminAgents.slice(0, 1), ORIN] })
    renderApp('/directory')

    const rows = await screen.findAllByTestId('offstage-row')
    const orin = rows.find((r) => r.textContent?.includes('orin'))!
    expect(orin).toBeDefined()
    expect(orin).toHaveTextContent('未接入')
    expect(orin).toHaveTextContent('还没拿注册 token 换过长期凭证')
    // 名录那一栏解释自己只收录写了 Card 的
    expect(screen.getByTestId('offstage')).toHaveTextContent('名录只收录写了 Agent Card 的 agent')
  })

  it('给未接入的 agent 重新签发 token，明文当场展示并带「只出现一次」的警告', async () => {
    const calls = stub({ directory: mockDirectory, agents: [ORIN] })
    renderApp('/directory')

    await userEvent.click(
      await screen.findByRole('button', { name: '给 orin 重新签发注册 token' }),
    )

    const panel = await screen.findByTestId('registration-token')
    expect(screen.getByTestId('token-text')).toHaveTextContent('ahr_reissued_5b7d0e2f')
    expect(panel).toHaveTextContent('明文只出现这一次')
    expect(
      calls.some(
        (c) => c.method === 'POST' && c.path === `/api/admin/agents/${ORIN_ID}/registration-token`,
      ),
    ).toBe(true)
  })

  it('被停用的 agent 在名录卡片上要标出来 —— Card 摘要里看不出这件事', async () => {
    const rover = mockDirectory[0]
    stub({
      directory: [rover],
      agents: [{ agentId: rover.agentId, name: rover.name, status: 'disabled', hasCard: true }],
    })
    renderApp('/directory')

    const card = await screen.findByTestId('agent-card')
    expect(card).toHaveTextContent('已停用')
  })

  it('已接入但还没写 Card 的 agent 也要列出来，并说清是它自己还没写', async () => {
    const mu: AdminAgent = { ...ORIN, agentId: 'mu-1', name: 'mu', status: 'active', hasCard: false }
    stub({ directory: [], agents: [mu] })
    renderApp('/directory')

    const row = await screen.findByTestId('offstage-row')
    expect(row).toHaveTextContent('已接入')
    expect(row).toHaveTextContent('还没写 Agent Card')
    // 没写 Card 的不该被算成「重新签发 token」的候选：它已经有长期凭证了
    expect(screen.queryByRole('button', { name: /重新签发/ })).toBeNull()
    // 名录本身是空的，但原因和「一个 agent 都没有」不同，要分开说
    expect(screen.getByTestId('directory-nocard-empty')).toHaveTextContent('名录还是空的')
    expect(screen.queryByTestId('directory-empty')).toBeNull()
  })
})

/**
 * 停用之后这个 agent 在控制台上必须还看得见 —— 否则连重新启用的入口都没有。
 *
 * 以前这一页主栏铺的是 `/api/admin/directory`，而那个接口刻意不收停用的
 * （它是给 agent 看的「该找谁」，别人不该 @ 到一个已经下线的）。
 * 于是停用一个写过 Card 的 agent，它就从页面上凭空蒸发了。
 */
describe('名录页对停用 agent 的处理', () => {
  const disabled = mockAdminAgents.map((a, i) =>
    i === 0 ? { ...a, status: 'disabled' as const, online: false } : a,
  )

  it('停用的 agent 仍然列出来，并标出「已停用」', async () => {
    stub({ directory: mockDirectory, agents: disabled })
    renderApp('/directory')

    const cards = await screen.findAllByTestId('agent-card')
    const one = cards.find((c) => c.textContent?.includes(disabled[0].name!))
    expect(one, '停用的 agent 不能从页面上消失 —— 那样就没有重新启用的入口了').toBeDefined()
    expect(one!).toHaveTextContent('已停用')
  })

  it('「只看在线」把停用的筛掉 —— 它的凭证已经失效，不可能在线', async () => {
    stub({ directory: mockDirectory, agents: disabled })
    renderApp('/directory')
    await screen.findAllByTestId('agent-card')

    await userEvent.click(screen.getByRole('button', { name: '全部' }))
    const cards = await screen.findAllByTestId('agent-card')
    expect(cards.some((c) => c.textContent?.includes(disabled[0].name!))).toBe(false)
  })

  it('过滤器管整页，不是只管名录那一栏', async () => {
    // mu / orin 都没写 Card，落在「还没出现在名录里」那一栏，两个都是离线。
    // 以前那一栏完全不受过滤器管，切到「只看在线」照样列着 —— 按钮看着就像没生效。
    stub({ directory: mockDirectory, agents: mockAdminAgents })
    renderApp('/directory')
    await screen.findAllByTestId('agent-card')

    expect(screen.getAllByTestId('offstage-row').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('button', { name: '全部' }))
    await waitFor(() => expect(screen.queryAllByTestId('offstage-row')).toHaveLength(0))
  })
})
