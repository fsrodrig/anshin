package io.github.fsrodrig.anshin

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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

    /** What [MachineView.Unreachable] says about the failed request, so the detail can say it in the card's words. */
    data class Unreachable(
        val retryAt: ComparableTimeMark,
        val vpnHint: Boolean,
        val outsideTailnet: Boolean = false,
        val httpStatus: Int? = null,
    ) : Connection
}

/**
 * Everything the detail screen draws. Values always come from [last]; only Live shows a current Verdict. [machine] is
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
 * The detail's input for a view that holds data: Live, Stale, and Unreachable with a last Pulse. `null` for every
 * view without values (Connecting, Unreachable before any Pulse, Forbidden, Incompatible), which the detail shows as
 * the card's message.
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
 * The Machine screen ([home], which holds the card) or the detail for one Machine. The only navigation state is
 * whether the detail is open, saved per Machine, so a new Machine starts on its card and the detail survives process
 * death (showing Connecting until the first Pulse). The detail stays open whatever the Watcher says next, so Stale and
 * Unreachable are read where the Owner already is. [view] comes from the app root's collector, so opening the detail
 * never starts a request of its own.
 */
@Composable
internal fun MachineScreens(
    machine: String,
    view: MachineView?,
    address: String = machine,
    now: () -> ComparableTimeMark,
    home: @Composable (openDetail: () -> Unit) -> Unit,
) {
    var detail by rememberSaveable(machine) { mutableStateOf(false) }
    if (!detail || view == null) {
        home { detail = true }
        return
    }
    BackHandler { detail = false }
    val state = view.detail(machine, address)
    if (state != null) {
        MachineDetail(state, now, onBack = { detail = false })
    } else {
        MachineDetailMessage(machine, view, now, onBack = { detail = false }, address = address)
    }
}

/**
 * A Machine's every section, under a header that stays put so the last Sample's age is always on screen. [now] reads
 * the Watcher's clock, which [MachineDetailState.last] is stamped by.
 */
@Composable
internal fun MachineDetail(state: MachineDetailState, now: () -> ComparableTimeMark, onBack: () -> Unit, modifier: Modifier = Modifier) {
    val items = remember(state) { detailItems(state) }
    val at = state.last.at
    // Read only where it is drawn, so a Live screen never recomposes for the ticking clock outside the header.
    val ago: () -> String? = if (state.live) ({ null }) else ({ age(now() - at) })
    val list = rememberLazyListState()
    val notice = items.firstOrNull() is DetailItem.Notice
    // The list keeps its first visible item by key, so a notice that appears above the top would stay out of view.
    LaunchedEffect(notice) {
        if (notice && list.firstVisibleItemIndex == 1 && list.firstVisibleItemScrollOffset == 0) list.scrollToItem(0)
    }
    DetailScaffold(state.machine, onBack, modifier, stateLine = { StateLine(state.connection, at, now) }) {
        LazyColumn(
            Modifier.fillMaxSize().testTag("machine-detail"),
            state = list,
            contentPadding = PaddingValues(horizontal = 24.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            items(items, key = { it.key }, contentType = { it.contentType }) { item ->
                DetailRow(item, state, now, ago, Modifier.testTag(item.key))
            }
        }
    }
}

/** The detail with no values to show: the same words the card shows for [view], under "Back to Machine". */
@Composable
internal fun MachineDetailMessage(
    machine: String,
    view: MachineView,
    now: () -> ComparableTimeMark,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    address: String = machine,
) {
    DetailScaffold(machine, onBack, modifier, stateLine = null) {
        Column(Modifier.padding(horizontal = 24.dp, vertical = 16.dp)) { PulseView(view, machine, address, now = now) }
    }
}

@Composable
private fun DetailScaffold(
    machine: String,
    onBack: () -> Unit,
    modifier: Modifier,
    stateLine: (@Composable () -> Unit)?,
    content: @Composable () -> Unit,
) {
    val colors = AnshinTheme.colors
    Column(modifier.fillMaxSize().background(colors.background).safeDrawingPadding()) {
        Column(Modifier.padding(start = 12.dp, end = 24.dp, top = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            TextButton(onClick = onBack, modifier = Modifier.heightIn(min = 48.dp)) { Text("Back to Machine") }
            Column(Modifier.padding(start = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(machine, color = colors.text, style = AnshinTheme.type.mono, fontSize = 20.sp)
                stateLine?.invoke()
            }
        }
        content()
    }
}

/** A Live marker never implies ok, so Live has no vial; Stale and Unreachable carry the empty one. */
@Composable
private fun StateLine(connection: Connection, at: ComparableTimeMark, now: () -> ComparableTimeMark) {
    val colors = AnshinTheme.colors
    // Only the word is a live region, so a change of state is announced once and the ticking age never is.
    val word: @Composable () -> Unit = {
        val (text, color) = when (connection) {
            Connection.Live -> "Live" to colors.accent
            Connection.Stale -> "Stale" to colors.unavailable
            is Connection.Unreachable -> "Unreachable" to colors.unavailable
        }
        Text(text, color = color, fontSize = 15.sp, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
    }
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (connection == Connection.Live) word() else VerdictLabel(null, word)
        Age(at, now)
    }
}

/**
 * Reads the ticking clock in this scope alone, so each second recomposes this text and nothing else. A new [at]
 * recomposes it at once, so the age never lags a new Sample by a tick.
 */
@Composable
private fun Age(at: ComparableTimeMark, now: () -> ComparableTimeMark) {
    val elapsed = now() - at
    Text(
        buildAnnotatedString {
            append(" · last Sample ")
            withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(age(elapsed)) }
            append(" ago")
        },
        color = AnshinTheme.colors.muted,
        fontSize = 15.sp,
        modifier = Modifier.semantics { contentDescription = "last Sample ${spokenAge(elapsed)} ago" },
    )
}

@Composable
private fun DetailRow(item: DetailItem, state: MachineDetailState, now: () -> ComparableTimeMark, ago: () -> String?, modifier: Modifier) {
    val colors = AnshinTheme.colors
    when (item) {
        is DetailItem.Notice -> Column(modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val connection = state.connection
            if (connection is Connection.Unreachable) {
                Text(unreachableCause(connection.outsideTailnet, connection.httpStatus, state.machine, state.address), color = colors.muted)
                NextTry(connection.retryAt, now)
                if (connection.vpnHint) VpnHint()
            }
            ago()?.let { Text(notCurrent(it), color = colors.muted) }
        }
        is DetailItem.Heading -> Column(modifier.padding(top = 12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Box(Modifier.fillMaxWidth().height(1.dp).background(colors.border))
            Text(item.title, color = colors.text, fontSize = 17.sp, modifier = Modifier.semantics { heading() })
        }
        is DetailItem.Subheading -> Text(item.title, color = colors.muted, fontSize = 15.sp, modifier = modifier.padding(top = 4.dp))
        is DetailItem.VerdictRow -> Column(modifier) {
            val since = ago()
            if (item.live || since == null) VerdictView(item.verdict) else LastVerdict(item.verdict, since)
        }
        is DetailItem.Value -> Column(modifier) {
            Text(item.label, color = colors.muted, fontSize = 13.sp)
            Text(item.value.annotated())
            item.coverage?.let { coverage ->
                CoverageStrip(coverage, item.value.plain, Modifier.fillMaxWidth().height(8.dp))
            }
            item.detail?.let { detail ->
                // The Owner copies a sensor ID from here into ANSHIN_CPU_SENSOR.
                SelectionContainer { Text(detail.annotated(), fontSize = 13.sp) }
            }
        }
        is DetailItem.BigValue -> {
            val unavailable = item.value.tone == Tone.Unavailable && item.value.text == UNAVAILABLE
            Text(
                buildAnnotatedString {
                    append(listOf(item.value).annotated())
                    withStyle(SpanStyle(fontSize = 16.sp)) { append(item.after.annotated()) }
                },
                style = AnshinTheme.type.mono,
                fontSize = if (unavailable) 16.sp else 34.sp,
                modifier = modifier,
            )
        }
        is DetailItem.Columns -> Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            item.columns.forEach { (label, value) ->
                Column(Modifier.weight(1f)) {
                    Text(label, color = colors.muted, fontSize = 13.sp)
                    Text(value.annotated())
                }
            }
        }
        is DetailItem.Text -> Text(item.line.annotated(), modifier = modifier)
        is DetailItem.Chart -> Column(modifier) {
            Text(item.label, color = colors.muted, fontSize = 13.sp)
            SeriesChart(item.series, item.tone.color(), ago = ago())
        }
        is DetailItem.CpuBars -> Column(modifier) {
            Text("Per CPU", color = colors.muted, fontSize = 13.sp)
            Column(Modifier.semantics { contentDescription = item.description }) {
                item.values.chunked(CPUS_PER_ROW).forEach { row ->
                    CpuBars(row, item.tone.color(), Modifier.fillMaxWidth().height(48.dp).padding(vertical = 4.dp))
                }
            }
            Text(item.caption, color = colors.muted, style = AnshinTheme.type.mono, fontSize = 11.sp)
        }
        is DetailItem.ListRow -> ListRow(item, modifier)
    }
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
        if (item.tags.isNotEmpty()) {
            Text(item.tags.join("  ").annotated(), fontSize = 12.sp)
        }
        item.usage?.let { UsageBar(it.fraction, it.tone.color(), Modifier.fillMaxWidth().height(8.dp)) }
        item.lines.forEach { Text(it.annotated(), fontSize = 13.sp) }
    }
}

private fun List<Line>.join(separator: String): Line =
    flatMapIndexed { i, line -> if (i > 0) listOf(Span(separator, Tone.Muted)) + line else line }

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
        modifier.graphicsLayer {}.drawWithCache {
            val hatch = if (fraction == null) hatchPath(0f, size.width, 0f, size.height, HATCH_STEP.toPx()) else null
            val outline = Stroke(width = 1.dp.toPx())
            onDrawBehind {
                drawRect(colors.surface)
                if (hatch != null) {
                    drawHatch(hatch, colors.unavailable, Offset.Zero, size)
                } else {
                    drawRect(fill, size = Size(size.width * fraction!!.coerceIn(0.0, 1.0).toFloat(), size.height))
                    for (tick in listOf(0.9f, 0.95f)) {
                        drawLine(colors.muted, Offset(size.width * tick, 0f), Offset(size.width * tick, size.height), strokeWidth = 1.dp.toPx())
                    }
                }
                drawRect(colors.border, style = outline)
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
                    drawRect(colors.surface, cells[i], cell)
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
private fun CoverageStrip(series: ChartSeries, description: String, modifier: Modifier) {
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
            spans.forEach { (at, box) -> drawRect(colors.muted, at, box) }
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
