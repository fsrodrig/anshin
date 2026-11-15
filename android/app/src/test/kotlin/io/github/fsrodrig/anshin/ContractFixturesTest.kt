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
                schema = 1, kind = PulseKind.Full, hostname = "future", cpuPct = 12.5,
                verdict = Verdict(VerdictLevel.Warn, listOf(Reason(code = "fan_stalled", level = null, message = "A fan stopped"))),
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
}
