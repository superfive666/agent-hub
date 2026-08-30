/**
 * 量出 textarea 里某个光标位置在屏幕上的落点。
 *
 * **为什么要做这件事**：`@` 下拉如果固定挂在输入框左下角，你在第三行行尾敲的 `@`，
 * 提示会跑到十几厘米外的左下方 —— 眼睛得离开正在打字的位置去找它，再挪回来。
 * inline popover 的全部意义就是「就在光标底下」，位置错了这个控件就等于没做。
 *
 * **为什么只能靠镜像 div**：DOM 没有任何 API 能给出 textarea 内部的光标坐标
 * （`Range`/`getClientRects` 只对可编辑元素成立，textarea 的文本节点不可达）。
 * 唯一可行的做法是造一个字体、宽度、padding、换行规则都一致的 div，把光标之前的
 * 文本放进去，再量紧随其后那个标记的位置 —— 换行、比例字体、CJK 混排都自动对上。
 */

/** 影响换行与字形的属性。少抄一条就会在某种输入下偏掉，所以宁可抄多。 */
const MIRRORED = [
  'box-sizing', 'width', 'border-top-width', 'border-right-width', 'border-bottom-width',
  'border-left-width', 'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
  'font-family', 'font-size', 'font-weight', 'font-style', 'font-variant', 'font-stretch',
  'letter-spacing', 'word-spacing', 'line-height', 'text-indent', 'text-transform',
  'text-rendering', 'word-break', 'overflow-wrap', 'tab-size',
] as const

export interface CaretPoint {
  /** 光标所在行的左边缘，相对 textarea 的**边框盒**（已减掉横向滚动） */
  left: number
  /** 光标所在行的**上**沿，相对边框盒（已减掉纵向滚动） */
  top: number
  /** 这一行的高度。往下开就落在 top+lineHeight，往上开就贴着 top */
  lineHeight: number
}

/** 光标在**视口**里的位置。popover 用 position:fixed 挂在 body 上，要的是这个。 */
export function caretViewportPoint(el: HTMLTextAreaElement, caret: number): CaretPoint {
  const pt = caretPoint(el, caret)
  const r = el.getBoundingClientRect()
  return { left: r.left + pt.left, top: r.top + pt.top, lineHeight: pt.lineHeight }
}

function num(v: string, fallback: number): number {
  const n = Number.parseFloat(v)
  return Number.isFinite(n) ? n : fallback
}

/**
 * jsdom 不做布局：`offsetLeft` 恒为 0，`getComputedStyle` 多数值是空串。
 * 这里不为测试环境加分支 —— 所有取值都走 `num()` 兜底，量不出来就退回 `{0,0}`，
 * 下拉贴在输入框左上角，功能一点不少。**位置是增强，可用性不依赖它。**
 */
export function caretPoint(el: HTMLTextAreaElement, caret: number): CaretPoint {
  const cs = getComputedStyle(el)
  const lineHeight = num(cs.lineHeight, num(cs.fontSize, 14) * 1.5)

  const mirror = document.createElement('div')
  for (const p of MIRRORED) mirror.style.setProperty(p, cs.getPropertyValue(p))
  // textarea 的换行规则：软换行保留、空白保留。用 pre-wrap 才能和它一致。
  mirror.style.whiteSpace = 'pre-wrap'
  mirror.style.overflowWrap = cs.overflowWrap || 'break-word'
  mirror.style.position = 'absolute'
  mirror.style.top = '0'
  mirror.style.left = '-9999px'
  mirror.style.visibility = 'hidden'
  mirror.style.height = 'auto'
  mirror.style.overflow = 'hidden'

  mirror.textContent = el.value.slice(0, caret)
  const mark = document.createElement('span')
  // 标记里必须有内容才有盒子。放光标之后的第一个字符，让它跟着真实换行走；
  // 光标在末尾就放一个不换行空格，宽度不重要，我们只取它的左上角。
  mark.textContent = el.value.slice(caret, caret + 1) || ' '
  mirror.appendChild(mark)

  document.body.appendChild(mirror)
  const left = mark.offsetLeft
  const top = mark.offsetTop
  document.body.removeChild(mirror)

  return {
    left: left - el.scrollLeft,
    top: top - el.scrollTop,
    lineHeight,
  }
}
