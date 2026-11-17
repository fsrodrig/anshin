package io.github.fsrodrig.anshin

import java.util.Locale
import kotlin.math.floor
import kotlin.math.roundToInt
import kotlin.time.Duration

/** The one wording for a measurement the Machine couldn't give. Never a zero, never a guess. */
internal const val UNAVAILABLE = "— unavailable"

internal fun percent(value: Double): String = String.format(Locale.ROOT, "%.1f %%", value)

internal fun wholePercent(value: Double): String = "${value.roundToInt()} %"

private val BINARY_UNITS = listOf("KiB", "MiB", "GiB", "TiB", "PiB", "EiB")

/** Binary units to one decimal. A mantissa that would round to 1024 moves up a unit, so it reads `1.0 MiB`, never `1024.0 KiB`. */
internal fun bytes(value: Long): String {
    if (value < 1024) return "$value B"
    var mantissa = value / 1024.0
    var unit = 0
    while (unit < BINARY_UNITS.lastIndex && oneDecimal(mantissa).toDouble() >= 1024) {
        mantissa /= 1024
        unit++
    }
    return "${oneDecimal(mantissa)} ${BINARY_UNITS[unit]}"
}

/** Always bytes, never bits: the Pulse sends bytes per second. */
internal fun rate(value: Long): String = "${bytes(value)}/s"

internal fun celsius(value: Double): String = "${oneDecimal(value)} °C"

internal fun watts(value: Double): String = "${oneDecimal(value)} W"

internal fun load(value: Double): String = String.format(Locale.ROOT, "%.2f", value)

internal fun span(seconds: Double): String {
    val s = floor(seconds.coerceAtLeast(0.0)).toLong()
    return when {
        s < 60 -> "$s s"
        s < 3_600 -> "${s / 60} min"
        s < 86_400 -> "${s / 3_600} h ${s % 3_600 / 60} min"
        else -> "${s / 86_400} d ${s % 86_400 / 3_600} h"
    }
}

/** An age as one token, its unit and "ago" included, so no other word or face splits it. */
internal fun ago(elapsed: Duration): String = "${age(elapsed)} ago"

/**
 * A formatted value cut before its unit, so the unit can be drawn smaller: `70.2 %` is `70.2` and ` %`. A value with no
 * unit, or a word, is all number.
 */
internal fun splitUnit(value: String): Pair<String, String> {
    val cut = value.lastIndexOf(' ')
    if (cut <= 0 || value.substring(0, cut).toDoubleOrNull() == null) return value to ""
    return value.substring(0, cut) to value.substring(cut)
}

/** [age] in words for TalkBack, rounded the same way, so what is read is what is drawn. */
internal fun spokenAge(elapsed: Duration): String {
    val (n, word) = age(elapsed).split(" ")
    val unit = when (word) {
        "s" -> "second"
        "min" -> "minute"
        "h" -> "hour"
        else -> "day"
    }
    return if (n == "1") "1 $unit" else "$n ${unit}s"
}

internal fun statusWord(status: BatteryStatus): String = when (status) {
    BatteryStatus.Charging -> "charging"
    BatteryStatus.Discharging -> "discharging"
    BatteryStatus.NotCharging -> "not charging"
    BatteryStatus.Full -> "full"
    BatteryStatus.Unknown -> "unknown"
}

private fun oneDecimal(value: Double): String = String.format(Locale.ROOT, "%.1f", value)
