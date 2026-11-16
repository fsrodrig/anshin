package io.github.fsrodrig.anshin

import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import java.util.Locale
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.ui.platform.testTag
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
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.runtime.State
import androidx.compose.runtime.key
import androidx.compose.runtime.produceState
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import kotlin.time.ComparableTimeMark
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeSource
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.flowOn
import okhttp3.HttpUrl
import kotlin.math.roundToInt

@Composable
fun PulseScreen(watcher: Watcher) {
    var input by rememberSaveable { mutableStateOf("") }
    // Watching starts on an explicit Watch, so a half-typed name is never looked up.
    var watched by rememberSaveable { mutableStateOf("") }
    val endpoint = remember(watched) { watched.takeIf { it.isNotEmpty() }?.let(::pulseUrl) }
    // key: a new address never shows the previous Machine's state, even for a frame.
    val view = endpoint?.let { key(it) { watch(watcher, it) } }
    val machines = if (view == null) emptyList() else listOf(WatchedMachine(watched, view))
    val now = ticker(watcher.clock)

    MachineScreens(watched, view, now = { now.value }) { openDetail ->
        PulseLayout(input, onInputChange = { input = it }, onWatch = { watched = input.trim() }, topLine = topLine(machines)) {
            when {
                watched.isEmpty() -> Unit
                view == null -> NotAMachineAddress()
                else -> PulseView(view, machine = watched, now = { now.value }, onDetail = openDetail)
            }
        }
    }
}

/**
 * Collection, and with it every request, stops whenever the screen is not visible. The Flow is remembered, so coming
 * back resumes with the held Window and Cursor, and the Watcher shows Stale rather than an old Live. Merging the
 * Window copies every series, so the Watcher runs off the UI thread.
 */
@Composable
private fun watch(watcher: Watcher, endpoint: HttpUrl): MachineView {
    val flow = remember(endpoint) { watcher.watch(endpoint).flowOn(Dispatchers.Default) }
    val view by flow.collectAsStateWithLifecycle(initialValue = MachineView.Connecting)
    return view
}

/** The Watcher's clock, read once a second while the screen is visible, so ages count up and stop in the background. */
@Composable
private fun ticker(clock: TimeSource.WithComparableMarks): State<ComparableTimeMark> {
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    return produceState(clock.markNow(), clock, lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                value = clock.markNow()
                delay(1.seconds)
            }
        }
    }
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
            AnshinMark()
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

/** [now] reads the Watcher's clock, which every mark in [view] comes from. A card that draws data opens [onDetail]. */
@Composable
internal fun PulseView(view: MachineView, machine: String, now: () -> ComparableTimeMark, onDetail: () -> Unit = {}) {
    when (view) {
        MachineView.Connecting -> Message("Connecting", valueDetail("Asking ", machine, " for its Pulse."))
        is MachineView.Live -> MachineCard(view.received.pulse, view.received.window, onDetail)
        is MachineView.Stale -> MachineCard(view.last.pulse, view.last.window, onDetail, History("Stale", view.since, now))
        MachineView.Forbidden -> Message("Forbidden", "This Agent only answers its Owner's untagged devices.")
        MachineView.Incompatible -> Message(
            "Incompatible",
            "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.",
        )
        is MachineView.Unreachable -> Unreachable(view, machine, now, onDetail)
    }
}

@Composable
private fun Unreachable(view: MachineView.Unreachable, machine: String, now: () -> ComparableTimeMark, onDetail: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Message("Unreachable", unreachableCause(view.outsideTailnet, view.httpStatus, machine))
        NextTry(view.retryAt, now)
        if (view.vpnHint) VpnHint()
        view.last?.let { MachineCard(it.pulse, it.window, onDetail, History(null, it.at, now)) }
    }
}

/** Why the latest request got no Pulse; the card and the detail say it in the same words. */
@Composable
internal fun unreachableCause(outsideTailnet: Boolean, httpStatus: Int?, machine: String): AnnotatedString {
    val mono = AnshinTheme.type.mono.toSpanStyle()
    return when {
        outsideTailnet -> valueDetail("", machine, " isn't a tailnet address, so Anshin won't connect to it.")
        httpStatus == 503 -> valueDetail("The Agent on ", machine, " can't ask tailscaled who this phone is.")
        httpStatus != null -> buildAnnotatedString {
            withStyle(mono) { append(machine) }
            append(" answered HTTP ")
            withStyle(mono) { append(httpStatus.toString()) }
            append(" instead of a Pulse.")
        }
        else -> valueDetail("No Pulse from ", machine, ". The Machine may be off, its Agent stopped, or this phone off the tailnet.")
    }
}

@Composable
internal fun VpnHint() {
    val colors = AnshinTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("VPN looks off", color = colors.text)
        Text("This phone shows no active VPN. If Tailscale is off, turn it on; Anshin keeps trying either way.", color = colors.muted)
    }
}

// Its own scope, so the ticking clock recomposes only this line.
@Composable
internal fun NextTry(retryAt: ComparableTimeMark, now: () -> ComparableTimeMark) {
    val wait = retryAt - now()
    val mono = AnshinTheme.type.mono.toSpanStyle()
    Text(buildAnnotatedString {
        if (wait.isPositive()) {
            append("Next try in ")
            withStyle(mono) { append(age(wait)) }
        } else {
            append("Trying again")
        }
    }, color = AnshinTheme.colors.muted)
}

/** A card drawn from the last data the phone holds, never current: [label] heads it and every value is aged from [at]. */
private class History(val label: String?, val at: ComparableTimeMark, val now: () -> ComparableTimeMark)

/**
 * One Machine's values. Live when [history] is null: level colours, the Verdict vial and the details button. With
 * [history] it is a record, not a reading: every value, series and the Verdict in the unavailable colour, with its age.
 * Either way a tap opens [onDetail], where the same record is shown in full.
 */
@Composable
private fun MachineCard(pulse: Pulse, window: HeldWindow?, onDetail: () -> Unit, history: History? = null) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    val ago = history?.let { age(it.now() - it.at) }
    fun ink(color: Color) = if (history == null) color else colors.unavailable
    Column(Modifier.testTag("machine-card").clickable(onClickLabel = "Machine details", onClick = onDetail),
        verticalArrangement = Arrangement.spacedBy(4.dp)) {
        if (history == null) {
            Text("Live", color = colors.accent, fontSize = 13.sp)
        } else {
            history.label?.let { Text(it, color = colors.unavailable, fontSize = 13.sp) }
            Text(notCurrent(ago!!), color = colors.muted)
        }
        Text(pulse.hostname ?: UNAVAILABLE, color = if (pulse.hostname == null) colors.unavailable else ink(colors.text), style = mono, fontSize = 22.sp)
        Spacer(Modifier.height(8.dp))
        if (ago == null) VerdictView(pulse.verdict) else LastVerdict(pulse.verdict, ago)
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
            color = if (pulse.cpuPct == null) colors.unavailable else ink(colors.cpu),
            style = mono,
            fontSize = 40.sp,
        )
        WindowChart("CPU", window, { it.cpuPct }, ink(colors.cpu), ago)
        Text("CPU temperature", color = colors.muted, fontSize = 13.sp)
        Text(pulse.cpuTempC?.let { "${it.roundToInt()} °C" } ?: UNAVAILABLE,
            color = if (pulse.cpuTempC == null) colors.unavailable else ink(colors.cpu), style = mono, fontSize = if (pulse.cpuTempC == null) 16.sp else 28.sp)
        Text("Memory", color = colors.muted, fontSize = 13.sp)
        Text(pulse.memUsedPct?.let { "${it.roundToInt()}%" } ?: UNAVAILABLE,
            color = if (pulse.memUsedPct == null) colors.unavailable else ink(colors.memory), style = mono, fontSize = if (pulse.memUsedPct == null) 16.sp else 28.sp)
        WindowChart("Memory", window, { it.memUsedPct }, ink(colors.memory), ago)
        val disk = pulse.filesystems?.firstOrNull { pulse.primaryDisk != null && it.majMin == pulse.primaryDisk }
        Text("Primary disk", color = colors.muted, fontSize = 13.sp)
        Text(disk?.usedPct?.let { "$it%" } ?: UNAVAILABLE,
            color = if (disk?.usedPct == null) colors.unavailable else ink(colors.storage), style = mono,
            fontSize = if (disk?.usedPct == null) 16.sp else 28.sp)
        if (disk?.usedPct == null && disk?.ageS != null) {
            Text("Capacity age ${disk.ageS.toLong()} s", color = colors.unavailable, style = mono)
        }
        Text("Swap", color = colors.muted, fontSize = 13.sp)
        val swap = if (pulse.swapUsedBytes == null && pulse.swapTotalBytes == null) UNAVAILABLE
            else "${pulse.swapUsedBytes?.let(::bytes) ?: UNAVAILABLE} / ${pulse.swapTotalBytes?.let(::bytes) ?: UNAVAILABLE}"
        Text(swap, color = if (pulse.swapUsedBytes == null && pulse.swapTotalBytes == null) colors.unavailable else ink(colors.text), style = mono)
        Text("Battery", color = colors.muted, fontSize = 13.sp)
        when (val report = pulse.battery) {
            is BatteryReport.Present -> {
                val pct = report.battery.pct
                Text(pct?.let { "${it.roundToInt()}%" } ?: UNAVAILABLE, color = if (pct == null) colors.unavailable else ink(colors.text),
                    style = mono, fontSize = if (pct == null) 16.sp else 28.sp)
                val status = report.battery.status
                Text(status?.let(::statusWord) ?: UNAVAILABLE, color = if (status == null) colors.unavailable else ink(colors.text), style = mono)
            }
            // A Machine without a system battery is a fact, not missing data.
            BatteryReport.NoBattery -> Text("no battery", color = ink(colors.text), style = mono, fontSize = 16.sp)
            BatteryReport.NotReported -> Text(UNAVAILABLE, color = colors.unavailable, style = mono, fontSize = 16.sp)
        }
        if (history == null) Button(onClick = onDetail) { Text("Machine details") }
    }
}

/** The framing of every record drawn from the last data: its age, and that none of it is current. */
@Composable
internal fun notCurrent(ago: String): AnnotatedString = buildAnnotatedString {
    append("Last Sample ")
    withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(ago) }
    append(" ago. Nothing here is current.")
}

/** A Verdict that is no longer current: quoted with its age, without level colours or a level mark. */
@Composable
internal fun LastVerdict(verdict: Verdict?, ago: String) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Text("Last Verdict", color = colors.muted, fontSize = 13.sp)
    VerdictLabel(null) {
        Text(verdict?.let { "${it.level.word}, $ago ago" } ?: "Verdict unavailable", color = colors.unavailable, style = mono, fontSize = 16.sp)
    }
    verdict?.reasons.orEmpty().forEach { reason ->
        Row(Modifier.height(IntrinsicSize.Min).padding(top = 4.dp)) {
            Box(Modifier.width(2.dp).fillMaxHeight().background(colors.unavailable))
            Text(reasonText(reason), color = colors.unavailable, modifier = Modifier.padding(start = 8.dp, top = 2.dp, bottom = 2.dp))
        }
    }
}

@Composable
private fun TopLineView(line: TopLine) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when (val headline = line.headline) {
            Headline.AllOk -> VerdictLabel(VerdictLevel.Ok) { Text("ALL MACHINES OK", color = colors.ok, style = mono, fontSize = 17.sp) }
            is Headline.Worst -> {
                VerdictLabel(headline.verdict.level) {
                    Text(
                        buildAnnotatedString {
                            withStyle(mono.copy(color = levelColor(headline.verdict.level, colors.text)).toSpanStyle()) {
                                append(headline.verdict.level.word.uppercase(Locale.ROOT))
                            }
                            append(" · ")
                            withStyle(mono.toSpanStyle()) { append(headline.machine) }
                        },
                        color = colors.text,
                        fontSize = 17.sp,
                    )
                }
                headline.verdict.reasons?.firstOrNull()?.let { Text(reasonText(it), color = colors.muted, fontSize = 13.sp) }
            }
            null -> Unit
        }
        line.unranked.forEach { machine ->
            VerdictLabel(machine.verdict.level) {
                Text(
                    buildAnnotatedString {
                        withStyle(mono.toSpanStyle()) { append(machine.verdict.level.word) }
                        append(" · ")
                        withStyle(mono.toSpanStyle()) { append(machine.machine) }
                    },
                    color = colors.text,
                    fontSize = 17.sp,
                )
            }
            machine.verdict.reasons?.firstOrNull()?.let { Text(reasonText(it), color = colors.text, fontSize = 13.sp) }
        }
        line.withoutVerdict.forEach { machine ->
            VerdictLabel(null) {
                Text(valueDetail("No current Verdict from ", machine, ""), color = colors.unavailable, fontSize = 13.sp)
            }
        }
    }
}

/** The word is always drawn, so a Verdict never relies on colour alone. */
@Composable
internal fun VerdictView(verdict: Verdict?) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Text("Verdict", color = colors.muted, fontSize = 13.sp)
    if (verdict == null) {
        VerdictLabel(null) { Text("Verdict unavailable", color = colors.unavailable, style = mono, fontSize = 16.sp) }
        return
    }
    VerdictLabel(verdict.level) { Text(verdict.level.word, color = levelColor(verdict.level, colors.text), style = mono, fontSize = 22.sp) }
    verdict.reasons.orEmpty().forEach { reason ->
        Row(Modifier.height(IntrinsicSize.Min).padding(top = 4.dp)) {
            Box(Modifier.width(2.dp).fillMaxHeight().background(levelColor(reason.level, colors.muted)))
            Text(reasonText(reason), color = colors.text, modifier = Modifier.padding(start = 8.dp, top = 2.dp, bottom = 2.dp))
        }
    }
}

@Composable
internal fun VerdictLabel(level: VerdictLevel?, content: @Composable () -> Unit) {
    val colors = AnshinTheme.colors
    val outline = if (level == null) colors.unavailable else colors.text
    val mark = levelColor(level, colors.text)
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Canvas(Modifier.size(36.dp, 21.dp)) {
            withTransform({
                scale(size.width / 48f, size.height / 28f, pivot = Offset.Zero)
                translate(0f, -10f)
            }) {
                if (level == VerdictLevel.Crit) {
                    drawPath(Path().apply {
                        moveTo(34f, 14f)
                        arcTo(androidx.compose.ui.geometry.Rect(24f, 14f, 44f, 34f), -90f, 180f, false)
                        close()
                    }, mark)
                }
                drawRoundRect(outline, Offset(4f, 14f), Size(40f, 20f), CornerRadius(10f), style = Stroke(4f))
                drawLine(outline, Offset(14f, 14f), Offset(14f, 34f), strokeWidth = 4f)
                drawLine(outline, Offset(34f, 14f), Offset(34f, 34f), strokeWidth = 4f)
                when (level) {
                    VerdictLevel.Ok -> drawCircle(mark, 5f, Offset(24f, 24f))
                    VerdictLevel.Warn -> drawCircle(mark, 5f, Offset(34f, 24f))
                    VerdictLevel.Crit -> Unit
                    is VerdictLevel.Other, null -> Unit
                }
            }
        }
        content()
    }
}

/** An unknown or missing level gets [neutral]; green, amber and red are only for the words the Agent defines. */
@Composable
internal fun levelColor(level: VerdictLevel?, neutral: Color): Color = when (level) {
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
