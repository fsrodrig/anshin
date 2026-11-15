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
                """{"schema":1,"kind":"full","hostname":"machine","cpu_pct":0,"future_counter":"123","future_object":{"level":"ok"}}""",
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
    fun filesystemFieldsAreAdditiveAndIndependentlyNullable() {
        val p = decodePulse("""{"schema":1,"primary_disk":"0:35","filesystems":[{"maj_min":"0:35","mount_point":"/","fs_type":"btrfs","total_bytes":1024,"used_bytes":0,"available_bytes":1024,"used_pct":0,"age_s":0},{"maj_min":"8:1","total_bytes":null,"used_pct":null,"age_s":90}]}""")!!
        assertEquals("0:35", p.primaryDisk)
        assertEquals(Filesystem("0:35", "/", "btrfs", 1024, 0, 1024, 0.0, 0.0), p.filesystems!![0])
        assertNull(p.filesystems[1].totalBytes)
        assertNull(p.filesystems[1].usedPct)
        assertEquals(90.0, p.filesystems[1].ageS)
        assertNull(decodePulse("""{"schema":1}""")!!.filesystems)
        assertNull(decodePulse("""{"schema":1}""")!!.primaryDisk)
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

    @Test
    fun temperatureFieldsAreAdditiveAndIndependentlyNullable() {
        val pulse = decodePulse(
            """{"schema":1,"cpu_temp_c":0,"cpu_sensor":"k10temp/Tctl","temperatures":[{"sensor":"k10temp/Tctl","temp_c":0},""" +
                """{"sensor":"amdgpu/edge","temp_c":null},{"sensor":"it87/temp3"},{"sensor":"k10temp/Tccd1","temp_c":-40.5}]}""",
        )!!
        assertEquals(0.0, pulse.cpuTempC)
        assertEquals("k10temp/Tctl", pulse.cpuSensor)
        assertEquals(
            listOf(Sensor("k10temp/Tctl", 0.0), Sensor("amdgpu/edge", null), Sensor("it87/temp3", null), Sensor("k10temp/Tccd1", -40.5)),
            pulse.temperatures,
        )
        for (body in listOf("""{"schema":1}""", """{"schema":1,"cpu_temp_c":null,"cpu_sensor":null,"temperatures":null}""")) {
            val p = decodePulse(body)!!
            assertNull(body, p.cpuTempC)
            assertNull(body, p.cpuSensor)
            assertNull(body, p.temperatures)
        }
    }

    @Test
    fun batteryIsThreeWay() {
        assertEquals(BatteryReport.NotReported, decodePulse("""{"schema":1}""")!!.battery)
        assertEquals(BatteryReport.NoBattery, decodePulse("""{"schema":1,"battery":null}""")!!.battery)
        assertEquals(
            BatteryReport.Present(Battery(pct = 64.0, status = BatteryStatus.Discharging)),
            decodePulse("""{"schema":1,"battery":{"pct":64,"status":"Discharging"}}""")!!.battery,
        )
        for (battery in listOf("1", "\"none\"", "[]")) {
            assertNull(battery, decodePulse("""{"schema":1,"battery":$battery}"""))
        }
    }

    @Test
    fun temperatureAgeIsAdditiveAndNullable() {
        val pulse = decodePulse(
            """{"schema":1,"temperatures":[{"sensor":"k10temp/Tctl","temp_c":61},{"sensor":"it87/temp3","temp_c":null,"age_s":null},""" +
                """{"sensor":"k10temp/Tccd1","temp_c":58,"age_s":0},{"sensor":"amdgpu/edge","temp_c":67,"age_s":28}]}""",
        )!!
        assertEquals(listOf(null, null, 0L, 28L), pulse.temperatures!!.map { it.ageS })
    }

    @Test
    fun batteryFieldsAreIndependentlyNullable() {
        val p = decodePulse("""{"schema":1,"battery":{"pct":0,"status":null,"power_w":0,"time_left_s":9000,"batteries":[{"name":"BAT0","pct":null,"status":"Charging","power_w":0,"health_pct":104.2},{"name":"BAT1"}]}}""")!!
        assertEquals(
            BatteryReport.Present(Battery(pct = 0.0, status = null, powerW = 0.0, timeLeftS = 9000.0, batteries = listOf(
                BatteryRow(name = "BAT0", pct = null, status = BatteryStatus.Charging, powerW = 0.0, healthPct = 104.2),
                BatteryRow(name = "BAT1"),
            ))),
            p.battery,
        )
        assertEquals(
            BatteryReport.Present(Battery()),
            decodePulse("""{"schema":1,"battery":{"pct":null,"status":null,"power_w":null,"time_left_s":null,"batteries":null}}""")!!.battery,
        )
        assertEquals(
            BatteryReport.Present(Battery(timeLeftS = 9000.5)),
            decodePulse("""{"schema":1,"battery":{"time_left_s":9000.5}}""")!!.battery,
        )
    }

    @Test
    fun batteryStatusIsAnOpenString() {
        for ((json, status) in listOf(
            "\"Charging\"" to BatteryStatus.Charging,
            "\"Discharging\"" to BatteryStatus.Discharging,
            "\"Not charging\"" to BatteryStatus.NotCharging,
            "\"Full\"" to BatteryStatus.Full,
            "\"Unknown\"" to BatteryStatus.Unknown,
            "\"Draining\"" to BatteryStatus.Unknown,
            "null" to null,
        )) {
            assertEquals(json, BatteryReport.Present(Battery(status = status)), decodePulse("""{"schema":1,"battery":{"status":$json}}""")?.battery)
        }
        assertEquals(BatteryReport.Present(Battery()), decodePulse("""{"schema":1,"battery":{}}""")?.battery)
        for (status in listOf("1", "{}")) {
            assertNull(status, decodePulse("""{"schema":1,"battery":{"status":$status}}"""))
        }
    }

    @Test
    fun ignoresUnknownBatteryKeys() {
        assertEquals(
            BatteryReport.Present(Battery(pct = 50.0, batteries = listOf(BatteryRow(name = "BAT0", pct = 50.0)))),
            decodePulse("""{"schema":1,"battery":{"pct":50,"cycle_count":3,"batteries":[{"name":"BAT0","pct":50,"cycle_count":3}]}}""")!!.battery,
        )
    }

    @Test
    fun deeplyNestedUnknownVerdictAndReasonFieldsAreSkipped() {
        val deep = "[".repeat(401) + "0" + "]".repeat(401)
        for ((verdict, expected) in listOf(
            """{"level":"warn","future":$deep}""" to Verdict(VerdictLevel.Warn),
            """{"level":"warn","reasons":[{"code":"fan_stalled","level":"warn","message":"A fan stopped","future":$deep}]}""" to
                Verdict(VerdictLevel.Warn, listOf(Reason("fan_stalled", VerdictLevel.Warn, "A fan stopped"))),
        )) {
            val pulse = checkNotNull(decodePulse("""{"schema":1,"cpu_pct":21,"verdict":$verdict}"""))
            assertEquals(21.0, pulse.cpuPct)
            assertEquals(expected, pulse.verdict)
        }
    }

    @Test
    fun malformedVerdictStringsWithinTheDepthLimitKeepTheRestOfThePulse() {
        for (verdict in listOf(
            "null", "{}",
            """{"level":null}""", """{"level":42}""", """{"level":true}""",
            """{"level":[]}""", """{"level":{}}""",
            """{"level":"warn","reasons":[{"message":[]}]}""",
            """{"level":"warn","reasons":[{"code":42}]}""",
            """{"level":"warn","reasons":[{"level":true}]}""",
        )) {
            val pulse = checkNotNull(decodePulse("""{"schema":1,"kind":"full","cpu_pct":21,"verdict":$verdict}"""))
            assertEquals(verdict, 21.0, pulse.cpuPct)
            assertNull(verdict, pulse.verdict)
        }
    }

    @Test
    fun malformedVerdictAndReasonContainersAreNotReadable() {
        for (verdict in listOf(
            "[]", "42", "\"warn\"",
            """{"level":"warn","reasons":{}}""", """{"level":"warn","reasons":"bad"}""",
            """{"level":"warn","reasons":[42]}""", """{"level":"warn","reasons":[null]}""",
        )) {
            assertNull(verdict, decodePulse("""{"schema":1,"cpu_pct":21,"verdict":$verdict}"""))
        }
    }

    private fun verdictOf(json: String): Verdict? =
        decodePulse("""{"schema":1,"kind":"full","verdict":$json}""").let { checkNotNull(it) { "$json does not decode" } }.verdict

    @Test
    fun decodesTheKnownVerdictLevels() {
        assertEquals(Verdict(VerdictLevel.Ok, emptyList()), verdictOf("""{"level":"ok","reasons":[]}"""))
        assertEquals(
            Verdict(VerdictLevel.Warn, listOf(Reason("filesystem_full", VerdictLevel.Warn, "/ is 91.2 % full"))),
            verdictOf("""{"level":"warn","reasons":[{"code":"filesystem_full","level":"warn","message":"/ is 91.2 % full"}]}"""),
        )
        assertEquals(
            Verdict(VerdictLevel.Crit, listOf(
                Reason("memory_low", VerdictLevel.Crit, "Memory available is 3 %"),
                Reason("cpu_hot", VerdictLevel.Warn, "CPU is 91 °C"),
            )),
            verdictOf("""{"level":"crit","reasons":[{"code":"memory_low","level":"crit","message":"Memory available is 3 %"},""" +
                """{"code":"cpu_hot","level":"warn","message":"CPU is 91 °C"}]}"""),
        )
    }

    @Test
    fun anUnknownVerdictLevelKeepsItsWord() {
        val verdict = verdictOf("""{"level":"dire","reasons":[{"code":"cpu_hot","level":"grave","message":"CPU is hot"}]}""")
        assertEquals(VerdictLevel.Other("dire"), verdict?.level)
        assertEquals("dire", verdict?.level?.word)
        assertEquals(VerdictLevel.Other("grave"), verdict?.reasons?.single()?.level)
    }

    @Test
    fun anUnknownReasonCodeDecodes() {
        assertEquals(
            Verdict(VerdictLevel.Warn, listOf(Reason("fan_stalled", VerdictLevel.Warn, "A fan stopped"))),
            verdictOf("""{"level":"warn","reasons":[{"code":"fan_stalled","level":"warn","message":"A fan stopped","since_s":4}]}"""),
        )
    }

    @Test
    fun aReasonWithoutALevelOrMessageDecodes() {
        assertEquals(
            Verdict(VerdictLevel.Warn, listOf(Reason("fan_stalled", null, "A fan stopped"), Reason())),
            verdictOf("""{"level":"warn","reasons":[{"code":"fan_stalled","message":"A fan stopped"},{}]}"""),
        )
    }

    @Test
    fun missingOrNullReasonsDecodeAsNone() {
        assertEquals(Verdict(VerdictLevel.Ok), verdictOf("""{"level":"ok"}"""))
        assertEquals(Verdict(VerdictLevel.Ok), verdictOf("""{"level":"ok","reasons":null}"""))
    }

    @Test
    fun aNullVerdictAndAnAbsentOneAreBothNoCurrentVerdict() {
        assertNull(verdictOf("null"))
        val absent = decodePulse("""{"schema":1,"kind":"full","cpu_pct":3}""")
        assertEquals(Pulse(schema = 1, kind = PulseKind.Full, cpuPct = 3.0), absent)
        assertNull(absent?.verdict)
    }

    @Test
    fun decodesTheWindowAndKeepsItsNullsNull() {
        val p = decodePulse("""{"schema":1,"kind":"full","cursor":"00000000000000ff.3","cpu_pct":4.5,"cpu_core_pct":[5,null],
            "disk_read_bps":0,"disk_write_bps":null,
            "window":{"span_s":1800,"boottime_ms":[1000,null,301000],"cpu_pct":[null,null,4.5],"mem_used_pct":[40.1,null,0],
            "net_rx_bps":[0,null,15003],"cpu_core_pct":[[null,null,5],[null,null,null]]}}""")!!
        assertEquals("00000000000000ff.3", p.cursor)
        assertEquals(listOf(5.0, null), p.cpuCorePct)
        assertEquals(0L, p.diskReadBps)
        assertNull(p.diskWriteBps)
        val w = p.window!!
        assertEquals(1800L, w.spanS)
        assertEquals(listOf(1000L, null, 301000L), w.boottimeMs)
        assertEquals(listOf(null, null, 4.5), w.cpuPct)
        assertEquals(listOf(40.1, null, 0.0), w.memUsedPct)
        assertEquals(listOf(0.0, null, 15003.0), w.netRxBps)
        assertEquals(listOf(listOf(null, null, 5.0), listOf(null, null, null)), w.cpuCorePct)
        assertNull(w.load1)
    }

    @Test
    fun anUnknownSeriesInTheWindowIsIgnored() {
        assertEquals(
            Window(spanS = 1800, boottimeMs = listOf(1000L), cpuPct = listOf(2.0)),
            decodePulse("""{"schema":1,"window":{"span_s":1800,"boottime_ms":[1000],"cpu_pct":[2],"fan_rpm":[1200]}}""")!!.window,
        )
    }

    @Test
    fun aPulseWithoutAWindowHasNoWindowAndNoCursor() {
        for (json in listOf("""{"schema":1,"kind":"full"}""", """{"schema":1,"kind":"full","cursor":null,"window":null}""")) {
            val p = decodePulse(json)!!
            assertNull(json, p.window)
            assertNull(json, p.cursor)
            assertNull(json, p.cpuCorePct)
        }
    }

    @Test
    fun aWordInAWindowSeriesIsNotReadable() {
        for (window in listOf(
            """{"span_s":1800,"boottime_ms":[1000],"cpu_pct":["high"]}""",
            """{"span_s":1800,"boottime_ms":["now"]}""",
            """{"span_s":1800,"boottime_ms":[1000],"cpu_core_pct":[["high"]]}""",
        )) {
            assertNull(window, decodePulse("""{"schema":1,"kind":"full","window":$window}"""))
        }
    }
}
