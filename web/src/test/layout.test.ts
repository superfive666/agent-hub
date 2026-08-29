import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const read = (p: string) => readFileSync(resolve(process.cwd(), p), 'utf8')
const css = read('src/styles/theme.css')
const shell = read('src/components/app-shell.tsx')
const thread = read('src/routes/thread.tsx')
const settings = read('src/routes/settings.tsx')

/**
 * 「板中有板，内板自己滚」这套构图有一个反复踩到的坑：
 * **flex 子项默认不会收缩到内容高度以下**。所以只写 overflow-y-auto 是不够的，
 * 缺了 min-height:0 的话内板会一路长高，滚的变成整个文档。
 *
 * 后果不是「滚动条位置不对」这种小事：
 *   - 输入框被推到几千像素以外，对话一长就得滚到底才能打字
 *   - 玻璃板被拉成内容那么高，backdrop-filter 铺在超大的层上，
 *     超过 Chromium 的纹理上限会整块不画
 *
 * 这个坑在本项目里已经踩到三次（看板的周日期带、消息流、右详情栏），
 * 而且**单元测试和类型检查都看不见** —— jsdom 不做布局。
 * 所以这里退一步，直接盯住让它成立的两条不变量。
 */
describe('内板滚动的两条不变量', () => {
  it('.stream 必须有 min-height:0，否则 overflow-y-auto 形同虚设', () => {
    expect(css).toMatch(/\.stream\s*\{[^}]*min-height\s*:\s*0/)
  })

  it('舞台必须被视口框住（h-dvh），不能用 min-h-dvh', () => {
    expect(shell).toMatch(/className="app h-dvh/)
    // min-h-dvh 会让舞台随内容长高，内板就永远没有「满了」的那一刻
    expect(shell).not.toMatch(/className="app min-h-dvh/)
  })
})

/**
 * 窄屏（<640）的两块玻璃板 —— 主板和底部 tab 条 —— 曾经互相穿插：
 * tab 条是 fixed 的，主板不知道它占了多高，只能拿 padding-bottom 去猜，
 * 猜出来的值和真实高度（还要加 safe-area）对不上；左右也对不齐，
 * 因为 tab 条贴视口、主板贴舞台内边距。
 *
 * 修法是让舞台在窄屏竖排，tab 条变成它的正常 flex 子项。
 * 和上面两条一样，**jsdom 不做布局，这个坑单测和类型检查都看不见**，
 * 所以这里同样退一步，盯住让它成立的两条不变量。
 */
describe('窄屏底部 tab 条不压主板', () => {
  it('舞台在 <640 竖排 —— tab 条要能排在主板下面而不是浮在上面', () => {
    expect(css).toMatch(/@media\s*\(max-width:\s*639px\)\s*\{[^}]*\.app\s*\{[^}]*flex-direction:\s*column/)
  })

  it('底部 tab 条不能是 fixed —— fixed 就没有 gap，只能靠 padding 去猜高度', () => {
    const nav = shell.match(/<nav[\s\S]*?>/)?.[0] ?? ''
    expect(nav, '没找到底部 tab 条').toContain('sm:hidden')
    expect(nav).not.toMatch(/\bfixed\b/)
  })
})

/**
 * 「右栏下沉」的坑（§4，640–1023）：右栏竖排到主内容下面时，
 * 它带着 shrink-0 就不许收缩，一千多像素的自然高度会把外面那条 flex 撑破，
 * 溢出的部分压在下面的输入区上 —— 看起来像输入框透进了别的卡片。
 * 必须靠 max-h 框住 + 自己滚。
 */
describe('下沉的右栏必须被框住', () => {
  for (const [name, src] of [['thread', thread], ['settings', settings]] as const) {
    it(`${name} 页的右栏有 max-h 与 overflow-y-auto，且 shrink-0 只在 lg 生效`, () => {
      const side = (src.match(/<Inset className="[^"]*"/g) ?? []).find((c) => c.includes('max-h-'))
      expect(side, '没找到带 max-h 的右栏').toBeDefined()
      expect(side!).toContain('overflow-y-auto')
      expect(side!).toContain('min-h-0')
      // 竖排时不许写裸 shrink-0；要不收缩只能挂在 lg: 上
      expect(side!.replace(/lg:shrink-0/g, '')).not.toContain('shrink-0')
    })
  }
})
