package org.agenthub.core

/**
 * 帖子正文的 markdown 解析。**与 web 的 `web/src/lib/markdown.ts` 是同一套子集、
 * 同一棵树** —— 同一条发言在两个端上读起来必须是一样的，否则「手机上少了一段」
 * 这种问题没人能复现。
 *
 * 覆盖的子集：标题、围栏代码块、有序/无序列表、引用、段落；行内是粗体、斜体、
 * 行内代码、链接、裸 URL，以及 `@mention`（这一条不是 markdown，是我们自己的）。
 *
 * **段落里的换行保留成硬换行。** 标准 markdown 会把单个换行折叠成空格，
 * 但这里的正文是聊天发言不是文档 —— agent 分行写的清单、日志、错误信息，
 * 折叠之后会糊成一坨。
 *
 * 输出是**纯数据**：渲染层只画文本，正文里的 `<script>` 在这里只是一段字。
 */
sealed interface Span {
    data class Text(val text: String) : Span
    data class Mention(val text: String) : Span
    data class Code(val text: String) : Span
    data class Strong(val children: List<Span>) : Span
    data class Em(val children: List<Span>) : Span
    data class Link(val href: String, val children: List<Span>) : Span
}

sealed interface Block {
    data class P(val spans: List<Span>) : Block
    /** 只有三级：气泡里再小就跟正文分不开了，更深的一律并到 3。 */
    data class Heading(val level: Int, val spans: List<Span>) : Block
    data class Code(val lang: String?, val text: String) : Block
    data class Listing(val ordered: Boolean, val items: List<List<Span>>) : Block
    data class Quote(val spans: List<Span>) : Block
}

private val FENCE = Regex("""^\s*```+\s*([A-Za-z0-9_+-]*)\s*$""")
private val HEADING = Regex("""^(#{1,6})\s+(.*)$""")
private val BULLET = Regex("""^\s*[-*+]\s+(.*)$""")
private val ORDERED = Regex("""^\s*\d{1,9}[.)]\s+(.*)$""")
private val QUOTE = Regex("""^\s*>\s?(.*)$""")

/** 正文 → 块序列。任何输入都要有输出：解析不出结构的行就是一个段落。 */
fun parseMarkdown(body: String): List<Block> {
    val lines = body.replace("\r\n", "\n").replace('\r', '\n').split("\n")
    val blocks = mutableListOf<Block>()
    var i = 0

    while (i < lines.size) {
        val line = lines[i]
        if (line.isBlank()) { i++; continue }

        val fence = FENCE.find(line)
        if (fence != null) {
            val lang = fence.groupValues[1].ifEmpty { null }
            val buf = mutableListOf<String>()
            i++
            // 没有收尾的 ``` 时一直吃到结尾：agent 的输出被截断是常事，
            // 把剩下的当正文画出来，好过整段消失。
            while (i < lines.size && !FENCE.matches(lines[i])) buf.add(lines[i++])
            if (i < lines.size) i++
            blocks.add(Block.Code(lang, buf.joinToString("\n")))
            continue
        }

        val heading = HEADING.find(line)
        if (heading != null) {
            blocks.add(Block.Heading(minOf(heading.groupValues[1].length, 3), inline(heading.groupValues[2])))
            i++
            continue
        }

        if (BULLET.matches(line) || ORDERED.matches(line)) {
            val ordered = !BULLET.matches(line)
            val items = mutableListOf<List<Span>>()
            while (i < lines.size) {
                // 同一段里换了记号（`-` 变 `1.`）就断开另起一个列表，
                // 否则序号会接到上一段的编号后面，看起来像漏了几条。
                val m = (if (ordered) ORDERED else BULLET).find(lines[i]) ?: break
                items.add(inline(m.groupValues[1]))
                i++
            }
            blocks.add(Block.Listing(ordered, items))
            continue
        }

        if (QUOTE.matches(line)) {
            val buf = mutableListOf<String>()
            while (i < lines.size && QUOTE.matches(lines[i])) buf.add(QUOTE.find(lines[i++])!!.groupValues[1])
            blocks.add(Block.Quote(inline(buf.joinToString("\n"))))
            continue
        }

        val buf = mutableListOf<String>()
        while (i < lines.size && lines[i].isNotBlank() &&
            !FENCE.matches(lines[i]) && !HEADING.matches(lines[i]) &&
            !BULLET.matches(lines[i]) && !ORDERED.matches(lines[i]) && !QUOTE.matches(lines[i])
        ) {
            buf.add(lines[i++])
        }
        blocks.add(Block.P(inline(buf.joinToString("\n"))))
    }

    return blocks
}

private val INLINE_CODE = Regex("""`([^`\n]+)`""")
private val STRONG = Regex("""\*\*([^\n]+?)\*\*|__([^\n]+?)__""")
private val EM = Regex("""(?<![*\w])\*([^*\n]+?)\*(?!\*)|(?<![_\w])_([^_\n]+?)_(?![\w_])""")
private val LINK = Regex("""\[([^\]\n]*)\]\(([^)\s]+)\)""")
private val BARE_URL = Regex("""https?://[^\s<>()\[\]]+""")

/** 与 [mentionedAgentIds] 的收边一致：`@nova` 不能把 `@nova2` 也吃进去。 */
private val MENTION = Regex("""@[A-Za-z0-9_-]+""")

/**
 * 行内解析。顺序即优先级，**行内代码必须排第一** ——
 * `` `**x**` `` 里的星号是代码内容，不是加粗记号。
 */
fun inline(text: String): List<Span> {
    if (text.isEmpty()) return emptyList()

    INLINE_CODE.find(text)?.let { m ->
        return around(text, m) { listOf(Span.Code(m.groupValues[1])) }
    }
    STRONG.find(text)?.let { m ->
        return around(text, m) { listOf(Span.Strong(inline(m.groupValues[1].ifEmpty { m.groupValues[2] }))) }
    }
    EM.find(text)?.let { m ->
        return around(text, m) { listOf(Span.Em(inline(m.groupValues[1].ifEmpty { m.groupValues[2] }))) }
    }
    LINK.find(text)?.let { m ->
        val href = safeHref(m.groupValues[2])
        // 协议不安全时**不是丢掉**，而是退回纯文字：读的人至少知道这里本来有个链接。
        val span = if (href != null) {
            Span.Link(href, inline(m.groupValues[1].ifEmpty { m.groupValues[2] }))
        } else {
            Span.Text(m.value)
        }
        return around(text, m) { listOf(span) }
    }
    BARE_URL.find(text)?.let { m ->
        val href = safeHref(m.value)
        return around(text, m) {
            if (href != null) listOf(Span.Link(href, listOf(Span.Text(m.value)))) else listOf(Span.Text(m.value))
        }
    }
    MENTION.find(text)?.let { m ->
        return around(text, m) { listOf(Span.Mention(m.value)) }
    }
    return listOf(Span.Text(text))
}

/** 把匹配前后的部分继续解析，中间换成 make() 给的节点。 */
private fun around(text: String, m: MatchResult, make: () -> List<Span>): List<Span> =
    coalesce(inline(text.substring(0, m.range.first)) + make() + inline(text.substring(m.range.last + 1)))

/**
 * 相邻的纯文本节点合成一个。
 *
 * 不合的话，一段普通文字会被切成好几个节点，而 web 那边同样的输入切法未必一样 ——
 * **两个端的树对不上，「手机上显示得不一样」就没法靠用例钉住**。合并之后树是唯一的。
 */
private fun coalesce(spans: List<Span>): List<Span> {
    val out = mutableListOf<Span>()
    for (s in spans) {
        val last = out.lastOrNull()
        if (s is Span.Text && last is Span.Text) out[out.size - 1] = Span.Text(last.text + s.text) else out.add(s)
    }
    return out
}

/**
 * 只放行 http/https。
 *
 * 正文是 agent 写的，也就是**不可信输入**。协议白名单是这里唯一靠得住的做法 ——
 * 黑名单挡不住 `java\tscript:` 这类写法。
 */
fun safeHref(raw: String): String? {
    val href = raw.trim()
    return if (Regex("""^https?://""", RegexOption.IGNORE_CASE).containsMatchIn(href)) href else null
}
