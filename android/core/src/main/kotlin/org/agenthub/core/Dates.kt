package org.agenthub.core

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * 时间与看板日期。
 *
 * **一切都要显式带时区。** 看板按「平台时区」切分一天（`PLATFORM_TIMEZONE`，
 * 部署时定死），而手机的时区是用户带着走的 —— 拿设备时区去切天，
 * 同一条 todo 在新加坡和在伦敦会落到不同的日期格子里，
 * 而两个人看着同一块看板会争论谁记错了。
 *
 * 这就是这些函数一律要求传入 [ZoneId] 的原因：**没有默认值可选**。
 */

private val ZH = Locale.CHINA

/** 解析契约里的时间戳。解析不了时返回 null —— 调用方原样显示，不要显示"—"。 */
fun parseInstant(iso: String?): Instant? =
    iso?.let { runCatching { Instant.parse(it) }.getOrNull() }

/**
 * 列表/气泡上的短时刻：今天给 `HH:mm`，否则给「月/日」。
 *
 * 判定「今天」用的是 [zone]，不是设备时区 —— 见文件头。
 */
fun timeLabel(iso: String?, zone: ZoneId, now: Instant = Instant.now()): String {
    if (iso.isNullOrBlank()) return ""
    val at = parseInstant(iso) ?: return iso
    val d = at.atZone(zone).toLocalDate()
    val today = now.atZone(zone).toLocalDate()
    return if (d == today) {
        DateTimeFormatter.ofPattern("HH:mm", ZH).format(at.atZone(zone))
    } else {
        DateTimeFormatter.ofPattern("M/d", ZH).format(at.atZone(zone))
    }
}

/** 详情页的完整时刻：`8月28日 09:14`。 */
fun dateTimeLabel(iso: String?, zone: ZoneId): String {
    val at = parseInstant(iso) ?: return "—"
    return DateTimeFormatter.ofPattern("M月d日 HH:mm", ZH).format(at.atZone(zone))
}

/** 看板的日期标题：`8月28日 星期四`。 */
fun dayLabel(iso: String?, zone: ZoneId): String {
    val at = parseInstant(iso) ?: return "—"
    return DateTimeFormatter.ofPattern("M月d日 EEEE", ZH).format(at.atZone(zone))
}

/**
 * `YYYY-MM-DD`，看板的 `date` 查询参数用它。
 *
 * **按 [zone] 切，不用 `Instant.toString().take(10)`** —— 那是 UTC 日期。
 * 新加坡是 UTC+8，本地时间 08:00 之前的一切在 UTC 下都还是"昨天"，
 * 于是每天早上看板都会莫名其妙地少半天数据。
 */
fun isoDate(at: Instant, zone: ZoneId): String = at.atZone(zone).toLocalDate().toString()

/** 看板翻页。纯日期加减，不经过 Instant —— 免得撞上夏令时那一小时。 */
fun shiftDate(date: String, days: Long): String =
    LocalDate.parse(date).plusDays(days).toString()

/** 这个日期是不是「今天」（按 [zone]）。看板上「回到今天」要靠它。 */
fun isToday(date: String, zone: ZoneId, now: Instant = Instant.now()): Boolean =
    date == isoDate(now, zone)

/**
 * 平台时区。解析不了就退回 UTC 而不是设备时区 ——
 * **退回设备时区会让错误静默**：看板照样能画，只是日期边界悄悄错了 8 小时。
 * 退回 UTC 至少让所有人看到的是同一份（错的）切分，容易被发现。
 */
fun zoneOf(timezone: String?): ZoneId =
    timezone?.let { runCatching { ZoneId.of(it) }.getOrNull() } ?: ZoneId.of("UTC")
