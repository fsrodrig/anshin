package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.key
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.roundToInt
import kotlin.time.ComparableTimeMark

internal const val ADD_MACHINE_TAG = "add-machine"

/** One saved Machine and how to read what the phone shows of it, read only by the panel that draws it. */
internal class Panel(val machine: Machine, val view: () -> MachineView)

/** Every saved Machine's panel, in saved order, then Add machine. */
@Composable
internal fun Dashboard(
    panels: List<Panel>,
    now: () -> ComparableTimeMark,
    onOpen: (MachineId) -> Unit,
    onAdd: () -> Unit,
    topLine: () -> TopLine? = { null },
) {
    AppFrame(topLine) {
        if (panels.isEmpty()) {
            Message("No Machines yet", "Add a Machine running the Anshin Agent by its MagicDNS name or tailnet IP.")
        }
        panels.forEach { panel ->
            key(panel.machine.id) { MachinePanel(panel.machine, panel.view, now, onOpen = { onOpen(panel.machine.id) }) }
        }
        Button(onClick = onAdd, modifier = Modifier.testTag(ADD_MACHINE_TAG)) { Text("Add machine") }
    }
}

/**
 * One Machine at a glance. It shows no Verdict: the top line carries the worst current one and the Machine screen
 * each one, so a panel never shows a reassuring Verdict beside data that isn't current.
 */
@Composable
internal fun MachinePanel(machine: Machine, view: () -> MachineView, now: () -> ComparableTimeMark, onOpen: () -> Unit) {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(8.dp)
    val current = view()
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(colors.surface)
            .border(1.dp, colors.border, shape)
            .clickable(role = Role.Button, onClickLabel = "Open ${machine.name}", onClick = onOpen)
            .heightIn(min = 48.dp)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Top) {
            Text(machine.name, color = colors.text, fontSize = 17.sp, lineHeight = 22.sp, modifier = Modifier.weight(1f))
            Spacer(Modifier.width(8.dp))
            val (word, ink) = stateWord(current)
            Text(word, color = ink, style = small)
        }
        Text(machine.address.toString(), color = colors.muted, style = small.merge(AnshinTheme.type.mono))
        Text(statusLine(current, now), color = colors.muted, style = small)
        when (current) {
            is MachineView.Live -> Values(current.received.pulse, history = false)
            is MachineView.Stale -> Values(current.last.pulse, history = true)
            is MachineView.Unreachable -> current.last?.let { Values(it.pulse, history = true) }
            // Forbidden and Incompatible withhold the values; Connecting has none yet.
            MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible -> Unit
        }
    }
}

private val small = TextStyle(fontSize = 13.sp, lineHeight = 18.sp)

@Composable
private fun stateWord(view: MachineView): Pair<String, Color> {
    val colors = AnshinTheme.colors
    return when (view) {
        MachineView.Connecting -> "Connecting" to colors.muted
        is MachineView.Live -> "Live" to colors.accent
        is MachineView.Stale -> "Stale" to colors.unavailable
        is MachineView.Unreachable -> "Unreachable" to colors.unavailable
        MachineView.Forbidden -> "Forbidden" to colors.text
        MachineView.Incompatible -> "Incompatible" to colors.text
    }
}

@Composable
private fun statusLine(view: MachineView, now: () -> ComparableTimeMark): AnnotatedString {
    val mono = AnshinTheme.type.mono.toSpanStyle()
    fun aged(prefix: String, at: ComparableTimeMark, suffix: String) = buildAnnotatedString {
        append(prefix)
        withStyle(mono) { append(age(now() - at)) }
        append(suffix)
    }
    return when (view) {
        MachineView.Connecting -> AnnotatedString("Asking for its first Pulse")
        is MachineView.Live -> aged("Updated ", view.received.at, " ago")
        is MachineView.Stale -> aged("Last Sample ", view.since, " ago. Not current.")
        is MachineView.Unreachable -> view.last?.let { aged("Last Sample ", it.at, " ago. Not current.") } ?: when {
            view.outsideTailnet -> AnnotatedString("Not a tailnet address")
            view.httpStatus == 503 -> AnnotatedString("Can't ask tailscaled")
            view.httpStatus != null -> buildAnnotatedString {
                append("HTTP ")
                withStyle(mono) { append(view.httpStatus.toString()) }
            }
            else -> AnnotatedString("No Pulse yet")
        }
        MachineView.Forbidden -> AnnotatedString("Only answers its Owner's untagged devices")
        MachineView.Incompatible -> AnnotatedString("Update Anshin and the Agent")
    }
}

/** With [history] every value is a record, not a reading, so all of them take the unavailable colour. */
@Composable
private fun Values(pulse: Pulse, history: Boolean) {
    val colors = AnshinTheme.colors
    fun ink(color: Color) = if (history) colors.unavailable else color
    fun pct(value: Double?) = value?.let { "${it.roundToInt()}%" }
    ValueRow("CPU", pct(pulse.cpuPct), ink(colors.cpu))
    ValueRow("Memory", pct(pulse.memUsedPct), ink(colors.memory))
    ValueRow("Primary disk", pct(pulse.primaryFilesystem()?.usedPct), ink(colors.storage))
    ValueRow("CPU temperature", pulse.cpuTempC?.let { "${it.roundToInt()} °C" }, ink(colors.cpu))
    when (val report = pulse.battery) {
        is BatteryReport.Present -> {
            val battery = report.battery
            val value = when {
                battery.pct == null && battery.status == null -> null
                else -> listOf(pct(battery.pct) ?: UNAVAILABLE, battery.status?.let(::statusWord)).filterNotNull().joinToString(" ")
            }
            ValueRow("Battery", value, if (battery.pct == null) colors.unavailable else ink(colors.text))
        }
        // The issue asks for the battery when present; the Machine screen still says "no battery" for a desktop.
        BatteryReport.NoBattery -> Unit
        BatteryReport.NotReported -> ValueRow("Battery", null, colors.unavailable)
    }
}

@Composable
private fun ValueRow(label: String, value: String?, ink: Color) {
    val colors = AnshinTheme.colors
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
        Text(label, color = colors.muted, style = small)
        Text(value ?: UNAVAILABLE, color = if (value == null) colors.unavailable else ink, style = small.merge(AnshinTheme.type.mono))
    }
}
