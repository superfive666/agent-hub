package org.agenthub.core

import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * 需求：看板按**平台时区**切分「一天」，全局统一（`PLATFORM_TIMEZONE`）。
 *
 * 这是 app 相对网页新增的一整类风险：手机的时区是用户带着走的。
 * 拿设备时区去切天，同一条 todo 在新加坡和在伦敦会落到不同的日期格子里。
 */
class DatesTest {

    private val sg = ZoneId.of("Asia/Singapore")
    private val utc = ZoneId.of("UTC")

    @Test
    fun `看板日期按平台时区切，不是 UTC`() {
        // 新加坡是 UTC+8：本地 08:00 之前的一切在 UTC 下都还算"昨天"。
        // 用 UTC 切的话，每天早上看板都会莫名其妙地少半天数据。
        val early = Instant.parse("2026-08-30T02:00:00Z") // 新加坡时间 10:00
        assertEquals("2026-08-30", isoDate(early, sg))
        assertEquals("2026-08-30", isoDate(early, utc))

        val lateNight = Instant.parse("2026-08-29T17:30:00Z") // 新加坡 8/30 01:30
        assertEquals("2026-08-30", isoDate(lateNight, sg))
        assertEquals("2026-08-29", isoDate(lateNight, utc)) // 这就是差别
    }

    @Test
    fun `今天的判定也按平台时区`() {
        val now = Instant.parse("2026-08-29T17:30:00Z") // 新加坡已是 8/30
        assertTrue(isToday("2026-08-30", sg, now))
        assertFalse(isToday("2026-08-30", utc, now))
    }

    @Test
    fun `翻页是纯日期加减，跨月份正确`() {
        assertEquals("2026-09-01", shiftDate("2026-08-31", 1))
        assertEquals("2026-08-31", shiftDate("2026-09-01", -1))
        assertEquals("2027-01-01", shiftDate("2026-12-31", 1))
    }

    @Test
    fun `翻页不会被夏令时那一小时带偏`() {
        // 纯 LocalDate 加减，不经过 Instant —— 经过的话在 DST 切换那天会跳错。
        assertEquals("2026-03-30", shiftDate("2026-03-29", 1))
        assertEquals("2026-10-26", shiftDate("2026-10-25", 1))
    }

    @Test
    fun `今天给时刻，别的日子给月日`() {
        val now = Instant.parse("2026-08-30T02:00:00Z")
        assertEquals("10:00", timeLabel("2026-08-30T02:00:00Z", sg, now))
        assertEquals("8/28", timeLabel("2026-08-28T02:00:00Z", sg, now))
    }

    @Test
    fun `解析不了的时间戳原样交回去，不显示破折号`() {
        // "—" 看起来像「没有这个字段」，而实际情况是「有值但我们读不懂」。
        // 原样显示至少让人能把这串东西贴给别人看。
        assertEquals("not-a-time", timeLabel("not-a-time", sg))
        assertEquals("", timeLabel(null, sg))
    }

    @Test
    fun `时区解析不了时退回 UTC 而不是设备时区`() {
        // 退回设备时区会让错误静默：看板照样能画，只是日期边界悄悄错了 8 小时。
        // 退回 UTC 至少让所有人看到的是同一份（错的）切分，容易被发现。
        assertEquals(ZoneId.of("UTC"), zoneOf("Nowhere/Nothing"))
        assertEquals(ZoneId.of("UTC"), zoneOf(null))
        assertEquals(sg, zoneOf("Asia/Singapore"))
    }
}
