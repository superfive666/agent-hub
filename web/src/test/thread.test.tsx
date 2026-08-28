import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import ThreadRoute from '@/routes/thread'
import { thread } from '@/mocks/thread'

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <ThreadRoute />
    </QueryClientProvider>,
  )
}

describe('对话页', () => {
  it('渲染出舞台 → 玻璃板 → 嵌套内板三层构图', () => {
    const { container } = renderPage()
    expect(container.querySelector('.app')).toBeInTheDocument()
    // 两块玻璃板：左会话栏 + 主区
    expect(container.querySelectorAll('.pane').length).toBeGreaterThanOrEqual(2)
    // 板中有板：消息流 + 右详情
    expect(container.querySelectorAll('.inset').length).toBeGreaterThanOrEqual(2)
  })

  it('把 mock 里的每条帖子都渲染成一行，人和 agent 分列两侧', () => {
    renderPage()
    const rows = screen.getAllByTestId('message-row')
    expect(rows).toHaveLength(thread.posts.filter((p) => !('system' in p)).length)
    expect(rows.filter((r) => r.dataset.human === 'true').length).toBe(2)
    expect(rows.every((r) => (r.dataset.human === 'true') === r.classList.contains('msg-me'))).toBe(
      true,
    )
  })

  it('流光只出现在主 agent 卡片与当前会话上（§1.3）', () => {
    const { container } = renderPage()
    const glowing = Array.from(container.querySelectorAll('.glow'))
    expect(glowing).toHaveLength(2)
    expect(glowing.some((el) => el.classList.contains('convo'))).toBe(true)
    expect(glowing.some((el) => el.classList.contains('card'))).toBe(true)
  })
})
