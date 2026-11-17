package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.time.ComparableTimeMark

/** How current the phone's view of a Machine is, in the glossary's words, for a screen that draws its last data. */
internal sealed interface Connection {
    data object Live : Connection

    data object Stale : Connection

    /** What [MachineView.Unreachable] says about the failed request, so the notice can say why. */
    data class Unreachable(
        val retryAt: ComparableTimeMark,
        val vpnHint: Boolean,
        val outsideTailnet: Boolean = false,
        val httpStatus: Int? = null,
    ) : Connection
}

/**
 * Everything the Machine screen draws from data. Values always come from [last]; only Live shows a current Verdict. [machine] is
 * the Machine name and [address] how the phone reaches it.
 */
@Immutable
internal data class MachineDetailState(
    val machine: String,
    val connection: Connection,
    val last: Received,
    val address: String = machine,
) {
    val live: Boolean get() = connection == Connection.Live
    val window: HeldWindow? get() = last.window
}

/**
 * The Machine screen's input for a view that holds data: Live, Stale, and Unreachable with a last Pulse. `null` for
 * every view without values (Connecting, Unreachable before any Pulse, Forbidden, Incompatible), which the screen
 * explains instead.
 */
internal fun MachineView.detail(machine: String, address: String = machine): MachineDetailState? = when (this) {
    is MachineView.Live -> MachineDetailState(machine, Connection.Live, received, address)
    is MachineView.Stale -> MachineDetailState(machine, Connection.Stale, last, address)
    is MachineView.Unreachable -> last?.let {
        MachineDetailState(machine, Connection.Unreachable(retryAt, vpnHint, outsideTailnet, httpStatus), it, address)
    }
    MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible -> null
}

/**
 * One Machine's screen: its address, then, for a view that holds data, every section of [detailItems]; for one that
 * holds none, the words that say why. It reads the view the app's root already collects, so opening it sends no
 * request, and it stays open whatever the Watcher says next, so Stale and Unreachable are read where the Owner already
 * is. [now] reads the Watcher's clock, which every mark in [view] comes from.
 */
@Composable
internal fun MachineScreen(
    machine: String,
    address: String,
    view: MachineView,
    now: () -> ComparableTimeMark,
    back: Back,
    onEdit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = AnshinTheme.colors
    val state = view.detail(machine, address)
    val items = remember(state) { state?.let(::detailItems).orEmpty() }
    // Read only where it is drawn, so a Live screen never recomposes for the ticking clock outside the state line.
    val ago: () -> String? = if (state == null || state.live) ({ null }) else ({ age(now() - state.last.at) })
    AnshinListScreen(
        title = machine,
        back = back,
        modifier = modifier,
        actions = { BarAction(AnshinIcon.Edit, "Edit $machine", onEdit) },
        pinned = { StateLine(view, now) },
    ) {
        // Always first, so the list keeps it in view when a notice appears or the view loses its data.
        item(key = "address", contentType = "address") {
            Text(
                breakableAddress(address),
                color = colors.muted,
                style = AnshinTheme.type.mono,
                fontSize = 13.sp,
                lineHeight = 18.sp,
                modifier = Modifier.testTag("address"),
            )
        }
        if (state == null) {
            item(key = "message", contentType = "message") { NoData(view, machine, address, now) }
        } else {
            items(items, key = { it.key }, contentType = { it.contentType }) { item ->
                DetailRow(item, state, now, ago, Modifier.testTag(item.key))
            }
        }
    }
}

/** Connecting, Forbidden, Incompatible and Unreachable before any Pulse: why there is nothing to show, and no values. */
@Composable
private fun NoData(view: MachineView, machine: String, address: String, now: () -> ComparableTimeMark) {
    val colors = AnshinTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        when (view) {
            MachineView.Connecting -> Text(valueDetail("Asking ", machine, " for its Pulse."), color = colors.text)
            MachineView.Forbidden -> Text("This Agent only answers its Owner's untagged devices.", color = colors.text)
            MachineView.Incompatible ->
                Text("The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.", color = colors.text)
            is MachineView.Unreachable -> {
                Text(unreachableCause(view.outsideTailnet, view.httpStatus, machine, address), color = colors.text)
                NextTry(view.retryAt, now)
                if (view.vpnHint) VpnHint()
            }
            is MachineView.Live, is MachineView.Stale -> Unit
        }
    }
}

/**
 * The state word, and for a view with data the last Sample's age. Only the word is a live region, so a change of state
 * is announced once and the ticking age never is. A Live marker never implies ok, so no state carries a vial here.
 */
@Composable
private fun StateLine(view: MachineView, now: () -> ComparableTimeMark) {
    val colors = AnshinTheme.colors
    val (word, ink) = when (view) {
        MachineView.Connecting -> "Connecting" to colors.muted
        is MachineView.Live -> "Live" to colors.accent
        is MachineView.Stale -> "Stale" to colors.unavailable
        is MachineView.Unreachable -> "Unreachable" to colors.unavailable
        MachineView.Forbidden -> "Forbidden" to colors.text
        MachineView.Incompatible -> "Incompatible" to colors.text
    }
    val at = when (view) {
        is MachineView.Live -> view.received.at
        is MachineView.Stale -> view.last.at
        is MachineView.Unreachable -> view.last?.at
        MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible -> null
    }
    FlowRow {
        Text(word, color = ink, fontSize = 13.sp, lineHeight = 18.sp, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
        at?.let { Age(it, now, if (view is MachineView.Live) colors.muted else colors.unavailable) }
    }
}

/**
 * Reads the ticking clock in this scope alone, so each second recomposes this text and nothing else. A new [at]
 * recomposes it at once, so the age never lags a new Sample by a tick.
 */
@Composable
private fun Age(at: ComparableTimeMark, now: () -> ComparableTimeMark, ink: Color) {
    val elapsed = now() - at
    Text(
        buildAnnotatedString {
            append(" · last Sample ")
            withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(ago(elapsed)) }
        },
        color = ink,
        fontSize = 13.sp,
        lineHeight = 18.sp,
        modifier = Modifier.semantics { contentDescription = "last Sample ${spokenAge(elapsed)} ago" },
    )
}

private val LABEL_SIZE = 13.sp
private val LABEL_LINE = 18.sp

@Composable
private fun DetailRow(item: DetailItem, state: MachineDetailState, now: () -> ComparableTimeMark, ago: () -> String?, modifier: Modifier) {
    val colors = AnshinTheme.colors
    when (item) {
        // The age is already pinned under the bar, so the notice says why and what happens next, not how old.
        is DetailItem.Notice -> Column(modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val connection = state.connection
            if (connection is Connection.Unreachable) {
                Text(unreachableCause(connection.outsideTailnet, connection.httpStatus, state.machine, state.address), color = colors.text)
                NextTry(connection.retryAt, now)
                if (connection.vpnHint) VpnHint()
            }
            Text(NOT_CURRENT, color = colors.muted)
        }
        // The list puts 12 dp above it, so the rule sits 24 dp below the previous section.
        is DetailItem.Heading -> Column(modifier.padding(top = 12.dp)) {
            Box(Modifier.fillMaxWidth().height(1.dp).background(colors.border))
            Spacer(Modifier.height(16.dp))
            Text(item.title, color = colors.text, style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        }
        is DetailItem.Subheading -> Text(item.title, color = colors.muted, modifier = modifier.padding(top = 4.dp))
        is DetailItem.VerdictRow -> {
            val since = ago()
            if (item.live || since == null) VerdictView(item.verdict, modifier) else LastVerdict(item.verdict, since, modifier)
        }
        is DetailItem.Value -> Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
            LabelledValue(item.label, item.value.annotated())
            item.coverage?.let { coverage ->
                val ink = if (state.live) colors.muted else colors.unavailable
                CoverageStrip(coverage, item.value.plain, ink, Modifier.fillMaxWidth().height(8.dp))
            }
            item.detail?.let { detail ->
                // The Owner copies a sensor ID from here into ANSHIN_CPU_SENSOR.
                SelectionContainer { Text(detail.annotated(), fontSize = LABEL_SIZE, lineHeight = LABEL_LINE) }
            }
        }
        is DetailItem.BigValue -> DisplayNumber(item, modifier)
        is DetailItem.Text -> Text(item.line.annotated(), modifier = modifier)
        is DetailItem.Footnote -> Text(item.text, color = colors.muted, fontSize = LABEL_SIZE, lineHeight = LABEL_LINE, modifier = modifier)
        is DetailItem.Chart -> Column(modifier.padding(bottom = 4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            if (item.labelled) Text(item.label, color = colors.muted, fontSize = LABEL_SIZE, lineHeight = LABEL_LINE)
            SeriesChart(item.series, item.tone.color(), ago = ago(), height = if (item.main) MAIN_CHART_HEIGHT else CHART_HEIGHT)
        }
        is DetailItem.CpuBars -> Column(modifier.padding(bottom = 4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("Per CPU", color = colors.muted, fontSize = LABEL_SIZE, lineHeight = LABEL_LINE)
            Column(Modifier.semantics { contentDescription = item.description }, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                item.values.chunked(CPUS_PER_ROW).forEach { row ->
                    CpuBars(row, item.tone.color(), Modifier.fillMaxWidth().height(40.dp))
                }
            }
            Text(item.caption, color = colors.muted, style = AnshinTheme.type.mono, fontSize = 11.sp, lineHeight = 16.sp)
        }
        is DetailItem.ListRow -> ListRow(item, modifier)
    }
}

/** Label left, value right; at a large font the value wraps under its label rather than being clipped. */
@Composable
private fun LabelledValue(label: String, value: AnnotatedString) {
    FlowRow(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = AnshinTheme.colors.muted, modifier = Modifier.padding(end = 12.dp))
        Text(value)
    }
}

/** A section's one headline number: the number large in the series colour, its unit and any words after it smaller. */
@Composable
private fun DisplayNumber(item: DetailItem.BigValue, modifier: Modifier) {
    val ink = item.value.tone.color()
    val mono = AnshinTheme.type.mono.toSpanStyle()
    val after = item.after.annotated()
    if (item.value.text == UNAVAILABLE) {
        Text(listOf(item.value).annotated() + after, modifier = modifier)
        return
    }
    val (number, unit) = splitUnit(item.value.text)
    Text(
        buildAnnotatedString {
            withStyle(mono.merge(SpanStyle(color = ink, fontSize = 32.sp))) { append(number) }
            withStyle(mono.merge(SpanStyle(color = ink))) { append(unit) }
            append(after)
        },
        lineHeight = 40.sp,
        modifier = modifier,
    )
}

@Composable
private fun ListRow(item: DetailItem.ListRow, modifier: Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(verticalAlignment = Alignment.Top) {
            val name = @Composable { Text(listOf(item.name).annotated()) }
            Box(Modifier.weight(1f)) { if (item.selectableName) SelectionContainer { name() } else name() }
            item.trailing?.let {
                Spacer(Modifier.width(12.dp))
                Text(listOf(it).annotated(), textAlign = TextAlign.End)
            }
        }
        if (item.tags.isNotEmpty()) MetaLine(item.tags)
        item.usage?.let { UsageBar(it.fraction, it.tone.color(), Modifier.fillMaxWidth().height(8.dp)) }
        item.lines.forEach { MetaLine(it.split()) }
    }
}

/** Label–value pairs that wrap as whole pairs, so no separator is left dangling at the end of a line. */
@Composable
private fun MetaLine(parts: List<Line>) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        parts.forEach { Text(it.annotated(), fontSize = LABEL_SIZE, lineHeight = LABEL_LINE) }
    }
}

private fun Line.split(): List<Line> {
    val parts = mutableListOf(mutableListOf<Span>())
    forEach { span -> if (span == SEPARATOR) parts += mutableListOf<Span>() else parts.last() += span }
    return parts.filter { it.isNotEmpty() }
}

@Composable
private fun Tone.color(): Color {
    val colors = AnshinTheme.colors
    return when (this) {
        Tone.Text -> colors.text
        Tone.Muted -> colors.muted
        Tone.Unavailable -> colors.unavailable
        Tone.Cpu -> colors.cpu
        Tone.Memory -> colors.memory
        Tone.Storage -> colors.storage
        Tone.Network -> colors.network
    }
}

@Composable
private fun Line.annotated(): AnnotatedString {
    val mono = AnshinTheme.type.mono.toSpanStyle()
    val colors = map { it.tone.color() }
    return buildAnnotatedString {
        forEachIndexed { i, span ->
            val style = (if (span.mono) mono else SpanStyle()).merge(SpanStyle(color = colors[i]))
            withStyle(style) { append(span.text) }
        }
    }
}

/**
 * A filesystem's used share on a track, with ticks at 90 % and 95 %: the Verdict's filesystem thresholds. An unread
 * capacity is a hatched track, never an empty one.
 */
@Composable
private fun UsageBar(fraction: Double?, fill: Color, modifier: Modifier) {
    val colors = AnshinTheme.colors
    Spacer(
        modifier.clip(RoundedCornerShape(2.dp)).graphicsLayer {}.drawWithCache {
            val hatch = if (fraction == null) hatchPath(0f, size.width, 0f, size.height, HATCH_STEP.toPx()) else null
            onDrawBehind {
                drawRect(colors.border)
                if (hatch != null) {
                    drawHatch(hatch, colors.unavailable, Offset.Zero, size)
                } else {
                    drawRect(fill, size = Size(size.width * fraction!!.coerceIn(0.0, 1.0).toFloat(), size.height))
                    for (tick in listOf(0.9f, 0.95f)) {
                        drawLine(colors.muted, Offset(size.width * tick, 0f), Offset(size.width * tick, size.height), strokeWidth = 1.dp.toPx())
                    }
                }
            }
        },
    )
}

/** One bar per logical CPU on a track; an unavailable CPU is a hatched cell, never an empty bar. */
@Composable
private fun CpuBars(values: List<Double?>, fill: Color, modifier: Modifier) {
    val colors = AnshinTheme.colors
    Spacer(
        modifier.graphicsLayer {}.drawWithCache {
            val spacing = 2.dp.toPx()
            val layout = cpuBarCells(values.size, size.width, spacing)
            val width = layout.firstOrNull()?.second ?: 0f
            val cells = layout.map { Offset(it.first, 0f) }
            val hatches = values.indices.associateWith { i ->
                if (values[i] == null) hatchPath(cells[i].x, cells[i].x + width, 0f, size.height, HATCH_STEP.toPx()) else null
            }
            onDrawBehind {
                values.forEachIndexed { i, value ->
                    val cell = Size(width, size.height)
                    drawRect(colors.border, cells[i], cell)
                    val hatch = hatches[i]
                    if (hatch != null) {
                        drawHatch(hatch, colors.unavailable, cells[i], cell)
                    } else {
                        val h = size.height * (value!!.coerceIn(0.0, 100.0) / 100).toFloat()
                        drawRect(fill, Offset(cells[i].x, size.height - h), Size(width, h))
                    }
                }
            }
        },
    )
}

private fun DrawScope.drawHatch(hatch: Path, ink: Color, topLeft: Offset, box: Size) {
    drawRect(ink.copy(alpha = 0.1f), topLeft, box)
    clipRect(topLeft.x, topLeft.y, topLeft.x + box.width, topLeft.y + box.height) {
        drawPath(hatch, ink.copy(alpha = 0.6f), style = Stroke(width = 1.dp.toPx()))
    }
}

@Composable
private fun CoverageStrip(series: ChartSeries, description: String, ink: Color, modifier: Modifier) {
    val colors = AnshinTheme.colors
    Spacer(modifier.semantics { contentDescription = description }.graphicsLayer {}.drawWithCache {
        val spans = series.runs.map { run ->
            val from = run.first() * size.width
            val to = run[run.size - 2] * size.width
            Offset(from, 0f) to Size((to - from).coerceAtLeast(1f), size.height)
        }
        val hatches = series.gaps.map { gap ->
            val from = gap.from * size.width
            val to = gap.to * size.width
            Triple(Offset(from, 0f), Size(to - from, size.height),
                hatchPath(from, to, 0f, size.height, HATCH_STEP.toPx()))
        }
        onDrawBehind {
            spans.forEach { (at, box) -> drawRect(ink, at, box) }
            hatches.forEach { (at, box, hatch) -> drawHatch(hatch, colors.unavailable, at, box) }
        }
    })
}

internal const val CPUS_PER_ROW = 16

internal fun cpuBarCells(count: Int, width: Float, spacing: Float): List<Pair<Float, Float>> {
    val columns = count.coerceAtMost(CPUS_PER_ROW).coerceAtLeast(1)
    val gap = spacing.coerceAtMost(width / columns)
    val cellWidth = ((width - gap * (columns - 1)) / columns).coerceAtLeast(0f)
    return List(count) { i -> (i % columns) * (cellWidth + gap) to cellWidth }
}
