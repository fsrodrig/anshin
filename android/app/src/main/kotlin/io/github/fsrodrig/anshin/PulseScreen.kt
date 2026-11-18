package io.github.fsrodrig.anshin

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.time.ComparableTimeMark

/**
 * Why the latest request got no Pulse; the Machine screen says it in the same words with or without data. The Machine
 * name is a name, so it stays in the sentence's face; the address and the status are values, in mono.
 */
@Composable
internal fun unreachableCause(outsideTailnet: Boolean, httpStatus: Int?, machine: String, address: String = machine): AnnotatedString =
    when {
        outsideTailnet -> valueDetail("", address, " isn't a tailnet address, so Anshin won't connect to it.")
        httpStatus == 503 -> AnnotatedString("The Agent on $machine can't ask tailscaled who this phone is.")
        httpStatus == 421 -> AnnotatedString("The Agent on $machine only answers to its tailnet address or MagicDNS name.")
        httpStatus != null -> valueDetail("$machine answered HTTP ", httpStatus.toString(), " instead of a Pulse.")
        else -> AnnotatedString("No Pulse from $machine. The Machine may be off, its Agent stopped, or this phone off the tailnet.")
    }

@Composable
internal fun VpnHint() =
    Message("VPN looks off", "This phone shows no active VPN. If Tailscale is off, turn it on; Anshin keeps trying either way.")

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

/** The framing of every record drawn from the last data, whose age the screen pins above it. */
internal const val NOT_CURRENT = "Nothing here is current"

/** A Verdict that is no longer current: quoted with its age, without level colours or a level mark. */
@Composable
internal fun LastVerdict(verdict: Verdict?, ago: String, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    VerdictBlock("Last Verdict", modifier) {
        VerdictLabel(null) {
            if (verdict == null) {
                VerdictUnavailable()
            } else {
                Text("${verdict.level.word}, $ago ago", color = colors.unavailable, style = AnshinTheme.type.mono, fontSize = 20.sp, lineHeight = 28.sp)
            }
        }
        Reasons(verdict, colors.unavailable) { colors.unavailable }
    }
}

@Composable
internal fun TopLineView(line: TopLine, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    // Mono is drawn in Regular only, so the Verdict word drops the headline's weight.
    val word = AnshinTheme.type.mono.toSpanStyle().copy(fontWeight = FontWeight.Normal)
    val headline = MaterialTheme.typography.titleLarge
    val body = MaterialTheme.typography.bodyLarge
    // A Reason starts where its row's words do, past the vial and its gap, so the vials stand in one column.
    val reasonIndent = 44.dp

    @Composable
    fun Verdicted(level: VerdictLevel, machine: String, reason: Reason?) {
        VerdictLabel(level) {
            Text(
                buildAnnotatedString {
                    withStyle(word.copy(color = levelColor(level, colors.text))) { append(level.word) }
                    append(" · ")
                    append(machine)
                },
                color = colors.text,
                style = headline,
            )
        }
        reason?.let { Text(reasonText(it), color = colors.text, style = body, modifier = Modifier.padding(start = reasonIndent)) }
    }

    Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when (val worst = line.headline) {
            Headline.AllOk -> VerdictLabel(VerdictLevel.Ok) {
                Text(
                    buildAnnotatedString {
                        append("All Machines ")
                        withStyle(word.copy(color = colors.ok)) { append(VerdictLevel.Ok.word) }
                    },
                    color = colors.text,
                    style = headline,
                )
            }
            is Headline.Worst -> Verdicted(worst.verdict.level, worst.machine, worst.verdict.reasons?.firstOrNull())
            null -> Unit
        }
        line.unranked.forEach { machine -> Verdicted(machine.verdict.level, machine.machine, machine.verdict.reasons?.firstOrNull()) }
        line.withoutVerdict.forEach { machine ->
            VerdictLabel(null) { Text("No current Verdict from $machine", color = colors.unavailable, style = body) }
        }
    }
}

/** The word is always drawn, so a Verdict never relies on colour alone. */
@Composable
internal fun VerdictView(verdict: Verdict?, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    VerdictBlock("Verdict", modifier) {
        if (verdict == null) {
            VerdictLabel(null) { VerdictUnavailable() }
        } else {
            VerdictLabel(verdict.level) {
                Text(verdict.level.word, color = levelColor(verdict.level, colors.text), style = AnshinTheme.type.mono, fontSize = 20.sp, lineHeight = 28.sp)
            }
            Reasons(verdict, colors.text) { levelColor(it.level, colors.muted) }
        }
    }
}

@Composable
private fun VerdictBlock(label: String, modifier: Modifier, content: @Composable () -> Unit) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(label, color = AnshinTheme.colors.muted, fontSize = 13.sp, lineHeight = 18.sp)
        content()
    }
}

@Composable
private fun VerdictUnavailable() =
    Text("Verdict unavailable", color = AnshinTheme.colors.unavailable, style = AnshinTheme.type.mono, fontSize = 16.sp)

@Composable
private fun Reasons(verdict: Verdict?, ink: Color, rule: @Composable (Reason) -> Color) {
    val reasons = verdict?.reasons.orEmpty()
    if (reasons.isEmpty()) return
    Column(Modifier.padding(top = 4.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        reasons.forEach { reason ->
            Row(Modifier.height(IntrinsicSize.Min)) {
                Box(Modifier.width(2.dp).fillMaxHeight().background(rule(reason)))
                Text(reasonText(reason), color = ink, modifier = Modifier.padding(start = 8.dp, top = 2.dp, bottom = 2.dp))
            }
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

internal fun reasonText(reason: Reason): String = reason.message ?: reason.code ?: "A Reason without a message"

@Composable
internal fun valueDetail(prefix: String, value: String, suffix: String): AnnotatedString = buildAnnotatedString {
    append(prefix)
    withStyle(AnshinTheme.type.mono.toSpanStyle()) { append(value) }
    append(suffix)
}

@Composable
internal fun Message(title: String, detail: String) = Message(title, AnnotatedString(detail))

@Composable
internal fun Message(title: String, detail: AnnotatedString) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(title, color = AnshinTheme.colors.text, fontWeight = FontWeight.Medium)
        Text(detail, color = AnshinTheme.colors.muted)
    }
}
