package io.github.fsrodrig.anshin

import androidx.datastore.core.DataStore
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import kotlin.time.Duration.Companion.minutes
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.runBlocking
import org.junit.rules.TemporaryFolder

/** Saved Machines and their Pulses for the Dashboard and form tests. Names and addresses are placeholders. */
internal object Machines {
    fun machine(name: String, host: String = name, port: Int = AGENT_PORT) = Machine(MachineId("id-$name"), name, MachineAddress(host, port))

    val laptop = machine("laptop", "laptop.example.ts.net")
    val desktop = machine("desktop", "100.64.0.2")

    private val ok = Verdict(VerdictLevel.Ok, emptyList())

    val laptopPulse = Pulse(
        schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = "laptop", cpuPct = 21.4, memUsedPct = 25.0, cpuTempC = 52.1,
        filesystems = listOf(Filesystem(majMin = "0:35", mountPoint = "/", usedPct = 41.3)), primaryDisk = "0:35",
        battery = BatteryReport.Present(Battery(pct = 64.0, status = BatteryStatus.Discharging)), verdict = ok,
    )

    val desktopPulse = Pulse(
        schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = "desktop", cpuPct = 7.8, memUsedPct = 38.2, cpuTempC = 44.0,
        filesystems = listOf(Filesystem(majMin = "0:36", mountPoint = "/", usedPct = 63.0)), primaryDisk = "0:36",
        battery = BatteryReport.NoBattery, verdict = ok,
    )

    /** Every value unavailable, and a battery the Agent couldn't classify. */
    val unavailablePulse = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, battery = BatteryReport.Present(Battery()))

    val threeMinutesOld = received(desktopPulse, age = 3.minutes)
}

internal fun panel(machine: Machine, view: MachineView) = Panel(machine) { view }

/** A Machine store in [folder], opened the way the app opens it, until [job] is cancelled. */
internal fun machineStore(folder: TemporaryFolder, job: Job) = PreferenceDataStoreFactory.create(
    corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
    scope = CoroutineScope(Dispatchers.IO + job),
    produceFile = { folder.root.resolve("machines.preferences_pb") },
)

/** A theme setting in [folder], opened the way the app opens it, until [job] is cancelled. */
internal fun themeSetting(folder: TemporaryFolder, job: Job) = ThemeSetting(
    PreferenceDataStoreFactory.create(
        corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
        scope = CoroutineScope(Dispatchers.IO + job),
        produceFile = { folder.root.resolve("settings.preferences_pb") },
    ),
)

/** Saves straight into [store] through a book whose test connection is always Forbidden, so it never asks a test's link. */
internal fun seed(store: DataStore<Preferences>, input: MachineInput): Machine =
    (runBlocking { MachineBook(store, { _, _, _ -> PulseResult.Forbidden }).save(input) } as SaveResult.Saved).machine
