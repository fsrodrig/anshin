package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class WindowMergeTest {
    // The full example of the contract: a laptop 8 s after start, then a 5-minute suspend (seq 3 is the Gap).
    private val full = """{
      "schema": 1, "kind": "full", "cursor": "9f3c2a71d04e5b68.5", "hostname": "laptop", "cpu_pct": 12.5,
      "cpu_core_pct": [14, 11], "mem_used_pct": 41.9,
      "window": {
        "span_s": 1800,
        "boottime_ms":     [90000000, 90002000, null, 90302001, 90304001],
        "cpu_pct":         [null,     12.0,     null, null,     12.5],
        "load_1":          [0.52,     0.5,      null, 0.61,     0.6],
        "mem_used_pct":    [41.2,     41.2,     null, 41.9,     41.9],
        "swap_used_bytes": [0,        0,        null, 0,        0],
        "cpu_temp_c":      [48.1,     48.4,     null, 51.0,     50.2],
        "net_rx_bps":      [null,     980,      null, null,     15003],
        "net_tx_bps":      [null,     410,      null, null,     2210],
        "disk_read_bps":   [null,     0,        null, null,     4096],
        "disk_write_bps":  [null,     0,        null, null,     81920],
        "battery_pct":     [81.0,     81.0,     null, 80.0,     80.0],
        "battery_power_w": [7.43,     7.4,      null, 9.1,      8.85],
        "cpu_core_pct":   [[null,     13,       null, null,     14],
                           [null,     11,       null, null,     11]]
      }
    }"""

    private val delta = """{
      "schema": 1, "kind": "delta", "cursor": "9f3c2a71d04e5b68.6", "hostname": "laptop", "cpu_pct": 9.8,
      "cpu_core_pct": [10, 9],
      "window": {
        "span_s": 1800, "boottime_ms": [90306001], "cpu_pct": [9.8], "load_1": [0.58], "mem_used_pct": [41.9],
        "swap_used_bytes": [0], "cpu_temp_c": [49.9], "net_rx_bps": [800], "net_tx_bps": [120], "disk_read_bps": [0],
        "disk_write_bps": [4096], "battery_pct": [80.0], "battery_power_w": [8.7], "cpu_core_pct": [[10], [9]]
      }
    }"""

    private val emptyDelta = """{
      "schema": 1, "kind": "delta", "cursor": "9f3c2a71d04e5b68.5", "cpu_pct": 12.5,
      "window": {
        "span_s": 1800, "boottime_ms": [], "cpu_pct": [], "load_1": [], "mem_used_pct": [], "swap_used_bytes": [],
        "cpu_temp_c": [], "net_rx_bps": [], "net_tx_bps": [], "disk_read_bps": [], "disk_write_bps": [],
        "battery_pct": [], "battery_power_w": [], "cpu_core_pct": [[], []]
      }
    }"""

    private fun pulse(json: String) = checkNotNull(decodePulse(json)) { "does not decode: $json" }

    private fun held() = checkNotNull(null.merge(pulse(full)))

    @Test
    fun aFullPulseReplacesWhatThePhoneHolds() {
        val p = pulse(full)
        assertEquals(HeldWindow(p.window!!, "9f3c2a71d04e5b68.5"), null.merge(p))
        val other = HeldWindow(Window(spanS = 1800, boottimeMs = listOf(1L), cpuPct = listOf(3.0)), "0000000000000001.1")
        assertEquals(HeldWindow(p.window, "9f3c2a71d04e5b68.5"), other.merge(p))
    }

    @Test
    fun aFullPulseFromAnAgentBeforeTheWindowHoldsNone() {
        assertNull(null.merge(pulse("""{"schema":1,"kind":"full","cpu_pct":3}""")))
        assertNull(held().merge(pulse("""{"schema":1,"kind":"full","cpu_pct":3}""")))
        assertNull(null.merge(pulse("""{"schema":1,"kind":"full","window":{"span_s":1800,"boottime_ms":[1],"cpu_pct":[3]}}""")))
        assertNull(null.merge(pulse("""{"schema":1,"kind":"full","cursor":"0000000000000001.1","window":null}""")))
    }

    @Test
    fun aDeltaAppendsItsRows() {
        val merged = checkNotNull(held().merge(pulse(delta)))
        assertEquals("9f3c2a71d04e5b68.6", merged.cursor)
        val w = merged.window
        assertEquals(listOf(90000000L, 90002000L, null, 90302001L, 90304001L, 90306001L), w.boottimeMs)
        assertEquals(listOf(null, 12.0, null, null, 12.5, 9.8), w.cpuPct)
        assertEquals(listOf(41.2, 41.2, null, 41.9, 41.9, 41.9), w.memUsedPct)
        assertEquals(listOf(7.43, 7.4, null, 9.1, 8.85, 8.7), w.batteryPowerW)
        assertEquals(
            listOf(listOf(null, 13.0, null, null, 14.0, 10.0), listOf(null, 11.0, null, null, 11.0, 9.0)),
            w.cpuCorePct,
        )
    }

    @Test
    fun aDeltaWithNoNewRowsKeepsTheWindow() {
        val before = held()
        assertEquals(before, before.merge(pulse(emptyDelta)))
    }

    @Test
    fun aSeriesAbsentFromOneSideIsUnavailableThere() {
        val start = HeldWindow(Window(spanS = 1800, boottimeMs = listOf(1000L), cpuPct = listOf(5.0)), "00000000000000aa.1")
        val next = pulse("""{"schema":1,"kind":"delta","cursor":"00000000000000aa.2",
            "window":{"span_s":1800,"boottime_ms":[3000],"mem_used_pct":[40.0]}}""")
        val w = checkNotNull(start.merge(next)).window
        assertEquals(listOf(5.0, null), w.cpuPct)
        assertEquals(listOf(null, 40.0), w.memUsedPct)
        assertNull(w.load1)
    }

    @Test
    fun aDeltaWithoutAHeldWindowAsksForTheWholeWindow() {
        assertNull(null.merge(pulse(delta)))
    }

    @Test
    fun aDeltaFromAnotherRunAsksForTheWholeWindow() {
        assertNull(held().merge(pulse(delta.replace("9f3c2a71d04e5b68.6", "0000000000000002.6"))))
    }

    @Test
    fun aDeltaWhoseRowsDisagreeWithItsCursorAsksForTheWholeWindow() {
        assertNull(held().merge(pulse(delta.replace("9f3c2a71d04e5b68.6", "9f3c2a71d04e5b68.7"))))
        assertNull(held().merge(pulse(delta.replace("9f3c2a71d04e5b68.6", "9f3c2a71d04e5b68.4"))))
        assertNull(held().merge(pulse(emptyDelta.replace("9f3c2a71d04e5b68.5", "9f3c2a71d04e5b68.6"))))
    }

    @Test
    fun aDeltaWithOtherCpuCoresAsksForTheWholeWindow() {
        assertNull(held().merge(pulse(delta.replace("[[10], [9]]", "[[10], [9], [8]]"))))
        assertNull(held().merge(pulse(delta.replace("\"cpu_core_pct\": [[10], [9]]", "\"cpu_core_pct\": null"))))
    }

    @Test
    fun aDeltaThatGoesBackInTimeAsksForTheWholeWindow() {
        assertNull(held().merge(pulse(delta.replace("[90306001]", "[90304001]"))))
    }

    @Test
    fun anUnknownKindAsksForTheWholeWindow() {
        assertNull(held().merge(pulse(full.replace("\"full\"", "\"snapshot\""))))
    }

    @Test
    fun aMalformedWindowAsksForTheWholeWindow() {
        for ((what, json) in listOf(
            "short series" to full.replace("[0.52,     0.5,      null, 0.61,     0.6]", "[0.52, 0.5]"),
            "short core" to full.replace("[null,     11,       null, null,     11]", "[null, 11]"),
            "times not increasing" to full.replace("90302001", "90001000"),
            "a Gap last" to full.replace("90304001]", "null]"),
            "no span" to full.replace("\"span_s\": 1800,", ""),
            "zero span" to full.replace("\"span_s\": 1800", "\"span_s\": 0"),
            "cursor without seq" to full.replace("9f3c2a71d04e5b68.5", "9f3c2a71d04e5b68"),
        )) {
            assertNull(what, null.merge(pulse(json)))
            assertNull(what, held().merge(pulse(json.replace("\"full\"", "\"delta\""))))
        }
    }

    @Test
    fun trimmingKeepsTheSpanAndALeadingGap() {
        val span = 60L
        // The Sample at 0 is older than the span; the Gap after it stays because the Sample after the Gap is kept.
        val w = Window(spanS = span, boottimeMs = listOf(0L, null, 40_000L, 70_000L), cpuPct = listOf(1.0, null, 2.0, 3.0))
        val held = checkNotNull(null.merge(Pulse(schema = 1, kind = PulseKind.Full, cursor = "0000000000000001.4", window = w)))
        assertEquals(listOf(null, 40_000L, 70_000L), held.window.boottimeMs)
        assertEquals(listOf(null, 2.0, 3.0), held.window.cpuPct)
    }

    @Test
    fun aGapIsEvictedWithTheSampleAfterIt() {
        val w = Window(spanS = 60, boottimeMs = listOf(0L, null, 5_000L, 70_000L), cpuPct = listOf(1.0, null, 2.0, 3.0))
        val held = checkNotNull(null.merge(Pulse(schema = 1, kind = PulseKind.Full, cursor = "0000000000000001.4", window = w)))
        assertEquals(listOf(70_000L), held.window.boottimeMs)
        assertEquals(listOf(3.0), held.window.cpuPct)
    }

    @Test
    fun aDeltaPushesTheOldestSamplesOut() {
        val start = HeldWindow(Window(spanS = 4, boottimeMs = listOf(0L, 2_000L), cpuPct = listOf(1.0, 2.0)), "0000000000000001.2")
        val next = Pulse(schema = 1, kind = PulseKind.Delta, cursor = "0000000000000001.3",
            window = Window(spanS = 4, boottimeMs = listOf(4_000L), cpuPct = listOf(3.0)))
        val w = checkNotNull(start.merge(next)).window
        assertEquals(listOf(2_000L, 4_000L), w.boottimeMs)
        assertEquals(listOf(2.0, 3.0), w.cpuPct)
    }
    @Test
    fun cadenceVariationStillLimitsTheWindowTo900Rows() {
        val times = List(900) { it * 2000L }
        assertCapacityMerge(times, listOf(1_799_999L), (times + 1_799_999L).drop(1))
    }

    @Test
    fun capacityEvictsALeadingGapAndItsResumeSample() {
        val times = listOf<Long?>(null) + List(899) { 1_860_000L + it * 2000L }
        val next = listOf(3_658_000L)
        assertCapacityMerge(times, next, (times + next).drop(2))
    }

    private fun assertCapacityMerge(times: List<Long?>, next: List<Long?>, expected: List<Long?>) {
        fun window(rows: List<Long?>) = Window(
            spanS = 1800, boottimeMs = rows,
            cpuPct = rows.map { it?.let { 25.0 } },
            cpuCorePct = listOf(rows.map { it?.let { 30.0 } }),
        )
        val start = Pulse(schema = 1, kind = PulseKind.Full, cursor = "0000000000000001.900", window = window(times))
        val delta = Pulse(schema = 1, kind = PulseKind.Delta, cursor = "0000000000000001.901", window = window(next))
        val full = delta.copy(kind = PulseKind.Full, window = window(expected))
        assertEquals(null.merge(full), null.merge(start).merge(delta))
    }

}
