import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const repo = (p: string) => resolve(process.cwd(), '..', p)
const bytes = (p: string) => readFileSync(repo(p))

const ICONS = ['favicon.svg', 'favicon.ico', 'apple-touch-icon.png']

/**
 * 站标只有一份源文件：`docs/design/brand/`。
 *
 * 三个站的分发方式不一样 —— api-docs 有构建步骤，`build.mjs` 直接从源文件拷，
 * 不会漂；web 和 developer-docs 都是静态托管，各自目录里必须放一份实体副本
 * （developer-docs 明确是零构建的，见它的 README）。**副本就会漂**：
 * 改了图标只更新了控制台，文档站还挂着上一版，是那种没人会专门去看、
 * 但一眼就露馅的不一致。所以这里按字节比一遍。
 *
 * 改图标的正确姿势：改 `docs/design/brand/` 里的源文件，然后把三个文件
 * 拷到 `web/public/` 和 `developer-docs/assets/`。
 */
describe('三个站的站标必须是同一份', () => {
  for (const f of ICONS) {
    it(`${f} 在 web/public 与源文件一致`, () => {
      expect(bytes(`web/public/${f}`).equals(bytes(`docs/design/brand/${f}`))).toBe(true)
    })
    it(`${f} 在 developer-docs/assets 与源文件一致`, () => {
      expect(bytes(`developer-docs/assets/${f}`).equals(bytes(`docs/design/brand/${f}`))).toBe(true)
    })
  }

  // 声明漏一条不会报错，只会在某类浏览器上悄悄退回默认图标。
  it('控制台的 index.html 三种图标都声明了', () => {
    const html = bytes('web/index.html').toString('utf8')
    expect(html).toMatch(/rel="icon"[^>]*favicon\.svg/)
    expect(html).toMatch(/favicon\.ico/)
    expect(html).toMatch(/rel="apple-touch-icon"/)
  })

  it('developer-docs 每一页都声明了图标，不是只有首页', () => {
    for (const page of ['index', 'tiers', 'agent-card', 'collaboration', 'faq']) {
      const html = bytes(`developer-docs/${page}.html`).toString('utf8')
      expect(html, `${page}.html`).toMatch(/rel="icon"[^>]*favicon\.svg/)
      expect(html, `${page}.html`).toMatch(/rel="apple-touch-icon"/)
    }
  })
})
