import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Bubble } from '@/components/ui/bubble'
import { MessageRow } from '@/components/message-row'
import type { Post, ThreadDetail } from '@/api/client'

const ROVER = '5f6a7f0e-9f4d-4b0a-8b1e-1f2c3d4e5a6b'
const NOVA = '9c1b2a3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d'

const thread: Pick<ThreadDetail, 'primaryAgentId' | 'watchers'> = {
  primaryAgentId: ROVER,
  watchers: [
    { agentId: ROVER, name: 'rover', reason: 'primary', online: true },
    { agentId: NOVA, name: 'nova', reason: 'mentioned', online: true },
  ],
}

const humanPost: Post = {
  id: 'p1',
  threadId: 'th-1',
  authorKind: 'admin',
  authorName: 'superfive',
  createdAt: '2026-08-28T09:14:00+08:00',
  body: '改成指数退避加抖动 @nova',
}
const primaryPost: Post = {
  id: 'p2',
  threadId: 'th-1',
  authorKind: 'agent',
  authorId: ROVER,
  authorName: 'rover',
  createdAt: '2026-08-28T09:21:00+08:00',
  body: '两个问题先确认',
}
const watcherPost: Post = {
  id: 'p3',
  threadId: 'th-1',
  authorKind: 'agent',
  authorId: NOVA,
  authorName: 'nova',
  createdAt: '2026-08-28T09:52:00+08:00',
  body: '补一句：建议不超过 30s',
}

describe('Bubble 三态', () => {
  it('me / primary / watch 各自落到自己的样式类', () => {
    const { rerender } = render(<Bubble tone="me">人</Bubble>)
    expect(screen.getByText('人')).toHaveClass('bub', 'bub-me')

    rerender(<Bubble tone="primary">主</Bubble>)
    expect(screen.getByText('主')).toHaveClass('bub', 'bub-pri')

    // 关注者：虚线透明底，重量最轻 —— 这是语义，不是装饰
    rerender(<Bubble tone="watch">关注</Bubble>)
    expect(screen.getByText('关注')).toHaveClass('bub', 'bub-watch')
  })
})

describe('§1.1 人和 agent 的四重区分信号', () => {
  it('人类：靠右 + 暖橘实底 + 无 @ 前缀 + 「人类」chip，四重都在', () => {
    render(<MessageRow post={humanPost} thread={thread} />)
    const row = screen.getByTestId('message-row')
    // 1 位置
    expect(row).toHaveClass('msg-me')
    // 2 气泡底色
    expect(row.querySelector('.bub')).toHaveClass('bub-me')
    // 3 名字不带 @ 前缀
    expect(within(row).getByText('superfive')).toBeInTheDocument()
    expect(within(row).queryByText('@superfive')).toBeNull()
    // 4 「人类」chip
    expect(within(row).getByText('人类')).toBeInTheDocument()
  })

  it('authorKind 是唯一依据：admin 一定靠右，agent 一定靠左', () => {
    const { rerender } = render(<MessageRow post={humanPost} thread={thread} />)
    expect(screen.getByTestId('message-row').dataset.human).toBe('true')
    rerender(<MessageRow post={primaryPost} thread={thread} />)
    expect(screen.getByTestId('message-row').dataset.human).toBe('false')
  })

  it('主 agent：靠左 + 青边气泡 + @ 前缀 +「主 agent」chip', () => {
    render(<MessageRow post={primaryPost} thread={thread} />)
    const row = screen.getByTestId('message-row')
    expect(row).not.toHaveClass('msg-me')
    expect(row.querySelector('.bub')).toHaveClass('bub-pri')
    expect(within(row).getByText('@rover')).toBeInTheDocument()
    expect(within(row).getByText('主 agent')).toBeInTheDocument()
    expect(within(row).queryByText('人类')).toBeNull()
  })

  it('关注者：靠左 + 虚线气泡 +「关注」chip，没有主 agent 的分量', () => {
    render(<MessageRow post={watcherPost} thread={thread} />)
    const row = screen.getByTestId('message-row')
    expect(row).not.toHaveClass('msg-me')
    expect(row.querySelector('.bub')).toHaveClass('bub-watch')
    expect(within(row).getByText('@nova')).toBeInTheDocument()
    expect(within(row).getByText('关注')).toBeInTheDocument()
  })

  it('正文里的 @name 被解析成 mention', () => {
    render(<MessageRow post={humanPost} thread={thread} />)
    expect(screen.getByText('@nova')).toHaveClass('at')
  })
})
