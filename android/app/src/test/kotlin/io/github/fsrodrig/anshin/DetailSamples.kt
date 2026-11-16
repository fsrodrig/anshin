package io.github.fsrodrig.anshin

import kotlin.math.sin
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TestTimeSource

/** A scrub-safe Machine with every section, and its Window, for the detail's tests and screenshots. */
internal object DetailSamples {
    const val MACHINE = "machine"
    private const val GIB = 1024L * 1024 * 1024
    private const val ROWS = 900

    val cores: List<Double?> = listOf(65.0, 81.0, 74.0, 77.0, 83.0, 63.0, 70.0, 60.0, 74.0, null, 66.0, 84.0, 69.0, 58.0, 64.0, 69.0)

    val interfaces = listOf(
        NetInterface("lo", false, 12_861, 12_861),
        NetInterface("eth0", true, 75_262, 23_269),
        NetInterface("wlan0", true, 42, 0),
        NetInterface("tailscale0", false, 3_100, 2_900),
    )
    val disks = listOf(Disk("nvme0n1", 86_016, 212_992), Disk("sda", 0, 0))
    val sensors = listOf(
        Sensor("k10temp/Tctl", 61.5, 0),
        Sensor("k10temp/Tccd1", 58.0, 0),
        Sensor("amdgpu/edge", 47.0, 12),
        Sensor("it87/temp3", null, null),
    )
    val filesystems = listOf(
        Filesystem("0:35", "/", "btrfs", 1022042832896, 89089556480, 926728601600, 8.8, 4.0),
        Filesystem("259:1", "/boot", "vfat", 1063256064, 230686720, 832569344, 21.7, 4.0),
        Filesystem("8:2", "/mnt/archive", "xfs", ageS = 90.0),
    )
    val battery = BatteryReport.Present(
        Battery(64.0, BatteryStatus.Discharging, 7.5, 9000.0, listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Discharging, 7.5, 87.5))),
    )

    val pulse = Pulse(
        schema = PULSE_SCHEMA, kind = PulseKind.Full, cursor = "00000000000000ff.$ROWS", hostname = MACHINE,
        cpuPct = 70.2, cpuCorePct = cores,
        memTotalBytes = 32 * GIB, memAvailableBytes = 14 * GIB, memUsedBytes = 18 * GIB, memUsedPct = 56.3,
        swapTotalBytes = 12 * GIB, swapUsedBytes = 1 * GIB,
        swapEntries = listOf(SwapEntry("/dev/zram0", true, 8 * GIB, 1 * GIB), SwapEntry("/swapfile", false, 4 * GIB, 0)),
        load1 = 1.25, load5 = 0.98, load15 = 0.75, uptimeS = 74_463.83,
        distro = "Example Linux", kernel = "6.12.0-example", arch = "x86_64", agentVersion = "(devel)",
        cpuTempC = 61.5, cpuSensor = "k10temp/Tctl", temperatures = sensors,
        filesystems = filesystems, primaryDisk = "0:35",
        battery = battery, verdict = Verdict(VerdictLevel.Ok, emptyList()),
        diskReadBps = 86_016, diskWriteBps = 212_992, disks = disks,
        netRxBps = 75_304, netTxBps = 23_269, netInterfaces = interfaces,
    )

    /** 900 Samples 2 s apart whose every series ends at [pulse]'s flat value, as the contract requires. */
    val window: HeldWindow = run {
        val times = List(ROWS) { WindowSamples.NEWEST_MS - 2_000L * (ROWS - 1 - it) }
        fun wave(now: Double?, period: Double, depth: Double = 0.4): List<Double?> =
            if (now == null) List(ROWS) { null }
            else List(ROWS) { i -> (now * (1 + depth * (sin(i / period) - sin((ROWS - 1) / period)))).coerceAtLeast(0.0) }
        fun whole(values: List<Double?>) = values.map { it?.let { v -> Math.round(v).toDouble() } }
        fun tenth(values: List<Double?>) = values.map { it?.let { v -> Math.round(v * 10) / 10.0 } }
        val p = pulse
        val b = battery.battery
        HeldWindow(
            Window(
                spanS = 1800, boottimeMs = times,
                cpuPct = tenth(wave(p.cpuPct, 23.0)),
                load1 = wave(p.load1, 61.0).map { it?.let { v -> Math.round(v * 100) / 100.0 } },
                memUsedPct = tenth(wave(p.memUsedPct, 170.0, 0.1)),
                swapUsedBytes = whole(wave(p.swapUsedBytes?.toDouble(), 300.0, 0.3)),
                cpuTempC = tenth(wave(p.cpuTempC, 40.0, 0.15)),
                netRxBps = whole(wave(p.netRxBps?.toDouble(), 17.0, 0.8)),
                netTxBps = whole(wave(p.netTxBps?.toDouble(), 29.0, 0.8)),
                diskReadBps = whole(wave(p.diskReadBps?.toDouble(), 13.0, 0.9)),
                diskWriteBps = whole(wave(p.diskWriteBps?.toDouble(), 19.0, 0.9)),
                batteryPct = tenth(wave(b.pct, 400.0, 0.2)),
                batteryPowerW = tenth(wave(b.powerW, 31.0, 0.3)),
                cpuCorePct = cores.mapIndexed { core, now -> tenth(wave(now, 11.0 + core)) },
            ),
            pulse.cursor!!,
        )
    }

    fun state(
        connection: Connection = Connection.Live,
        age: Duration = 2.seconds,
        pulse: Pulse = this.pulse,
        window: HeldWindow? = this.window,
        machine: String = MACHINE,
    ): MachineDetailState {
        val clock = TestTimeSource()
        val mark = clock.markNow()
        clock += age
        return MachineDetailState(machine, connection, LastPulse(pulse, mark), window)
    }
}
