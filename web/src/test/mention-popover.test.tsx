import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { MentionTextarea } from '@/components/mention-textarea'
import { caretPoint, caretViewportPoint } from '@/lib/caret'
import type { AgentSummary } from '@/api/client'

const AGENTS = [
  { agentId: 'a1', name: 'ubuntu-warrior', runtime: 'claude-code', tier: 'longpoll', online: true },
  { agentId: 'a2', name: 'nova', runtime: 'codex', tier: 'longpoll', online: false },
] as unknown as AgentSummary[]

function Harness() {
  return <MentionTextarea value="" onChange={() => {}} agents={AGENTS} aria-label="正文" />
}

describe('@ 提及的 inline popover', () => {
  /**
   * 这个控件的全部意义是「提示就在你打字的地方」。挂在输入框左下角的话，
   * 在第三行行尾敲的 `@` 会把提示甩到十几厘米外 —— 眼睛得离开光标去找它。
   * jsdom 量不出真实坐标，所以这里查的是**位置由内联样式给出**，
   * 而不是回到 `left-4 / top-full` 那种和光标无关的静态类。
   */
  it('位置写在内联样式上，不是钉死在输入框角上的静态类', async () => {
    render(<Harness />)
    await userEvent.type(screen.getByLabelText('正文'), '@')

    const pop = await screen.findByTestId('mention-list')
    expect(pop.style.left).not.toBe('')
    expect(pop.style.top).not.toBe('')
    expect(pop.className).not.toMatch(/\bleft-4\b|\btop-full\b|\bbottom-full\b/)
  })

  /**
   * 玻璃板 `.pane` 是 `overflow:hidden`（§2 的构图靠它裁圆角）。popover 挂在
   * 输入框旁边的话，只要比板子高出一点就被切掉半截 —— composer 就在板子底部，
   * 那是必然发生而不是可能发生。所以它必须 portal 到 body，
   * 且**不能**是输入框容器的后代，否则任何祖先的 overflow 都能把它裁掉。
   */
  it('portal 到 body，不在输入框的容器里 —— 否则会被玻璃板裁掉', async () => {
    const { container } = render(<Harness />)
    await userEvent.type(screen.getByLabelText('正文'), '@')

    const pop = await screen.findByTestId('mention-list')
    expect(container.contains(pop)).toBe(false)
    expect(pop.parentElement).toBe(document.body)
  })

  /** 视口坐标 = 输入框的 rect + 框内偏移。少加一项，popover 会落到页面别处。 */
  it('视口坐标把输入框自己的位置加了进去', () => {
    const ta = document.createElement('textarea')
    ta.value = 'hi'
    document.body.appendChild(ta)
    // jsdom 的 getBoundingClientRect 恒为 0，自己塞一个非零的进去
    ta.getBoundingClientRect = () => ({ left: 120, top: 240 }) as DOMRect

    const inner = caretPoint(ta, 2)
    const vp = caretViewportPoint(ta, 2)
    expect(vp.left).toBe(120 + inner.left)
    expect(vp.top).toBe(240 + inner.top)
    ta.remove()
  })

  /**
   * 镜像 div 是每次定位都新建一个。忘了摘掉的话，每敲一个字就往 body 上挂一个
   * 隐藏 div —— 页面不会有任何异常表现，只是越用越慢。这种泄漏没别的地方看得见。
   */
  it('量完光标不在 body 上留下镜像节点', () => {
    const before = document.body.childElementCount
    const ta = document.createElement('textarea')
    ta.value = '一行字\n第二行 @no'
    document.body.appendChild(ta)

    for (let i = 0; i <= ta.value.length; i++) {
      const pt = caretPoint(ta, i)
      // 量不出来也必须是数字：NaN 进了 style.left 会让整块 popover 定位失效
      expect(Number.isFinite(pt.left)).toBe(true)
      expect(Number.isFinite(pt.top)).toBe(true)
      expect(pt.lineHeight).toBeGreaterThan(0)
    }

    ta.remove()
    expect(document.body.childElementCount).toBe(before)
  })
})

describe('popover 的观感', () => {
  const css = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')
  const bare = css.replace(/\/\*[\s\S]*?\*\//g, '')
  const rule = bare.split('}').find((r) => r.includes('.mention-pop{')) ?? ''

  /**
   * 毛玻璃是「面板」的语言。这块浮在正文之上，底下是密排的文字，
   * 透上来会和候选人的名字叠在一起 —— 读一行选一行的列表不该需要辨认。
   */
  it('是实底，没有 backdrop-filter', () => {
    expect(rule).toContain('.mention-pop{')
    expect(rule).not.toMatch(/backdrop-filter/)
    expect(rule).toContain('background:var(--menu-bg)')
  })

  it('--menu-bg 在两个主题下都是不透明色', () => {
    const vals = [...bare.matchAll(/--menu-bg:\s*([^;]+);/g)].map((m) => m[1].trim())
    expect(vals).toHaveLength(2) // 亮色一份、暗色一份
    for (const v of vals) {
      expect(v).toMatch(/^(#[0-9a-f]{6}|rgb\()/i)
      expect(v).not.toMatch(/rgba|gradient/)
    }
  })
})
