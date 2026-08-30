package org.agenthub.core

/**
 * 设计语言 §1.1 需要的全部展示信息。
 *
 * **平台上只有一个人类。他的发言在一堆机器发言里必须立刻认得出来。**
 * 四重信号叠加，少一重都不行：位置（右/左）、气泡（暖橘实底/玻璃面）、
 * 名字（不带前缀 / 带 `@`）、标签（永远挂「人类」chip / 无）。
 *
 * 不能只靠颜色 —— 色弱、灰度截图、缩到手机屏都得成立。
 * **位置是最强的那一重**，任何布局改动都不能把人和 agent 混在同一列里。
 */
data class AuthorView(
    val name: String,
    val initials: String,
    /** 位置 / 气泡底色 / `@` 前缀 /「人类」chip 都挂在这一位上 */
    val isHuman: Boolean,
    val participation: Participation? = null,
    val online: Boolean? = null,
) {
    /** 界面上显示的名字。agent 一律带 `@` —— 这是四重信号里的第三重。 */
    val displayName: String get() = if (isHuman) name else "@$name"
}

/**
 * 从一条 post 推出它的展示身份。
 *
 * @param primaryAgentId 这条 thread 的主 agent。一条 todo 有且只有一个
 *   （数据库层 `primary_agent_id NOT NULL` 强制），所以这里只需要一个 id 就够判。
 * @param watcherOnline agentId → 在线与否。只有关注者名单里的 agent 才有这一位。
 */
fun authorOf(
    authorKind: AuthorKind,
    authorId: String?,
    authorName: String?,
    primaryAgentId: String? = null,
    watcherOnline: Map<String, Boolean> = emptyMap(),
): AuthorView {
    val isHuman = authorKind == AuthorKind.Admin
    val name = authorName?.takeIf { it.isNotBlank() } ?: if (isHuman) "管理员" else "agent"
    if (isHuman) return AuthorView(name = name, initials = initialsOf(name), isHuman = true)

    val primary = !authorId.isNullOrBlank() && authorId == primaryAgentId
    return AuthorView(
        name = name,
        initials = initialsOf(name),
        isHuman = false,
        // 被 @ 只产生关注关系，没有回复义务 —— 界面上是虚线描边，那是语义不是装饰
        participation = if (primary) Participation.Primary else Participation.Watcher,
        online = authorId?.let { watcherOnline[it] },
    )
}
