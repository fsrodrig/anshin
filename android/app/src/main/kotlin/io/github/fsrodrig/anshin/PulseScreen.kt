package io.github.fsrodrig.anshin

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import java.util.Locale
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Button
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.onStart
import okhttp3.HttpUrl
import kotlin.math.roundToInt

private const val UNAVAILABLE = "— unavailable"

@Composable
fun PulseScreen(link: AgentLink) {
    var input by rememberSaveable { mutableStateOf("") }
    // Polling starts on an explicit Watch, so a half-typed name is never looked up.
    var watched by rememberSaveable { mutableStateOf("") }
    val endpoint = remember(watched) { watched.takeIf { it.isNotEmpty() }?.let(::pulseUrl) }
    val result = endpoint?.let { watchPulses(link, it) }
    val machines = if (endpoint == null) emptyList() else listOf(WatchedMachine(watched, result))

    PulseLayout(input, onInputChange = { input = it }, onWatch = { watched = input.trim() }, topLine = topLine(machines)) {
        when {
            watched.isEmpty() -> Unit
            endpoint == null -> NotAMachineAddress()
            else -> PulseView(result, machine = watched)
        }
    }
}

@Composable
private fun watchPulses(link: AgentLink, endpoint: HttpUrl): PulseResult? {
    // Collection, and with it every request, stops whenever the screen is not visible. Each restart begins at null
    // (Connecting), so a Pulse from before the screen was hidden is never shown as Live.
    val result by remember(endpoint) { link.pulses(endpoint).onStart<PulseResult?> { emit(null) } }
        .collectAsStateWithLifecycle(initialValue = null)
    return result
}

/** The screen without its state, so each state can be drawn on its own. [status] shows what Watch found. */
@Composable
internal fun PulseLayout(
    input: String,
    onInputChange: (String) -> Unit,
    onWatch: () -> Unit,
    topLine: TopLine? = null,
    status: @Composable () -> Unit,
) {
    val colors = AnshinTheme.colors
    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(colors.background)
            .safeDrawingPadding()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Image(painterResource(R.drawable.ic_anshin), contentDescription = null, modifier = Modifier.size(24.dp))
            Text("anshin", color = colors.text, fontSize = 20.sp)
        }
        topLine?.let { TopLineView(it) }
        OutlinedTextField(
            value = input,
            onValueChange = onInputChange,
            modifier = Modifier.fillMaxWidth(),
            textStyle = LocalTextStyle.current.merge(AnshinTheme.type.mono),
            label = { Text("Machine") },
            placeholder = { Text("MagicDNS name or tailnet IP") },
            supportingText = {
                Text(buildAnnotatedString {
                    append("The Agent's port is ")
                    withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(AGENT_PORT.toString()) }
                    append(" unless you add :port")
                })
            },
            singleLine = true,
            keyboardOptions = KeyboardOptions(
                autoCorrectEnabled = false,
                keyboardType = KeyboardType.Uri,
                imeAction = ImeAction.Go,
            ),
            keyboardActions = KeyboardActions(onGo = { onWatch() }),
        )
        Button(onClick = onWatch, enabled = input.isNotBlank()) { Text("Watch") }
        status()
    }
}

@Composable
internal fun NotAMachineAddress() {
    Message("Not a Machine address", "Enter a MagicDNS name or a tailnet IP, optionally with :port.")
}

@Composable
internal fun PulseView(result: PulseResult?, machine: String) {
    var detail by rememberSaveable(machine) { mutableStateOf(false) }
    BackHandler(enabled = detail) { detail = false }
    when (result) {
        null -> Message("Connecting", valueDetail("Asking ", machine, " for its Pulse."))
        is PulseResult.Ok -> if (detail) {
            MachineDetail(result.pulse, onBack = { detail = false })
        } else {
            Live(result.pulse, onDetail = { detail = true })
        }
        PulseResult.Forbidden -> Message("Forbidden", "This Agent only answers its Owner's untagged devices.")
        PulseResult.Incompatible -> Message(
            "Incompatible",
            "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.",
        )
        is PulseResult.Unreachable -> if (result.outsideTailnet) {
            Message("Unreachable", valueDetail("", machine, " isn't a tailnet address, so Anshin won't connect to it."))
        } else {
            Message(
                "Unreachable",
                valueDetail("No Pulse from ", machine, ". The Machine may be off, its Agent stopped, or this phone off the tailnet."),
            )
        }
        is PulseResult.Failed -> if (result.httpStatus == 503) {
            Message("Agent can't reach tailscaled", valueDetail("The Agent on ", machine, " can't ask tailscaled who this phone is."))
        } else {
            Message("No Pulse", buildAnnotatedString {
                withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(machine) }
                append(" answered HTTP ")
                withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(result.httpStatus.toString()) }
                append(" instead of a Pulse.")
            })
        }
    }
}

@Composable
private fun Live(pulse: Pulse, onDetail: () -> Unit) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("Live", color = colors.accent, fontSize = 13.sp)
        Text(pulse.hostname ?: UNAVAILABLE, color = if (pulse.hostname == null) colors.unavailable else colors.text, style = mono, fontSize = 22.sp)
        Spacer(Modifier.height(8.dp))
        VerdictView(pulse.verdict)
        Spacer(Modifier.height(8.dp))
        Text("CPU", color = colors.muted, fontSize = 13.sp)
        Text(
            buildAnnotatedString {
                if (pulse.cpuPct == null) {
                    append("— ")
                    withStyle(mono.copy(fontSize = 16.sp).toSpanStyle()) { append("unavailable") }
                } else {
                    append("${pulse.cpuPct.roundToInt()}%")
                }
            },
            color = if (pulse.cpuPct == null) colors.unavailable else colors.cpu,
            style = mono,
            fontSize = 40.sp,
        )
        Text("CPU temperature", color = colors.muted, fontSize = 13.sp)
        Text(pulse.cpuTempC?.let { "${it.roundToInt()} °C" } ?: UNAVAILABLE,
            color = if (pulse.cpuTempC == null) colors.unavailable else colors.cpu, style = mono, fontSize = if (pulse.cpuTempC == null) 16.sp else 28.sp)
        Text("Memory", color = colors.muted, fontSize = 13.sp)
        Text(pulse.memUsedPct?.let { "${it.roundToInt()}%" } ?: UNAVAILABLE,
            color = if (pulse.memUsedPct == null) colors.unavailable else colors.memory, style = mono, fontSize = if (pulse.memUsedPct == null) 16.sp else 28.sp)
        val disk = pulse.filesystems?.firstOrNull { pulse.primaryDisk != null && it.majMin == pulse.primaryDisk }
        Text("Primary disk", color = colors.muted, fontSize = 13.sp)
        Text(disk?.usedPct?.let { "$it%" } ?: UNAVAILABLE,
            color = if (disk?.usedPct == null) colors.unavailable else colors.storage, style = mono,
            fontSize = if (disk?.usedPct == null) 16.sp else 28.sp)
        if (disk?.usedPct == null && disk?.ageS != null) {
            Text("Capacity age ${disk.ageS.toLong()} s", color = colors.unavailable, style = mono)
        }
        Text("Swap", color = colors.muted, fontSize = 13.sp)
        val swap = if (pulse.swapUsedBytes == null && pulse.swapTotalBytes == null) UNAVAILABLE
            else "${bytes(pulse.swapUsedBytes)} / ${bytes(pulse.swapTotalBytes)}"
        Text(swap, color = if (pulse.swapUsedBytes == null && pulse.swapTotalBytes == null) colors.unavailable else colors.text, style = mono)
        Text("Battery", color = colors.muted, fontSize = 13.sp)
        when (val report = pulse.battery) {
            is BatteryReport.Present -> {
                val pct = report.battery.pct
                Text(pct?.let { "${it.roundToInt()}%" } ?: UNAVAILABLE, color = if (pct == null) colors.unavailable else colors.text,
                    style = mono, fontSize = if (pct == null) 16.sp else 28.sp)
                val status = report.battery.status
                Text(status?.let(::statusWord) ?: UNAVAILABLE, color = if (status == null) colors.unavailable else colors.text, style = mono)
            }
            // A Machine without a system battery is a fact, not missing data.
            BatteryReport.NoBattery -> Text("no battery", color = colors.text, style = mono, fontSize = 16.sp)
            BatteryReport.NotReported -> Text(UNAVAILABLE, color = colors.unavailable, style = mono, fontSize = 16.sp)
        }
        Button(onClick = onDetail) { Text("Machine details") }
    }
}

@Composable
private fun TopLineView(line: TopLine) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when (val headline = line.headline) {
            Headline.AllOk -> Text("ALL MACHINES OK", color = colors.ok, style = mono, fontSize = 17.sp)
            is Headline.Worst -> {
                Text(
                    buildAnnotatedString {
                        withStyle(mono.copy(color = levelColor(headline.verdict.level, colors.text)).toSpanStyle()) {
                            append(headline.verdict.level.let { if (it is VerdictLevel.Other) it.word else it.word.uppercase(Locale.ROOT) })
                        }
                        append(" · ")
                        withStyle(mono.toSpanStyle()) { append(headline.machine) }
                    },
                    color = colors.text,
                    fontSize = 17.sp,
                )
                headline.verdict.reasons?.firstOrNull()?.let { Text(reasonText(it), color = colors.muted, fontSize = 13.sp) }
            }
            null -> Unit
        }
        line.unranked.forEach { machine ->
            Text(valueDetail(machine.verdict.level.word + " · ", machine.machine, ""), color = colors.text, fontSize = 17.sp)
            machine.verdict.reasons?.firstOrNull()?.let { Text(reasonText(it), color = colors.text, fontSize = 13.sp) }
        }
        line.withoutVerdict.forEach { machine ->
            Text(valueDetail("No current Verdict from ", machine, ""), color = colors.unavailable, fontSize = 13.sp)
        }
    }
}

/** The word is always drawn, so a Verdict never relies on colour alone. */
@Composable
private fun VerdictView(verdict: Verdict?) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Text("Verdict", color = colors.muted, fontSize = 13.sp)
    if (verdict == null) {
        Text("Verdict unavailable", color = colors.unavailable, style = mono, fontSize = 16.sp)
        return
    }
    Text(verdict.level.word, color = levelColor(verdict.level, colors.text), style = mono, fontSize = 22.sp)
    verdict.reasons.orEmpty().forEach { reason ->
        Row(Modifier.height(IntrinsicSize.Min).padding(top = 4.dp)) {
            Box(Modifier.width(2.dp).fillMaxHeight().background(levelColor(reason.level, colors.muted)))
            Text(reasonText(reason), color = colors.text, modifier = Modifier.padding(start = 8.dp, top = 2.dp, bottom = 2.dp))
        }
    }
}

/** An unknown or missing level gets [neutral]; green, amber and red are only for the words the Agent defines. */
@Composable
private fun levelColor(level: VerdictLevel?, neutral: Color): Color = when (level) {
    VerdictLevel.Ok -> AnshinTheme.colors.ok
    VerdictLevel.Warn -> AnshinTheme.colors.warn
    VerdictLevel.Crit -> AnshinTheme.colors.crit
    is VerdictLevel.Other, null -> neutral
}

private fun reasonText(reason: Reason): String = reason.message ?: reason.code ?: "A Reason without a message"

@Composable
private fun valueDetail(prefix: String, value: String, suffix: String): AnnotatedString = buildAnnotatedString {
    append(prefix)
    withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(value) }
    append(suffix)
}

@Composable
private fun Message(title: String, detail: String) = Message(title, AnnotatedString(detail))

@Composable
private fun Message(title: String, detail: AnnotatedString) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(title, color = AnshinTheme.colors.text, fontSize = 17.sp)
        Text(detail, color = AnshinTheme.colors.muted)
    }
}

private fun statusWord(status: BatteryStatus): String = when (status) {
    BatteryStatus.Charging -> "charging"
    BatteryStatus.Discharging -> "discharging"
    BatteryStatus.NotCharging -> "not charging"
    BatteryStatus.Full -> "full"
    BatteryStatus.Unknown -> "unknown"
}

private fun bytes(value: Long?): String = when {
    value == null -> UNAVAILABLE
    value < 1024 -> "$value B"
    value < 1024L * 1024 -> String.format(Locale.ROOT, "%.1f KiB", value / 1024.0)
    value < 1024L * 1024 * 1024 -> String.format(Locale.ROOT, "%.1f MiB", value / (1024.0 * 1024))
    else -> String.format(Locale.ROOT, "%.1f GiB", value / (1024.0 * 1024 * 1024))
}

@Composable
internal fun MachineDetail(pulse: Pulse, onBack: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Button(onClick = onBack) { Text("Back to Machine") }
        Text("Machine details", color = AnshinTheme.colors.accent, fontSize = 22.sp)
        DetailValue("Hostname", pulse.hostname)
        DetailValue("CPU", pulse.cpuPct?.let { "$it%" })
        DetailValue("CPU temperature", pulse.cpuTempC?.let { "$it °C" })
        DetailValue("CPU sensor", pulse.cpuSensor, selectable = true)
        DetailValue("Memory used", pulse.memUsedBytes?.let { "$it B" })
        DetailValue("Memory total", pulse.memTotalBytes?.let { "$it B" })
        DetailValue("Memory available", pulse.memAvailableBytes?.let { "$it B" })
        DetailValue("Memory used %", pulse.memUsedPct?.let { "$it%" })
        DetailValue("Swap used", pulse.swapUsedBytes?.let { "$it B" })
        DetailValue("Swap total", pulse.swapTotalBytes?.let { "$it B" })
        if (pulse.swapEntries.isNullOrEmpty()) {
            DetailValue("Swap entries", null)
        } else {
            pulse.swapEntries.forEach { entry ->
                DetailValue("Swap entry", entry.name)
                DetailValue("Swap type", when (entry.zram) { true -> "zram"; false -> "swap"; null -> null })
                DetailValue("Entry used", entry.usedBytes?.let { "$it B" })
                DetailValue("Entry total", entry.totalBytes?.let { "$it B" })
            }
        }
        Text("Filesystems", color = AnshinTheme.colors.storage, fontSize = 17.sp)
        if (pulse.filesystems.isNullOrEmpty()) {
            DetailValue("Filesystem", null)
        } else {
            pulse.filesystems.forEach { disk ->
                DetailValue("Mount point", disk.mountPoint)
                DetailValue("Filesystem type", disk.fsType)
                DetailValue("Used %", disk.usedPct?.let { "$it%" })
                DetailValue("Used", disk.usedBytes?.let { "$it B" })
                DetailValue("Total", disk.totalBytes?.let { "$it B" })
                DetailValue("Available", disk.availableBytes?.let { "$it B" })
                DetailValue("Capacity age", disk.ageS?.let { "${it.toLong()} s" })
            }
        }
        Text("Temperatures", color = AnshinTheme.colors.text, fontSize = 17.sp)
        if (pulse.temperatures.isNullOrEmpty()) {
            DetailValue("Sensor", null)
        } else {
            pulse.temperatures.forEach { sensor ->
                DetailValue("Sensor", sensor.id, selectable = true)
                // A slow-lane value is the last one read, so its age stays beside it rather than passing as current.
                DetailValue("Temperature", sensor.tempC?.let { t -> sensor.ageS?.takeIf { it > 0 }?.let { "$t °C ($it s ago)" } ?: "$t °C" })
            }
        }
        Text("Battery", color = AnshinTheme.colors.text, fontSize = 17.sp)
        when (val report = pulse.battery) {
            is BatteryReport.Present -> BatteryDetail(report.battery)
            BatteryReport.NoBattery -> DetailValue("Battery", "no battery")
            BatteryReport.NotReported -> DetailValue("Battery", null)
        }
        DetailValue("Load 1 min", pulse.load1?.toString())
        DetailValue("Load 5 min", pulse.load5?.toString())
        DetailValue("Load 15 min", pulse.load15?.toString())
        DetailValue("Uptime", pulse.uptimeS?.let { "$it s" })
        DetailValue("Distro", pulse.distro)
        DetailValue("Kernel", pulse.kernel)
        DetailValue("Architecture", pulse.arch)
        DetailValue("Agent version", pulse.agentVersion)
        Button(onClick = onBack) { Text("Back to Machine") }
    }
}

@Composable
private fun BatteryDetail(battery: Battery) {
    DetailValue("Battery charge", battery.pct?.let { "$it%" })
    DetailValue("Battery status", battery.status?.let(::statusWord))
    DetailValue("Battery power", battery.powerW?.let { "$it W" })
    // The Agent sends time left only while discharging; an unknown status leaves the reason unknown too.
    DetailValue("Time left", when (battery.status) {
        BatteryStatus.Discharging -> battery.timeLeftS?.let { "${it.toLong()} s" }
        null, BatteryStatus.Unknown -> null
        else -> "not discharging"
    })
    if (battery.batteries.isNullOrEmpty()) {
        DetailValue("Battery", null)
    } else {
        battery.batteries.forEach { row ->
            DetailValue("Battery", row.name)
            DetailValue("Charge", row.pct?.let { "$it%" })
            DetailValue("Status", row.status?.let(::statusWord))
            DetailValue("Power", row.powerW?.let { "$it W" })
            DetailValue("Health", row.healthPct?.let { "$it%" })
        }
    }
}

@Composable
private fun DetailValue(label: String, value: String?, selectable: Boolean = false) {
    Column {
        Text(label, color = AnshinTheme.colors.muted, fontSize = 13.sp)
        val text = @Composable {
            Text(value ?: UNAVAILABLE, color = if (value == null) AnshinTheme.colors.unavailable else AnshinTheme.colors.text,
                style = AnshinTheme.type.mono)
        }
        // The Owner copies a sensor ID from here into ANSHIN_CPU_SENSOR.
        if (selectable && value != null) SelectionContainer { text() } else text()
    }
}
