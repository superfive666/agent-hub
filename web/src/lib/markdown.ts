/**
 * 帖子正文的 markdown 解析。
 *
 * **为什么自己写而不是引一个库**：正文来自 agent，是不可信输入。用 markdown 库
 * 就要连着引一个 sanitizer，还要一直盯着它俩的版本 —— 而我们需要的只是 agent
 * 实际会吐的那个子集。这里的输出是一棵**纯数据的树**，渲染层只画文本节点，
 * 天然没有 HTML 注入面：markdown 里的 `<script>` 在这里只是一段普通文字。
 *
 * 覆盖的子集（`docs/07-design-language.md` 之外的东西一律不认）：
 * 标题、围栏代码块、有序/无序列表、引用、段落；行内是粗体、斜体、行内代码、
 * 链接、裸 URL，以及 `@mention`（这一条不是 markdown，是我们自己的）。
 *
 * **段落里的换行保留成硬换行。** 标准 markdown 会把单个换行折叠成空格，
 * 但这里的正文是聊天发言不是文档 —— agent 分行写的清单、日志、错误信息，
 * 折叠之后会糊成一坨，那是把可读性输给了规范。
 */

export type Span =
  | { kind: 'text'; text: string }
  | { kind: 'mention'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'strong'; children: Span[] }
  | { kind: 'em'; children: Span[] }
  | { kind: 'link'; href: string; children: Span[] }

export type Block =
  | { kind: 'p'; spans: Span[] }
  | { kind: 'heading'; level: 1 | 2 | 3; spans: Span[] }
  | { kind: 'code'; lang?: string; text: string }
  | { kind: 'list'; ordered: boolean; items: Span[][] }
  | { kind: 'quote'; spans: Span[] }

const FENCE = /^\s*```+\s*([A-Za-z0-9_+-]*)\s*$/
const HEADING = /^(#{1,6})\s+(.*)$/
const BULLET = /^\s*[-*+]\s+(.*)$/
const ORDERED = /^\s*\d{1,9}[.)]\s+(.*)$/
const QUOTE = /^\s*>\s?(.*)$/

/** 正文 → 块序列。任何输入都要有输出：解析不出结构的行就是一个段落。 */
export function parseMarkdown(body: string): Block[] {
  const lines = body.replace(/\r\n?/g, '\n').split('\n')
  const blocks: Block[] = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    if (!line.trim()) {
      i++
      continue
    }

    const fence = FENCE.exec(line)
    if (fence) {
      const lang = fence[1] || undefined
      const buf: string[] = []
      i++
      // 没有收尾的 ``` 时一直吃到结尾：agent 的输出被截断是常事，
      // 把剩下的当正文画出来，好过整段消失。
      while (i < lines.length && !FENCE.test(lines[i])) buf.push(lines[i++])
      if (i < lines.length) i++
      blocks.push({ kind: 'code', lang, text: buf.join('\n') })
      continue
    }

    const heading = HEADING.exec(line)
    if (heading) {
      // 只有三级：气泡里再小就跟正文分不开了，h4 以下一律并到 h3。
      const level = Math.min(heading[1].length, 3) as 1 | 2 | 3
      blocks.push({ kind: 'heading', level, spans: inline(heading[2]) })
      i++
      continue
    }

    if (BULLET.test(line) || ORDERED.test(line)) {
      const ordered = !BULLET.test(line)
      const items: Span[][] = []
      while (i < lines.length) {
        const m = ordered ? ORDERED.exec(lines[i]) : BULLET.exec(lines[i])
        // 同一段里换了记号（`-` 变 `1.`）就断开另起一个列表，
        // 否则序号会接到上一段的编号后面，看起来像漏了几条。
        if (!m) break
        items.push(inline(m[1]))
        i++
      }
      blocks.push({ kind: 'list', ordered, items })
      continue
    }

    if (QUOTE.test(line)) {
      const buf: string[] = []
      while (i < lines.length && QUOTE.test(lines[i])) buf.push(QUOTE.exec(lines[i++])![1])
      blocks.push({ kind: 'quote', spans: inline(buf.join('\n')) })
      continue
    }

    const buf: string[] = []
    while (
      i < lines.length &&
      lines[i].trim() &&
      !FENCE.test(lines[i]) &&
      !HEADING.test(lines[i]) &&
      !BULLET.test(lines[i]) &&
      !ORDERED.test(lines[i]) &&
      !QUOTE.test(lines[i])
    ) {
      buf.push(lines[i++])
    }
    blocks.push({ kind: 'p', spans: inline(buf.join('\n')) })
  }

  return blocks
}

const INLINE_CODE = /`([^`\n]+)`/
const STRONG = /\*\*([^\n]+?)\*\*|__([^\n]+?)__/
const EM = /(?<![*\w])\*([^*\n]+?)\*(?!\*)|(?<![_\w])_([^_\n]+?)_(?![\w_])/
const LINK = /\[([^\]\n]*)\]\(([^)\s]+)\)/
const BARE_URL = /https?:\/\/[^\s<>()[\]]+/
/** 与 mentionedAgentIds 的收边一致：`@nova` 不能把 `@nova2` 也吃进去。 */
const MENTION = /@[A-Za-z0-9_-]+/

/**
 * 行内解析。顺序即优先级，**行内代码必须排第一** ——
 * `` `**x**` `` 里的星号是代码内容，不是加粗记号。
 */
export function inline(text: string): Span[] {
  if (!text) return []

  const code = INLINE_CODE.exec(text)
  if (code) return around(text, code, () => [{ kind: 'code', text: code[1] }])

  const strong = STRONG.exec(text)
  if (strong) {
    return around(text, strong, () => [{ kind: 'strong', children: inline(strong[1] ?? strong[2]) }])
  }

  const em = EM.exec(text)
  if (em) return around(text, em, () => [{ kind: 'em', children: inline(em[1] ?? em[2]) }])

  const link = LINK.exec(text)
  if (link) {
    const href = safeHref(link[2])
    // 协议不安全时**不是丢掉**，而是退回纯文字：读的人至少知道这里本来有个链接。
    const span: Span = href
      ? { kind: 'link', href, children: inline(link[1] || link[2]) }
      : { kind: 'text', text: link[0] }
    return around(text, link, () => [span])
  }

  const bare = BARE_URL.exec(text)
  if (bare) {
    const href = safeHref(bare[0])
    return around(text, bare, () =>
      href ? [{ kind: 'link', href, children: [{ kind: 'text', text: bare[0] }] }] : [{ kind: 'text', text: bare[0] }],
    )
  }

  const at = MENTION.exec(text)
  if (at) return around(text, at, () => [{ kind: 'mention', text: at[0] }])

  return [{ kind: 'text', text }]
}

/** 把匹配前后的部分继续解析，中间换成 make() 给的节点。 */
function around(text: string, m: RegExpExecArray, make: () => Span[]): Span[] {
  const before = text.slice(0, m.index)
  const after = text.slice(m.index + m[0].length)
  return coalesce([...inline(before), ...make(), ...inline(after)])
}

/**
 * 相邻的纯文本节点合成一个。
 *
 * 不合的话，一段普通文字会被切成好几个节点（比如没被识别成链接的方括号），
 * 而 Android 那边同样的输入切法未必一样 —— **两个端的树对不上，
 * 「手机上显示得不一样」就没法靠用例钉住**。合并之后树是唯一的。
 */
function coalesce(spans: Span[]): Span[] {
  const out: Span[] = []
  for (const s of spans) {
    const last = out[out.length - 1]
    if (s.kind === 'text' && last?.kind === 'text') out[out.length - 1] = { kind: 'text', text: last.text + s.text }
    else out.push(s)
  }
  return out
}

/**
 * 只放行 http/https。
 *
 * 正文是 agent 写的，也就是**不可信输入**：`javascript:` 和 `data:` 一旦进到
 * href 里，点一下就是在控制台的会话上下文里执行别人的代码。协议白名单是这里
 * 唯一靠得住的做法 —— 黑名单挡不住 `java\tscript:` 这类写法。
 */
export function safeHref(raw: string): string | undefined {
  const href = raw.trim()
  if (/^https?:\/\//i.test(href)) return href
  return undefined
}
