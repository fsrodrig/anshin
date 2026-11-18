package io.github.fsrodrig.anshin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextMeasurer
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.util.Locale
import kotlin.math.floor
import kotlin.math.log10
import kotlin.math.pow

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

internal fun age(d: kotlin.time.Duration): String {
    val ms = d.inWholeMilliseconds.coerceAtLeast(0)
    return when {
        ms < 60_000 -> "${(ms + 500) / 1000} s"
        ms < 3_600_000 -> "${(ms + 30_000) / 60_000} min"
        ms < 86_400_000 -> "${(ms + 1_800_000) / 3_600_000} h"
        else -> "${(ms + 43_200_000) / 86_400_000} d"
    }
}

internal fun gapLabel(gap: GapSpan): String = "Gap · ${duration(gap.toMs - gap.fromMs)}"

internal fun caption(times: List<Long?>, spanMs: Long): String = "last ${duration(coverageMs(times, spanMs))}"

/**
 * What TalkBack reads instead of the chart: the range, the newest value and every Gap. With [ago], the chart is
 * history, so the newest value is "last", with its age, never "now".
 */
internal fun chartSummary(
    label: String,
    times: List<Long?>,
    values: List<Double?>,
    spanMs: Long,
    unit: ChartUnit = ChartUnit.Percent,
    ago: String? = null,
): String = summaryParts(label, times, values, spanMs, unit).read(ago)

/** A chart's summary before its age is known, so a ticking age never rebuilds the chart's geometry. */
@Immutable
internal class SummaryParts(private val head: String, private val range: String?, private val newest: String?, private val gapText: String) {
    fun read(ago: String?): String {
        val body = if (range == null) {
            "unavailable."
        } else {
            val word = if (ago == null) "now" else "last"
            val value = "$word ${newest ?: "unavailable"}"
            "$range, ${if (ago == null) value else "$value, $ago ago"}."
        }
        return "$head: $body$gapText"
    }
}

private fun summaryParts(label: String, times: List<Long?>, values: List<Double?>, spanMs: Long, unit: ChartUnit): SummaryParts {
    val known = knownValues(times, values)
    val spans = gaps(times, spanMs)
    val gapText = when (spans.size) {
        0 -> ""
        1 -> " 1 Gap of ${duration(spans[0].toMs - spans[0].fromMs)}."
        else -> " ${spans.size} Gaps of ${spans.joinToString(", ") { duration(it.toMs - it.fromMs) }}."
    }
    return SummaryParts(
        "$label, ${caption(times, spanMs)}",
        if (known.isEmpty()) null else "from ${unit.format(known.min())} to ${unit.format(known.max())}",
        values.getOrNull(times.lastIndex)?.let(unit::format),
        gapText,
    )
}

private fun knownValues(times: List<Long?>, values: List<Double?>): List<Double> =
    times.indices.mapNotNull { i -> values.getOrNull(i)?.takeIf { times[i] != null } }

private const val KIB = 1024.0
private const val MIB = 1024.0 * 1024

/** What a series measures, which decides its value format, its top rule's label and its scale with nothing measured. */
internal enum class ChartUnit {
    Percent, Bytes, BytesPerSecond, Watts, Celsius, Load;

    internal fun format(value: Double): String = when (this) {
        Percent -> wholePercent(value)
        Bytes -> bytes(value.toLong())
        BytesPerSecond -> rate(value.toLong())
        Watts -> watts(value)
        Celsius -> celsius(value)
        Load -> load(value)
    }

    internal fun label(value: Double): String = when (this) {
        Percent -> "${plain(value)} %"
        Bytes -> binaryLabel(value, "")
        BytesPerSecond -> binaryLabel(value, "/s")
        Watts -> "${plain(value)} W"
        Celsius -> "${plain(value)} °C"
        Load -> plain(value)
    }

    // A scale for a series with nothing above zero: wide enough that a real zero sits on the bottom rule, and no
    // scale is invented from nothing.
    internal val floor: Double
        get() = when (this) {
            Percent -> 100.0
            Bytes -> MIB
            BytesPerSecond -> KIB
            Watts -> 1.0
            Celsius -> 10.0
            Load -> 1.0
        }

    internal val binary: Boolean get() = this == Bytes || this == BytesPerSecond
}

@Immutable
internal data class ChartScale(val bottom: Double, val top: Double, val unit: ChartUnit) {
    val topLabel: String get() = unit.label(top)
    val bottomLabel: String get() = if (bottom == 0.0) "0" else unit.label(bottom)
}

/**
 * [fixedTop] when given (100 for a percent, the swap total for swap); else the known values' maximum rounded up to
 * 1, 2 or 5 × 10ⁿ in the unit that displays it. The bottom is 0 unless a temperature is below zero, because clipping
 * −40.5 °C to 0 would draw a false zero.
 */
internal fun chartScale(unit: ChartUnit, values: List<Double?>, fixedTop: Double? = null): ChartScale {
    val known = values.filterNotNull()
    val min = known.minOrNull()
    val bottom = if (unit == ChartUnit.Celsius && min != null && min < 0) -nice(-min) else 0.0
    val max = known.maxOrNull()
    val top = fixedTop?.takeIf { it > 0 } ?: when {
        max == null || max <= 0 -> unit.floor
        unit.binary -> niceBinary(max)
        else -> nice(max)
    }
    return ChartScale(bottom, top, unit)
}

private fun nice(value: Double): Double {
    val power = 10.0.pow(floor(log10(value)))
    val fraction = value / power
    val step = when {
        fraction <= 1 + 1e-9 -> 1.0
        fraction <= 2 + 1e-9 -> 2.0
        fraction <= 5 + 1e-9 -> 5.0
        else -> 10.0
    }
    return step * power
}

// Rounded in the binary unit that displays the value, so 900 KiB/s becomes 1000 KiB/s and never "1 MiB/s" in disguise.
private fun niceBinary(value: Double): Double {
    var scale = 1.0
    while (value / scale >= 1024) scale *= 1024
    var mantissa = nice(value / scale)
    if (mantissa >= 1024) {
        scale *= 1024
        mantissa = nice(value / scale)
    }
    return mantissa * scale
}

private val BINARY_LABELS = listOf("B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB")

private fun binaryLabel(value: Double, suffix: String): String {
    var mantissa = value
    var unit = 0
    while (unit < BINARY_LABELS.lastIndex && mantissa >= 1024) {
        mantissa /= 1024
        unit++
    }
    return "${plain(mantissa)} ${BINARY_LABELS[unit]}$suffix"
}

private fun plain(value: Double): String =
    if (value == floor(value)) value.toLong().toString()
    else String.format(Locale.ROOT, "%.2f", value).trimEnd('0').trimEnd('.')

@Immutable
internal data class ChartGap(val from: Float, val to: Float, val label: String)

/**
 * One series laid out once per Window update. Each run is `x0, y0, x1, y1, …` with x and y in 0..1 (y = 0 on the
 * bottom rule), so a draw only multiplies by its size. Compared by identity: a new Window makes a new series.
 */
@Immutable
internal class ChartSeries(
    val label: String,
    val runs: List<FloatArray>,
    val gaps: List<ChartGap>,
    val scale: ChartScale,
    val known: Boolean,
    private val parts: SummaryParts,
    val caption: String,
) {
    val summary: String get() = parts.read(null)

    /** The summary of a chart drawn from history, [ago] being the age of its newest Sample. */
    fun summary(ago: String?): String = parts.read(ago)
}

internal fun chartSeries(
    label: String,
    held: HeldWindow?,
    values: List<Double?>?,
    unit: ChartUnit,
    fixedTop: Double? = null,
): ChartSeries? {
    val window = held?.window ?: return null
    val spanMs = window.spanS?.takeIf { it > 0 }?.let { it * 1000 } ?: return null
    val last = window.lastMs ?: return null
    val times = window.boottimeMs
    val series = values ?: List(window.rows) { null }
    val known = knownValues(times, series)
    val scale = chartScale(unit, known, fixedTop)
    val left = last - spanMs
    fun x(t: Long) = (t - left).toFloat() / spanMs
    fun y(v: Double) = ((v.coerceIn(scale.bottom, scale.top) - scale.bottom) / (scale.top - scale.bottom)).toFloat()
    val runs = segments(times, series).map { run ->
        FloatArray(run.size * 2) { i -> if (i % 2 == 0) x(run[i / 2].tMs) else y(run[i / 2].value) }
    }
    val gapSpans = gaps(times, spanMs).map { ChartGap(x(it.fromMs), x(it.toMs), gapLabel(it)) }
    return ChartSeries(label, runs, gapSpans, scale, known.isNotEmpty(), summaryParts(label, times, series, spanMs, unit), caption(times, spanMs))
}

/** The plot of a section's one main chart, between its two rules. */
internal val MAIN_PLOT_HEIGHT = 80.dp
internal val PLOT_HEIGHT = 40.dp
private val LABEL_SPACING = 4.dp
private val GAP_LABEL_PADDING = 2.dp

/** The draw and the caption's Gap-fit check share this, so a Gap named in the caption is exactly one drawn unlabelled. */
internal fun plotWidth(total: Float, labelWidth: Float, spacing: Float) = total - labelWidth - spacing

private fun ruleLabelWidth(measurer: TextMeasurer, scale: ChartScale, style: TextStyle): Int =
    maxOf(measurer.measure(scale.topLabel, style).size.width, measurer.measure(scale.bottomLabel, style).size.width)

/** The widest scale label of a rate in KiB/s, the common wide case, so the gutter holds even before any rate is measured. */
private const val GUTTER_SAMPLE = "1000 KiB/s"

/**
 * One scale gutter, in pixels, for every chart drawn from [scales]: at least [GUTTER_SAMPLE] and the widest label any of
 * them draws. Charts that share it end at one x, so the same moment sits at the same x in each of them.
 */
@Composable
internal fun scaleGutter(scales: List<ChartScale>): Int {
    val measurer = rememberTextMeasurer()
    val style = axisLabelStyle()
    val labels = scales.flatMap { listOf(it.topLabel, it.bottomLabel) }.toSet() + GUTTER_SAMPLE
    return remember(labels, style, measurer) { labels.maxOf { measurer.measure(it, style).size.width } }
}

@Composable
private fun axisLabelStyle() = AnshinTheme.type.mono.copy(color = AnshinTheme.colors.muted, fontSize = 11.sp)

@Composable
private fun gapLabelStyle() = AnshinTheme.type.mono.copy(color = AnshinTheme.colors.text, fontSize = 11.sp)

/** Diagonal lines across a box, to be clipped to it: the one mark for "no value here" in charts and bars. */
internal fun hatchPath(from: Float, to: Float, top: Float, bottom: Float, step: Float): Path = Path().apply {
    val height = bottom - top
    var start = from - height
    while (start < to) {
        moveTo(start, bottom)
        lineTo(start + height, top)
        start += step
    }
}

internal val HATCH_STEP = 5.dp

internal fun gapLabelFits(hatchWidth: Float, labelWidth: Float, padding: Float) = labelWidth + 2 * padding <= hatchWidth

/**
 * A time-based chart of one series: x runs from `newest − spanMs` to the newest Sample, so a Gap takes the width of
 * the time it lasted. The line breaks at every null and every Gap and is never joined across one. Paths and text
 * are built in the draw cache, which rebuilds only for a new size or [series], and the chart has its own layer, so a
 * scroll moves it without redrawing it.
 */
@Composable
internal fun Sparkline(series: ChartSeries, color: Color, modifier: Modifier = Modifier, ago: String? = null, gutter: Int = 0) {
    val colors = AnshinTheme.colors
    val measurer = rememberTextMeasurer()
    val axisStyle = axisLabelStyle()
    val gapStyle = gapLabelStyle()
    // No live region: a chart that announced each arriving Sample would talk over everything else.
    Spacer(
        modifier
            .semantics { contentDescription = series.summary(ago) }
            .graphicsLayer {}
            .drawWithCache {
                val topText = measurer.measure(series.scale.topLabel, axisStyle)
                val bottomText = measurer.measure(series.scale.bottomLabel, axisStyle)
                // Each scale label is centred on its rule, so the rules sit far enough in to keep the labels inside.
                val top = maxOf(4.dp.toPx(), topText.size.height / 2f)
                val bottom = size.height - maxOf(4.dp.toPx(), bottomText.size.height / 2f)
                val plot = plotWidth(size.width, maxOf(gutter, topText.size.width, bottomText.size.width).toFloat(), LABEL_SPACING.toPx())
                fun x(f: Float) = f * plot
                fun y(f: Float) = bottom - f * (bottom - top)
                val stroke = Stroke(width = 1.5.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round)
                val hatchStroke = Stroke(width = 1.dp.toPx())
                val dotRadius = 1.5.dp.toPx()
                val padding = GAP_LABEL_PADDING.toPx()
                val step = HATCH_STEP.toPx()
                val lines = series.runs.filter { it.size > 2 }.map { run ->
                    Path().apply {
                        moveTo(x(run[0]), y(run[1]))
                        for (i in 2 until run.size step 2) lineTo(x(run[i]), y(run[i + 1]))
                    }
                }
                val dots = series.runs.filter { it.size == 2 }.map { Offset(x(it[0]), y(it[1])) }
                class GapDraw(val from: Float, val to: Float, val hatch: Path, val label: Offset?, val text: TextLayoutResult)
                val gapDraws = series.gaps.map { gap ->
                    val from = x(gap.from)
                    val to = x(gap.to)
                    val hatch = hatchPath(from, to, top, bottom, step)
                    val text = measurer.measure(gap.label, gapStyle)
                    val at = if (gapLabelFits(to - from, text.size.width.toFloat(), padding)) {
                        Offset((from + to - text.size.width) / 2, (top + bottom - text.size.height) / 2)
                    } else {
                        null
                    }
                    GapDraw(from, to, hatch, at, text)
                }
                onDrawBehind {
                    drawLine(colors.border, Offset(0f, top), Offset(plot, top), strokeWidth = 1.dp.toPx())
                    drawText(topText, topLeft = Offset(size.width - topText.size.width, top - topText.size.height / 2))
                    drawLine(colors.border, Offset(0f, bottom), Offset(plot, bottom), strokeWidth = 1.dp.toPx())
                    drawText(bottomText, topLeft = Offset(size.width - bottomText.size.width, bottom - bottomText.size.height / 2))
                    for (gap in gapDraws) {
                        drawRect(colors.unavailable.copy(alpha = 0.1f), Offset(gap.from, top), Size(gap.to - gap.from, bottom - top))
                        clipRect(gap.from, top, gap.to, bottom) {
                            drawPath(gap.hatch, colors.unavailable.copy(alpha = 0.6f), style = hatchStroke)
                        }
                        gap.label?.let { drawText(gap.text, topLeft = it) }
                    }
                    for (dot in dots) drawCircle(color, radius = dotRadius, center = dot)
                    for (line in lines) drawPath(line, color, style = stroke)
                }
            },
    )
}

/** One series of the held Window with its caption, or the words that say the Window is unavailable. */
@Composable
internal fun WindowChart(
    label: String,
    held: HeldWindow?,
    select: (Window) -> List<Double?>?,
    color: Color,
    ago: String? = null,
    unit: ChartUnit = ChartUnit.Percent,
    fixedTop: Double? = 100.0,
    plotHeight: Dp = PLOT_HEIGHT,
) {
    val series = remember(label, held, unit, fixedTop) { chartSeries(label, held, held?.window?.let(select), unit, fixedTop) }
    SeriesChart(series, color, ago = ago, plotHeight = plotHeight)
}

/**
 * [ago] is set when the chart is history: the age of its newest Sample, which its summary says instead of "now".
 * [gutter] is the scale label width the chart reserves, shared by every chart on a screen. [plotHeight] is the height between
 * the rules; the chart adds one scale label's height, so a larger font grows the chart rather than squeezing its plot.
 */
@Composable
internal fun SeriesChart(
    series: ChartSeries?,
    color: Color,
    modifier: Modifier = Modifier,
    ago: String? = null,
    plotHeight: Dp = PLOT_HEIGHT,
    gutter: Int = 0,
) {
    val colors = AnshinTheme.colors
    if (series == null) {
        Unplotted("— Window unavailable", modifier)
        return
    }
    val measurer = rememberTextMeasurer()
    val axisStyle = axisLabelStyle()
    val gapStyle = gapLabelStyle()
    if (!series.known) {
        // A frame and a scale with nothing in them would read as a measured zero, so only the words are drawn, Gaps named.
        val coverage = (listOf(series.caption) + series.gaps.map { it.label }).joinToString(" · ")
        Unplotted("— unavailable, $coverage", modifier.semantics { contentDescription = series.summary(ago) })
        return
    }
    val density = LocalDensity.current
    BoxWithConstraints(modifier.fillMaxWidth()) {
        // A Gap too narrow for its label at this width is named in the caption instead, so it is never unlabelled.
        val plot = plotWidth(
            constraints.maxWidth.toFloat(),
            maxOf(gutter, ruleLabelWidth(measurer, series.scale, axisStyle)).toFloat(),
            with(density) { LABEL_SPACING.toPx() },
        )
        val padding = with(density) { GAP_LABEL_PADDING.toPx() }
        val unlabelled = series.gaps.mapNotNull { gap ->
            gap.label.takeUnless { gapLabelFits((gap.to - gap.from) * plot, measurer.measure(it, gapStyle).size.width.toFloat(), padding) }
        }
        val coverage = (listOf(series.caption) + unlabelled).joinToString(" · ")
        // Half a label above the top rule and half below the bottom one, as the Sparkline insets them.
        val insets = with(density) {
            val least = 4.dp.toPx()
            (maxOf(measurer.measure(series.scale.topLabel, axisStyle).size.height / 2f, least) +
                maxOf(measurer.measure(series.scale.bottomLabel, axisStyle).size.height / 2f, least)).toDp()
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Sparkline(series, color, Modifier.fillMaxWidth().height(plotHeight + insets), ago, gutter)
            Text(coverage, color = colors.muted, style = AnshinTheme.type.mono, fontSize = 11.sp, lineHeight = 16.sp)
        }
    }
}

@Composable
private fun Unplotted(text: String, modifier: Modifier) =
    Text(text, color = AnshinTheme.colors.unavailable, style = AnshinTheme.type.mono, fontSize = 13.sp, lineHeight = 18.sp, modifier = modifier)
