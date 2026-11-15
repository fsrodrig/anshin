package io.github.fsrodrig.anshin

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * An Agent's Window, one column per series, each named after the flat Pulse field it records. A row whose
 * [boottimeMs] is `null` is a Gap; a Sample whose value is merely unavailable has a time and a `null` value.
 */
@Serializable
data class Window(
    @SerialName("span_s") val spanS: Long? = null,
    @SerialName("boottime_ms") val boottimeMs: List<Long?> = emptyList(),
    // Doubles even where the wire sends integers, so a fractional future value never makes the Pulse unreadable.
    @SerialName("cpu_pct") val cpuPct: List<Double?>? = null,
    @SerialName("load_1") val load1: List<Double?>? = null,
    @SerialName("mem_used_pct") val memUsedPct: List<Double?>? = null,
    @SerialName("swap_used_bytes") val swapUsedBytes: List<Double?>? = null,
    @SerialName("cpu_temp_c") val cpuTempC: List<Double?>? = null,
    @SerialName("net_rx_bps") val netRxBps: List<Double?>? = null,
    @SerialName("net_tx_bps") val netTxBps: List<Double?>? = null,
    @SerialName("disk_read_bps") val diskReadBps: List<Double?>? = null,
    @SerialName("disk_write_bps") val diskWriteBps: List<Double?>? = null,
    @SerialName("battery_pct") val batteryPct: List<Double?>? = null,
    @SerialName("battery_power_w") val batteryPowerW: List<Double?>? = null,
    @SerialName("cpu_core_pct") val cpuCorePct: List<List<Double?>>? = null,
) {
    val rows: Int get() = boottimeMs.size

    private val series: List<List<Double?>?>
        get() = listOf(cpuPct, load1, memUsedPct, swapUsedBytes, cpuTempC, netRxBps, netTxBps, diskReadBps, diskWriteBps,
            batteryPct, batteryPowerW)

    internal fun isWellFormed(): Boolean {
        if (spanS == null || spanS <= 0) return false
        if (series.any { it != null && it.size != rows }) return false
        if (cpuCorePct?.any { it.size != rows } == true) return false
        val times = boottimeMs.filterNotNull()
        if (times.zipWithNext().any { (a, b) -> b <= a }) return false
        return boottimeMs.isEmpty() || boottimeMs.last() != null
    }

    internal val lastMs: Long? get() = boottimeMs.lastOrNull { it != null }

    /** Mirrors the Agent's eviction, so a Gap leaves together with the Sample after it and the Window never starts mid-Gap. */
    internal fun trimmed(): Window {
        val last = lastMs ?: return this
        val cutoff = last - spanS!! * 1000
        val firstKept = boottimeMs.indexOfFirst { it != null && it > cutoff }
        val from = if (firstKept > 0 && boottimeMs[firstKept - 1] == null) firstKept - 1 else firstKept
        return if (from <= 0) this else slice(from)
    }

    private fun slice(from: Int): Window = copy(
        boottimeMs = boottimeMs.drop(from),
        cpuPct = cpuPct?.drop(from),
        load1 = load1?.drop(from),
        memUsedPct = memUsedPct?.drop(from),
        swapUsedBytes = swapUsedBytes?.drop(from),
        cpuTempC = cpuTempC?.drop(from),
        netRxBps = netRxBps?.drop(from),
        netTxBps = netTxBps?.drop(from),
        diskReadBps = diskReadBps?.drop(from),
        diskWriteBps = diskWriteBps?.drop(from),
        batteryPct = batteryPct?.drop(from),
        batteryPowerW = batteryPowerW?.drop(from),
        cpuCorePct = cpuCorePct?.map { it.drop(from) },
    )

    internal fun appended(newer: Window): Window {
        fun join(old: List<Double?>?, new: List<Double?>?): List<Double?>? =
            if (old == null && new == null) null else (old ?: nulls(rows)) + (new ?: nulls(newer.rows))
        return Window(
            spanS = newer.spanS,
            boottimeMs = boottimeMs + newer.boottimeMs,
            cpuPct = join(cpuPct, newer.cpuPct),
            load1 = join(load1, newer.load1),
            memUsedPct = join(memUsedPct, newer.memUsedPct),
            swapUsedBytes = join(swapUsedBytes, newer.swapUsedBytes),
            cpuTempC = join(cpuTempC, newer.cpuTempC),
            netRxBps = join(netRxBps, newer.netRxBps),
            netTxBps = join(netTxBps, newer.netTxBps),
            diskReadBps = join(diskReadBps, newer.diskReadBps),
            diskWriteBps = join(diskWriteBps, newer.diskWriteBps),
            batteryPct = join(batteryPct, newer.batteryPct),
            batteryPowerW = join(batteryPowerW, newer.batteryPowerW),
            cpuCorePct = cpuCorePct?.zip(newer.cpuCorePct.orEmpty()) { old, new -> old + new },
        )
    }
}

private fun nulls(n: Int): List<Double?> = List(n) { null }

/** The phone's copy of one Machine's Window and the Cursor that asks for what's new since. */
data class HeldWindow(val window: Window, val cursor: String)

/**
 * A full Pulse replaces what the phone holds; a delta appends to it. Anything inconsistent returns null, so the next
 * request has no `after` and gets the whole Window.
 */
fun HeldWindow?.merge(pulse: Pulse): HeldWindow? {
    val window = pulse.window?.takeIf { it.isWellFormed() } ?: return null
    val cursor = pulse.cursor?.let(::parseCursor) ?: return null
    return when (pulse.kind) {
        PulseKind.Full -> HeldWindow(window.trimmed(), pulse.cursor)
        PulseKind.Delta -> {
            val held = this ?: return null
            val since = parseCursor(held.cursor) ?: return null
            if (cursor.epoch != since.epoch || cursor.seq < since.seq) return null
            if (cursor.seq - since.seq != window.rows.toULong()) return null
            if (window.cpuCorePct?.size != held.window.cpuCorePct?.size) return null
            val heldLast = held.window.lastMs
            val firstNew = window.boottimeMs.firstOrNull { it != null }
            if (heldLast != null && firstNew != null && firstNew <= heldLast) return null
            HeldWindow(held.window.appended(window).trimmed(), pulse.cursor)
        }
        PulseKind.Unknown -> null
    }
}

private data class Cursor(val epoch: String, val seq: ULong)

// The phone only compares Cursors it was given; the Agent alone decides whether one is valid.
private fun parseCursor(text: String): Cursor? {
    val epoch = text.substringBefore('.', missingDelimiterValue = "")
    val seq = text.substringAfter('.', missingDelimiterValue = "").toULongOrNull() ?: return null
    return if (epoch.isEmpty()) null else Cursor(epoch, seq)
}
