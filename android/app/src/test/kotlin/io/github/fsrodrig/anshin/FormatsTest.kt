package io.github.fsrodrig.anshin

import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import org.junit.Assert.assertEquals
import org.junit.Test

class FormatsTest {
    @Test
    fun percentsKeepOneDecimalAndAreNotClamped() {
        assertEquals("70.2 %", percent(70.2))
        assertEquals("0.0 %", percent(0.0))
        assertEquals("104.2 %", percent(104.2))
        assertEquals("65 %", wholePercent(65.4))
    }

    @Test
    fun bytesUseBinaryUnitsAndStepUpWhenTheMantissaRoundsTo1024() {
        assertEquals("0 B", bytes(0))
        assertEquals("512 B", bytes(512))
        assertEquals("1023 B", bytes(1023))
        assertEquals("1.0 KiB", bytes(1024))
        assertEquals("2.0 KiB", bytes(2048))
        assertEquals("1.0 MiB", bytes(1024L * 1024 - 1))
        assertEquals("1.0 MiB", bytes(1024L * 1024))
        assertEquals("951.9 GiB", bytes(1022042832896))
        assertEquals("8.0 EiB", bytes(Long.MAX_VALUE))
    }

    @Test
    fun ratesAreBytesPerSecond() {
        assertEquals("0 B/s", rate(0))
        assertEquals("73.5 KiB/s", rate(75304))
    }

    @Test
    fun temperaturesWattsAndLoad() {
        assertEquals("87.9 °C", celsius(87.9))
        assertEquals("0.0 °C", celsius(0.0))
        assertEquals("-40.5 °C", celsius(-40.5))
        assertEquals("7.5 W", watts(7.5))
        assertEquals("0.0 W", watts(0.0))
        assertEquals("15.13", load(15.13))
        assertEquals("0.00", load(0.0))
    }

    @Test
    fun spansRoundDownToWholeSeconds() {
        assertEquals("0 s", span(0.0))
        assertEquals("45 s", span(45.9))
        assertEquals("59 min", span(3_599.0))
        assertEquals("1 h 0 min", span(3_600.0))
        assertEquals("2 h 30 min", span(9_000.0))
        assertEquals("2 h 30 min", span(9_059.99))
        assertEquals("20 h 41 min", span(74_463.83))
        assertEquals("1 d 0 h", span(86_400.0))
        assertEquals("2 s ago", ago(2.seconds))
        assertEquals("3 min ago", ago(3.minutes))
    }

    @Test
    fun spokenSpansSpellTheirUnitsOut() {
        assertEquals("1 second ago", spokenAgo(1.seconds))
        assertEquals("3 minutes ago", spokenAgo(3.minutes))
        assertEquals("2 hours 30 minutes", spokenSpan(9_000.0))
        assertEquals("1 day 0 hours", spokenSpan(86_400.0))
    }

    @Test
    fun batteryStatusWords() {
        assertEquals(
            listOf("charging", "discharging", "not charging", "full", "unknown"),
            BatteryStatus.entries.map(::statusWord),
        )
    }
}
