package io.github.fsrodrig.anshin

import kotlin.math.sin

/** Deterministic Windows for screenshots: Samples 2 s apart ending at [NEWEST_MS], like an Agent's. */
internal object WindowSamples {
    const val NEWEST_MS = 3_600_000L
    private const val STEP_MS = 2_000L

    fun cpu(i: Int) = 22.0 + 14.0 * sin(i / 45.0) + 5.0 * sin(i / 6.0)
    fun memory(i: Int) = 41.0 + 3.0 * sin(i / 170.0)

    /** [rows] Samples; the first has no CPU, as an Agent's first Sample has no rates yet. */
    fun samples(rows: Int, cpu: (Int) -> Double? = ::cpu, memory: (Int) -> Double? = ::memory): HeldWindow {
        val times = List(rows) { NEWEST_MS - STEP_MS * (rows - 1 - it) }
        return held(times, List(rows) { if (it == 0) null else cpu(it) }, List(rows) { memory(it) })
    }

    /** 900 Samples whose newest are exactly [cpuNow] and [memoryNow], as the flat fields beside them. */
    fun endingAt(cpuNow: Double, memoryNow: Double): HeldWindow =
        samples(900, cpu = { cpu(it) - cpu(899) + cpuNow }, memory = { memory(it) - memory(899) + memoryNow })

    /** 30 minutes with a Gap of [gapMs] ending [endsBeforeNewestMs] before the newest Sample. */
    fun withGap(gapMs: Long = 300_000, endsBeforeNewestMs: Long = 600_000): HeldWindow {
        val resume = NEWEST_MS - endsBeforeNewestMs
        val suspend = resume - gapMs
        val oldest = NEWEST_MS - 1_798_000
        val before = generateSequence(oldest) { it + STEP_MS }.takeWhile { it <= suspend }.toList()
        val after = generateSequence(resume) { it + STEP_MS }.takeWhile { it <= NEWEST_MS }.toList()
        val times = before + listOf(null) + after
        val cpu = times.mapIndexed { i, t -> if (t == null || t == resume || i == 0) null else cpu(i) }
        // The same memory on both sides of the Gap: a line joined across it would be flat and look real.
        val memory = times.map { t -> t?.let { 44.0 } }
        return held(times, cpu, memory)
    }

    private fun held(times: List<Long?>, cpu: List<Double?>, memory: List<Double?>) = HeldWindow(
        Window(spanS = 1800, boottimeMs = times, cpuPct = cpu.map(::wire), memUsedPct = memory.map(::wire)),
        "00000000000000ff.${times.size}",
    )

    private fun wire(value: Double?) = value?.let { Math.round(it * 10) / 10.0 }
}
