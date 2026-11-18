package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DetailItemsTest {
    private fun items(pulse: Pulse, connection: Connection = Connection.Live, window: HeldWindow? = null) =
        detailItems(DetailSamples.state(connection, pulse = pulse, window = window))

    private fun List<DetailItem>.section(key: String): List<DetailItem> {
        val from = indexOfFirst { it.key == key }
        val to = drop(from + 1).indexOfFirst { it is DetailItem.Heading }.let { if (it < 0) size else from + 1 + it }
        return subList(from, to)
    }

    private val DetailItem.texts: List<String>
        get() = when (this) {
            is DetailItem.Notice -> emptyList()
            is DetailItem.Heading -> listOf(title)
            is DetailItem.Subheading -> listOf(title)
            is DetailItem.VerdictRow -> listOf(verdict?.level?.word ?: "Verdict unavailable")
            is DetailItem.Value -> listOfNotNull(label, value.plain, detail?.plain)
            is DetailItem.BigValue -> listOf(value.text + after.plain)
            is DetailItem.Text -> listOf(line.plain)
            is DetailItem.Footnote -> listOf(text)
            is DetailItem.Chart -> listOfNotNull(label, series?.summary)
            is DetailItem.CpuBars -> listOf(caption, description)
            is DetailItem.ListRow -> listOfNotNull(name.text, trailing?.text) + tags.map { it.plain } + lines.map { it.plain }
        }

    private fun List<DetailItem>.texts() = flatMap { it.texts }

    private val measurementSections = DETAIL_SECTIONS - "machine"

    @Test
    fun everySectionWithNothingSaysUnavailableOnce() {
        val all = items(Pulse(schema = 1))
        assertEquals(DETAIL_SECTIONS, all.filterIsInstance<DetailItem.Heading>().map { it.key })
        for (key in measurementSections) {
            assertEquals(key, listOf(UNAVAILABLE), all.section(key).drop(1).texts())
        }
        assertEquals("Verdict unavailable", all.first().texts.single())
        val machine = all.section("machine")
        assertEquals(7, machine.filterIsInstance<DetailItem.Value>().count { it.value.plain == UNAVAILABLE })
        assertTrue(all.none { it is DetailItem.Notice })
    }

    @Test
    fun aMeasuredZeroIsNeverUnavailable() {
        val all = items(Pulse(schema = 1, cpuPct = 0.0, netRxBps = 0, netTxBps = 0, diskReadBps = 0, diskWriteBps = 0,
            filesystems = listOf(Filesystem("0:35", "/", "btrfs", 4096, 0, 4096, 0.0, 0.0))))
        assertTrue("0.0 %" in all.section("cpu").texts())
        val network = all.section("network").texts()
        assertTrue(network.containsAll(listOf("0 B/s")))
        assertEquals(2, network.count { it == "0 B/s" })
        val fs = all.filterIsInstance<DetailItem.ListRow>().single { it.key == "fs:0" }
        assertEquals("0.0 %", fs.trailing!!.text)
        assertEquals(0.0, fs.usage!!.fraction!!, 0.0)
        assertTrue("used 0 B · available 4.0 KiB · total 4.0 KiB" in fs.lines.map { it.plain })
        val storage = all.section("storage")
        assertEquals(listOf("Read", "0 B/s", "Write", "0 B/s"), storage.filter { it.key == "disk-read" || it.key == "disk-write" }.texts())
    }

    @Test
    fun aSectionWithOnlyWindowHistoryIsNotUnavailable() {
        val all = items(Pulse(schema = 1), window = WindowSamples.withGap())
        val cpu = all.section("cpu")
        assertTrue(cpu.any { it is DetailItem.Chart && it.series!!.known })
        assertEquals(UNAVAILABLE, (cpu[1] as DetailItem.BigValue).value.text)
        assertEquals(listOf(UNAVAILABLE), all.section("network").drop(1).texts())
        assertEquals("last 30 min, 1 Gap of 5 min", all.filterIsInstance<DetailItem.Value>().single { it.key == "window" }.value.plain)
    }

    @Test fun theVerdictComesFirstAndTheMachinesFactsLast() {
        val live = items(Pulse(schema = 1))
        assertTrue(live.first() is DetailItem.VerdictRow)
        val stale = items(Pulse(schema = 1), Connection.Stale)
        assertEquals(listOf("notice", "verdict"), stale.take(2).map { it.key })
        assertEquals(listOf("machine", "hostname", "uptime", "window", "distro", "kernel", "arch", "agent-version"),
            live.section("machine").map { it.key })
        assertEquals(live.size, live.indexOfFirst { it.key == "machine" } + 8)
    }

    @Test fun onlyADisplayNumberAChartOrABarCarriesItsSectionColour() {
        val all = items(DetailSamples.pulse, window = DetailSamples.window)
        val sectionColours = setOf(Tone.Cpu, Tone.Memory, Tone.Storage, Tone.Network)
        val valueTones = all.flatMap { item ->
            when (item) {
                is DetailItem.Value -> item.value.map { it.tone }
                is DetailItem.ListRow -> listOfNotNull(item.trailing?.tone) + item.lines.flatten().map { it.tone }
                else -> emptyList()
            }
        }
        assertTrue(valueTones.none { it in sectionColours })
        assertEquals(Tone.Cpu, all.filterIsInstance<DetailItem.BigValue>().single { it.key == "cpu-now" }.value.tone)
        assertEquals(Tone.Memory, all.filterIsInstance<DetailItem.BigValue>().single { it.key == "memory-now" }.value.tone)
        assertEquals(Tone.Storage, all.filterIsInstance<DetailItem.ListRow>().single { it.key == "fs:0" }.usage!!.tone)
        assertEquals(Tone.Cpu, all.filterIsInstance<DetailItem.CpuBars>().single().tone)
    }

    @Test fun aDisplayNumberIsFollowedByItsMainChartWithoutRepeatingItsLabel() {
        val all = items(DetailSamples.pulse, window = DetailSamples.window)
        val keys = all.map { it.key }
        val charts = all.filterIsInstance<DetailItem.Chart>()
        assertEquals(listOf("cpu-chart", "memory-chart", "battery-charge-chart", "cpu-temperature-chart"), charts.filter { it.main }.map { it.key })
        for ((number, chart) in listOf("cpu-now" to "cpu-chart", "memory-now" to "memory-chart", "battery-now" to "battery-charge-chart")) {
            assertEquals(keys.indexOf(number) + 1, keys.indexOf(chart))
            assertFalse(charts.single { it.key == chart }.labelled)
        }
        assertTrue("no display number names it", charts.single { it.key == "cpu-temperature-chart" }.labelled)
        // A secondary chart sits under the row that carries its value, so a label of its own would read as a row without one.
        val underValue = listOf("load-1" to "load-chart", "swap-used" to "swap-chart", "disk-read" to "disk-read-chart",
            "disk-write" to "disk-write-chart", "battery-power" to "battery-power-chart", "received" to "received-chart", "sent" to "sent-chart")
        for ((row, chart) in underValue) {
            assertEquals(chart, keys.indexOf(row) + 1, keys.indexOf(chart))
            assertFalse(chart, charts.single { it.key == chart }.labelled)
        }
        assertEquals(underValue.map { it.second }.toSet(), charts.filter { !it.main }.map { it.key }.toSet())
        assertEquals(keys.indexOf("sent-chart") + 1, keys.indexOf("network-note"))
        assertEquals(listOf("Load 1 min", "Load 5 min", "Load 15 min"),
            all.filterIsInstance<DetailItem.Value>().filter { it.key.startsWith("load-") }.map { it.label })
        val noLoad = items(Pulse(schema = 1, cpuPct = 3.0)).map { it.key }
        assertEquals(listOf("load", "load-chart"), noLoad.filter { it.startsWith("load") })
    }

    @Test fun windowCoverageUsesTheWindowTimeAxisAndGaps() {
        fun coverage(held: HeldWindow?) = items(Pulse(schema = 1), window = held)
            .filterIsInstance<DetailItem.Value>().single { it.key == "window" }
        assertNull(coverage(null).coverage)
        val gap = coverage(WindowSamples.withGap())
        assertEquals(gap.value.plain, gap.coverage!!.caption + ", 1 Gap of 5 min")
        assertEquals(1, gap.coverage.gaps.size)
        val short = coverage(HeldWindow(Window(spanS = 1800, boottimeMs = listOf(1_798_000L, 1_800_000L)), ""))
        assertEquals(1798f / 1800, short.coverage!!.runs.single().first(), 0.0001f)
        assertEquals(1f, short.coverage.runs.single()[2], 0f)
        assertEquals("last 2 s", short.value.plain)
        assertNull(coverage(HeldWindow(Window(spanS = 1800, boottimeMs = listOf(2L, 1L)), "")).coverage)
        assertEquals(5f / 30, gap.coverage.gaps.single().to - gap.coverage.gaps.single().from, 0.001f)

    }

    @Test fun everyCpuFitsInBoundedRows() {
        for (count in listOf(16, 192, 256)) {
            val cells = cpuBarCells(count, 312f, 2f)
            assertEquals(count, cells.size)
            cells.forEach { (left, width) ->
                assertTrue(left >= 0f && width > 0f && left + width <= 312.001f)
            }
            assertEquals((count + 15) / 16, List(count) { it }.chunked(CPUS_PER_ROW).size)
            assertEquals(17.625f, cells.first().second, 0.001f)
        }
    }

    @Test
    fun noBatteryIsAFactAndNotReportedIsUnavailable() {
        assertEquals(listOf("no battery"), items(Pulse(schema = 1, battery = BatteryReport.NoBattery)).section("battery").drop(1).texts())
        assertEquals(listOf(UNAVAILABLE), items(Pulse(schema = 1)).section("battery").drop(1).texts())
        assertEquals(listOf(UNAVAILABLE),
            items(Pulse(schema = 1, battery = BatteryReport.Present(Battery()))).section("battery").drop(1).texts())
        val present = items(DetailSamples.pulse, window = DetailSamples.window).section("battery").texts()
        assertTrue("64.0 % discharging" in present)
        assertTrue("7.5 W" in present)
        assertTrue("2 h 30 min" in present)
        assertTrue("charge 64.0 % discharging · power 7.5 W · health 87.5 %" in present)
        val unknownStatus = BatteryReport.Present(Battery(64.0, null, 7.5, null, listOf(BatteryRow("BAT0", 64.0, null, 7.5, 87.5))))
        assertTrue("charge 64.0 % · status $UNAVAILABLE · power 7.5 W · health 87.5 %" in
            items(Pulse(schema = 1, battery = unknownStatus)).section("battery").texts())
    }

    @Test
    fun timeLeftFollowsTheStatus() {
        fun timeLeft(status: BatteryStatus?) = items(Pulse(schema = 1, battery = BatteryReport.Present(Battery(64.0, status, 7.5, 9000.0))))
            .filterIsInstance<DetailItem.Value>().single { it.key == "battery-time-left" }.value.plain
        assertEquals("2 h 30 min", timeLeft(BatteryStatus.Discharging))
        for (status in listOf(BatteryStatus.Charging, BatteryStatus.NotCharging, BatteryStatus.Full)) {
            assertEquals("not discharging", timeLeft(status))
        }
        assertEquals(UNAVAILABLE, timeLeft(BatteryStatus.Unknown))
        assertEquals(UNAVAILABLE, timeLeft(null))
        val noStatus = items(Pulse(schema = 1, battery = BatteryReport.Present(Battery(64.0, null))))
        assertEquals("64.0 % status — unavailable", noStatus.filterIsInstance<DetailItem.BigValue>().single().texts.single())
    }

    @Test
    fun theCpuSensorIsTaggedOnceEvenWhenIdsRepeat() {
        val repeated = listOf(Sensor("coretemp/Core 0", 50.0), Sensor("coretemp/Core 0", 52.0), Sensor("acpitz/temp1", 40.0))
        val rows = items(Pulse(schema = 1, cpuSensor = "coretemp/Core 0", temperatures = repeated))
            .filterIsInstance<DetailItem.ListRow>().filter { it.key.startsWith("sensor:") }
        assertEquals(listOf(listOf("CPU sensor"), emptyList(), emptyList()), rows.map { row -> row.tags.map { it.plain } })
        assertTrue(rows.all { it.selectableName })
    }

    @Test
    fun keysAreUniqueEvenWhenSensorIdsRepeat() {
        val repeated = DetailSamples.pulse.copy(temperatures = List(3) { Sensor("coretemp/Core 0", 50.0) })
        for (connection in listOf(Connection.Live, Connection.Stale, DetailSamples.unreachable())) {
            for (pulse in listOf(repeated, Pulse(schema = 1))) {
                val keys = items(pulse, connection, DetailSamples.window).map { it.key }
                assertEquals(keys.size, keys.toSet().size)
            }
        }
    }

    @Test
    fun aBlockedFilesystemShowsItsCapacityAge() {
        val blocked = items(DetailSamples.pulse).filterIsInstance<DetailItem.ListRow>().single { it.key == "fs:2" }
        assertEquals("/mnt/archive", blocked.name.text)
        assertEquals(UNAVAILABLE, blocked.trailing!!.text)
        assertNull(blocked.usage!!.fraction)
        assertEquals(listOf("Capacity age 90\u00A0s"), blocked.lines.map { it.plain })
        val primary = items(DetailSamples.pulse).filterIsInstance<DetailItem.ListRow>().single { it.key == "fs:0" }
        assertEquals(listOf("btrfs", "primary"), primary.tags.map { it.plain })
        assertEquals("8.8 %", primary.trailing!!.text)
        assertEquals(listOf("used 83.0 GiB · available 863.1 GiB · total 951.9 GiB", "Capacity age 4\u00A0s"), primary.lines.map { it.plain })
    }

    @Test
    fun uiWordsAreSansAndOnlyValuesAndIdentifiersAreMono() {
        val all = items(DetailSamples.pulse)
        val rows = all.filterIsInstance<DetailItem.ListRow>()
        val primary = rows.single { it.key == "fs:0" }
        assertEquals(listOf(true, false), primary.tags.map { it.single().mono })
        assertEquals(listOf(false, true, false, true), rows.single { it.key == "fs:2" }.lines.single().map { it.mono })
        val sensor = rows.single { it.tags.isNotEmpty() && it.key.startsWith("sensor:") }.tags.single().single()
        assertEquals(Span("CPU sensor", Tone.Muted, mono = false), sensor)
        val interfaces = items(Pulse(schema = 1, netInterfaces = listOf(NetInterface("eth0", true, 1, 1), NetInterface("wg0", null, null, 5))))
            .filterIsInstance<DetailItem.ListRow>().map { it.tags.single() }
        assertEquals(listOf(listOf(false), listOf(false, true)), interfaces.map { tag -> tag.map { it.mono } })
    }

    @Test
    fun aFilesystemWithNoSizesSaysCapacityUnavailableOnce() {
        fun lines(fs: Filesystem) = items(Pulse(schema = 1, filesystems = listOf(fs)))
            .filterIsInstance<DetailItem.ListRow>().single().lines.map { it.plain }
        assertEquals(listOf("capacity $UNAVAILABLE"), lines(Filesystem("8:1", "/", "ext4", usedPct = 91.0)))
        assertEquals(emptyList<String>(), lines(Filesystem("8:1", "/", "ext4")))
        assertEquals(listOf("used 512 B · available $UNAVAILABLE · total $UNAVAILABLE"), lines(Filesystem("8:1", "/", "ext4", usedBytes = 512)))
    }

    @Test
    fun swapEntriesAreLabelledZramOrSwap() {
        val pulse = Pulse(schema = 1, swapEntries = listOf(
            SwapEntry("/dev/zram0", true, 8L shl 30, 1L shl 30), SwapEntry("/swapfile", false, 4L shl 30, 0), SwapEntry("/dev/sdb2", null, null, null),
        ))
        val rows = items(pulse).filterIsInstance<DetailItem.ListRow>().filter { it.key.startsWith("swap:") }
        assertEquals(listOf(listOf("zram"), listOf("swap"), listOf("type — unavailable")), rows.map { row -> row.tags.map { it.plain } })
        assertEquals(
            listOf("used 1.0 GiB · total 8.0 GiB", "used 0 B · total 4.0 GiB", "used — unavailable · total — unavailable"),
            rows.map { it.lines.single().plain },
        )
    }

    @Test fun missingSwapDevicesAndCpuValuesSayUnavailable() {
        val all = items(Pulse(schema = 1, memUsedPct = 20.0, cpuCorePct = List(16) { if (it == 3 || it == 9) null else 0.0 }))
        assertEquals(UNAVAILABLE, all.filterIsInstance<DetailItem.Value>().single { it.label == "Swap devices" }.value.plain)
        assertEquals("16 CPUs · CPU 3, CPU 9 unavailable", all.filterIsInstance<DetailItem.CpuBars>().single().caption)
        val many = items(Pulse(schema = 1, cpuCorePct = List(256) { null }))
        assertEquals("256 CPUs · 256 CPUs unavailable", many.filterIsInstance<DetailItem.CpuBars>().single().caption)
        val unknownType = items(Pulse(schema = 1, swapEntries = listOf(SwapEntry(null, null, null, null))))
            .filterIsInstance<DetailItem.ListRow>().single().tags.single()
        assertEquals(Tone.Unavailable, unknownType.last().tone)
    }

    @Test
    fun interfacesSayDeviceNoDeviceOrUnavailable() {
        val pulse = Pulse(schema = 1, netRxBps = 75_304, netInterfaces = listOf(
            NetInterface("eth0", true, 75_262, 23_269), NetInterface("lo", false, 0, 0), NetInterface("wg0", null, null, 5),
        ))
        val all = items(pulse)
        val rows = all.filterIsInstance<DetailItem.ListRow>().filter { it.key.startsWith("if:") }
        assertEquals(listOf("device", "no device", "device — unavailable"), rows.map { it.tags.single().plain })
        assertEquals(
            listOf("received 73.5 KiB/s · sent 22.7 KiB/s", "received 0 B/s · sent 0 B/s", "received — unavailable · sent 5 B/s"),
            rows.map { it.lines.single().plain },
        )
        val network = all.section("network").texts()
        assertTrue("73.5 KiB/s" in network)
        assertTrue("Received and sent count only interfaces with a device" in network)
        assertEquals(listOf(UNAVAILABLE),
            items(Pulse(schema = 1, netRxBps = 5)).filter { it.key == "interfaces-unavailable" }.flatMap { it.texts })
    }

    @Test
    fun notLiveDimsValuesAndQuotesTheVerdictOnlyAsHistory() {
        val live = items(DetailSamples.pulse, Connection.Live, DetailSamples.window)
        assertTrue(live.none { it is DetailItem.Notice })
        val liveVerdict = live.filterIsInstance<DetailItem.VerdictRow>().single()
        assertEquals(VerdictLevel.Ok, liveVerdict.verdict?.level)
        assertTrue(liveVerdict.live)
        assertEquals(Tone.Cpu, live.filterIsInstance<DetailItem.Chart>().first().tone)

        val notLive = listOf(
            Connection.Stale,
            DetailSamples.unreachable(),
            DetailSamples.unreachable(outsideTailnet = true),
            DetailSamples.unreachable(httpStatus = 503, vpnHint = true),
            DetailSamples.unreachable(httpStatus = 500),
        )
        for (connection in notLive) {
            val all = items(DetailSamples.pulse, connection, DetailSamples.window)
            assertEquals(connection.toString(), DetailItem.Notice("notice"), all.first())
            val verdict = all.filterIsInstance<DetailItem.VerdictRow>().single()
            assertEquals("the last Verdict is kept, to be quoted with its age", VerdictLevel.Ok, verdict.verdict?.level)
            assertFalse(verdict.live)
            val tones = all.flatMap { item ->
                when (item) {
                    is DetailItem.Value -> item.value.map { it.tone }
                    is DetailItem.BigValue -> listOf(item.value.tone)
                    is DetailItem.Chart -> listOf(item.tone)
                    is DetailItem.CpuBars -> listOf(item.tone)
                    is DetailItem.ListRow -> listOfNotNull(item.trailing?.tone, item.usage?.tone)
                    else -> emptyList()
                }
            }
            assertEquals(setOf(Tone.Unavailable), tones.toSet())
            assertEquals("70.2 %", (all.section("cpu")[1] as DetailItem.BigValue).value.text)
        }
    }

    @Test
    fun theFullSampleReadsAsTheIssueAsks() {
        val all = items(DetailSamples.pulse, window = DetailSamples.window)
        val texts = all.texts()
        for (expected in listOf(
            "20 h 41 min", "last 30 min", "(devel)", "70.2 %", "16 CPUs · CPU 9 unavailable", "1.25", "0.98", "0.75", "61.5 °C", "from k10temp/Tctl",
            "56.3 % used", "18.0 GiB", "/dev/zram0", "nvme0n1", "84.0 KiB/s", "208.0 KiB/s", "47.0 °C (12 s ago)",
            "eth0",
        )) {
            assertTrue("no $expected", expected in texts)
        }
        val bars = all.filterIsInstance<DetailItem.CpuBars>().single()
        assertTrue(bars.description.startsWith("Per CPU, 16 CPUs: CPU 0 65 %, CPU 1 81 %"))
        assertTrue("CPU 9 unavailable" in bars.description)
        assertTrue(all.filterIsInstance<DetailItem.Chart>().all { it.series!!.known })
        assertEquals("1 CPU", items(Pulse(schema = 1, cpuCorePct = listOf(3.0))).filterIsInstance<DetailItem.CpuBars>().single().caption)
        assertEquals("Per CPU", items(Pulse(schema = 1, cpuPct = 3.0)).filterIsInstance<DetailItem.Value>().single { it.key == "cpu-bars" }.label)
    }
}
