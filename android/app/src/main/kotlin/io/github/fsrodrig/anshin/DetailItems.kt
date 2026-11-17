package io.github.fsrodrig.anshin

import androidx.compose.runtime.Immutable

/** The theme token a piece of text or a chart is drawn in, chosen here so every copy decision is testable on the JVM. */
internal enum class Tone { Text, Muted, Unavailable, Cpu, Memory, Storage, Network }

@Immutable
internal data class Span(val text: String, val tone: Tone, val mono: Boolean = true)

internal typealias Line = List<Span>

internal val Line.plain: String get() = joinToString("") { it.text }

/** Between the label–value pairs of a meta line; the screen breaks the line here, so it never dangles at a wrap. */
internal val SEPARATOR = Span(" · ", Tone.Muted, mono = false)

/**
 * One row of the detail's lazy list. [key] is unique by construction (list rows are keyed by index, because sensor
 * IDs can repeat across packages) and each kind is its own content type, so compositions are reused while scrolling.
 */
@Immutable
internal sealed interface DetailItem {
    val key: String
    val contentType: Any get() = this::class

    /**
     * Heads a detail that is not Live. Its words tick (the age, the next try), so the screen draws them from the
     * state's [Connection] and the Watcher's clock rather than from here.
     */
    data class Notice(override val key: String) : DetailItem

    data class Heading(override val key: String, val title: String) : DetailItem

    data class Subheading(override val key: String, val title: String) : DetailItem

    data class VerdictRow(override val key: String, val verdict: Verdict?, val live: Boolean) : DetailItem

    data class Value(override val key: String, val label: String, val value: Line, val detail: Line? = null, val coverage: ChartSeries? = null) : DetailItem

    data class BigValue(override val key: String, val value: Span, val after: Line = emptyList()) : DetailItem

    data class Text(override val key: String, val line: Line) : DetailItem

    /** A sentence about the values above it, smaller than they are. */
    data class Footnote(override val key: String, val text: String) : DetailItem

    /**
     * [main] is a section's one large chart. Its [label] still names the series for TalkBack, but is drawn only when
     * [labelled], because under the section's display number it would repeat it.
     */
    data class Chart(
        override val key: String,
        val label: String,
        val series: ChartSeries?,
        val tone: Tone,
        val main: Boolean = false,
        val labelled: Boolean = true,
    ) : DetailItem

    /** One bar per logical CPU; a `null` entry is drawn hatched, never as an empty bar. */
    data class CpuBars(
        override val key: String,
        val values: List<Double?>,
        val tone: Tone,
        val caption: String,
        val description: String,
    ) : DetailItem

    @Immutable
    data class Usage(val fraction: Double?, val tone: Tone)

    data class ListRow(
        override val key: String,
        val name: Span,
        val tags: List<Line> = emptyList(),
        val trailing: Span? = null,
        val usage: Usage? = null,
        val lines: List<Line> = emptyList(),
        val selectableName: Boolean = false,
    ) : DetailItem
}

internal val DETAIL_SECTIONS = listOf("cpu", "memory", "storage", "battery", "temperatures", "network", "machine")

internal fun detailItems(state: MachineDetailState): List<DetailItem> = DetailBuilder(state).build()

private class DetailBuilder(private val state: MachineDetailState) {
    private val pulse = state.last.pulse
    private val held = state.window
    private val window = held?.window
    private val items = mutableListOf<DetailItem>()

    // Not Live: every value and chart line is slate, as on the board; hatching stays reserved for Gaps.
    private fun tone(section: Tone) = if (state.live) section else Tone.Unavailable

    private fun value(text: String?, section: Tone): Line =
        listOf(text?.let { Span(it, tone(section)) } ?: Span(UNAVAILABLE, Tone.Unavailable))

    private fun word(text: String) = Span(text, Tone.Muted, mono = false)

    private fun pairs(vararg pairs: Pair<String, String?>): Line = pairs.flatMapIndexed { i, (label, text) ->
        listOfNotNull(if (i > 0) SEPARATOR else null, word("$label ")) + value(text, Tone.Text)
    }

    private fun known(series: List<Double?>?): Boolean {
        val times = window?.boottimeMs ?: return false
        return series.orEmpty().withIndex().any { (i, v) -> v != null && times.getOrNull(i) != null }
    }

    // A Gap can make current values null while the Window still has useful measurements.
    private fun unavailable(flat: List<Any?>, series: List<List<Double?>?>) =
        flat.all { it == null } && series.none(::known)

    private fun chart(
        key: String,
        label: String,
        series: List<Double?>?,
        unit: ChartUnit,
        section: Tone,
        fixedTop: Double? = null,
        main: Boolean = false,
        labelled: Boolean = true,
    ) {
        items += DetailItem.Chart(key, label, chartSeries(label, held, series, unit, fixedTop), tone(section), main, labelled)
    }

    // Only a section's display number, charts and bars carry its colour; every other value is text.
    private fun row(key: String, label: String, text: String?) {
        items += DetailItem.Value(key, label, value(text, Tone.Text))
    }

    private fun section(key: String, title: String, isUnavailable: Boolean, content: () -> Unit) {
        items += DetailItem.Heading(key, title)
        if (isUnavailable) items += DetailItem.Text("$key-unavailable", value(null, Tone.Text)) else content()
    }

    fun build(): List<DetailItem> {
        notice()
        verdict()
        cpu()
        memory()
        storage()
        battery()
        temperatures()
        network()
        machine()
        return items
    }

    private fun notice() {
        if (!state.live) items += DetailItem.Notice("notice")
    }

    private fun verdict() {
        items += DetailItem.VerdictRow("verdict", pulse.verdict, state.live)
    }

    /** The facts about the Machine itself, which change rarely, so they come after every measurement. */
    private fun machine() {
        items += DetailItem.Heading("machine", "Machine")
        items += DetailItem.Value("hostname", "Hostname", value(pulse.hostname, Tone.Text))
        items += DetailItem.Value("uptime", "Uptime", value(pulse.uptimeS?.let(::span), Tone.Text))
        items += DetailItem.Value("window", "Window", value(windowCoverage(), Tone.Text),
            coverage = window?.takeIf { it.isWellFormed() }?.let {
                chartSeries("Window", held, it.boottimeMs.map { t -> t?.let { 1.0 } }, ChartUnit.Load, 1.0)
            })
        items += DetailItem.Value("distro", "Distro", value(pulse.distro, Tone.Text))
        items += DetailItem.Value("kernel", "Kernel", value(pulse.kernel, Tone.Text))
        items += DetailItem.Value("arch", "Architecture", value(pulse.arch, Tone.Text))
        items += DetailItem.Value("agent-version", "Agent version", value(pulse.agentVersion, Tone.Text))
    }

    private fun windowCoverage(): String? {
        val w = window?.takeIf { it.isWellFormed() && it.lastMs != null } ?: return null
        val spanMs = w.spanS!! * 1000
        val spans = gaps(w.boottimeMs, spanMs)
        val gapText = when (spans.size) {
            0 -> ""
            1 -> ", 1 Gap of ${duration(spans[0].toMs - spans[0].fromMs)}"
            else -> ", ${spans.size} Gaps of ${spans.joinToString(", ") { duration(it.toMs - it.fromMs) }}"
        }
        return caption(w.boottimeMs, spanMs) + gapText
    }

    private fun cpu() {
        val flat = listOf(pulse.cpuPct, pulse.cpuCorePct, pulse.load1, pulse.load5, pulse.load15, pulse.cpuTempC)
        section("cpu", "CPU", unavailable(flat, listOf(window?.cpuPct, window?.load1))) {
            items += DetailItem.BigValue("cpu-now", value(pulse.cpuPct?.let(::percent), Tone.Cpu).single())
            chart("cpu-chart", "CPU", window?.cpuPct, ChartUnit.Percent, Tone.Cpu, fixedTop = 100.0, main = true, labelled = false)
            items += pulse.cpuCorePct?.let(::cpuBars) ?: DetailItem.Value("cpu-bars", "Per CPU", value(null, Tone.Text))
            val loads = listOf("1" to pulse.load1, "5" to pulse.load5, "15" to pulse.load15)
            // No load at all is one row rather than three that say the same.
            if (loads.all { it.second == null }) {
                row("load", "Load", null)
            } else {
                loads.forEach { (minutes, v) -> row("load-$minutes", "Load $minutes min", v?.let(::load)) }
            }
            chart("load-chart", "Load 1 min", window?.load1, ChartUnit.Load, Tone.Cpu)
            items += DetailItem.Value(
                "cpu-temperature",
                "CPU temperature",
                value(pulse.cpuTempC?.let(::celsius), Tone.Text),
                pulse.cpuSensor?.let { listOf(word("from "), Span(it, Tone.Muted)) },
            )
        }
    }

    private fun cpuBars(values: List<Double?>): DetailItem.CpuBars {
        val count = if (values.size == 1) "1 CPU" else "${values.size} CPUs"
        val each = values.withIndex().joinToString(", ") { (i, v) -> "CPU $i ${v?.let(::wholePercent) ?: "unavailable"}" }
        val missing = values.indices.filter { values[it] == null }
        val unavailable = when {
            missing.isEmpty() -> ""
            missing.size <= 3 -> " · " + missing.joinToString(", ") { "CPU $it" } + " unavailable"
            else -> " · ${missing.size} CPUs unavailable"
        }
        return DetailItem.CpuBars("cpu-bars", values, tone(Tone.Cpu), count + unavailable, "Per CPU, $count: $each")
    }

    private fun memory() {
        val flat = listOf(pulse.memUsedPct, pulse.memUsedBytes, pulse.memAvailableBytes, pulse.memTotalBytes,
            pulse.swapUsedBytes, pulse.swapTotalBytes, pulse.swapEntries)
        section("memory", "Memory", unavailable(flat, listOf(window?.memUsedPct, window?.swapUsedBytes))) {
            items += DetailItem.BigValue(
                "memory-now",
                value(pulse.memUsedPct?.let(::percent), Tone.Memory).single(),
                if (pulse.memUsedPct == null) emptyList() else listOf(word(" used")),
            )
            chart("memory-chart", "Memory", window?.memUsedPct, ChartUnit.Percent, Tone.Memory, fixedTop = 100.0, main = true, labelled = false)
            row("memory-used", "Used", pulse.memUsedBytes?.let(::bytes))
            row("memory-available", "Available", pulse.memAvailableBytes?.let(::bytes))
            row("memory-total", "Total", pulse.memTotalBytes?.let(::bytes))
            row("swap-used", "Swap used", pulse.swapUsedBytes?.let(::bytes))
            row("swap-total", "Swap total", pulse.swapTotalBytes?.let(::bytes))
            chart("swap-chart", "Swap used", window?.swapUsedBytes, ChartUnit.Bytes, Tone.Memory,
                fixedTop = pulse.swapTotalBytes?.toDouble())
            if (pulse.swapEntries == null) {
                row("swap-devices", "Swap devices", null)
            }
            pulse.swapEntries.orEmpty().forEachIndexed { i, entry ->
                val tag = when (entry.zram) {
                    true -> listOf(Span("zram", Tone.Muted))
                    false -> listOf(Span("swap", Tone.Muted))
                    null -> listOf(word("type "), Span(UNAVAILABLE, Tone.Unavailable))
                }
                items += DetailItem.ListRow(
                    "swap:$i",
                    name(entry.name),
                    tags = listOf(tag),
                    lines = listOf(pairs("used" to entry.usedBytes?.let(::bytes), "total" to entry.totalBytes?.let(::bytes))),
                )
            }
        }
    }

    private fun name(text: String?) = text?.let { Span(it, Tone.Text) } ?: Span(UNAVAILABLE, Tone.Unavailable)

    private fun storage() {
        val flat = listOf(pulse.filesystems, pulse.disks, pulse.diskReadBps, pulse.diskWriteBps)
        section("storage", "Storage", unavailable(flat, listOf(window?.diskReadBps, window?.diskWriteBps))) {
            items += DetailItem.Subheading("filesystems", "Filesystems")
            val filesystems = pulse.filesystems
            if (filesystems == null) {
                items += DetailItem.Text("filesystems-unavailable", value(null, Tone.Text))
            } else {
                filesystems.forEachIndexed { i, fs -> items += filesystem(i, fs) }
            }
            items += DetailItem.Subheading("disk-io", "Disk I/O")
            if (unavailable(listOf(pulse.disks, pulse.diskReadBps, pulse.diskWriteBps), listOf(window?.diskReadBps, window?.diskWriteBps))) {
                items += DetailItem.Text("disk-io-unavailable", value(null, Tone.Text))
                return@section
            }
            row("disk-read", "Read", pulse.diskReadBps?.let(::rate))
            row("disk-write", "Write", pulse.diskWriteBps?.let(::rate))
            chart("disk-read-chart", "Disk read", window?.diskReadBps, ChartUnit.BytesPerSecond, Tone.Storage)
            chart("disk-write-chart", "Disk write", window?.diskWriteBps, ChartUnit.BytesPerSecond, Tone.Storage)
            val disks = pulse.disks
            if (disks == null) {
                items += DetailItem.Text("disks-unavailable", value(null, Tone.Text))
            } else {
                disks.forEachIndexed { i, disk ->
                    items += DetailItem.ListRow(
                        "disk:$i",
                        name(disk.name),
                        lines = listOf(pairs("read" to disk.readBps?.let(::rate), "write" to disk.writeBps?.let(::rate))),
                    )
                }
            }
        }
    }

    private fun filesystem(i: Int, fs: Filesystem): DetailItem.ListRow {
        val capacity = listOf(fs.usedPct, fs.usedBytes, fs.availableBytes, fs.totalBytes)
        val tags = listOfNotNull(
            fs.fsType?.let { listOf(Span(it, Tone.Muted)) },
            if (pulse.primaryDisk != null && fs.majMin == pulse.primaryDisk) listOf(Span("primary", Tone.Muted)) else null,
        )
        val lines = listOfNotNull(
            if (capacity.all { it == null }) null else pairs(
                "used" to fs.usedBytes?.let(::bytes), "available" to fs.availableBytes?.let(::bytes), "total" to fs.totalBytes?.let(::bytes)),
            fs.ageS?.takeIf { it > 0 }?.let { listOf(Span("Capacity age ${it.toLong()} s", Tone.Muted)) },
        )
        return DetailItem.ListRow(
            "fs:$i",
            name(fs.mountPoint),
            tags = tags,
            trailing = value(fs.usedPct?.let(::percent), Tone.Text).single(),
            usage = DetailItem.Usage(fs.usedPct?.let { it / 100 }, tone(Tone.Storage)),
            lines = lines,
        )
    }

    private fun battery() {
        val report = pulse.battery
        val present = (report as? BatteryReport.Present)?.battery
        val isUnavailable = when (report) {
            BatteryReport.NoBattery -> false
            BatteryReport.NotReported -> true
            is BatteryReport.Present -> unavailable(
                listOf(report.battery.pct, report.battery.status, report.battery.powerW, report.battery.timeLeftS, report.battery.batteries),
                listOf(window?.batteryPct, window?.batteryPowerW),
            )
        }
        section("battery", "Battery", isUnavailable) {
            // A Machine without a system battery is a fact, not missing data.
            if (present == null) {
                items += DetailItem.Text("no-battery", value("no battery", Tone.Text))
                return@section
            }
            val status = present.status
            items += DetailItem.BigValue(
                "battery-now",
                value(present.pct?.let(::percent), Tone.Text).single(),
                listOf(word(" ")) + (status?.let { value(statusWord(it), Tone.Text) } ?: pairs("status" to null)),
            )
            chart("battery-charge-chart", "Battery charge", window?.batteryPct, ChartUnit.Percent, Tone.Text, fixedTop = 100.0,
                main = true, labelled = false)
            row("battery-power", "Power", present.powerW?.let(::watts))
            // The Agent sends time left only while discharging; an unknown status leaves the reason unknown too.
            val timeLeft = when (status) {
                BatteryStatus.Discharging -> present.timeLeftS?.let(::span)
                null, BatteryStatus.Unknown -> null
                else -> "not discharging"
            }
            row("battery-time-left", "Time left", timeLeft)
            chart("battery-power-chart", "Battery power", window?.batteryPowerW, ChartUnit.Watts, Tone.Text)
            present.batteries.orEmpty().forEachIndexed { i, row ->
                val parts = listOf(
                    pairs("charge" to row.pct?.let(::percent)),
                    row.status?.let { value(statusWord(it), Tone.Text) } ?: pairs("status" to null),
                    pairs("power" to row.powerW?.let(::watts)),
                    pairs("health" to row.healthPct?.let(::percent)),
                )
                items += DetailItem.ListRow(
                    "battery:$i",
                    name(row.name),
                    lines = listOf(parts.flatMapIndexed { j, part -> if (j > 0) listOf(SEPARATOR) + part else part }),
                )
            }
        }
    }

    private fun temperatures() {
        section("temperatures", "Temperatures", unavailable(listOf(pulse.temperatures), listOf(window?.cpuTempC))) {
            // The section has no display number above it, so this main chart keeps its label.
            chart("cpu-temperature-chart", "CPU temperature", window?.cpuTempC, ChartUnit.Celsius, Tone.Cpu, main = true)
            val sensors = pulse.temperatures
            if (sensors == null) {
                items += DetailItem.Text("sensors-unavailable", value(null, Tone.Text))
                return@section
            }
            val cpuRow = sensors.indexOfFirst { pulse.cpuSensor != null && it.id == pulse.cpuSensor }
            sensors.forEachIndexed { i, sensor ->
                // A slow-lane value is the last one read, so its age stays beside it rather than passing as current.
                val reading = sensor.tempC?.let { t -> celsius(t) + (sensor.ageS?.takeIf { it > 0 }?.let { " ($it s ago)" } ?: "") }
                items += DetailItem.ListRow(
                    "sensor:$i",
                    name(sensor.id),
                    tags = if (i == cpuRow) listOf(listOf(Span("CPU sensor", Tone.Muted))) else emptyList(),
                    trailing = value(reading, Tone.Text).single(),
                    selectableName = sensor.id != null,
                )
            }
        }
    }

    private fun network() {
        val flat = listOf(pulse.netRxBps, pulse.netTxBps, pulse.netInterfaces)
        section("network", "Network", unavailable(flat, listOf(window?.netRxBps, window?.netTxBps))) {
            row("received", "Received", pulse.netRxBps?.let(::rate))
            row("sent", "Sent", pulse.netTxBps?.let(::rate))
            chart("received-chart", "Received", window?.netRxBps, ChartUnit.BytesPerSecond, Tone.Network)
            chart("sent-chart", "Sent", window?.netTxBps, ChartUnit.BytesPerSecond, Tone.Network)
            items += DetailItem.Footnote("network-note", "Received and sent count only interfaces with a device")
            val interfaces = pulse.netInterfaces
            if (interfaces == null) {
                items += DetailItem.Text("interfaces-unavailable", value(null, Tone.Text))
                return@section
            }
            interfaces.forEachIndexed { i, net ->
                val tag = when (net.deviceBacked) {
                    true -> listOf(Span("device", Tone.Muted))
                    false -> listOf(Span("no device", Tone.Muted))
                    null -> listOf(Span("device ", Tone.Muted), Span(UNAVAILABLE, Tone.Unavailable))
                }
                items += DetailItem.ListRow(
                    "if:$i",
                    name(net.name),
                    tags = listOf(tag),
                    lines = listOf(pairs("received" to net.rxBps?.let(::rate), "sent" to net.txBps?.let(::rate))),
                )
            }
        }
    }
}
