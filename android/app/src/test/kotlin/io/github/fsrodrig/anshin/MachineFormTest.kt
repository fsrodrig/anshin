package io.github.fsrodrig.anshin

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.emptyPreferences
import kotlin.time.Duration
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import okhttp3.HttpUrl
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/** The stateful form against a real temp-file store and a scripted link. */
@RunWith(RobolectricTestRunner::class)
class MachineFormTest {
    @get:Rule val compose = createComposeRule()
    @get:Rule val folder = TemporaryFolder()

    private val job = Job()
    private val store by lazy {
        PreferenceDataStoreFactory.create(
            corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
            scope = CoroutineScope(Dispatchers.IO + job),
            produceFile = { folder.root.resolve("machines.preferences_pb") },
        )
    }

    @After
    fun closeStore() = job.cancel()

    /** Each request waits for the test to answer it. */
    private class Link : AgentLink {
        val asked = mutableListOf<Pair<HttpUrl, CompletableDeferred<PulseResult>>>()
        var cancelled = 0

        override suspend fun pulse(endpoint: HttpUrl, after: String?, wait: Duration?): PulseResult {
            val answer = CompletableDeferred<PulseResult>()
            synchronized(asked) { asked += endpoint to answer }
            try {
                return answer.await()
            } catch (e: kotlin.coroutines.cancellation.CancellationException) {
                synchronized(asked) { cancelled++ }
                throw e
            }
        }

        fun count() = synchronized(asked) { asked.size }

        fun answer(result: PulseResult) = synchronized(asked) { asked.last().second.complete(result) }
    }

    private val link = Link()
    private val book by lazy { MachineBook(store, link) }
    private var shown by mutableStateOf(true)
    private var done: MachineId? = null
    private var removed = false

    private fun show(machine: Machine? = null) = compose.setContent {
        AnshinTheme(Theme.Default) {
            if (shown) MachineFormScreen(book, machine, onDone = { done = it }, onRemoved = { removed = true })
        }
    }

    private fun type(name: String, address: String, port: String = "") {
        compose.onNodeWithTag(NAME_FIELD_TAG).performTextReplacement(name)
        compose.onNodeWithTag(ADDRESS_FIELD_TAG).performTextReplacement(address)
        compose.onNodeWithTag(PORT_FIELD_TAG).performTextReplacement(port)
    }

    private fun click(text: String) {
        compose.onNodeWithText(text).performScrollTo().performClick()
    }

    private fun listed() = runBlocking { book.machines.first() }

    private fun saveAndAnswer(result: PulseResult) {
        val before = link.count()
        click("Save")
        compose.waitUntil(5_000) { link.count() == before + 1 }
        compose.onNodeWithText("Saved. Asking 100.64.0.2 for a Pulse…").assertExists()
        link.answer(result)
    }

    @Test
    fun eachTestConnectionOutcomeIsReportedInPlainWordsAndTheMachineStaysSaved() {
        val outcomes = listOf(
            PulseResult.Ok(checkNotNull(decodePulse("""{"schema":1}"""))) to listOf("Live", "100.64.0.2 answered with a Pulse. desktop is on your dashboard."),
            PulseResult.Forbidden to listOf("Forbidden", "Saved, but this Agent only answers its Owner's untagged devices. Anshin asks again about once a minute."),
            PulseResult.Incompatible to listOf("Incompatible", "Saved, but the Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent."),
            PulseResult.Unreachable(outsideTailnet = false) to listOf("Unreachable", "Saved, but no Pulse came from 100.64.0.2. The Machine may be off, its Agent stopped, or this phone off the tailnet. Anshin keeps trying."),
            PulseResult.Unreachable(outsideTailnet = true) to listOf("Unreachable", "Saved, but 100.64.0.2 isn't a tailnet address, so Anshin won't connect to it."),
            PulseResult.Failed(503) to listOf("Unreachable", "Saved, but the Agent on 100.64.0.2 can't ask tailscaled who this phone is."),
            PulseResult.Failed(500) to listOf("Unreachable", "Saved, but 100.64.0.2 answered HTTP 500 instead of a Pulse."),
        )
        show()
        type("desktop", "100.64.0.2")
        for ((result, words) in outcomes) {
            saveAndAnswer(result)
            for (text in words) compose.onNodeWithText(text).assertExists()
            compose.onNodeWithText("Done").assertExists()
            assertEquals("a second Save edits the same Machine", listOf("desktop"), listed().map { it.name })
        }
    }

    @Test
    fun aRejectionShowsUnderItsFieldAndAsksNothing() {
        show()
        type(" ", "192.0.2.1", "0")
        click("Save")
        compose.onNodeWithText("Give this Machine a name.").assertExists()
        compose.onNodeWithText("192.0.2.1 isn't a tailnet address. Tailnet IPs are in 100.64.0.0/10 or fd7a:115c:a1e0::/48.").assertExists()
        compose.onNodeWithText("Enter a port from 1 to 65535.").assertExists()
        compose.onNodeWithText("Cancel").assertExists()
        assertEquals(0, link.count())
        assertEquals(emptyList<Machine>(), listed())
    }

    @Test
    fun aCorrectionAfterSavingEditsTheSameMachine() {
        show()
        type("desktop", "100.64.0.2")
        saveAndAnswer(PulseResult.Unreachable(outsideTailnet = false))
        val first = listed().single()
        compose.onNodeWithTag(PORT_FIELD_TAG).performTextInput("9000")
        compose.onNodeWithText("Unreachable").assertDoesNotExist()
        click("Save")
        compose.waitUntil(5_000) { link.count() == 2 }
        link.answer(PulseResult.Forbidden)
        compose.waitUntil(5_000) { listed().singleOrNull()?.address?.port == 9000 }
        assertEquals(first.id, listed().single().id)
        click("Done")
        assertEquals(first.id, done)
    }

    @Test
    fun removeAsksFirstAndKeepChangesNothing() {
        val machine = (runBlocking { MachineBook(store, { _, _, _ -> PulseResult.Forbidden }).save(MachineInput("laptop", "laptop")) } as SaveResult.Saved).machine
        show(machine)
        compose.onNodeWithText("Edit machine").assertExists()
        click("Remove machine")
        compose.onNodeWithText("Remove laptop?").assertExists()
        click("Keep")
        compose.onNodeWithText("Remove laptop?").assertDoesNotExist()
        assertEquals(listOf(machine), listed())

        click("Remove machine")
        click("Remove")
        compose.waitUntil(5_000) { removed }
        assertEquals(emptyList<Machine>(), listed())
        assertEquals(0, link.count())
    }

    @Test
    fun leavingDuringTheTestConnectionCancelsItAndKeepsTheMachine() {
        show()
        type("desktop", "100.64.0.2")
        click("Save")
        compose.waitUntil(5_000) { link.count() == 1 }
        compose.runOnIdle { shown = false }
        compose.waitUntil(5_000) { synchronized(link.asked) { link.cancelled } == 1 }
        assertEquals(listOf("desktop"), listed().map { it.name })
        assertNull(done)
        assertTrue(!removed)
    }
    @Test
    fun anEarlierAnswerCannotReplaceTheLatestSave() {
        show()
        type("desktop", "100.64.0.2")
        click("Save")
        compose.waitUntil(5_000) { link.count() == 1 }
        val first = listed().single()
        compose.onNodeWithTag(ADDRESS_FIELD_TAG).performTextReplacement("100.64.0.3")
        click("Save")
        compose.waitUntil(5_000) { link.count() == 2 }
        link.answer(PulseResult.Ok(checkNotNull(decodePulse("""{"schema":1}"""))))
        compose.onNodeWithText("Live").assertExists()
        synchronized(link.asked) { link.asked.first().second.complete(PulseResult.Forbidden) }
        compose.waitForIdle()
        compose.onNodeWithText("Live").assertExists()
        compose.onNodeWithText("Forbidden").assertDoesNotExist()
        assertEquals(first.id, listed().single().id)
        assertEquals("100.64.0.3", listed().single().address.host)
        assertEquals(1, synchronized(link.asked) { link.cancelled })
    }

    @Test
    fun hidingTheAppCancelsTheTestAndTheNextSaveEditsTheSameMachine() {
        val owner = object : androidx.lifecycle.LifecycleOwner {
            val registry = androidx.lifecycle.LifecycleRegistry.createUnsafe(this)
            override val lifecycle: androidx.lifecycle.Lifecycle get() = registry
        }
        owner.registry.currentState = androidx.lifecycle.Lifecycle.State.RESUMED
        compose.setContent {
            androidx.compose.runtime.CompositionLocalProvider(androidx.lifecycle.compose.LocalLifecycleOwner provides owner) {
                AnshinTheme(Theme.Default) {
                    MachineFormScreen(book, null, onDone = { done = it }, onRemoved = {})
                }
            }
        }
        type("desktop", "100.64.0.2")
        click("Save")
        compose.waitUntil(5_000) { link.count() == 1 }
        val first = listed().single()
        compose.runOnIdle { owner.registry.currentState = androidx.lifecycle.Lifecycle.State.CREATED }
        compose.waitForIdle()
        assertEquals(1, synchronized(link.asked) { link.cancelled })
        link.answer(PulseResult.Forbidden)
        compose.onNodeWithText("Forbidden").assertDoesNotExist()
        compose.runOnIdle { owner.registry.currentState = androidx.lifecycle.Lifecycle.State.RESUMED }
        click("Save")
        compose.waitUntil(5_000) { link.count() == 2 }
        link.answer(PulseResult.Forbidden)
        compose.onNodeWithText("Forbidden").assertExists()
        assertEquals(first.id, listed().single().id)
    }

    @Test
    fun removalStorageFailureKeepsTheMachineAndAllowsRetry() {
        val machine = (runBlocking { MachineBook(store, { _, _, _ -> PulseResult.Forbidden }).save(MachineInput("laptop", "laptop")) } as SaveResult.Saved).machine
        var fail = true
        val retryStore = object : androidx.datastore.core.DataStore<androidx.datastore.preferences.core.Preferences> {
            override val data = store.data
            override suspend fun updateData(
                transform: suspend (androidx.datastore.preferences.core.Preferences) -> androidx.datastore.preferences.core.Preferences,
            ): androidx.datastore.preferences.core.Preferences {
                if (fail) throw java.io.IOException("storage unavailable")
                return store.updateData(transform)
            }
        }
        compose.setContent {
            AnshinTheme(Theme.Default) {
                MachineFormScreen(MachineBook(retryStore, link), machine, onDone = {}, onRemoved = { removed = true })
            }
        }
        click("Remove machine")
        click("Remove")
        compose.onNodeWithText("Anshin couldn't remove this Machine. Try again.").assertExists()
        assertTrue(!removed)
        assertEquals(listOf(machine), listed())
        compose.runOnIdle { fail = false }
        click("Remove")
        compose.waitUntil(5_000) { removed }
        assertEquals(emptyList<Machine>(), listed())
    }

}
