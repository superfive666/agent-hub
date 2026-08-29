import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, noContent, renderApp } from './harness'
import { OIDC_START_PATH } from '@/api/client'
import { mockDirectory, mockTodos } from '@/mocks/data'

afterEach(() => vi.unstubAllGlobals())

describe('登录页', () => {
  it('凭据不对时把后端那句话展示出来，且不区分用户名/密码', async () => {
    installFetch({
      'GET /api/admin/me': () => json({ code: 'unauthorized', message: '没有会话', retryable: false }, 401),
      'POST /api/admin/login': () =>
        json({ code: 'invalid_credentials', message: '凭据不对', retryable: false }, 401),
    })
    renderApp('/login')

    await userEvent.type(await screen.findByLabelText('用户名'), 'someone')
    await userEvent.type(screen.getByLabelText('密码'), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: '进入控制台' }))

    const err = await screen.findByTestId('login-error')
    expect(err).toHaveTextContent('凭据不对')
    // 还留在登录页
    expect(screen.getByLabelText('用户名')).toBeInTheDocument()
  })

  it('后端没给 message 时也要有话说，不能是空白告警', async () => {
    installFetch({
      'GET /api/admin/me': () => new Response(null, { status: 401 }),
      'POST /api/admin/login': () => new Response(null, { status: 401 }),
    })
    renderApp('/login')
    await userEvent.type(await screen.findByLabelText('用户名'), 'a')
    await userEvent.type(screen.getByLabelText('密码'), 'b')
    await userEvent.click(screen.getByRole('button', { name: '进入控制台' }))
    expect(await screen.findByTestId('login-error')).toHaveTextContent('只有预置的那个账号')
  })

  it('登录请求带上凭据，成功后进入控制台', async () => {
    let loggedIn = false
    const calls = installFetch({
      'GET /api/admin/me': () =>
        loggedIn ? json(ADMIN) : json({ code: 'unauthorized', message: '没有会话', retryable: false }, 401),
      'POST /api/admin/login': () => {
        loggedIn = true
        return noContent()
      },
      'GET /api/admin/todos': () => json({ todos: mockTodos }),
      'GET /api/admin/health': () => json(HEALTHY),
      'GET /api/admin/directory': () => json({ agents: mockDirectory }),
      'GET /api/admin/board': () => json({ groupBy: 'activity', items: [] }),
    })
    renderApp('/login')

    await userEvent.type(await screen.findByLabelText('用户名'), 'superfive')
    await userEvent.type(screen.getByLabelText('密码'), 'hunter2')
    await userEvent.click(screen.getByRole('button', { name: '进入控制台' }))

    // 会话是 HttpOnly Cookie —— 请求必须带 credentials，否则登录成功也白搭
    const post = calls.find((c) => c.method === 'POST' && c.path === '/api/admin/login')
    expect(post?.body).toContain('superfive')
    expect(post?.credentials).toBe('include')

    expect(await screen.findByText('唯一管理员')).toBeInTheDocument()
  })
})

describe('Google OIDC 入口', () => {
  const stubLoggedOut = () =>
    installFetch({
      'GET /api/admin/me': () =>
        json({ code: 'unauthorized', message: '没有会话', retryable: false }, 401),
    })

  it('未登录时拿不到 authMode，所以两种入口都摆着', async () => {
    stubLoggedOut()
    renderApp('/login')
    expect(await screen.findByRole('radio', { name: '密码' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Google 账号' })).toBeInTheDocument()
  })

  it('切到 Google 账号后给出一个整页跳转的链接，不是 fetch', async () => {
    const calls = stubLoggedOut()
    renderApp('/login')

    await userEvent.click(await screen.findByRole('radio', { name: 'Google 账号' }))

    const link = await screen.findByTestId('oidc-start')
    // 必须是 <a href>：302 后面的跨域跳转和 Set-Cookie，fetch 拿不到
    expect(link.tagName).toBe('A')
    expect(link.getAttribute('href')).toMatch(new RegExp(`${OIDC_START_PATH}$`))
    // 口令表单让位，免得同时出现两套提交
    expect(screen.queryByLabelText('用户名')).toBeNull()
    expect(screen.queryByRole('button', { name: '进入控制台' })).toBeNull()

    // 这条路上一个 XHR 都不该发（真点下去 jsdom 会喊 "navigation not implemented"，
    // 那反而正说明它是整页跳转）
    expect(calls.some((c) => c.path === OIDC_START_PATH)).toBe(false)
    expect(calls.some((c) => c.path === '/api/admin/login')).toBe(false)
  })

  it('切回密码模式，口令表单原样回来', async () => {
    stubLoggedOut()
    renderApp('/login')
    await userEvent.click(await screen.findByRole('radio', { name: 'Google 账号' }))
    await screen.findByTestId('oidc-start')
    await userEvent.click(screen.getByRole('radio', { name: '密码' }))
    expect(await screen.findByLabelText('用户名')).toBeInTheDocument()
    expect(screen.queryByTestId('oidc-start')).toBeNull()
  })
})
