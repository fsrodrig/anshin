package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import kotlin.time.Duration.Companion.days
import kotlin.time.Duration.Companion.hours
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import org.junit.Test

class SparklineTest {
    private val span = 1_800_000L

    @Test
    fun aNullBreaksTheLineAndALoneValueIsADot() {
        assertEquals(
            listOf(
                listOf(ChartPoint(0, 1.0), ChartPoint(2_000, 2.0)),
                listOf(ChartPoint(6_000, 4.0)),
            ),
            segments(listOf(0L, 2_000L, 4_000L, 6_000L), listOf(1.0, 2.0, null, 4.0)),
        )
    }

    // Memory is a level series: the same value on both sides of a Gap must still not be joined into a flat line.
    @Test
    fun aGapBreaksTheLineEvenBetweenEqualValues() {
        assertEquals(
            listOf(listOf(ChartPoint(0, 41.0), ChartPoint(2_000, 41.0)), listOf(ChartPoint(302_000, 41.0), ChartPoint(304_000, 41.0))),
            segments(listOf(0L, 2_000L, null, 302_000L, 304_000L), listOf(41.0, 41.0, null, 41.0, 41.0)),
        )
    }

    @Test
    fun aGapIsAsWideAsTheTimeItLasted() {
        val times = listOf(1_000_000L, null, 1_300_000L, 2_800_000L)
        val gap = gaps(times, span).single()
        assertEquals(GapSpan(1_000_000, 1_300_000), gap)
        assertEquals(300.0 / 1800.0, (gap.toMs - gap.fromMs).toDouble() / span, 1e-9)
        assertEquals("Gap · 5 min", gapLabel(gap))
    }

    @Test
    fun aLeadingGapIsHatchedFromTheLeftEdge() {
        assertEquals(listOf(GapSpan(1_000_000, 1_500_000)), gaps(listOf(null, 1_500_000L, 2_800_000L), span))
    }

    @Test
    fun aShortGapIsLabelledInSeconds() {
        assertEquals("Gap · 45 s", gapLabel(GapSpan(0, 45_000)))
    }

    @Test
    fun theCaptionSaysHowMuchTheWindowCovers() {
        val full = List(900) { 2_000L * it }
        assertEquals("last 30 min", caption(full, span))
        assertEquals("last 30 min", caption(listOf(null, 1_500_000L, 2_800_000L), span))
        assertEquals("last 2 min", caption(List(60) { 2_000L * it }, span))
        assertEquals("last 40 s", caption(List(21) { 2_000L * it }, span))
    }

    @Test
    fun theSummaryGivesTheRangeTheNewestValueAndEachGap() {
        assertEquals(
            "CPU, last 30 min: from 3 % to 55 %, now 12 %. 1 Gap of 5 min.",
            chartSummary("CPU", listOf(1_000_000L, 1_002_000L, null, 1_302_000L, 2_800_000L), listOf(3.2, 55.0, null, 20.0, 12.4), span),
        )
        assertEquals(
            "Memory, last 2 min: from 40 % to 41 %, now unavailable.",
            chartSummary("Memory", List(61) { 2_000L * it }, List(61) { if (it == 60) null else 40.0 + it % 2 }, span),
        )
        assertEquals("CPU, last 2 min: unavailable.", chartSummary("CPU", List(61) { 2_000L * it }, List(61) { null }, span))
    }

    @Test
    fun percentIsFixedAtHundredAndSwapAtItsTotal() {
        assertEquals(ChartScale(0.0, 100.0, ChartUnit.Percent), chartScale(ChartUnit.Percent, listOf(3.0, 12.0), fixedTop = 100.0))
        assertEquals("100 %", chartScale(ChartUnit.Percent, listOf(3.0), fixedTop = 100.0).topLabel)
        val swapTotal = 8.0 * 1024 * 1024 * 1024
        assertEquals("8 GiB", chartScale(ChartUnit.Bytes, listOf(1e9), fixedTop = swapTotal).topLabel)
        // A swap total of 0 says nothing about the scale, so the top falls back to the values.
        assertEquals("2 MiB", chartScale(ChartUnit.Bytes, listOf(1.5 * 1024 * 1024), fixedTop = 0.0).topLabel)
    }

    @Test
    fun anAutoScaleRoundsUpToANiceValueInItsUnit() {
        fun top(unit: ChartUnit, vararg values: Double) = chartScale(unit, values.toList()).topLabel
        assertEquals("100 KiB/s", top(ChartUnit.BytesPerSecond, 75_304.0))
        assertEquals("2 MiB/s", top(ChartUnit.BytesPerSecond, 1.3 * 1024 * 1024))
        assertEquals("1000 KiB/s", top(ChartUnit.BytesPerSecond, 900.0 * 1024))
        assertEquals("1 MiB/s", top(ChartUnit.BytesPerSecond, 1023.5 * 1024))
        assertEquals("10 W", top(ChartUnit.Watts, 7.5))
        assertEquals("20 W", top(ChartUnit.Watts, 15.0))
        assertEquals("100 °C", top(ChartUnit.Celsius, 87.9))
        assertEquals("20", top(ChartUnit.Load, 15.13))
        assertEquals("0.5", top(ChartUnit.Load, 0.3))
        assertEquals("100 °C", top(ChartUnit.Celsius, 100.0))
    }

    @Test
    fun aZeroOrUnknownSeriesKeepsZeroOnTheBottomRule() {
        assertEquals(ChartScale(0.0, 1024.0, ChartUnit.BytesPerSecond), chartScale(ChartUnit.BytesPerSecond, listOf(0.0, 0.0)))
        assertEquals(ChartScale(0.0, 1024.0, ChartUnit.BytesPerSecond), chartScale(ChartUnit.BytesPerSecond, listOf(null, null)))
        assertEquals("1 MiB", chartScale(ChartUnit.Bytes, emptyList()).topLabel)
        assertEquals("1 W", chartScale(ChartUnit.Watts, listOf(0.0)).topLabel)
        assertEquals("10 °C", chartScale(ChartUnit.Celsius, listOf(null)).topLabel)
        assertEquals("1", chartScale(ChartUnit.Load, listOf(0.0)).topLabel)
        assertEquals("0", chartScale(ChartUnit.Load, listOf(0.0)).bottomLabel)
        val held = HeldWindow(Window(spanS = 1800, boottimeMs = listOf(0L, 2_000L), netRxBps = listOf(0.0, 0.0)), "00000000000000ff.2")
        assertEquals(listOf(0f, 0f), chartSeries("Received", held, held.window.netRxBps, ChartUnit.BytesPerSecond)!!.runs.single().odd())
    }

    @Test
    fun aNegativeTemperatureLowersTheBottom() {
        val scale = chartScale(ChartUnit.Celsius, listOf(-40.5, 12.0))
        assertEquals(ChartScale(-50.0, 20.0, ChartUnit.Celsius), scale)
        assertEquals("-50 °C", scale.bottomLabel)
        assertEquals(0.0, chartScale(ChartUnit.Watts, listOf(-1.0, 3.0)).bottom, 0.0)
    }

    @Test
    fun theSummaryUsesTheSeriesUnit() {
        assertEquals(
            "Received, last 30 min: from 0 B/s to 1.2 MiB/s, now 73.5 KiB/s. 1 Gap of 5 min.",
            chartSummary("Received", listOf(1_000_000L, 1_002_000L, null, 1_302_000L, 2_800_000L),
                listOf(0.0, 1_258_291.0, null, 20.0, 75_304.0), span, ChartUnit.BytesPerSecond),
        )
        assertEquals(
            "CPU temperature, last 2 min: from 41.0 °C to 87.9 °C, now 87.9 °C.",
            chartSummary("CPU temperature", List(2) { 2_000L * it } + 120_000L, listOf(41.0, 50.0, 87.9), span, ChartUnit.Celsius),
        )
        assertEquals(
            "Load 1 min, last 2 min: from 0.00 to 15.13, now 15.13.",
            chartSummary("Load 1 min", listOf(0L, 120_000L), listOf(0.0, 15.13), span, ChartUnit.Load),
        )
    }

    @Test
    fun aHistoricalSummarySaysLastWithItsAgeNeverNow() {
        assertEquals(
            "CPU, last 30 min: from 3 % to 55 %, last 12 %, 42 s ago. 1 Gap of 5 min.",
            chartSummary("CPU", listOf(1_000_000L, 1_002_000L, null, 1_302_000L, 2_800_000L), listOf(3.2, 55.0, null, 20.0, 12.4), span, ago = "42 s"),
        )
        assertEquals(
            "Received, last 2 min: from 0 B/s to 73.5 KiB/s, last unavailable, 3 min ago.",
            chartSummary("Received", listOf(0L, 60_000L, 120_000L), listOf(0.0, 75_304.0, null), span, ChartUnit.BytesPerSecond, ago = "3 min"),
        )
        assertEquals("CPU, last 2 min: unavailable.", chartSummary("CPU", List(61) { 2_000L * it }, List(61) { null }, span, ago = "42 s"))
    }

    @Test
    fun normalizedGeometryKeepsEveryBreakAndGap() {
        val times = listOf(1_000_000L, 1_002_000L, null, 1_302_000L, 1_304_000L, 1_306_000L, 2_800_000L)
        val values = listOf(10.0, 20.0, null, null, 40.0, 50.0, 100.0)
        val held = HeldWindow(Window(spanS = 1800, boottimeMs = times, cpuPct = values), "00000000000000ff.7")
        val series = chartSeries("CPU", held, values, ChartUnit.Percent, fixedTop = 100.0)!!
        val expectedRuns = segments(times, values)
        assertEquals(expectedRuns.map { it.size * 2 }, series.runs.map { it.size })
        for ((run, points) in series.runs.zip(expectedRuns)) {
            points.forEachIndexed { i, p ->
                assertEquals((p.tMs - 1_000_000).toFloat() / span, run[2 * i], 1e-6f)
                assertEquals((p.value / 100).toFloat(), run[2 * i + 1], 1e-6f)
            }
        }
        assertTrue(series.runs.all { run -> run.all { it in 0f..1f } })
        assertEquals(
            gaps(times, span).map { ChartGap((it.fromMs - 1_000_000).toFloat() / span, (it.toMs - 1_000_000).toFloat() / span, gapLabel(it)) },
            series.gaps,
        )
        assertEquals("last 30 min", series.caption)
        assertTrue(series.known)
        assertEquals(chartSummary("CPU", times, values, span), series.summary)
        assertEquals(chartSummary("CPU", times, values, span, ago = "42 s"), series.summary("42 s"))
        assertNull(chartSeries("CPU", null, null, ChartUnit.Percent))
        assertFalse(chartSeries("Sent", held, null, ChartUnit.BytesPerSecond)!!.known)
    }

    private fun FloatArray.odd() = filterIndexed { i, _ -> i % 2 == 1 }

    @Test
    fun agesCountSecondsThenMinutesHoursAndDays() {
        assertEquals(listOf("0 s", "42 s", "1 min", "3 min", "59 min", "1 h", "5 h", "1 d", "3 d"),
            listOf(0.seconds, 42.seconds, 60.seconds, 3.minutes, 59.minutes, 60.minutes, 5.hours, 24.hours, 3.days).map(::age))
        assertEquals("0 s", age((-1).seconds))
    }
}
