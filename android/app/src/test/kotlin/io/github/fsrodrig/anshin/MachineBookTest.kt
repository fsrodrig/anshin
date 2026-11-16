package io.github.fsrodrig.anshin

import androidx.datastore.core.DataStore
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import java.io.IOException
import kotlin.time.Duration
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import okhttp3.HttpUrl
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** Against a real Preferences DataStore in a temporary file, opened the way the app opens it. A restart is a new store on the same file. */
class MachineBookTest {
    @get:Rule val folder = TemporaryFolder()

    private val file by lazy { folder.root.resolve("machines.preferences_pb") }

    private class Asked(val endpoint: HttpUrl, val after: String?, val wait: Duration?)

    /** Answers [result] and records every request; [during] runs inside the request, before it answers. */
    private class RecordingLink(
        private val result: PulseResult = PulseResult.Forbidden,
        private val during: suspend () -> Unit = {},
    ) : AgentLink {
        val asked = mutableListOf<Asked>()

        override suspend fun pulse(endpoint: HttpUrl, after: String?, wait: Duration?): PulseResult {
            asked += Asked(endpoint, after, wait)
            during()
            return result
        }
    }

    private fun <T> withBook(link: AgentLink = RecordingLink(), ids: () -> MachineId = sequentialIds(), block: suspend (MachineBook) -> T): T =
        runBlocking {
            val job = Job()
            val store = PreferenceDataStoreFactory.create(
                corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
                scope = CoroutineScope(Dispatchers.IO + job),
                produceFile = { file },
            )
            try {
                block(MachineBook(store, link, ids))
            } finally {
                job.cancelAndJoin()
            }
        }

    private var nextId = 0

    private fun sequentialIds(): () -> MachineId = { MachineId("id-${++nextId}") }

    private fun listed(): List<Machine> = withBook { it.machines.first() }

    private fun saved(result: SaveResult): Machine = (result as SaveResult.Saved).machine

    private fun rejected(result: SaveResult): Problems = (result as SaveResult.Rejected).problems

    private val laptop = MachineInput("laptop", "laptop")
    private val desktop = MachineInput("desktop", "100.64.0.2", "9000")

    @Test
    fun anEmptyBookListsNoMachines() {
        assertEquals(emptyList<Machine>(), listed())
    }

    @Test
    fun theOwnersMachinesSurviveARestartThenAnEditThenARemoval() {
        val (first, second) = withBook { book -> saved(book.save(laptop)) to saved(book.save(desktop)) }

        assertEquals(
            listOf(
                Machine(first.id, "laptop", MachineAddress("laptop", AGENT_PORT)),
                Machine(second.id, "desktop", MachineAddress("100.64.0.2", 9000)),
            ),
            listed(),
        )

        withBook { book ->
            saved(book.save(MachineInput(" work laptop ", "Laptop.example.ts.net.", "9001"), replacing = first.id))
        }
        assertEquals(
            listOf(
                Machine(first.id, "work laptop", MachineAddress("laptop.example.ts.net", 9001)),
                Machine(second.id, "desktop", MachineAddress("100.64.0.2", 9000)),
            ),
            listed(),
        )

        withBook { it.remove(second.id) }
        assertEquals(listOf(Machine(first.id, "work laptop", MachineAddress("laptop.example.ts.net", 9001))), listed())
    }

    @Test
    fun aMachineNeverConflictsWithItself() {
        val machine = withBook { saved(it.save(laptop)) }
        withBook { book ->
            assertEquals("Laptop", saved(book.save(MachineInput("Laptop", "laptop"), replacing = machine.id)).name)
            assertEquals(9000, saved(book.save(MachineInput("Laptop", "laptop", "9000"), replacing = machine.id)).address.port)
            saved(book.save(MachineInput("Laptop", "laptop", "9000"), replacing = machine.id))
        }
        assertEquals(listOf(Machine(machine.id, "Laptop", MachineAddress("laptop", 9000))), listed())
    }

    /** Asserts the exact problems, a byte-identical file and no request. */
    private fun assertRejected(input: MachineInput, expected: Problems, replacing: MachineId? = null) {
        val before = file.readBytes()
        val link = RecordingLink()
        val result = withBook(link) { it.save(input, replacing) }
        assertEquals(input.toString(), SaveResult.Rejected(expected), result)
        assertArrayEquals(input.toString(), before, file.readBytes())
        assertEquals(input.toString(), 0, link.asked.size)
    }

    @Test
    fun eachRejectionWritesNothingAndAsksNothing() {
        val machine = withBook { saved(it.save(laptop)) }

        for (name in listOf("", "   ")) assertRejected(MachineInput(name, "desktop"), Problems(name = NameProblem.Blank))
        assertRejected(MachineInput("a".repeat(41), "desktop"), Problems(name = NameProblem.TooLong))
        assertRejected(MachineInput("lap\ntop", "desktop"), Problems(name = NameProblem.NotOneLine))
        assertRejected(MachineInput("Laptop ", "desktop"), Problems(name = NameProblem.Taken(machine)))

        assertRejected(MachineInput("desktop", " "), Problems(address = AddressProblem.Blank))
        for (address in listOf("http://desktop", "lap_top", "100.064.0.1", "[::1")) {
            assertRejected(MachineInput("desktop", address), Problems(address = AddressProblem.Malformed))
        }
        assertRejected(MachineInput("desktop", "desktop:7247"), Problems(address = AddressProblem.HasPort))
        for ((address, literal) in listOf("192.0.2.1" to "192.0.2.1", "::1" to "::1", "fd7a:115c:a1e1::1" to "fd7a:115c:a1e1::1")) {
            assertRejected(MachineInput("desktop", address), Problems(address = AddressProblem.OutsideTailnet(literal)))
        }
        for (address in listOf("laptop", "LAPTOP", "laptop.")) {
            assertRejected(MachineInput("desktop", address), Problems(address = AddressProblem.Taken(machine)))
        }
        assertRejected(MachineInput("desktop", "laptop", "7247"), Problems(address = AddressProblem.Taken(machine)))

        for (port in listOf("0", "65536")) assertRejected(MachineInput("desktop", "desktop", port), Problems(port = PortProblem))

        assertRejected(
            MachineInput(" ", "192.0.2.1", "0"),
            Problems(NameProblem.Blank, AddressProblem.OutsideTailnet("192.0.2.1"), PortProblem),
        )
    }

    @Test
    fun anIpv6LiteralIsTakenWhateverItsSpelling() {
        val machine = withBook { saved(it.save(MachineInput("laptop", "fd7a:115c:a1e0::0001"))) }
        assertRejected(MachineInput("desktop", "[FD7A:115C:A1E0::1]"), Problems(address = AddressProblem.Taken(machine)))
    }

    @Test
    fun theSameHostOnAnotherPortIsAnotherAgent() {
        withBook { book ->
            saved(book.save(laptop))
            saved(book.save(MachineInput("laptop test agent", "laptop", "9000")))
        }
        assertEquals(listOf(7247, 9000), listed().map { it.address.port })
    }

    @Test
    fun editingToAnotherMachinesNameOrAddressIsTaken() {
        val (first, second) = withBook { book -> saved(book.save(laptop)) to saved(book.save(desktop)) }
        assertRejected(MachineInput("DESKTOP", "laptop"), Problems(name = NameProblem.Taken(second)), replacing = first.id)
        assertRejected(MachineInput("laptop", "100.64.0.2", "9000"), Problems(address = AddressProblem.Taken(second)), replacing = first.id)
    }

    @Test
    fun replacingAMachineThatWasRemovedIsNotFound() {
        withBook { saved(it.save(laptop)) }
        val before = file.readBytes()
        val link = RecordingLink()
        assertEquals(SaveResult.NotFound, withBook(link) { it.save(desktop, replacing = MachineId("gone")) })
        assertArrayEquals(before, file.readBytes())
        assertEquals(0, link.asked.size)
    }

    @Test
    fun removingAnUnknownMachineChangesNothing() {
        val machine = withBook { saved(it.save(laptop)) }
        val before = file.readBytes()
        withBook { it.remove(MachineId("gone")) }
        assertArrayEquals(before, file.readBytes())
        assertEquals(listOf(machine), listed())
    }

    @Test
    fun aCorruptFileListsNoMachines() {
        file.writeBytes(byteArrayOf(0x7f, 0x00, 0x13, 0x37))
        assertEquals(emptyList<Machine>(), listed())
    }

    private fun writeRaw(value: String) = runBlocking {
        val job = Job()
        val store = PreferenceDataStoreFactory.create(scope = CoroutineScope(Dispatchers.IO + job), produceFile = { file })
        store.edit { it[stringPreferencesKey("machines")] = value }
        job.cancelAndJoin()
    }

    @Test
    fun anUnreadableStoredListListsNoMachines() {
        writeRaw("not json")
        assertEquals(emptyList<Machine>(), listed())
        writeRaw("""{"machines":[{"id":"id-1","name":"laptop"}]}""")
        assertEquals(emptyList<Machine>(), listed())
    }

    @Test
    fun aStoredMachineOutsideTheRulesIsNeverListed() {
        writeRaw(
            """{"machines":[
                {"id":"id-1","name":"outside","host":"192.0.2.1","port":7247},
                {"id":"id-2","name":"laptop","host":"laptop","port":7247,"added_later":true},
                {"id":"id-3","name":"LAPTOP","host":"desktop","port":7247},
                {"id":"id-4","name":"desktop","host":"laptop","port":7247},
                {"id":"id-5","name":"","host":"other","port":7247},
                {"id":"id-6","name":"other","host":"other","port":0}
            ]}""",
        )
        assertEquals(listOf(Machine(MachineId("id-2"), "laptop", MachineAddress("laptop", AGENT_PORT))), listed())
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun aTransientReadFailureKeepsTheListWithoutAnEmptyFlash() = runTest {
        val machines = """{"machines":[{"id":"id-1","name":"laptop","host":"laptop","port":7247}]}"""
        val preferences = MutableStateFlow(preferencesOf(stringPreferencesKey("machines") to machines))
        var reads = 0
        val store = object : DataStore<Preferences> {
            override val data: Flow<Preferences> = flow {
                if (++reads <= 2) throw IOException("temporary read failure")
                emitAll(preferences)
            }

            override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences =
                transform(preferences.value).also { preferences.value = it }
        }
        val book = MachineBook(store, RecordingLink())
        val seen = mutableListOf<List<Machine>>()
        backgroundScope.launch { book.machines.collect { seen += it } }
        runCurrent()
        assertEquals("nothing is shown while the store can't be read", emptyList<List<Machine>>(), seen)
        advanceTimeBy(3_100)
        runCurrent()
        assertEquals(listOf(listOf(Machine(MachineId("id-1"), "laptop", MachineAddress("laptop", AGENT_PORT)))), seen)
        assertEquals(3, reads)
    }

    @Test
    fun theTestConnectionReportsWhatTheAgentAnswered() {
        val outcomes = listOf(
            PulseResult.Ok(checkNotNull(decodePulse("""{"schema":1}"""))) to TestConnection.Live,
            PulseResult.Forbidden to TestConnection.Forbidden,
            PulseResult.Incompatible to TestConnection.Incompatible,
            PulseResult.Unreachable(outsideTailnet = false) to TestConnection.Unreachable(),
            PulseResult.Unreachable(outsideTailnet = true) to TestConnection.Unreachable(outsideTailnet = true),
            PulseResult.Failed(503) to TestConnection.Unreachable(httpStatus = 503),
            PulseResult.Failed(500) to TestConnection.Unreachable(httpStatus = 500),
        )
        for ((answer, connection) in outcomes) {
            file.delete()
            lateinit var book: MachineBook
            var listedDuringRequest: List<Machine>? = null
            val link = RecordingLink(answer) { listedDuringRequest = book.machines.first() }
            val result = withBook(link) {
                book = it
                it.save(desktop)
            } as SaveResult.Saved
            assertEquals(answer.toString(), connection, result.connection)
            assertEquals(1, link.asked.size)
            val asked = link.asked.single()
            assertEquals("http://100.64.0.2:9000/v1/pulse", asked.endpoint.toString())
            assertNull(asked.after)
            assertNull(asked.wait)
            assertEquals("persisted before the request", listOf(result.machine), listedDuringRequest)
        }
    }

    @Test
    fun leavingDuringTheTestConnectionCancelsItAndKeepsTheMachine() {
        val asked = CompletableDeferred<Unit>()
        var cancelled = false
        val link = AgentLink { _, _, _ ->
            asked.complete(Unit)
            try {
                awaitCancellation()
            } finally {
                cancelled = true
            }
        }
        withBook(link) { book ->
            val save = CoroutineScope(Dispatchers.Default).async { book.save(laptop) }
            asked.await()
            save.cancelAndJoin()
        }
        assertTrue(cancelled)
        assertEquals(listOf("laptop"), listed().map { it.name })
    }
}
