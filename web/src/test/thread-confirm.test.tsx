import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ThreadDetail, TodoStep } from '@/api/client'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { AGENT_IDS, mockDirectory, mockThread, mockTodos } from '@/mocks/data'
import { progressOf } from '@/lib/format'

afterEach(() => vi.unstubAllGlobals())

/** 还没被确认的 todo：闸门关着，主 agent 只能提问 */
const unconfirmed: ThreadDetail = {
  ...mockThread('th-0140'),
  status: 'clarifying',
  confirmedAt: null,
}

/**
 * 故意乱序返回，用来证明界面自己按 seq 升序画 ——
 * 「第几步」是这块界面的全部意义，顺序错了整块就是错的。
 */
const steps: TodoStep[] = [
  {
    id: 's3',
    threadId: 'th-0140',
    seq: 3,
    kind: 'blocked',
    title: '等 hermes 那边给 webhook 地址',
    status: 'blocked',
    actorKind: 'agent',
    actorAgentId: AGENT_IDS.kilo,
    actorName: 'kilo',
    createdAt: '2026-08-28T11:00:00+08:00',
    updatedAt: '2026-08-28T11:00:00+08:00',
  },
  {
    id: 's1',
    threadId: 'th-0140',
    seq: 1,
    kind: 'clarification',
    title: '告警要发到哪个通道',
    detail: '先问清楚再动手，免得接完发现方向不对。',
    status: 'done',
    actorKind: 'agent',
    actorAgentId: AGENT_IDS.kilo,
    actorName: 'kilo',
    createdAt: '2026-08-28T09:30:00+08:00',
    updatedAt: '2026-08-28T09:40:00+08:00',
  },
  {
    id: 's2',
    threadId: 'th-0140',
    seq: 2,
    kind: 'plan',
    title: '接 outbox_lag 到值班通道',
    status: 'in_progress',
    actorKind: 'agent',
    actorAgentId: AGENT_IDS.kilo,
    actorName: 'kilo',
    createdAt: '2026-08-28T10:00:00+08:00',
    updatedAt: '2026-08-28T10:00:00+08:00',
  },
]

function stub(thread: ThreadDetail, stepList: TodoStep[] = steps, state?: () => Response) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    [`GET /api/admin/threads/${thread.threadId}`]: () => json(thread),
    [`GET /api/admin/todos/${thread.threadId}/steps`]: () => json({ steps: stepList }),
    [`POST /api/admin/todos/${thread.threadId}/state`]:
      state ?? (() => json({ status: 'in_progress', confirmedAt: '2026-08-28T12:00:00+08:00' })),
  })
}

describe('用户确认闸门', () => {
  it('confirmedAt 为空时显示「确认需求，开工」，并说清在此之前 agent 只会提问', async () => {
    stub(unconfirmed)
    renderApp('/threads/th-0140')

    // 右详情栏一份 + <640px 顶部一份，同一个组件、同一个 mutation
    const gates = await screen.findAllByTestId('confirm-gate')
    expect(gates.length).toBeGreaterThanOrEqual(1)
    expect(gates[0]).toHaveTextContent('在你确认之前，agent 只会提问和澄清，不会开始做')
    expect(screen.queryByTestId('confirmed-at')).toBeNull()
  })

  it('点确认发出的是 action: approve，不是 confirm（那是确认「完成」）', async () => {
    const calls = stub(unconfirmed)
    renderApp('/threads/th-0140')

    await userEvent.click((await screen.findAllByTestId('approve-todo'))[0])

    await waitFor(() => {
      const post = calls.find(
        (c) => c.method === 'POST' && c.path === '/api/admin/todos/th-0140/state',
      )
      expect(post).toBeDefined()
      expect(JSON.parse(post!.body!)).toEqual({ action: 'approve' })
    })
  })

  it('已确认的 todo 不再画确认按钮，改成写出确认时刻', async () => {
    // mockThread('th-0142') 是 awaiting_review，confirmedAt 有值
    const confirmed = mockThread('th-0142')
    expect(confirmed.confirmedAt).toBeTruthy()
    stub(confirmed)
    renderApp('/threads/th-0142')

    await screen.findByTestId('confirmed-at')
    expect(screen.queryByTestId('confirm-gate')).toBeNull()
    expect(screen.queryByTestId('approve-todo')).toBeNull()
  })

  it('确认是进度条上一个看得见的节点，夹在「澄清中」和「进行中」之间', async () => {
    stub(unconfirmed)
    renderApp('/threads/th-0140')

    const progress = await screen.findByTestId('progress-card')
    const t = progress.textContent ?? ''
    expect(t).toContain('需求确认')
    expect(t.indexOf('需求确认')).toBeGreaterThan(t.indexOf('澄清中'))
    expect(t.indexOf('需求确认')).toBeLessThan(t.indexOf('进行中'))
  })

  // 确认不是一个 status（ADR-0008 明确拒绝了新增状态），所以它只是画出来的一个节点：
  // 不传 confirmedAt 的老调用方拿到的还是契约里那 5 个状态，一个不多。
  it('progressOf 不传 confirmedAt 时行为不变 —— 状态机没有多出一个状态', () => {
    expect(progressOf('in_progress').map((p) => p.label)).toEqual([
      '待响应',
      '澄清中',
      '进行中',
      '待确认',
      '已完成',
    ])
    expect(progressOf('in_progress', null).map((p) => p.label)).toContain('需求确认')
    expect(progressOf('in_progress', '2026-08-28T12:00:00+08:00').find((p) => p.label === '需求确认')?.state).toBe('done')
    expect(progressOf('clarifying', null).find((p) => p.label === '需求确认')?.state).toBe('todo')
  })

  it('打回被服务端 409 挡回来时，错误要显示出来而不是静默', async () => {
    const awaiting: ThreadDetail = { ...mockThread('th-0142'), status: 'awaiting_review' }
    stub(awaiting, steps, () =>
      json(
        { code: 'invalid_todo_transition', message: '当前状态不允许打回', retryable: false },
        409,
      ),
    )
    renderApp('/threads/th-0142')

    await userEvent.click(await screen.findByRole('button', { name: '打回，继续做' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('当前状态不允许打回')
  })
})

describe('处理步骤', () => {
  it('按 seq 升序渲染，kind 和 status 显示的是中文而不是英文枚举', async () => {
    stub(unconfirmed)
    renderApp('/threads/th-0140')

    // 步骤面板在 DOM 里有两份：右详情栏一份，窄屏的可展开抽屉一份
    // （右栏在 <640px 整块 hidden，不补一份手机上就看不到过程）。
    // 两份是同一个组件、同一份数据，断言取第一份即可。
    const rows = (await screen.findAllByTestId('step-row')).slice(0, 3)
    expect(rows).toHaveLength(3)
    // 接口乱序给的，界面自己排
    expect(rows.map((r) => r.textContent?.slice(0, 1))).toEqual(['1', '2', '3'])
    expect(rows[0]).toHaveTextContent('澄清')
    expect(rows[0]).toHaveTextContent('已完成')
    expect(rows[1]).toHaveTextContent('计划')
    expect(rows[1]).toHaveTextContent('进行中')
    expect(rows[2]).toHaveTextContent('受阻')
    expect(rows[2]).toHaveTextContent('卡住了')
    // 英文枚举一个都不许漏出去
    const text = screen.getAllByTestId('steps-card')[0].textContent ?? ''
    for (const en of ['clarification', 'plan', 'blocked', 'in_progress', 'pending', 'done']) {
      expect(text).not.toContain(en)
    }
  })

  it('hub 写的确认步骤标成 hub 记录，agent 记的挂 @ 名字', async () => {
    const withConfirmation: TodoStep[] = [
      ...steps,
      {
        id: 's4',
        threadId: 'th-0140',
        seq: 4,
        kind: 'confirmation',
        title: '管理员确认需求，放行开工',
        status: 'done',
        actorKind: 'admin',
        createdAt: '2026-08-28T12:00:00+08:00',
        updatedAt: '2026-08-28T12:00:00+08:00',
      },
    ]
    stub(unconfirmed, withConfirmation)
    renderApp('/threads/th-0140')

    const rows = await screen.findAllByTestId('step-row')
    expect(rows[3]).toHaveTextContent('确认放行')
    expect(rows[3]).toHaveTextContent('hub 记录')
    expect(rows[0]).toHaveTextContent('@kilo')
  })

  it('没有步骤时说清这里将来会有什么，而不是留一片空白', async () => {
    stub(unconfirmed, [])
    renderApp('/threads/th-0140')

    const empties = await screen.findAllByTestId('steps-empty')
    expect(empties[0]).toHaveTextContent('澄清、计划、进展、卡点、交付物')
    expect(screen.queryByTestId('step-row')).toBeNull()
  })

  /**
   * 右详情栏在 <640px 整块是 hidden 的。闸门补了一份到顶部（它是动作，
   * 点不到就等于 todo 卡死），步骤是读物 —— 摊开会把消息流挤没，所以折成抽屉。
   */
  it('窄屏够不着右栏，所以步骤另有一个可展开的抽屉', async () => {
    stub(unconfirmed)
    renderApp('/threads/th-0140')

    const drawer = await screen.findByTestId('steps-drawer')
    expect(drawer.tagName).toBe('DETAILS')
    // 折叠态也要能看出里面有几条，否则没人知道值不值得点开。
    // 条数要等步骤那个 query 落地才出现，所以这里必须 waitFor —— 抽屉本身
    // 在 query 还在飞的时候就已经渲染出来了。
    await waitFor(() =>
      expect(drawer.querySelector('summary')).toHaveTextContent('处理步骤 · 3'),
    )
    // 抽屉里是同一个组件，不是另抄一份
    expect(within(drawer).getByTestId('steps-card')).toBeInTheDocument()
  })

  it('tweet 没有步骤，不对着它发注定 404 的请求', async () => {
    const tweet: ThreadDetail = {
      threadId: 'tw-0031',
      kind: 'tweet',
      startedAt: '2026-08-28T10:02:00+08:00',
      watchers: [],
      posts: [],
    }
    const calls = installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /api/admin/threads/tw-0031': () => json(tweet),
    })
    renderApp('/threads/tw-0031')

    await waitFor(() => expect(calls.some((c) => c.path === '/api/admin/threads/tw-0031')).toBe(true))
    expect(calls.some((c) => c.path.includes('/steps'))).toBe(false)
    expect(screen.queryByTestId('steps-card')).toBeNull()
    expect(screen.queryByTestId('confirm-gate')).toBeNull()
  })
})
