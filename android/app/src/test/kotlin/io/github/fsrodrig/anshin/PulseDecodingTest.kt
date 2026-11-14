package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PulseDecodingTest {
    @Test
    fun decodesTheCurrentShape() {
        assertEquals(
            Pulse(schema = 1, kind = PulseKind.Full, hostname = "machine", cpuPct = 21.5),
            decodePulse("""{"schema":1,"kind":"full","hostname":"machine","cpu_pct":21.5}"""),
        )
    }

    @Test
    fun keepsNullMeasurementsNull() {
        assertEquals(
            Pulse(schema = 1, kind = PulseKind.Full, hostname = null, cpuPct = null),
            decodePulse("""{"schema":1,"kind":"full","hostname":null,"cpu_pct":null}"""),
        )
    }

    @Test
    fun aMissingMeasurementIsNullNotZero() {
        assertEquals(
            Pulse(schema = 1, kind = PulseKind.Full, hostname = null, cpuPct = null),
            decodePulse("""{"schema":1,"kind":"full"}"""),
        )
    }

    @Test
    fun ignoresFieldsAddedLaterInSchemaOne() {
        assertEquals(
            Pulse(schema = 1, kind = PulseKind.Full, hostname = "machine", cpuPct = 0.0),
            decodePulse(
                """{"schema":1,"kind":"full","hostname":"machine","cpu_pct":0,"future_counter":"123","verdict":{"level":"ok"}}""",
            ),
        )
    }

    @Test
    fun decodesMeasurementsAndKeepsZeroSeparateFromNull() {
        val p = decodePulse("""{"schema":1,"mem_total_bytes":1024,"mem_available_bytes":1024,"mem_used_bytes":0,"mem_used_pct":0,"swap_total_bytes":2048,"swap_used_bytes":0,"swap_entries":[{"name":"/dev/zram0","zram":true,"total_bytes":2048,"used_bytes":0}],"load_1":0,"load_5":1.25,"load_15":2.5,"uptime_s":0,"distro":"Example","kernel":"test","arch":"aarch64","agent_version":"test-build"}""")!!
        assertEquals(1024L, p.memTotalBytes)
        assertEquals(1024L, p.memAvailableBytes)
        assertEquals(0L, p.memUsedBytes)
        assertEquals(0.0, p.memUsedPct)
        assertEquals(2048L, p.swapTotalBytes)
        assertEquals(0L, p.swapUsedBytes)
        assertEquals(listOf(SwapEntry("/dev/zram0", true, 2048, 0)), p.swapEntries)
        assertEquals(0.0, p.load1)
        assertEquals(1.25, p.load5)
        assertEquals(2.5, p.load15)
        assertEquals(0.0, p.uptimeS)
        assertEquals("Example", p.distro)
        assertEquals("test", p.kernel)
        assertEquals("aarch64", p.arch)
        assertEquals("test-build", p.agentVersion)
        val absent = decodePulse("""{"schema":1,"swap_total_bytes":null,"swap_used_bytes":null,"swap_entries":null}""")!!
        assertNull(absent.memUsedPct)
        assertNull(absent.swapUsedBytes)
        assertNull(absent.swapEntries)
    }

    @Test
    fun kindIsAnOpenString() {
        for ((json, kind) in listOf(
            """"kind":"full",""" to PulseKind.Full,
            """"kind":"delta",""" to PulseKind.Delta,
            """"kind":"patch",""" to PulseKind.Unknown,
            "" to PulseKind.Unknown,
        )) {
            assertEquals(json, kind, decodePulse("""{"schema":1,$json"hostname":"machine"}""")?.kind)
        }
    }

    @Test
    fun aKindThatIsNotAStringIsNotReadable() {
        for (kind in listOf("1", "null", "{}")) {
            assertNull(kind, decodePulse("""{"schema":1,"kind":$kind}"""))
        }
    }

    @Test
    fun anotherSchemaIsNotReadable() {
        assertNull(decodePulse("""{"schema":2,"kind":"full","hostname":"machine","cpu_pct":3}"""))
    }

    @Test
    fun somethingThatIsNotAPulseIsNotReadable() {
        for (body in listOf("", "<html></html>", "[]", """{"hostname":"machine"}""", """{"schema":null}""")) {
            assertNull(body, decodePulse(body))
        }
    }
}
