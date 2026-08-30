package org.agenthub.app.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color

/**
 * 语义变量。**这是 `web/src/styles/theme.css` 的第二份实现**，
 * 唯一事实来源仍是 `docs/07-design-language.md`。
 *
 * 组件里只引用这里的名字，**不写死颜色** —— 双主题成立的唯一原因。
 * 改 token 要同时改 web 那份，这条负担是 ADR-0009 选原生换来的，明写在那里。
 *
 * 值直接取自 theme.css，一个都不重新调 —— 「几乎一致」的下限是配色一致。
 */
@Immutable
data class Tokens(
    val dark: Boolean,

    // ── 文字三档 ──
    val ink: Color,
    val ink2: Color,
    val ink3: Color,

    // ── 语义锚点。§1.1 的四重信号里，颜色只是其中一重 ──
    /** agent：青 */
    val agent: Color,
    val agentInk: Color,
    val agentSoft: Color,
    /** 人类：暖橘。平台上只有一个人，他的发言必须立刻认得出来 */
    val human: Color,
    val humanInk: Color,
    val humanSoft: Color,
    val alert: Color,
    val alertSoft: Color,
    val warn: Color,
    val warnSoft: Color,

    /** 虹彩四色。**只用于流光与球体，不参与信息编码** */
    val i1: Color,
    val i2: Color,
    val i3: Color,
    val i4: Color,
    val i5: Color,

    // ── 玻璃板 ──
    val paneBg: Brush,
    val paneBd: Color,
    /** 玻璃板背后那层底色。模糊不可用时它顶上，所以必须够不透明 */
    val paneFallback: Color,

    // ── 嵌套内板。板中有板：更淡、更小圆角，靠 inset 阴影"嵌"进去 ──
    val insetBg: Color,
    val insetBd: Color,

    /** 浮层菜单。**故意不透明** —— 悬在密排文字上方，毛玻璃会把两层字叠在一起 */
    val menuBg: Color,
    val menuBd: Color,

    /** 输入框的底。压在会滚动的消息流上方，透太多的话滚过去的字会浮上来 */
    val composerBg: Color,

    val chipBg: Color,
    val hair: Color,
    val hair2: Color,

    /** 主按钮：亮色薄荷渐变，暗色紫洋红。整套里最强的形状记号 */
    val priGrad: Brush,
    val priInk: Color,
    /** 人类气泡：暖橘渐变实底 */
    val meGrad: Brush,
    val meInk: Color,

    /** 棱镜描边的不透明度。亮色 .42 是色散，暗色 .85 就成了霓虹描边 */
    val prismOpacity: Float,
    /** 高光扫过的不透明度 */
    val sheenOpacity: Float,

    /** 舞台：有天气的背景。**深→浅的纵向落差**，玻璃浮在落差上才有质感 */
    val stage: List<Pair<Float, Color>>,
    /** 内容直接坐在舞台上时（全屏文档）用这版：不带深色段 */
    val stageFlat: List<Pair<Float, Color>>,
    /** 舞台上那几团色雾的颜色 */
    val stageAura: List<Color>,
)

/**
 * 亮色 · 液态玻璃。
 *
 * 舞台是**深青蓝 → 冰白的纵向落差**。把背景做成均匀的高键淡彩，
 * 厚度感就没了 —— 那是第一版亮色失败的原因。
 * 正因为上半部分是深色，**玻璃必须够奶白**，否则深色文字压在深底上会糊。
 */
val LightTokens = Tokens(
    dark = false,
    ink = Color(0xFF123243),
    ink2 = Color(0xFF3F6A7D),
    ink3 = Color(0xFF7BA2B2),
    agent = Color(0xFF10B493),
    agentInk = Color(0xFF087A64),
    agentSoft = Color(0x2B10C8A0),
    human = Color(0xFFE8792A),
    humanInk = Color(0xFFA85210),
    humanSoft = Color(0x33FF9646),
    alert = Color(0xFFD24634),
    alertSoft = Color(0x26D24634),
    warn = Color(0xFFA37C10),
    warnSoft = Color(0x2EC39B1E),
    i1 = Color(0xFF6CEAFF),
    i2 = Color(0xFFA4B8FF),
    i3 = Color(0xFFFFA3E2),
    i4 = Color(0xFFFFE3A0),
    i5 = Color(0xFF8DF0D2),
    paneBg = Brush.linearGradient(
        0f to Color(0xCCFFFFFF),
        0.46f to Color(0x8FFFFFFF),
        1f to Color(0xB3FFFFFF),
        start = Offset.Zero,
        end = Offset(0f, Float.POSITIVE_INFINITY),
    ),
    paneBd = Color(0xEBFFFFFF),
    // 模糊不可用时顶上的实底。**不是半透明白** —— 半透明白压在舞台的
    // 深青蓝上会把深色文字吃掉，这正是设计语言里记过的那个坑。
    paneFallback = Color(0xFFF3FAFC),
    insetBg = Color(0x8CFFFFFF),
    insetBd = Color(0xC7FFFFFF),
    menuBg = Color(0xFFFCFDFF),
    menuBd = Color(0x4D78A5BE),
    composerBg = Color(0xFFFAFBFD),
    chipBg = Color(0xA8FFFFFF),
    hair = Color(0xF2FFFFFF),
    hair2 = Color(0x335A96AF),
    priGrad = Brush.linearGradient(
        listOf(Color(0xFFBDF6D8), Color(0xFF7CE3BD), Color(0xFF5ED9CB)),
    ),
    priInk = Color(0xFF04443A),
    meGrad = Brush.linearGradient(
        listOf(Color(0xFFFFDCAE), Color(0xFFFF9F5A), Color(0xFFF2803A)),
    ),
    meInk = Color(0xFF4A2004),
    prismOpacity = 0.48f,
    sheenOpacity = 0.6f,
    stage = listOf(
        0.00f to Color(0xFF164A63),
        0.15f to Color(0xFF20687E),
        0.33f to Color(0xFF3F93A8),
        0.52f to Color(0xFF88C4D6),
        0.68f to Color(0xFFC2E4EE),
        0.82f to Color(0xFFDFF2F8),
        1.00f to Color(0xFFF0FBFD),
    ),
    stageFlat = listOf(
        0.00f to Color(0xFF9FCEDE),
        0.34f to Color(0xFFCBE9F2),
        0.68f to Color(0xFFE6F6FA),
        1.00f to Color(0xFFF4FCFD),
    ),
    stageAura = listOf(Color(0x3D6CEAFF), Color(0x2EFFA3E2)),
)

/** 暗色 · 霓虹光影：近黑底、角度渐变描边、面板背后有彩色氛围光。 */
val DarkTokens = Tokens(
    dark = true,
    ink = Color(0xFFEEF2FA),
    ink2 = Color(0xFF94A2B8),
    ink3 = Color(0xFF5D6A80),
    agent = Color(0xFF3EE0C8),
    agentInk = Color(0xFF8AF3E2),
    agentSoft = Color(0x243EE0C8),
    human = Color(0xFFFF9F5A),
    humanInk = Color(0xFFFFC08F),
    humanSoft = Color(0x29FF9F5A),
    alert = Color(0xFFFF6F5C),
    alertSoft = Color(0x24FF6F5C),
    warn = Color(0xFFF0C25A),
    warnSoft = Color(0x21F0C25A),
    i1 = Color(0xFF41E0FF),
    i2 = Color(0xFFA06BFF),
    i3 = Color(0xFFFF5CC8),
    i4 = Color(0xFFFFB35C),
    i5 = Color(0xFF3EE0C8),
    paneBg = Brush.linearGradient(
        0f to Color(0x16FFFFFF),
        0.46f to Color(0x05FFFFFF),
        1f to Color(0x0EFFFFFF),
        start = Offset.Zero,
        end = Offset(0f, Float.POSITIVE_INFINITY),
    ),
    paneBd = Color(0x1AFFFFFF),
    paneFallback = Color(0xFF161A20),
    insetBg = Color(0x07FFFFFF),
    insetBd = Color(0x0FFFFFFF),
    menuBg = Color(0xFF1E242A),
    menuBd = Color(0x21FFFFFF),
    // 暗色下 .028 的白几乎等于没有，输入框根本看不出边界。抬到实底。
    composerBg = Color(0xFF242B2E),
    chipBg = Color(0x0FFFFFFF),
    hair = Color(0x24FFFFFF),
    hair2 = Color(0x12FFFFFF),
    priGrad = Brush.linearGradient(
        listOf(Color(0xFF8B5CFF), Color(0xFFC94BFF), Color(0xFFFF5CC8)),
    ),
    priInk = Color(0xFFFFFFFF),
    meGrad = Brush.linearGradient(
        listOf(Color(0xFFFFC98A), Color(0xFFFF9F5A), Color(0xFFF07A3C)),
    ),
    meInk = Color(0xFF3A1A04),
    prismOpacity = 0.85f,
    sheenOpacity = 0.16f,
    stage = listOf(
        0.00f to Color(0xFF0B0E13),
        0.42f to Color(0xFF0A0C10),
        1.00f to Color(0xFF07090C),
    ),
    stageFlat = listOf(
        0.00f to Color(0xFF0E1219),
        1.00f to Color(0xFF07090C),
    ),
    stageAura = listOf(Color(0x332855FF), Color(0x2E7850FF)),
)

/**
 * 当前主题的 token。用 static 是因为它整体替换（切主题），
 * 不需要为读取它的每个 Composable 建立细粒度依赖。
 */
val LocalTokens = staticCompositionLocalOf { LightTokens }
