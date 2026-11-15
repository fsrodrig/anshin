package io.github.fsrodrig.anshin

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextMeasurer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.roundToInt

internal data class ChartPoint(val tMs: Long, val value: Double)

/** Where a Gap is hatched: from the Sample before it (or the chart's left edge) to the Sample after it. */
internal data class GapSpan(val fromMs: Long, val toMs: Long)

/**
 * The runs of consecutive Samples with values. A run of one is drawn as a dot, so a value between two nulls stays
 * visible without being joined to anything.
 */
internal fun segments(times: List<Long?>, values: List<Double?>): List<List<ChartPoint>> {
    val runs = mutableListOf<List<ChartPoint>>()
    var run = mutableListOf<ChartPoint>()
    times.forEachIndexed { i, t ->
        val v = values.getOrNull(i)
        if (t != null && v != null) {
            run.add(ChartPoint(t, v))
        } else if (run.isNotEmpty()) {
            runs.add(run)
            run = mutableListOf()
        }
    }
    if (run.isNotEmpty()) runs.add(run)
    return runs
}

internal fun gaps(times: List<Long?>, spanMs: Long): List<GapSpan> {
    val last = times.lastOrNull { it != null } ?: return emptyList()
    val left = last - spanMs
    val spans = mutableListOf<GapSpan>()
    var i = 0
    while (i < times.size) {
        if (times[i] != null) {
            i++
            continue
        }
        val before = times.getOrNull(i - 1)
        while (i < times.size && times[i] == null) i++
        val after = times.getOrNull(i) ?: last
        spans.add(GapSpan(maxOf(before ?: left, left), after))
    }
    return spans
}

/** How much of the span the Window covers: the whole span when it starts with a Gap or reaches the left edge. */
internal fun coverageMs(times: List<Long?>, spanMs: Long): Long {
    val last = times.lastOrNull { it != null } ?: return 0
    val first = times.firstOrNull() ?: return spanMs
    return minOf(last - first, spanMs)
}

internal fun duration(ms: Long): String =
    if (ms < 60_000) "${(ms + 500) / 1000} s" else "${(ms + 30_000) / 60_000} min"

internal fun gapLabel(gap: GapSpan): String = "Gap · ${duration(gap.toMs - gap.fromMs)}"

internal fun caption(times: List<Long?>, spanMs: Long): String = "last ${duration(coverageMs(times, spanMs))}"

/** What TalkBack reads instead of the chart: the range, the newest value and every Gap. */
internal fun chartSummary(label: String, times: List<Long?>, values: List<Double?>, spanMs: Long): String {
    val head = "$label, ${caption(times, spanMs)}"
    val known = times.indices.mapNotNull { i -> values.getOrNull(i)?.takeIf { times[i] != null } }
    val body = if (known.isEmpty()) {
        "unavailable."
    } else {
        val now = values.getOrNull(times.lastIndex)?.let { "now ${percent(it)}" } ?: "now unavailable"
        "from ${percent(known.min())} to ${percent(known.max())}, $now."
    }
    val spans = gaps(times, spanMs)
    val gapText = when (spans.size) {
        0 -> ""
        1 -> " 1 Gap of ${duration(spans[0].toMs - spans[0].fromMs)}."
        else -> " ${spans.size} Gaps of ${spans.joinToString(", ") { duration(it.toMs - it.fromMs) }}."
    }
    return "$head: $body$gapText"
}

private fun percent(value: Double) = "${value.roundToInt()} %"

private val CHART_HEIGHT = 72.dp
private val LABEL_WIDTH = 24.dp
private val HATCH_STEP = 5.dp
private val MIN_GAP_LABEL_WIDTH = 56.dp

/**
 * A time-based chart of one series: x runs from `newest − spanMs` to the newest Sample, so a Gap takes the width of
 * the time it lasted. The line breaks at every null and every Gap and is never joined across one.
 */
@Composable
internal fun Sparkline(
    times: List<Long?>,
    values: List<Double?>,
    spanMs: Long,
    color: Color,
    contentDescription: String,
    modifier: Modifier = Modifier,
    max: Double = 100.0,
) {
    val colors = AnshinTheme.colors
    val measurer = rememberTextMeasurer()
    val axisStyle = AnshinTheme.type.mono.copy(color = colors.muted, fontSize = 8.sp)
    val gapStyle = AnshinTheme.type.mono.copy(color = colors.text, fontSize = 9.sp)
    // No live region: a chart that announced each arriving Sample would talk over everything else.
    Canvas(modifier.semantics { this.contentDescription = contentDescription }) {
        val plotWidth = size.width - LABEL_WIDTH.toPx()
        val top = 4.dp.toPx()
        val bottom = size.height - 4.dp.toPx()
        val last = times.lastOrNull { it != null }
        rule(top, plotWidth, "100", measurer, axisStyle, colors.border)
        rule(bottom, plotWidth, "0", measurer, axisStyle, colors.border)
        if (last == null) return@Canvas
        val left = last - spanMs
        fun x(t: Long) = ((t - left).toFloat() / spanMs) * plotWidth
        fun y(v: Double) = bottom - (v.coerceIn(0.0, max) / max).toFloat() * (bottom - top)

        for (gap in gaps(times, spanMs)) {
            val from = x(gap.fromMs)
            val to = x(gap.toMs)
            hatch(from, to, top, bottom, colors.unavailable)
            if (to - from >= MIN_GAP_LABEL_WIDTH.toPx()) {
                val text = measurer.measure(gapLabel(gap), gapStyle)
                drawText(text, topLeft = Offset((from + to - text.size.width) / 2, (top + bottom - text.size.height) / 2))
            }
        }
        val stroke = Stroke(width = 1.5.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round)
        for (run in segments(times, values)) {
            if (run.size == 1) {
                drawCircle(color, radius = 1.5.dp.toPx(), center = Offset(x(run[0].tMs), y(run[0].value)))
                continue
            }
            val path = Path()
            run.forEachIndexed { i, p -> if (i == 0) path.moveTo(x(p.tMs), y(p.value)) else path.lineTo(x(p.tMs), y(p.value)) }
            drawPath(path, color, style = stroke)
        }
    }
}

private fun DrawScope.rule(y: Float, width: Float, label: String, measurer: TextMeasurer, style: TextStyle, color: Color) {
    drawLine(color, Offset(0f, y), Offset(width, y), strokeWidth = 1.dp.toPx())
    val text = measurer.measure(label, style)
    drawText(text, topLeft = Offset(size.width - text.size.width, y - text.size.height / 2))
}

private fun DrawScope.hatch(from: Float, to: Float, top: Float, bottom: Float, ink: Color) {
    drawRect(ink.copy(alpha = 0.1f), Offset(from, top), Size(to - from, bottom - top))
    clipRect(from, top, to, bottom) {
        val step = HATCH_STEP.toPx()
        val height = bottom - top
        var start = from - height
        while (start < to) {
            drawLine(ink.copy(alpha = 0.6f), Offset(start, bottom), Offset(start + height, top), strokeWidth = 1.dp.toPx())
            start += step
        }
    }
}

/** One series of the held Window with its caption, or the words that say there is no history to draw. */
@Composable
internal fun WindowChart(label: String, held: HeldWindow?, select: (Window) -> List<Double?>?, color: Color) {
    val colors = AnshinTheme.colors
    val window = held?.window
    val spanMs = window?.spanS?.takeIf { it > 0 }?.let { it * 1000 }
    if (window?.lastMs == null || spanMs == null) {
        Text("— history unavailable", color = colors.unavailable, style = AnshinTheme.type.mono, fontSize = 13.sp)
        return
    }
    val values = select(window) ?: List(window.rows) { null }
    val known = values.any { it != null }
    Column {
        Sparkline(
            times = window.boottimeMs,
            values = values,
            spanMs = spanMs,
            color = color,
            contentDescription = chartSummary(label, window.boottimeMs, values, spanMs),
            modifier = Modifier.fillMaxWidth().height(CHART_HEIGHT),
        )
        Text(
            if (known) caption(window.boottimeMs, spanMs) else "— unavailable, ${caption(window.boottimeMs, spanMs)}",
            color = if (known) colors.muted else colors.unavailable,
            style = AnshinTheme.type.mono,
            fontSize = 11.sp,
        )
    }
}
