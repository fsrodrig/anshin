package io.github.fsrodrig.anshin

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.roundToInt
import kotlin.time.ComparableTimeMark

internal const val ADD_MACHINE_TAG = "add-machine"
internal const val MACHINE_PANEL_TAG = "machine-panel"

/** One saved Machine and how to read what the phone shows of it, read only by the panel that draws it. */
internal class Panel(val machine: Machine, val view: () -> MachineView)

@Composable
internal fun Dashboard(
    panels: List<Panel>,
    now: () -> ComparableTimeMark,
    onOpen: (MachineId) -> Unit,
    onAdd: () -> Unit,
    onSettings: () -> Unit,
    topLine: () -> TopLine? = { null },
) {
    AnshinScreen(
        title = null,
        back = null,
        actions = {
            BarAction(AnshinIcon.Add, "Add Machine", onAdd, Modifier.testTag(ADD_MACHINE_TAG))
            BarAction(AnshinIcon.Settings, "Settings", onSettings)
        },
    ) {
        TopLineSlot(topLine)
        if (panels.isEmpty()) Empty(onAdd)
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            panels.forEach { panel ->
                key(panel.machine.id) { MachinePanel(panel.machine, panel.view, now, onOpen = { onOpen(panel.machine.id) }) }
            }
        }
    }
}

@Composable
private fun Empty(onAdd: () -> Unit) {
    val colors = AnshinTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("No Machines yet", color = colors.text, style = MaterialTheme.typography.titleLarge)
        Text("Add a Machine running the Anshin Agent by its MagicDNS name or tailnet IP.", color = colors.muted)
    }
    PrimaryButton("Add Machine", onAdd)
}

/** [topLine] is read here, so a Machine's update recomposes the line without recomposing the panels under it. */
@Composable
private fun TopLineSlot(topLine: () -> TopLine?) {
    topLine()?.let { TopLineView(it, Modifier.semantics(mergeDescendants = true) { heading() }) }
}

/**
 * One Machine at a glance, and one TalkBack stop that reads [panelAnnouncement]. Only a Live Machine shows a current
 * Verdict, with its level colour and vial; Stale and Unreachable quote the last one as history, in the unavailable
 * colour, and the other states show none, so a panel never puts a reassuring Verdict beside data that isn't current.
 * It is never a live region: a Sample arriving is not worth interrupting the Owner for.
 */
@Composable
internal fun MachinePanel(machine: Machine, view: () -> MachineView, now: () -> ComparableTimeMark, onOpen: () -> Unit) {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(8.dp)
    val current = view()
    val announcement = panelAnnouncement(machine.name, current, now())
    Column(
        modifier = Modifier
            .testTag(MACHINE_PANEL_TAG)
            .fillMaxWidth()
            .clip(shape)
            .background(colors.surface)
            .border(1.dp, colors.border, shape)
            .clickable(role = Role.Button, onClickLabel = "Open ${machine.name}", onClick = onOpen)
            .clearAndSetSemantics { contentDescription = announcement }
            .heightIn(min = 48.dp)
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    breakableName(machine.name),
                    color = colors.text,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.padding(end = 8.dp).align(Alignment.CenterVertically),
                )
                if (current is MachineView.Live) {
                    Box(Modifier.align(Alignment.CenterVertically)) { CurrentVerdict(current.received.pulse.verdict) }
                }
            }
            Text(breakableAddress(machine.address.toString()), color = colors.muted, style = small.merge(AnshinTheme.type.mono))
            Text(stateLine(current, now), color = colors.muted, style = small)
            when (current) {
                is MachineView.Live -> current.received.pulse.verdict?.reasons?.firstOrNull()?.let {
                    Text(reasonText(it), color = colors.text, style = small)
                }
                is MachineView.Stale -> LastPanelVerdict(current.last.pulse.verdict, age(now() - current.last.at))
                is MachineView.Unreachable -> current.last?.let { LastPanelVerdict(it.pulse.verdict, age(now() - it.at)) }
                MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible -> Unit
            }
        }
        when (current) {
            is MachineView.Live -> Values(current.received.pulse, history = false)
            is MachineView.Stale -> Values(current.last.pulse, history = true)
            is MachineView.Unreachable -> current.last?.let { Values(it.pulse, history = true) }
            // Forbidden and Incompatible withhold the values; Connecting has none yet.
            MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible -> Unit
        }
    }
}

/**
 * An address has no spaces, so at a large font it would break mid-label; a zero-width space after each dot and colon
 * lets it break between labels instead.
 */
internal fun breakableAddress(address: String): String = address.replace(".", ".\u200B").replace(":", ":\u200B")

/** A Machine name such as `workstation-in-the-rack` breaks after its hyphens, underscores and dots, not mid-word. */
internal fun breakableName(name: String): String = name.replace("-", "-\u200B").replace("_", "_\u200B").replace(".", ".\u200B")

private val small = TextStyle(fontSize = 13.sp, lineHeight = 18.sp)

@Composable
private fun CurrentVerdict(verdict: Verdict?) {
    val colors = AnshinTheme.colors
    if (verdict == null) {
        VerdictLabel(null, small.lineHeight) { Text("Verdict unavailable", color = colors.unavailable, style = small) }
        return
    }
    VerdictLabel(verdict.level, 24.sp) {
        Text(verdict.level.word, color = levelColor(verdict.level, colors.text), style = AnshinTheme.type.mono, fontSize = 16.sp, lineHeight = 24.sp)
    }
}

/** As on the Machine screen: a Verdict that is no longer current, quoted with its age and without level colour or mark. */
@Composable
private fun LastPanelVerdict(verdict: Verdict?, ago: String) {
    val colors = AnshinTheme.colors
    VerdictLabel(null, small.lineHeight) {
        Text(
            buildAnnotatedString {
                append("Last Verdict ")
                val mono = AnshinTheme.type.mono.toSpanStyle()
                if (verdict == null) {
                    withStyle(mono) { append("unavailable") }
                } else {
                    withStyle(mono) { append("${verdict.level.word}, ") }
                    appendAge("$ago ago", mono)
                }
            },
            color = colors.unavailable,
            style = small,
        )
    }
    verdict?.reasons?.firstOrNull()?.let { Text(reasonText(it), color = colors.unavailable, style = small) }
}

/**
 * The state word, then either the data's age as one mono token or, without data, why there is none, in the panel's
 * one line: `Live · 0 s ago`, `Stale · 42 s ago`, `Forbidden · only answers its Owner's untagged devices`.
 */
@Composable
private fun stateLine(view: MachineView, now: () -> ComparableTimeMark): AnnotatedString {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono.toSpanStyle()
    fun line(word: String, ink: Color, rest: AnnotatedString.Builder.() -> Unit) = buildAnnotatedString {
        withStyle(SpanStyle(color = ink)) { append(word) }
        append(" · ")
        rest()
    }
    fun aged(word: String, ink: Color, at: ComparableTimeMark) = line(word, ink) { appendAge("${age(now() - at)} ago", mono) }
    fun told(word: String, ink: Color, detail: String) = line(word, ink) { append(detail.replaceFirstChar { it.lowercase() }) }
    // A Machine whose data is not current is quoted entirely in the unavailable colour.
    fun history(word: String, at: ComparableTimeMark) = buildAnnotatedString {
        withStyle(SpanStyle(color = colors.unavailable)) { append(aged(word, colors.unavailable, at)) }
    }
    return when (view) {
        MachineView.Connecting -> told("Connecting", colors.muted, CONNECTING_DETAIL)
        is MachineView.Live -> aged("Live", colors.accent, view.received.at)
        is MachineView.Stale -> history("Stale", view.since)
        is MachineView.Unreachable -> view.last?.let { history("Unreachable", it.at) }
            ?: view.httpStatus?.takeIf { !view.outsideTailnet && it != 503 && it != 421 }?.let { status ->
                line("Unreachable", colors.unavailable) {
                    append("HTTP ")
                    withStyle(mono) { append(status.toString()) }
                }
            }
            ?: told("Unreachable", colors.unavailable, unreachableDetail(view))
        MachineView.Forbidden -> told("Forbidden", colors.text, FORBIDDEN_DETAIL)
        MachineView.Incompatible -> told("Incompatible", colors.text, INCOMPATIBLE_DETAIL)
    }
}

private const val CONNECTING_DETAIL = "Asking for its first Pulse"
private const val FORBIDDEN_DETAIL = "Only answers its Owner's untagged devices"
private const val INCOMPATIBLE_DETAIL = "Update Anshin and the Agent"

private fun unreachableDetail(view: MachineView.Unreachable): String = when {
    view.outsideTailnet -> "Not a tailnet address"
    view.httpStatus == 503 -> "Can't ask tailscaled"
    view.httpStatus == 421 -> "Use its tailnet address or name"
    view.httpStatus != null -> "HTTP ${view.httpStatus}"
    else -> "No Pulse yet"
}

/**
 * What TalkBack reads for a panel, in one fixed order: the Machine name, its state (with the data's age when it isn't
 * current), the Verdict, then CPU, memory, primary disk, CPU temperature and the battery when there is one. Every
 * value the Machine couldn't give is read as "unavailable", never as zero.
 */
internal fun panelAnnouncement(name: String, view: MachineView, now: ComparableTimeMark): String {
    fun history(word: String, last: Received): List<String> {
        val ago = spokenAge(now - last.at)
        val verdict = last.pulse.verdict
        val quoted = verdict?.let { "Last Verdict ${it.level.word}, $ago ago" + reasonClause(it) } ?: "Last Verdict unavailable"
        return listOf("$word, last Sample $ago ago, not current", quoted) + spokenValues(last.pulse)
    }
    val parts = when (view) {
        MachineView.Connecting -> listOf("Connecting", CONNECTING_DETAIL, NO_CURRENT_VERDICT)
        is MachineView.Live -> {
            val pulse = view.received.pulse
            val verdict = pulse.verdict?.let { "Verdict ${it.level.word}" + reasonClause(it) } ?: "Verdict unavailable"
            listOf("Live", verdict) + spokenValues(pulse)
        }
        is MachineView.Stale -> history("Stale", view.last)
        is MachineView.Unreachable -> view.last?.let { history("Unreachable", it) }
            ?: listOf("Unreachable", unreachableDetail(view), NO_CURRENT_VERDICT)
        MachineView.Forbidden -> listOf("Forbidden", FORBIDDEN_DETAIL, NO_CURRENT_VERDICT)
        MachineView.Incompatible -> listOf("Incompatible", INCOMPATIBLE_DETAIL, NO_CURRENT_VERDICT)
    }
    return (listOf(name) + parts).joinToString(". ", postfix = ".")
}

private const val NO_CURRENT_VERDICT = "No current Verdict"

private fun reasonClause(verdict: Verdict): String = verdict.reasons?.firstOrNull()?.let { ": ${reasonText(it)}" } ?: ""

private fun spokenValues(pulse: Pulse): List<String> {
    fun percent(value: Double?) = value?.let { "${it.roundToInt()} percent" } ?: "unavailable"
    val battery = when (val report = pulse.battery) {
        is BatteryReport.Present -> "Battery " + listOfNotNull(percent(report.battery.pct), report.battery.status?.let(::statusWord)).joinToString(", ")
        BatteryReport.NotReported -> "Battery unavailable"
        BatteryReport.NoBattery -> null
    }
    return listOfNotNull(
        "CPU ${percent(pulse.cpuPct)}",
        "Memory ${percent(pulse.memUsedPct)}",
        "Primary disk ${percent(pulse.primaryFilesystem()?.usedPct)}",
        "CPU temperature ${pulse.cpuTempC?.let { "${it.roundToInt()} degrees Celsius" } ?: "unavailable"}",
        battery,
    )
}

private class Resource(val label: String, val value: Double?, val fill: Color, val ticks: List<Double> = emptyList())

/** With [history] every value is a record, not a reading, so all of them take the unavailable colour. */
@Composable
private fun Values(pulse: Pulse, history: Boolean) {
    val colors = AnshinTheme.colors
    fun ink(color: Color) = if (history) colors.unavailable else color
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Resources(
            listOf(
                Resource("CPU", pulse.cpuPct, ink(colors.cpu)),
                Resource("Memory", pulse.memUsedPct, ink(colors.memory)),
                // The Agent's Verdict marks any filesystem warn at 90 % and crit at 95 %; CPU and memory use have no threshold.
                Resource("Primary disk", pulse.primaryFilesystem()?.usedPct, ink(colors.storage), ticks = listOf(90.0, 95.0)),
            ),
            history,
        )
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            ValueRow("CPU temperature", pulse.cpuTempC?.let { "${it.roundToInt()} °C" }, ink(colors.text))
            when (val report = pulse.battery) {
                is BatteryReport.Present -> {
                    val battery = report.battery
                    val value = when {
                        battery.pct == null && battery.status == null -> null
                        else -> listOf(battery.pct?.let(::wholePercent) ?: UNAVAILABLE, battery.status?.let(::statusWord)).filterNotNull().joinToString(" ")
                    }
                    ValueRow("Battery", value, if (battery.pct == null) colors.unavailable else ink(colors.text))
                }
                // The issue asks for the battery when present; the Machine screen still says "no battery" for a desktop.
                BatteryReport.NoBattery -> Unit
                BatteryReport.NotReported -> ValueRow("Battery", null, colors.unavailable)
            }
        }
    }
}

private val GAP = 12.dp

/**
 * Side by side when the widest label and a full "100 %" fit three abreast; otherwise one resource per row, so a large
 * font or a narrow phone reflows rather than clips.
 */
@Composable
private fun Resources(resources: List<Resource>, history: Boolean) {
    val measurer = rememberTextMeasurer()
    val number = numberStyle()
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val widest = (resources.map { measurer.measure(it.label, small).size.width } + measurer.measure("100 %", number).size.width).max()
        val gap = with(LocalDensity.current) { GAP.roundToPx() }
        if (widest * resources.size + gap * (resources.size - 1) <= constraints.maxWidth) {
            Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(GAP)) {
                resources.forEach { resource ->
                    Column(Modifier.weight(1f).fillMaxHeight(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(resource.label, color = AnshinTheme.colors.muted, style = small)
                        ResourceValue(resource, history)
                        Spacer(Modifier.weight(1f))
                        Bar(resource, history)
                    }
                }
            }
        } else {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                resources.forEach { resource ->
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(resource.label, color = AnshinTheme.colors.muted, style = small,
                                modifier = Modifier.padding(end = 8.dp).alignByBaseline())
                            ResourceValue(resource, history, Modifier.alignByBaseline())
                        }
                        Bar(resource, history)
                    }
                }
            }
        }
    }
}

@Composable
private fun numberStyle() = AnshinTheme.type.mono.merge(TextStyle(fontSize = 20.sp, lineHeight = 28.sp))

/**
 * A whole number with its unit as a smaller span, leading space included: `70 %`. The series colour stays on the bar:
 * at 20 sp a number is normal-size text, which needs 4.5:1, and the series tokens only promise 3:1.
 */
@Composable
private fun ResourceValue(resource: Resource, history: Boolean, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    val quiet = resource.value == null || history
    val unit = SpanStyle(fontSize = 13.sp, color = if (quiet) colors.unavailable else colors.muted)
    val text = buildAnnotatedString {
        if (resource.value == null) {
            append("—")
            withStyle(unit) { append(" unavailable") }
        } else {
            append(resource.value.roundToInt().toString())
            withStyle(unit) { append(" %") }
        }
    }
    Text(text, color = if (quiet) colors.unavailable else colors.text, style = numberStyle(), modifier = modifier)
}

/**
 * The usage bar shared with the Machine screen: an 8 dp track with the value's fill, which moves to a new value with
 * [animateCalmly]. A missing value is a hatched, empty track, never a zero-width fill.
 */
@Composable
private fun Bar(resource: Resource, history: Boolean) {
    val colors = AnshinTheme.colors
    val fraction by animateCalmly(((resource.value ?: 0.0) / 100).coerceIn(0.0, 1.0).toFloat(), label = "${resource.label} bar")
    val tick = if (history) colors.unavailable else colors.muted
    Canvas(Modifier.fillMaxWidth().height(8.dp)) {
        val radius = CornerRadius(2.dp.toPx())
        drawRoundRect(colors.border, Offset.Zero, size, radius)
        if (resource.value == null) {
            clipRect {
                drawPath(hatchPath(0f, size.width, 0f, size.height, HATCH_STEP.toPx()), colors.unavailable, style = Stroke(1.dp.toPx()))
            }
        } else if (fraction > 0f) {
            drawRoundRect(resource.fill, Offset.Zero, Size(size.width * fraction, size.height), radius)
        }
        resource.ticks.forEach { at ->
            val x = (size.width * at / 100).toFloat()
            drawLine(tick, Offset(x, 0f), Offset(x, size.height), strokeWidth = 1.dp.toPx())
        }
    }
}

@Composable
private fun ValueRow(label: String, value: String?, ink: Color) {
    val colors = AnshinTheme.colors
    FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label, color = colors.muted, style = small, modifier = Modifier.padding(end = 8.dp).alignByBaseline())
        Text(
            value ?: UNAVAILABLE,
            color = if (value == null) colors.unavailable else ink,
            style = small.merge(AnshinTheme.type.mono),
            modifier = Modifier.alignByBaseline(),
        )
    }
}
