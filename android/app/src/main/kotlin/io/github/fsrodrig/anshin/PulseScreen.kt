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
import java.util.Locale
import kotlin.time.ComparableTimeMark

/** Why the latest request got no Pulse; the Machine screen says it in the same words with or without data. */
@Composable
internal fun unreachableCause(outsideTailnet: Boolean, httpStatus: Int?, machine: String, address: String = machine): AnnotatedString {
    val mono = AnshinTheme.type.mono.toSpanStyle()
    return when {
        outsideTailnet -> valueDetail("", address, " isn't a tailnet address, so Anshin won't connect to it.")
        httpStatus == 503 -> valueDetail("The Agent on ", machine, " can't ask tailscaled who this phone is.")
        httpStatus == 421 -> valueDetail("The Agent on ", machine, " only answers to its tailnet address or MagicDNS name.")
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
internal fun TopLineView(line: TopLine, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    val mono = AnshinTheme.type.mono
    Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
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
