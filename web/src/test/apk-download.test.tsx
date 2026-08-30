import { screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockSettings, mockTodos } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

const NO_SESSION = () => json({ code: 'unauthorized', message: '没有会话', retryable: false }, 401)

const HAS_APK = {
  available: true,
  version: '0.1.0',
  filename: 'agent-hub-0.1.0.apk',
  sizeBytes: 13_107_200,
  updatedAt: '2026-08-30T02:00:00Z',
}

describe('Android 客户端下载入口', () => {
  // 这是这个入口存在的**全部理由**。装 app 的那一刻用户手上还没有会话，
  // 而他很可能正是想在手机上登录才来装的 —— 入口只放在登录后的页面里，
  // 就成了「要先登录才能拿到用来登录的东西」。
  it('未登录的登录页上就有下载入口', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/login')

    const link = await screen.findByTestId('apk-download')
    expect(link).toBeInTheDocument()
    // 还停在登录页 —— 入口不是登录之后才出现的
    expect(screen.getByLabelText('用户名')).toBeInTheDocument()
  })

  // 路径写错了症状很轻微：/api/download 也能下到（hub 上是同一个处理器），
  // 所以肉眼看不出问题。但这是给人看、会被抄给别人的地址，必须是对外那一个。
  it('指向 /download —— 对外的正式地址，不是 /api/ 下那个同义词', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/login')

    const link = await screen.findByTestId('apk-download')
    expect(link.getAttribute('href')).toMatch(/\/download$/)
    expect(link.getAttribute('href')).not.toContain('/api/')
  })

  // 十几 MB 的文件要交给浏览器自己下：要进度条、要断点续传、手机上下完还要
  // 交给系统安装器。fetch 成 blob 再塞回 a[download] 会先在内存里攒完整个文件，
  // 手机上很容易被系统直接杀掉。所以它必须是一个真的 <a href>。
  it('是一个真的链接，不是 fetch 出来的 blob', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/login')

    const link = await screen.findByTestId('apk-download')
    expect(link.tagName).toBe('A')
    expect(link).toHaveAttribute('download')
  })

  it('把版本和体积写在按钮边上 —— 点之前就知道会得到什么', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/login')

    const entry = await screen.findByTestId('apk-entry')
    expect(entry).toHaveTextContent('v0.1.0')
    expect(entry).toHaveTextContent('12.5 MB')
  })

  // 没包时给禁用按钮加说明，而不是让人点下去拿到一段 JSON 错误。
  it('这台 hub 没发包时，入口是禁用加说明，不是一个会报错的按钮', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json({ available: false }),
    })
    renderApp('/login')

    expect(await screen.findByTestId('apk-unavailable')).toBeDisabled()
    expect(screen.queryByTestId('apk-download')).not.toBeInTheDocument()
    expect(screen.getByTestId('apk-entry')).toHaveTextContent('还没有发布安装包')
  })

  // hub 不可达时安静降级。在登录页上画一个红色报错块，会让人以为是自己账号出了问题 ——
  // 而真正要登录的那个表单就在旁边、完全正常。
  it('meta 端点挂了也不在登录页上报错', async () => {
    installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => new Response(null, { status: 500 }),
    })
    renderApp('/login')

    expect(await screen.findByTestId('apk-unavailable')).toBeDisabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('设置页上也有同一个入口', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/settings': () => json(mockSettings),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/settings')

    expect(await screen.findByTestId('apk-download')).toBeInTheDocument()
  })

  // 设计语言 §1.4：outbox 告警不可折叠、不可降级、不能被挪到二级页面。
  // 往设置页里加卡片时最容易犯的错，就是把新卡片插到告警前面。
  it('设置页加了下载卡之后，outbox 告警仍然排在它前面', async () => {
    installFetch({
      'GET /api/admin/me': () => json(ADMIN),
      'GET /api/admin/settings': () => json(mockSettings),
      'GET /api/admin/health': () => json({ ...HEALTHY, workerAlive: false, outboxLagSeconds: 900 }),
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/settings')

    const apk = await screen.findByTestId('apk-entry')
    const outbox = screen.getByTestId('outbox-card')
    // compareDocumentPosition：outbox 在 apk 之前 → FOLLOWING
    expect(outbox.compareDocumentPosition(apk) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  // 这个端点是公开的，登录页上就要打。它一旦被当成"要会话的接口"，
  // 401 会把未登录用户的入口整个吞掉 —— 而那正是最需要它的人。
  it('meta 是在没有会话的情况下打出去的', async () => {
    const calls = installFetch({
      'GET /api/admin/me': NO_SESSION,
      'GET /download/meta': () => json(HAS_APK),
    })
    renderApp('/login')
    await screen.findByTestId('apk-download')

    await waitFor(() => {
      expect(calls.some((c) => c.path === '/download/meta')).toBe(true)
    })
  })
})
