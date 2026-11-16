package io.github.fsrodrig.anshin

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.io.IOException
import java.util.Locale
import java.util.UUID
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.retryWhen
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json

@JvmInline value class MachineId(val value: String)

/** A Machine the Owner saved: their Machine name for it and how the phone reaches its Agent. */
data class Machine(val id: MachineId, val name: String, val address: MachineAddress)

/** What the Owner typed, before MachineBook checks it. A blank port is the Agent's, 7247. */
data class MachineInput(val name: String, val address: String, val port: String = "")

/** What one test connection found. The words are the glossary's. */
sealed interface Connection {
    data object Live : Connection
    data object Forbidden : Connection
    data object Incompatible : Connection

    /** [outsideTailnet] means Anshin itself refused the address; [httpStatus] is an answer that wasn't a Pulse. */
    data class Unreachable(val outsideTailnet: Boolean = false, val httpStatus: Int? = null) : Connection
}

sealed interface NameProblem {
    data object Blank : NameProblem
    data object TooLong : NameProblem
    data object NotOneLine : NameProblem
    data class Taken(val by: Machine) : NameProblem
}

/** Each field's problem on its own, so every message can sit under its field. */
data class Problems(val name: NameProblem? = null, val address: AddressProblem? = null, val port: PortProblem? = null) {
    val any: Boolean get() = name != null || address != null || port != null
}

sealed interface SaveResult {
    /** Saved, then tested. The Machine stays saved whatever [connection] says. */
    data class Saved(val machine: Machine, val connection: Connection) : SaveResult

    /** Nothing was written and no request was made. */
    data class Rejected(val problems: Problems) : SaveResult

    /** [MachineBook.save] was asked to replace a Machine that is no longer in the book. */
    data object NotFound : SaveResult
}

const val MAX_NAME_LENGTH = 40

/**
 * The Owner's Machines, in the order saved, kept on the phone. Saving validates, persists and only then makes one
 * test connection, whose outcome is a report: a Machine that is asleep, refuses this phone or runs an older Agent is
 * still worth watching, and the Watcher already lives with each of those states.
 */
class MachineBook(
    private val store: DataStore<Preferences>,
    private val link: AgentLink,
    private val newId: () -> MachineId = { MachineId(UUID.randomUUID().toString()) },
) {
    /** Every saved Machine, in the order saved. Never emits a guess while the store can't be read. */
    val machines: Flow<List<Machine>> = store.data
        // Unlike the theme, no empty list stands in while the file can't be read: it would show "No Machines yet" and
        // stop every Watcher.
        .retryWhen { cause, attempt ->
            if (cause !is IOException) throw cause
            delay(1_000L * (1L shl attempt.coerceAtMost(5).toInt()))
            true
        }
        .map { decode(it[KEY]) }
        .distinctUntilChanged()

    /** Adds a Machine, or with [replacing] edits that one in place, keeping its id and position, then tests it. */
    suspend fun save(input: MachineInput, replacing: MachineId? = null): SaveResult {
        val name = input.name.trim()
        val nameProblem = nameProblem(name)
        val parsed = parseAddress(input.address, input.port)
        var outcome: SaveResult = SaveResult.NotFound
        var saved: Machine? = null
        // The duplicate checks run against the list as persisted, inside the edit, so two quick saves can't both pass.
        store.edit { prefs ->
            val book = decode(prefs[KEY])
            if (replacing != null && book.none { it.id == replacing }) return@edit
            val others = book.filter { it.id != replacing }
            val problems = Problems(
                name = nameProblem ?: others.firstOrNull { sameName(it.name, name) }?.let(NameProblem::Taken),
                address = parsed.addressProblem
                    ?: parsed.address?.let { address -> others.firstOrNull { it.address == address } }?.let(AddressProblem::Taken),
                port = parsed.portProblem,
            )
            if (problems.any) {
                outcome = SaveResult.Rejected(problems)
                return@edit
            }
            val machine = Machine(replacing ?: newId(), name, checkNotNull(parsed.address))
            val updated = if (replacing == null) book + machine else book.map { if (it.id == replacing) machine else it }
            prefs[KEY] = encode(updated)
            saved = machine
        }
        val machine = saved ?: return outcome
        return SaveResult.Saved(machine, link.pulse(machine.address.pulseUrl, after = null, wait = null).toConnection())
    }

    /** Stops nothing on the Machine and asks it nothing; an unknown id changes nothing. */
    suspend fun remove(id: MachineId) {
        store.edit { prefs ->
            val book = decode(prefs[KEY])
            if (book.any { it.id == id }) prefs[KEY] = encode(book.filter { it.id != id })
        }
    }

    private companion object {
        val KEY = stringPreferencesKey("machines")
        val json = Json { ignoreUnknownKeys = true }

        fun encode(machines: List<Machine>): String = json.encodeToString(
            StoredBook.serializer(),
            StoredBook(machines.map { StoredMachine(it.id.value, it.name, it.address.host, it.address.port) }),
        )

        /**
         * Each entry is checked by the same rules a save uses, and one that fails is dropped rather than repaired, so a
         * hand-edited file can never make the phone watch a literal outside the tailnet.
         */
        fun decode(value: String?): List<Machine> {
            if (value == null) return emptyList()
            val stored = try {
                json.decodeFromString(StoredBook.serializer(), value).machines
            } catch (_: SerializationException) {
                return emptyList()
            } catch (_: IllegalArgumentException) {
                return emptyList()
            }
            val kept = mutableListOf<Machine>()
            for (entry in stored) {
                val name = entry.name.trim()
                val address = parseAddress(entry.host, entry.port.toString()).address ?: continue
                val clash = kept.any { it.id.value == entry.id || sameName(it.name, name) || it.address == address }
                if (entry.id.isBlank() || nameProblem(name) != null || clash) continue
                kept += Machine(MachineId(entry.id), name, address)
            }
            return kept
        }
    }
}

private fun nameProblem(name: String): NameProblem? = when {
    name.isEmpty() -> NameProblem.Blank
    name.codePointCount(0, name.length) > MAX_NAME_LENGTH -> NameProblem.TooLong
    name.any { Character.isISOControl(it) || it == ' ' || it == ' ' } -> NameProblem.NotOneLine
    else -> null
}

// The top line names Machines, so two that differ only in case or spacing would read as one.
private fun sameName(a: String, b: String) = a.trim().lowercase(Locale.ROOT) == b.trim().lowercase(Locale.ROOT)

// A Failed answer is Unreachable with its status, as the Watcher shows it.
private fun PulseResult.toConnection(): Connection = when (this) {
    is PulseResult.Ok -> Connection.Live
    PulseResult.Forbidden -> Connection.Forbidden
    PulseResult.Incompatible -> Connection.Incompatible
    is PulseResult.Unreachable -> Connection.Unreachable(outsideTailnet = outsideTailnet)
    is PulseResult.Failed -> Connection.Unreachable(httpStatus = httpStatus)
}

@Serializable
private data class StoredBook(val machines: List<StoredMachine> = emptyList())

@Serializable
private data class StoredMachine(val id: String, val name: String, val host: String, val port: Int)

/**
 * The Owner's Machines, in `files/datastore/machines.preferences_pb`, apart from `settings` so that replacing the
 * settings file (as QA's theme seed does) never loses them. A corrupt file is replaced with an empty one.
 */
val Context.machineStore: DataStore<Preferences> by preferencesDataStore(
    name = "machines",
    corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
)
