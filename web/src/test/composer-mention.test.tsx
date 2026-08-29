import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp, type Call } from './harness'
import { AGENT_IDS, mockDirectory, mockThread, mockTodos } from '@/mocks/data'

const thread = mockThread('th-0142')

afterEach(() => vi.unstubAllGlobals())

function stub(agents = mockDirectory) {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents }),
    'GET /api/admin/threads/th-0142': () => json(thread),
    'POST /api/admin/threads/th-0142/posts': () => json({ id: 'p-new' }, 201),
  })
}

const posts = (calls: Call[]) =>
  calls.filter((c) => c.method === 'POST' && c.path === '/api/admin/threads/th-0142/posts')

/**
 * 缺陷 3：thread 的回复框以前是个裸 textarea，placeholder 写着「输入 @ 把别的 agent
 * 拉进来关注」，敲下去却什么都不弹 —— 提及下拉早就实现好了，只是这里没用上。
 */
describe('对话页的回复框：@ 提及', () => {
  it('刚敲下 @ 还没打字时就把名录里的 agent 全摊开', async () => {
    stub()
    renderApp('/threads/th-0142')

    const composer = await screen.findByLabelText('回复这条 thread')
    await userEvent.type(composer, '@')

    const list = await screen.findByTestId('mention-list')
    expect(list).toHaveTextContent('只是拉人关注，不指派')
    expect(screen.getAllByRole('option')).toHaveLength(mockDirectory.length)
  })

  it('继续打字按名字收窄候选', async () => {
    stub()
    renderApp('/threads/th-0142')

    await userEvent.type(await screen.findByLabelText('回复这条 thread'), '@no')
    await waitFor(() => expect(screen.getAllByRole('option')).toHaveLength(1))
    expect(screen.getByRole('option')).toHaveTextContent('nova')
  })

  it('名录为空时不弹下拉 —— 没人可 @，弹一个空框只是噪音', async () => {
    stub([])
    renderApp('/threads/th-0142')

    await userEvent.type(await screen.findByLabelText('回复这条 thread'), '@')
    expect(screen.queryByTestId('mention-list')).toBeNull()
  })

  // 最容易写错的一处：composer 上 Enter 是"发送"，下拉开着时这一下必须先归下拉。
  it('下拉打开时方向键 + Enter 只选中候选项，不会把消息发出去', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')

    const composer = (await screen.findByLabelText('回复这条 thread')) as HTMLTextAreaElement
    await userEvent.type(composer, '@')
    await screen.findByTestId('mention-list')

    await userEvent.keyboard('{ArrowDown}{Enter}')

    // 第二个候选被选中，消息一条都没发出去
    await waitFor(() => expect(composer.value).toBe('@nova '))
    expect(posts(calls)).toHaveLength(0)
    expect(screen.queryByTestId('mention-list')).toBeNull()
  })

  it('下拉关掉之后 Enter 才是发送', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')

    const composer = await screen.findByLabelText('回复这条 thread')
    await userEvent.type(composer, '收到，我看一下')
    await userEvent.keyboard('{Enter}')

    await waitFor(() => expect(posts(calls)).toHaveLength(1))
    expect(JSON.parse(posts(calls)[0].body!)).toMatchObject({ body: '收到，我看一下' })
  })

  it('输入法组词时的 Enter 是"选候选词"，不是发送', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')

    const composer = await screen.findByLabelText('回复这条 thread')
    await userEvent.type(composer, '收到')
    // 中文输入法还在组词：这一下 Enter 属于输入法，误判一次就是把半句话发出去
    fireEvent.keyDown(composer, { key: 'Enter', isComposing: true })

    expect(posts(calls)).toHaveLength(0)
  })

  it('发送时把正文里 @ 到的 agent 解析成 mentions 一起提交', async () => {
    const calls = stub()
    renderApp('/threads/th-0142')

    const composer = await screen.findByLabelText('回复这条 thread')
    await userEvent.type(composer, '@')
    await screen.findByTestId('mention-list')
    await userEvent.click(await screen.findByRole('option', { name: /kilo/ }))
    await userEvent.type(composer, '帮忙接一下告警 @查不到这个人')
    await userEvent.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(posts(calls)).toHaveLength(1))
    const sent = JSON.parse(posts(calls)[0].body!)
    // 名录里匹配得上的才进 mentions；匹配不上的 @xxx 忽略掉，不报错
    expect(sent.mentions).toEqual([AGENT_IDS.kilo])
    expect(sent.body).toContain('@kilo')
  })
})
