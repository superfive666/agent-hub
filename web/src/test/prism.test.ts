import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// vitest 跑在 jsdom 环境里，import.meta.url 不是 file: 协议，所以从工作目录解析。
const css = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')

/**
 * 「遮罩出一圈渐变描边」这个写法有个静默的坑。
 *
 * 描边是 conic-gradient 铺满整个盒子、再用 mask-composite 抠掉中间那块得到的。
 * 但动画改的是注册过的自定义属性 --ang，Chromium 会因此把伪元素扔到另一条合成路径，
 * 而那条路径**不应用 mask-composite** —— 遮罩失效，整块 conic 直接铺满。
 * conic 的色标边界是从圆心射出的直线，所以症状是几条锐利的斜线横穿整个界面。
 *
 * 加 transform:translateZ(0) 能把它钉在会应用遮罩的那条路径上。
 * 这条用例盯着这个约束：**任何带 mask-composite 又在跑 spin 动画的规则，都必须带 translateZ**。
 * 单元测试看不见像素，所以这里查的是规则本身。
 */
describe('棱镜描边的合成路径', () => {
  // 先把注释去掉再拆规则。上面那段警告注释里就写着 translateZ(0) 这几个字，
  // 不去掉的话，声明被删了这条用例也照样通过 —— 查的是注释，不是代码。
  const bare = css.replace(/\/\*[\s\S]*?\*\//g, '')
  const rules = bare.split('}').map((r) => r + '}')

  const masked = rules.filter(
    (r) => /mask-composite\s*:\s*(exclude|xor)/.test(r) && /animation\s*:\s*spin/.test(r),
  )

  it('确实存在这类规则（选择器改名了也要能发现）', () => {
    expect(masked.length).toBeGreaterThanOrEqual(3)
  })

  it('每一条都带 translateZ(0)，否则遮罩会静默失效', () => {
    const bad = masked.filter((r) => !/translateZ\(0\)/.test(r))
    expect(
      bad.map((r) => r.slice(0, 90).replace(/\s+/g, ' ')),
      '这些规则会让 conic-gradient 铺满整个盒子，界面上出现横穿全屏的斜线',
    ).toEqual([])
  })
})
