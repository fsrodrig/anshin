package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DetailItemsTest {
    private fun items(pulse: Pulse, connection: Connection = Connection.Live, window: HeldWindow? = null) =
        detailItems(DetailSamples.state(connection, pulse = pulse, window = window))

    /** The items of one section, from its heading to the next. */
    private fun List<DetailItem>.section(key: String): List<DetailItem> {
        val from = indexOfFirst { it.key == key }
        val to = drop(from + 1).indexOfFirst { it is DetailItem.Heading }.let { if (it < 0) size else from + 1 + it }
        return subList(from, to)
    }

    private val DetailItem.texts: List<String>
        get() = when (this) {
            is DetailItem.Notice -> paragraphs.map { it.plain }
            is DetailItem.Heading -> listOf(title)
            is DetailItem.Subheading -> listOf(title)
            is DetailItem.VerdictRow -> listOf(verdict?.level?.word ?: if (live) "Verdict unavailable" else "No current Verdict")
            is DetailItem.Value -> listOfNotNull(label, value.plain, detail?.plain)
            is DetailItem.BigValue -> listOf(value.text + after.plain)
            is DetailItem.Columns -> columns.flatMap { (label, v) -> listOf(label, v.plain) }
            is DetailItem.Text -> listOf(line.plain)
            is DetailItem.Chart -> listOfNotNull(label, series?.summary)
            is DetailItem.CpuBars -> listOf(caption, description)
            is DetailItem.ListRow -> listOfNotNull(name.text, trailing?.text) + tags.map { it.plain } + lines.map { it.plain }
        }

    private fun List<DetailItem>.texts() = flatMap { it.texts }

    private val sectionsButOverview = DETAIL_SECTIONS - "overview"

    @Test
    fun everySectionWithNothingSaysUnavailableOnce() {
        val all = items(Pulse(schema = 1))
        assertEquals(DETAIL_SECTIONS, all.filterIsInstance<DetailItem.Heading>().map { it.key })
        for (key in sectionsButOverview) {
            assertEquals(key, listOf(UNAVAILABLE), all.section(key).drop(1).texts())
        }
        val overview = all.section("overview")
        assertEquals("Verdict unavailable", overview[1].texts.single())
        assertEquals(7, overview.filterIsInstance<DetailItem.Value>().count { it.value.plain == UNAVAILABLE })
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
        assertTrue("read 0 B/s · write 0 B/s" in all.section("storage").texts())
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
        assertTrue("charge 64.0 % · discharging · power 7.5 W · health 87.5 %" in present)
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
        for (connection in listOf(Connection.Live, Connection.Stale, Connection.Unreachable(false))) {
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
        assertEquals(listOf("Capacity age 90 s"), blocked.lines.map { it.plain })
        val primary = items(DetailSamples.pulse).filterIsInstance<DetailItem.ListRow>().single { it.key == "fs:0" }
        assertEquals(listOf("btrfs", "primary"), primary.tags.map { it.plain })
        assertEquals("8.8 %", primary.trailing!!.text)
        assertEquals(listOf("used 83.0 GiB · available 863.1 GiB · total 951.9 GiB", "Capacity age 4 s"), primary.lines.map { it.plain })
    }

    @Test
    fun swapEntriesAreLabelledZramOrSwap() {
        val pulse = Pulse(schema = 1, swapEntries = listOf(
            SwapEntry("/dev/zram0", true, 8L shl 30, 1L shl 30), SwapEntry("/swapfile", false, 4L shl 30, 0), SwapEntry("/dev/sdb2", null, null, null),
        ))
        val rows = items(pulse).filterIsInstance<DetailItem.ListRow>().filter { it.key.startsWith("swap:") }
        assertEquals(listOf(listOf("zram"), listOf("swap"), emptyList()), rows.map { row -> row.tags.map { it.plain } })
        assertEquals(
            listOf("used 1.0 GiB · total 8.0 GiB", "used 0 B · total 4.0 GiB", "used — unavailable · total — unavailable"),
            rows.map { it.lines.single().plain },
        )
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
        assertTrue("Received and sent count only interfaces with a device." in network)
        assertEquals(listOf(UNAVAILABLE),
            items(Pulse(schema = 1, netRxBps = 5)).filter { it.key == "interfaces-unavailable" }.flatMap { it.texts })
    }

    @Test
    fun notLiveDimsValuesAndWithholdsTheVerdict() {
        val live = items(DetailSamples.pulse, Connection.Live, DetailSamples.window)
        assertTrue(live.none { it is DetailItem.Notice })
        assertEquals(VerdictLevel.Ok, live.filterIsInstance<DetailItem.VerdictRow>().single().verdict?.level)
        assertEquals(Tone.Cpu, live.filterIsInstance<DetailItem.Chart>().first().tone)

        val notices = mapOf(
            Connection.Stale to listOf("These are the last values Anshin received, not current ones."),
            Connection.Unreachable(false) to listOf(
                "No Pulse from machine. The Machine may be off, its Agent stopped, or this phone off the tailnet.",
                "These are the last values Anshin received, not current ones.",
            ),
            Connection.Unreachable(true) to listOf(
                "machine isn't a tailnet address, so Anshin won't connect to it.",
                "These are the last values Anshin received, not current ones.",
            ),
        )
        for ((connection, notice) in notices) {
            val all = items(DetailSamples.pulse, connection, DetailSamples.window)
            assertEquals(notice, all.first().texts)
            val verdict = all.filterIsInstance<DetailItem.VerdictRow>().single()
            assertNull(verdict.verdict)
            assertFalse(verdict.live)
            assertEquals("No current Verdict", verdict.texts.single())
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
            "20 h 41 min", "last 30 min", "(devel)", "70.2 %", "16 CPUs", "1.25", "0.98", "0.75", "61.5 °C", "from k10temp/Tctl",
            "56.3 % used", "18.0 GiB", "/dev/zram0", "nvme0n1", "read 84.0 KiB/s · write 208.0 KiB/s", "47.0 °C (12 s ago)",
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
