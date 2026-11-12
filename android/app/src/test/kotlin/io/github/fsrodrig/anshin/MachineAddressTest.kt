package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class MachineAddressTest {
    @Test
    fun defaultsToTheAgentPort() {
        assertEquals("http://machine:7247/v1/pulse", pulseUrl("machine").toString())
        assertEquals("http://machine.example.ts.net:7247/v1/pulse", pulseUrl(" machine.example.ts.net ").toString())
        assertEquals("http://100.64.0.1:7247/v1/pulse", pulseUrl("100.64.0.1").toString())
        assertEquals("http://[fd7a:115c:a1e0::1]:7247/v1/pulse", pulseUrl("fd7a:115c:a1e0::1").toString())
    }

    @Test
    fun acceptsAnExplicitPort() {
        assertEquals("http://machine:9000/v1/pulse", pulseUrl("machine:9000").toString())
        assertEquals("http://[fd7a:115c:a1e0::1]:9000/v1/pulse", pulseUrl("[fd7a:115c:a1e0::1]:9000").toString())
        assertEquals("http://[fd7a:115c:a1e0::1]:7247/v1/pulse", pulseUrl("[fd7a:115c:a1e0::1]").toString())
    }

    @Test
    fun rejectsWhatIsNotAMachineAddress() {
        for (input in listOf("", "  ", ":7247", "machine:0", "machine:65536", "machine:port", "[fd7a::1", "[::1]x",
            "http://machine", "machine/v1/pulse", "two words")) {
            assertNull(input, pulseUrl(input))
        }
    }
}
