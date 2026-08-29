import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const read = (p: string) => readFileSync(resolve(process.cwd(), p), 'utf8')
const css = read('src/styles/theme.css')
const shell = read('src/components/app-shell.tsx')

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
