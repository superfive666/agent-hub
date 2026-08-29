import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { HEALTHY, installFetch, json, renderApp } from './harness'
import { mockDirectory, mockSettings, mockTodos } from '@/mocks/data'
import { initialsOf, maskEmail } from '@/lib/format'

afterEach(() => vi.unstubAllGlobals())

const LONG = 'wuchao900726@gmail.com'
const OIDC_ADMIN = { username: LONG, authMode: 'oidc', timezone: 'Asia/Singapore' }

function stub(me: Record<string, unknown>) {
  return installFetch({
    'GET /api/admin/me': () => json(me),
    'GET /api/admin/todos': () => json({ todos: mockTodos }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: mockDirectory }),
    'GET /api/admin/settings': () => json(mockSettings),
  })
}

describe('maskEmail：长邮箱要缩短，但不能让人认不出账号', () => {
  it('本地部分超过阈值时打码，域名一个字符都不动', () => {
    expect(maskEmail(LONG)).toBe('wuchao**@gmail.com')
    expect(maskEmail('averyveryverylongname@some.company.example')).toBe(
      'averyv**@some.company.example',
    )
  })

  it('本地部分够短的邮箱原样返回，不做无谓的遮挡', () => {
    expect(maskEmail('bob@gmail.com')).toBe('bob@gmail.com')
    expect(maskEmail('wuchao@gmail.com')).toBe('wuchao@gmail.com')
  })

  it('口令模式下的普通用户名不是邮箱，绝不打码', () => {
    expect(maskEmail('superfive')).toBe('superfive')
    expect(maskEmail('a-very-long-admin-username')).toBe('a-very-long-admin-username')
  })

  it('空、undefined、以及不成邮箱的怪串都安全', () => {
    expect(maskEmail(undefined)).toBe('')
    expect(maskEmail('')).toBe('')
    expect(maskEmail('@gmail.com')).toBe('@gmail.com')
    expect(maskEmail('wuchao900726@')).toBe('wuchao900726@')
  })

  it('头像缩写照旧对邮箱取前两位', () => {
    expect(initialsOf(LONG)).toBe('WU')
  })
})

describe('侧栏与设置页画的是打码后的账号，完整值仍然拿得到', () => {
  it('OIDC 的长邮箱在侧栏被缩短，完整值挂在 title 上', async () => {
    stub(OIDC_ADMIN)
    renderApp('/threads')

    expect(await screen.findByText('wuchao**@gmail.com')).toBeInTheDocument()
    expect(screen.queryByText(LONG)).toBeNull()
    // 缩短只是缩短，用户还得能确认自己登的是哪个账号
    expect(screen.getByTitle(LONG)).toHaveTextContent('wuchao**@gmail.com')
    // 省略不是布局保证：窄屏兜底的 truncate 也得在
    expect(screen.getByTitle(LONG).className).toContain('truncate')
  })

  it('设置页的「账号」同样打码，且带完整值', async () => {
    stub(OIDC_ADMIN)
    renderApp('/settings')

    await screen.findByText('账号')
    const shown = screen.getAllByTitle(LONG)
    expect(shown.length).toBeGreaterThanOrEqual(1)
    expect(shown.every((el) => el.textContent === 'wuchao**@gmail.com')).toBe(true)
  })

  it('口令模式的短用户名原样显示，不被打码', async () => {
    stub({ username: 'superfive', authMode: 'password', timezone: 'Asia/Singapore' })
    renderApp('/settings')

    await screen.findByText('账号')
    const shown = screen.getAllByTitle('superfive')
    expect(shown.length).toBeGreaterThanOrEqual(1)
    expect(shown.every((el) => el.textContent === 'superfive')).toBe(true)
    expect(screen.queryByText(/\*\*/)).toBeNull()
  })
})
