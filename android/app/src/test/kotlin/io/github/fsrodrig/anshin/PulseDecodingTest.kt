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
                """{"schema":1,"kind":"full","hostname":"machine","cpu_pct":0,"mem_used_bytes":"123","verdict":{"level":"ok"}}""",
            ),
        )
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
