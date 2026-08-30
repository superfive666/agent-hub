package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * 需求：agent 的回复本来就是 markdown 写的。**两个端必须解析成同一棵树** ——
 * 用例与 `web/src/test/markdown.test.tsx` 一一对应，
 * 不然「手机上少了一段」这种问题没人能复现。
 */
class MarkdownTest {

    private fun spansOf(body: String): List<Span> =
        (parseMarkdown(body).single() as Block.P).spans

    @Test
    fun `粗体、斜体、行内代码各自成形`() {
        val spans = spansOf("**要紧的**和*次要的*")
        assertEquals(Span.Strong(listOf(Span.Text("要紧的"))), spans[0])
        assertEquals(Span.Text("和"), spans[1])
        assertEquals(Span.Em(listOf(Span.Text("次要的"))), spans[2])
        assertEquals(listOf(Span.Code("systemctl restart")), spansOf("`systemctl restart`"))
    }

    @Test
    fun `行内代码里的星号是内容，不是加粗记号`() {
        // 顺序即优先级。反过来的话，agent 贴一段 `a ** b` 会被吃掉两个星号，
        // 而那正是它想让人照抄的东西。
        assertEquals(listOf(Span.Code("**x**")), spansOf("`**x**`"))
    }

    @Test
    fun `围栏代码块整段保留，换行和缩进一个不少`() {
        val blocks = parseMarkdown("先跑：\n```bash\ncd web\n  npm test\n```")
        val code = blocks[1] as Block.Code
        assertEquals("bash", code.lang)
        assertEquals("cd web\n  npm test", code.text)
    }

    @Test
    fun `没有收尾的围栏也照画 —— agent 的输出被截断是常事`() {
        // 整段消失是最坏的结果：人看到一条空气泡，不知道是没说还是没画出来。
        val code = parseMarkdown("日志：\n```\nboom\n还没写完")[1] as Block.Code
        assertEquals("boom\n还没写完", code.text)
    }

    @Test
    fun `无序与有序列表都成列表，序号不接到上一段后面`() {
        val blocks = parseMarkdown("要做：\n- 一\n- 二\n\n步骤：\n1. 起服务\n2. 自检")
        val ul = blocks[1] as Block.Listing
        val ol = blocks[3] as Block.Listing
        assertTrue(!ul.ordered && ul.items.size == 2)
        assertTrue(ol.ordered && ol.items.size == 2)
    }

    @Test
    fun `标题最多三级`() {
        assertEquals(1, (parseMarkdown("# 一级")[0] as Block.Heading).level)
        assertEquals(3, (parseMarkdown("###### 六级")[0] as Block.Heading).level)
    }

    @Test
    fun `段落里的换行保留，不折叠成一行`() {
        // 标准 markdown 会折叠单个换行，但这里是聊天发言：agent 分行写的
        // 清单和错误信息，折叠之后会糊成一坨。
        assertEquals(listOf(Span.Text("第一行\n第二行")), spansOf("第一行\n第二行"))
    }

    @Test
    fun `mention 仍然是 mention，加粗里面的也是`() {
        val spans = spansOf("**@nova** 你看一下，@rover 也知道")
        assertEquals(Span.Strong(listOf(Span.Mention("@nova"))), spans[0])
        assertTrue(spans.any { it == Span.Mention("@rover") })
    }

    @Test
    fun `@nova 不会把 @nova2 一起吃掉`() {
        assertEquals(Span.Mention("@nova2"), spansOf("@nova2 收到")[0])
    }

    @Test
    fun `链接与裸 URL 都认`() {
        assertEquals(
            listOf(Span.Link("https://hub.example.com/docs", listOf(Span.Text("部署文档")))),
            spansOf("[部署文档](https://hub.example.com/docs)"),
        )
        assertEquals(
            listOf(Span.Link("https://hub.example.com/t/1", listOf(Span.Text("https://hub.example.com/t/1")))),
            spansOf("https://hub.example.com/t/1"),
        )
    }

    /** 正文是 agent 写的，也就是不可信输入。这两条是安全边界，不是格式偏好。 */
    @Test
    fun `javascript 之类的协议不给 href，退回纯文字`() {
        assertNull(safeHref("javascript:alert(1)"))
        assertNull(safeHref("data:text/html,<script>"))
        assertEquals(listOf(Span.Text("[点我](javascript:alert(1))")), spansOf("[点我](javascript:alert(1))"))
    }

    @Test
    fun `正文里的 HTML 只是文字`() {
        // 树里只有 Text 节点，渲染层也只画文本 —— 这条用例是那个约定的钉子。
        assertEquals(listOf(Span.Text("<b>粗</b>")), spansOf("<b>粗</b>"))
    }

    @Test
    fun `纯文本正文原样成段，不多出任何东西`() {
        assertEquals(listOf(Block.P(listOf(Span.Text("就一句普通的话")))), parseMarkdown("就一句普通的话"))
    }
}
