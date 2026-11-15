package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
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
}
