import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Bubble } from '@/components/ui/bubble'
import { MessageRow } from '@/components/message-row'
import type { Post } from '@/mocks/thread'

const humanPost: Post = {
  postId: 'p1',
  at: '09:14',
  createdAt: '2026-08-28T09:14:00+08:00',
  author: { name: 'superfive', initials: '李', isHuman: true },
  body: '改成指数退避加抖动 @nova',
}
const primaryPost: Post = {
  postId: 'p2',
  at: '09:21',
  createdAt: '2026-08-28T09:21:00+08:00',
  author: { name: 'rover', initials: 'RO', isHuman: false, participation: 'primary', online: true },
  body: '两个问题先确认',
}
const watcherPost: Post = {
  postId: 'p3',
  at: '09:52',
  createdAt: '2026-08-28T09:52:00+08:00',
  author: { name: 'nova', initials: 'NO', isHuman: false, participation: 'watcher', online: true },
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
    render(<MessageRow post={humanPost} />)
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

  it('主 agent：靠左 + 青边气泡 + @ 前缀 +「主 agent」chip', () => {
    render(<MessageRow post={primaryPost} />)
    const row = screen.getByTestId('message-row')
    expect(row).not.toHaveClass('msg-me')
    expect(row.querySelector('.bub')).toHaveClass('bub-pri')
    expect(within(row).getByText('@rover')).toBeInTheDocument()
    expect(within(row).getByText('主 agent')).toBeInTheDocument()
    expect(within(row).queryByText('人类')).toBeNull()
  })

  it('关注者：靠左 + 虚线气泡 +「关注」chip，没有主 agent 的分量', () => {
    render(<MessageRow post={watcherPost} />)
    const row = screen.getByTestId('message-row')
    expect(row).not.toHaveClass('msg-me')
    expect(row.querySelector('.bub')).toHaveClass('bub-watch')
    expect(within(row).getByText('@nova')).toBeInTheDocument()
    expect(within(row).getByText('关注')).toBeInTheDocument()
  })

  it('正文里的 @name 被解析成 mention', () => {
    render(<MessageRow post={humanPost} />)
    expect(screen.getByText('@nova')).toHaveClass('at')
  })
})
