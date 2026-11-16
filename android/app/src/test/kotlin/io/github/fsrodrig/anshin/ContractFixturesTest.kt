package io.github.fsrodrig.anshin

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Wire compatibility: this app reads every Pulse in contract/fixtures, the Agent's current ones and kept old ones. */
class ContractFixturesTest {
    private val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })

    @Test
    fun everyFixtureDecodes() {
        val files = dir.walkTopDown().filter { it.isFile && it.extension == "json" }.toList()
        // A wrong directory must not pass on zero files.
        assertTrue("no future.json in $dir", File(dir, "future.json") in files)
        assertTrue("no generated fixture in $dir", files.size >= 2)
        for (file in files) {
            val pulse = decodePulse(file.readText())
            assertNotNull("${file.relativeTo(dir)} does not decode", pulse)
            assertEquals("${file.relativeTo(dir)} schema", PULSE_SCHEMA, pulse!!.schema)
        }
    }

    @Test
    fun theFutureFixtureDecodes() {
        assertEquals(
            Pulse(
                schema = 1, kind = PulseKind.Full, cursor = "00000000000000ff.3", hostname = "future", cpuPct = 12.5,
                verdict = Verdict(VerdictLevel.Warn, listOf(Reason(code = "fan_stalled", level = null, message = "A fan stopped"))),
                window = Window(spanS = 1800, boottimeMs = listOf(1000L, null, 301000L), cpuPct = listOf(10.0, null, 12.5)),
            ),
            decodePulse(File(dir, "future.json").readText()),
        )
    }

    @Test
    fun aPulseFromBeforeTemperaturesDecodesThemAsUnavailable() {
        val pulse = decodePulse(File(dir, "kept/2026-11-14-4/desktop.json").readText())!!
        assertNull(pulse.cpuTempC)
        assertNull(pulse.cpuSensor)
        assertNull(pulse.temperatures)
    }

    // Catches a @SerialName that disagrees with what the Agent emits: the field would decode as null here.
    @Test
    fun theCurrentFixtureNamesItsCpuSensorAmongItsTemperatures() {
        val pulse = decodePulse(File(dir, "desktop.json").readText())!!
        val sensor = checkNotNull(pulse.cpuSensor) { "no cpu_sensor" }
        val temperatures = pulse.temperatures.orEmpty()
        assertTrue("no temperatures", temperatures.isNotEmpty())
        assertNotNull(pulse.cpuTempC)
        assertEquals(temperatures.first { it.id == sensor }.tempC, pulse.cpuTempC)
    }

    @Test
    fun theDesktopHasNoBatteryAndOldFixturesDidNotReportOne() {
        assertEquals(BatteryReport.NoBattery, decodePulse(File(dir, "desktop.json").readText())!!.battery)
        assertEquals(BatteryReport.NotReported, decodePulse(File(dir, "kept/2026-11-14-3/desktop.json").readText())!!.battery)
    }

    @Test
    fun aPulseFromBeforeTheWindowHasNone() {
        val pulse = decodePulse(File(dir, "kept/2026-11-14-5/desktop.json").readText())!!
        assertNull(pulse.window)
        assertNull(pulse.cursor)
        assertNull(null.merge(pulse))
    }

    private fun fixture(name: String) = checkNotNull(decodePulse(File(dir, name).readText())) { "$name does not decode" }

    @Test
    fun theCurrentFixtureCarriesItsWindow() {
        val pulse = fixture("desktop.json")
        assertEquals(PulseKind.Full, pulse.kind)
        assertTrue(pulse.cursor!!.endsWith(".3"))
        val window = pulse.window!!
        assertEquals(3, window.rows)
        assertNull(window.cpuPct!!.first())
        assertEquals(pulse.cpuPct, window.cpuPct.last())
    }

    @Test
    fun theDeltaFixtureCompletesTheWindowItAnswers() {
        val full = fixture("desktop.json")
        val delta = fixture("desktop.delta.json")
        assertEquals(PulseKind.Delta, delta.kind)
        assertEquals(1, delta.window!!.rows)
        val w = full.window!!
        val firstTwo = w.copy(
            boottimeMs = w.boottimeMs.take(2), cpuPct = w.cpuPct?.take(2), load1 = w.load1?.take(2),
            memUsedPct = w.memUsedPct?.take(2), swapUsedBytes = w.swapUsedBytes?.take(2), cpuTempC = w.cpuTempC?.take(2),
            netRxBps = w.netRxBps?.take(2), netTxBps = w.netTxBps?.take(2), diskReadBps = w.diskReadBps?.take(2),
            diskWriteBps = w.diskWriteBps?.take(2), batteryPct = w.batteryPct?.take(2), batteryPowerW = w.batteryPowerW?.take(2),
            cpuCorePct = w.cpuCorePct?.map { it.take(2) },
        )
        assertEquals(HeldWindow(w, full.cursor!!), HeldWindow(firstTwo, "0000000000000001.2").merge(delta))
    }

    @Test
    fun theSuspendFixtureHasOneGapAndNoRatesRightAfterIt() {
        val pulse = fixture("desktop.suspend.json")
        val window = pulse.window!!
        assertEquals(1, window.boottimeMs.count { it == null })
        val gap = window.boottimeMs.indexOf(null)
        assertNull(window.netRxBps!![gap + 1])
        assertNotNull(window.memUsedPct!![gap + 1])
        assertNull(pulse.netRxBps)
        assertNull(pulse.diskReadBps)
        assertTrue("no interfaces", pulse.netInterfaces.orEmpty().isNotEmpty())
        assertTrue("no disks", pulse.disks.orEmpty().isNotEmpty())
        assertTrue(pulse.netInterfaces!!.all { it.rxBps == null && it.txBps == null })
        assertTrue(pulse.disks!!.all { it.readBps == null && it.writeBps == null })
    }

    // Catches a @SerialName that disagrees with what the Agent emits: the field would decode as null here.
    @Test
    fun theCurrentFixtureListsItsInterfacesAndDisks() {
        val pulse = fixture("desktop.json")
        val interfaces = pulse.netInterfaces.orEmpty()
        val disks = pulse.disks.orEmpty()
        assertTrue("no interfaces", interfaces.isNotEmpty())
        assertTrue("no disks", disks.isNotEmpty())
        assertTrue(interfaces.all { it.name != null } && disks.all { it.name != null })
        assertTrue(interfaces.any { it.deviceBacked == true })
        assertTrue(interfaces.any { it.deviceBacked == false })
        val window = pulse.window!!
        assertEquals(pulse.netRxBps!!.toDouble(), window.netRxBps!!.last())
        assertEquals(pulse.netTxBps!!.toDouble(), window.netTxBps!!.last())
        assertEquals(pulse.diskReadBps!!.toDouble(), window.diskReadBps!!.last())
        assertEquals(pulse.diskWriteBps!!.toDouble(), window.diskWriteBps!!.last())
    }

    @Test
    fun aPulseFromBeforeNetworkAndDiskRatesDecodesThemAsUnavailable() {
        val pulse = fixture("kept/2026-11-14-2/desktop.json")
        assertNull(pulse.netRxBps)
        assertNull(pulse.netTxBps)
        assertNull(pulse.netInterfaces)
        assertNull(pulse.disks)
    }
}
