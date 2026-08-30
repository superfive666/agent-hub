package org.agenthub.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.agenthub.app.ui.theme.MonoFont
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.Block
import org.agenthub.core.Span
import org.agenthub.core.parseMarkdown

/**
 * 帖子正文。树由 `org.agenthub.core.parseMarkdown` 给，**与 web 是同一套子集** ——
 * 同一条发言在两个端上读起来必须一样。
 *
 * 这里只画文本：正文来自 agent，是不可信输入，没有任何一条路径会把它当标记执行。
 * 链接只认 http/https（解析层已经过滤），点开走系统浏览器。
 *
 * 气泡里的层级只用**字重和间距**做，不画分隔线 ——
 * 一条发言里出现横线会读成两条发言，那是在破 §1.1 的一条一气泡。
 */
@Composable
fun MarkdownText(
    body: String,
    color: Color,
    /** 人类气泡是暖橘实底，代码底色和链接色要跟着换 —— 中性 token 在上面对比度不够。 */
    onWarmFill: Boolean = false,
    modifier: Modifier = Modifier,
) {
    val t = tokens()
    val linkColor = if (onWarmFill) color else t.agentInk
    val codeBg = if (onWarmFill) Color(0x47FFFFFF) else t.chipBg

    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(7.dp)) {
        for (block in parseMarkdown(body)) {
            when (block) {
                is Block.Heading -> Text(
                    text = annotate(block.spans, linkColor, t.agentInk, onWarmFill),
                    color = color,
                    style = MaterialTheme.typography.bodyMedium.copy(
                        fontWeight = FontWeight.ExtraBold,
                        fontSize = when (block.level) {
                            1 -> 16.sp
                            2 -> 15.sp
                            else -> 14.sp
                        },
                    ),
                )

                is Block.Code -> Column(
                    modifier = Modifier
                        .background(codeBg, RoundedCornerShape(10.dp))
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                ) {
                    // 长命令横向可滚，**但不许把气泡撑宽** —— 气泡被顶出屏幕，
                    // 人和 agent 的左右分列（§1.1 最强的那一重信号）就跟着塌了。
                    Row(modifier = Modifier.horizontalScroll(rememberScrollState())) {
                        Text(
                            text = block.text,
                            color = color,
                            softWrap = false,
                            style = LocalTextStyle.current.copy(
                                fontFamily = MonoFont,
                                fontSize = 12.sp,
                                lineHeight = 18.sp,
                            ),
                        )
                    }
                }

                is Block.Listing -> Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                    block.items.forEachIndexed { i, item ->
                        Row {
                            Text(
                                text = if (block.ordered) "${i + 1}." else "·",
                                color = color,
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.width(if (block.ordered) 22.dp else 14.dp),
                            )
                            Text(
                                text = annotate(item, linkColor, t.agentInk, onWarmFill),
                                color = color,
                                style = MaterialTheme.typography.bodyMedium,
                            )
                        }
                    }
                }

                // height(IntrinsicSize.Min) 是竖线能画出来的前提：没有它，
                // 那个 Box 没有可撑的高度，引用条会是一条零高度的线（也就是看不见）。
                is Block.Quote -> Row(modifier = Modifier.height(IntrinsicSize.Min)) {
                    // 左边一道竖线 = 引用。用 hair2 而不是主色：引用是次要内容。
                    Box(
                        modifier = Modifier
                            .width(2.dp)
                            .fillMaxHeight()
                            .background(if (onWarmFill) color.copy(alpha = .35f) else t.hair2),
                    )
                    Text(
                        text = annotate(block.spans, linkColor, t.agentInk, onWarmFill),
                        color = if (onWarmFill) color else t.ink2,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(start = 9.dp),
                    )
                }

                is Block.P -> Text(
                    text = annotate(block.spans, linkColor, t.agentInk, onWarmFill),
                    color = color,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}

/**
 * 行内节点 → AnnotatedString。
 *
 * `@mention` 用**加粗 + 变色**两重表示，和 web 的 `.at` 对齐：只靠颜色的话，
 * 灰度截图和色弱用户那里它就消失了 —— 而「谁被拉进来关注了」是读 thread 时最常扫的东西。
 */
private fun annotate(
    spans: List<Span>,
    linkColor: Color,
    mentionColor: Color,
    onWarmFill: Boolean,
): AnnotatedString = buildAnnotatedString {
    fun walk(list: List<Span>, style: SpanStyle) {
        for (s in list) {
            when (s) {
                is Span.Text -> withStyle(style) { append(s.text) }
                is Span.Mention -> withStyle(
                    style.merge(
                        SpanStyle(
                            fontWeight = FontWeight.ExtraBold,
                            color = if (onWarmFill) Color.Unspecified else mentionColor,
                        ),
                    ),
                ) { append(s.text) }
                is Span.Code -> withStyle(
                    style.merge(SpanStyle(fontFamily = MonoFont, fontSize = 12.sp)),
                ) { append(s.text) }
                is Span.Strong -> walk(s.children, style.merge(SpanStyle(fontWeight = FontWeight.Bold)))
                is Span.Em -> walk(s.children, style.merge(SpanStyle(fontStyle = FontStyle.Italic)))
                is Span.Link -> {
                    // LinkAnnotation 让系统去开浏览器，我们不自己拼 Intent。
                    // href 在解析层已经只剩 http/https。
                    withLink(
                        LinkAnnotation.Url(
                            s.href,
                            TextLinkStyles(
                                SpanStyle(color = linkColor, textDecoration = TextDecoration.Underline),
                            ),
                        ),
                    ) { walk(s.children, style) }
                }
            }
        }
    }
    walk(spans, SpanStyle())
}
