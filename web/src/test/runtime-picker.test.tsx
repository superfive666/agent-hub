import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'

afterEach(() => vi.unstubAllGlobals())

function stub() {
  return installFetch({
    'GET /api/admin/me': () => json(ADMIN),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/directory': () => json({ agents: [] }),
    'GET /api/admin/agents': () => json({ agents: [] }),
    'POST /api/admin/agents': () =>
      json(
        {
          agentId: '11111111-2222-3333-4444-555555555555',
          registrationToken: 'ahr_test_9f8e7d6c',
          expiresAt: '2026-08-30T11:30:00+08:00',
        },
        201,
      ),
  })
}

/** 建一个 agent 并走到展示 token 那一屏 */
async function createAgent(name = 'orin') {
  await userEvent.type(await screen.findByLabelText(/名称/), name)
  await userEvent.click(screen.getByRole('button', { name: /创建/ }))
  return screen.findByTestId('onboard-prompt')
}

describe('runtime 选择器', () => {
  it('是一组 radio 而不是下拉菜单 —— 选项本身带信息量，藏起来用户选完才发现还差东西', async () => {
    stub()
    renderApp('/directory/new')

    const picker = await screen.findByTestId('runtime-picker')
    expect(picker).toHaveAttribute('role', 'radiogroup')
    // 七个 runtime 全都摆在外面，不是点开才看得到
    expect(within(picker).getAllByRole('radio')).toHaveLength(7)
    expect(screen.queryByRole('combobox')).toBeNull()
  })

  /**
   * 这是用户报的那个 bug：给出去的那段东西永远写死 codex，
   * 跑 claude-code 的人拿过去必然对不上。
   */
  it('选中的 runtime 会写进给 agent 的 prompt，不再永远是 codex', async () => {
    stub()
    renderApp('/directory/new')

    const picker = await screen.findByTestId('runtime-picker')
    // 默认就是 claude-code，不是 codex
    expect(within(picker).getByTestId('runtime-claude-code')).toHaveAttribute('aria-checked', 'true')

    const p = await createAgent()
    expect(p).toHaveTextContent('你的 runtime：claude-code')
    expect(p).not.toHaveTextContent('runtime：codex')
  })

  it('用的是全称 claude-code，不是产品名 claude —— 照着它去查文档才对得上', async () => {
    stub()
    renderApp('/directory/new')
    const p = await createAgent()
    expect(p.textContent).toMatch(/claude-code\b/)
  })

  it('换成 codex 之后 prompt 跟着变', async () => {
    stub()
    renderApp('/directory/new')

    const picker = await screen.findByTestId('runtime-picker')
    await userEvent.click(within(picker).getByTestId('runtime-codex'))
    await waitFor(() =>
      expect(within(picker).getByTestId('runtime-codex')).toHaveAttribute('aria-checked', 'true'),
    )

    const p = await createAgent()
    expect(p).toHaveTextContent('你的 runtime：codex')
  })

  /**
   * 常驻服务型还需要 agent 那边的 webhook 地址，缺了接不完。
   * agent 得在动手之前就知道这件事，而不是走到一半卡住。
   */
  it('选常驻服务型时 prompt 里点名还需要 webhook 地址', async () => {
    stub()
    renderApp('/directory/new')

    const picker = await screen.findByTestId('runtime-picker')
    await userEvent.click(within(picker).getByTestId('runtime-hermes'))
    // 形态提示要当场变，用户在建之前就该知道还要准备一个 webhook URL
    expect(await screen.findByTestId('runtime-hint')).toHaveTextContent('常驻服务')

    const p = await createAgent()
    expect(p).toHaveTextContent('你的 runtime：hermes')
    expect(p).toHaveTextContent('webhook 地址')
  })

  /**
   * 这段话是复制给 agent 的，不是给人在终端里跑的 —— 接入该由 agent 自己完成。
   * 它里面不该出现「你去 clone 一下、你去执行一下」这类要人代劳的命令行。
   */
  it('给出的是 prompt 不是命令行：带 hub 地址、token 和说明 URL，让 agent 自己去读', async () => {
    stub()
    renderApp('/directory/new')
    const p = await createAgent()
    const text = p.textContent ?? ''

    expect(text).toContain('ahr_test_9f8e7d6c')
    expect(text).toMatch(/\/api\/onboarding/)
    // 三件事都要交代到，尤其是「保持在线」—— 少了它 agent 注册完就没人拉 inbox
    expect(text).toContain('长期凭证')
    expect(text).toContain('保持在线')
    expect(text).toContain('Agent Card')
    // 步骤不内联在这段话里，是让 agent 去 URL 读的
    expect(text).not.toContain('curl')
    expect(text).not.toContain('git clone')
  })

  it('方向键能在选项间走 —— radiogroup 是一个控件，不是七个 Tab 停靠点', async () => {
    stub()
    renderApp('/directory/new')

    const picker = await screen.findByTestId('runtime-picker')
    const first = within(picker).getByTestId('runtime-claude-code')
    first.focus()
    await userEvent.keyboard('{ArrowRight}')
    await waitFor(() =>
      expect(within(picker).getByTestId('runtime-codex')).toHaveAttribute('aria-checked', 'true'),
    )
    // 未选中的项不进 Tab 序
    expect(within(picker).getByTestId('runtime-claude-code')).toHaveAttribute('tabindex', '-1')
  })
})
