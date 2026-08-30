package org.agenthub.core

/**
 * 头像缩写：拉丁名取前两位大写，CJK 取第一个字。
 *
 * CJK 只取一个字不是省事 —— 两个汉字在 34dp 的圆里会小到认不出，
 * 而一个汉字的信息量本来就够。
 */
fun initialsOf(name: String?): String {
    val n = name?.trim().orEmpty()
    if (n.isEmpty()) return "??"
    val first = n[0]
    // 与 web 的 /[㐀-鿿぀-ヿ]/ 同一区间：CJK 统一表意文字扩展 A + 基本区、
    // 以及日文假名。
    val cjk = first in '㐀'..'鿿' || first in '぀'..'ヿ'
    if (cjk) return first.toString()
    return n.take(2).uppercase()
}

/**
 * 邮箱局部打码：`wuchao900726@gmail.com` → `wuchao**@gmail.com`。
 *
 * OIDC 模式下管理员名就是那个 Google 邮箱，整串画出来会把侧栏/卡片撑破。
 * 三条规则一条都不能少：
 * - **域名完整保留** —— 用户要靠它认出自己登录的是哪个账号；
 * - 本地部分不超过 [keep] 就原样返回，短邮箱不该被无谓地遮起来；
 * - **不是邮箱就原样返回** —— 口令模式下这是普通用户名（`superfive`），
 *   把用户名也打码只会让人以为自己登错了。
 *
 * 这只是缩短，**不是布局保证** —— 用它的地方仍然要自己做截断兜底。
 */
fun maskEmail(value: String?, keep: Int = 6): String {
    val s = value?.trim().orEmpty()
    val at = s.lastIndexOf('@')
    // 没有 @、@ 开头、@ 结尾：都不是邮箱，原样交回去
    if (at <= 0 || at == s.length - 1) return s
    val local = s.substring(0, at)
    if (local.length <= keep) return s
    return local.take(keep) + "**" + s.substring(at)
}

/** 名字里可能有正则元字符（`.`、`+`），拼进 Regex 之前先转义。 */
private fun escapeRe(s: String): String = Regex.escape(s)

/**
 * 从正文里解析出被 @ 到的 agent，按名录映射成 agentId。
 *
 * 匹配不上的 `@xxx` **直接忽略，不报错** —— 正文里本来就可能有普通的 @。
 * 后面用 `(?![A-Za-z0-9_-])` 收边，否则 `@nova` 会把 `@nova2` 也算进来 ——
 * 那是**发错通知给一个无关的 agent**，而发出去就收不回来了。
 *
 * ⚠️ 这里解析出来的是**关注者**，不是负责人。被 @ 只产生关注关系，
 * 没有回复义务；主 agent 走单独的字段，且必选唯一。
 */
fun mentionedAgentIds(text: String, agents: List<Pair<String, String>>): List<String> {
    val ids = mutableListOf<String>()
    for ((agentId, name) in agents) {
        if (agentId.isBlank() || name.isBlank()) continue
        val re = Regex("(^|\\s)@${escapeRe(name)}(?![A-Za-z0-9_-])")
        if (re.containsMatchIn(text)) ids.add(agentId)
    }
    return ids
}

/**
 * 光标前那一小段：行首或空白之后的 `@`，后面跟着还在打的名字。
 *
 * `*` 而不是 `+` 是关键 —— 刚敲下 `@`、一个字都还没打时返回空串，
 * 空串能前缀匹配任何名字，下拉立刻把全部 agent 摊开。写成 `+` 就变成
 * 「要先猜对首字母才有提示」。
 *
 * @return 正在打的那截名字；不在 @ 上下文里时返回 null（**不是空串** ——
 *   空串是「刚敲下 @」这个有意义的状态）。
 */
fun mentionQueryAt(text: String, cursor: Int): String? {
    val head = text.take(cursor.coerceIn(0, text.length))
    val m = Regex("(^|\\s)@([A-Za-z0-9_-]*)$").find(head) ?: return null
    return m.groupValues[2]
}
