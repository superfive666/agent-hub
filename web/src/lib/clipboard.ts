/**
 * 复制到剪贴板，带降级路径。
 *
 * **不能只写 `navigator.clipboard.writeText`。** 那个 API 只在安全上下文里存在 ——
 * 局域网 http:// 部署（这个平台大概率就跑在物理机的内网地址上）、老浏览器、
 * 以及 jsdom 测试环境里，`navigator.clipboard` 根本是 undefined，直接调会抛
 * `TypeError`，按钮看起来点了没反应。而它复制的是**只出现一次的注册 token**，
 * 悄悄失败的代价是用户得作废重发。
 *
 * 三层：异步 API → `execCommand('copy')`（已废弃但到处都还能用）→ 返回 false
 * 让调用方提示「手动选中下面这串」。**token 本身永远是选得中的文本**，
 * 复制按钮只是省事，不是唯一出路。
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 权限被拒 / 不在安全上下文 —— 往下走降级路径
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    // 放在视口内但不可见：iOS 上 display:none 的元素选不中
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none'
    document.body.appendChild(ta)
    ta.select()
    const ok = typeof document.execCommand === 'function' && document.execCommand('copy')
    document.body.removeChild(ta)
    return !!ok
  } catch {
    return false
  }
}
