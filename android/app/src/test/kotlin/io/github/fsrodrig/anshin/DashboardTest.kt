package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.LifecycleRegistry
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.test.espresso.Espresso
import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes
import kotlin.time.TestTimeSource
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import okhttp3.HttpUrl
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * The app's wiring: every saved Machine is watched on its own, so one that never answers holds back no other, and an
 * edit or a removal touches only its Machine. Hidden, the app asks nothing.
 */
@RunWith(RobolectricTestRunner::class)
class DashboardTest {
    @get:Rule val compose = createComposeRule()
    @get:Rule val folder = TemporaryFolder()

    private val job = Job()
    private val store by lazy { machineStore(folder, job) }

    @After
    fun closeStore() = job.cancel()

    private class Owner : LifecycleOwner {
        val registry = LifecycleRegistry.createUnsafe(this)
        override val lifecycle: Lifecycle get() = registry
    }

    private class Asked(val host: String, val after: String?, val wait: Duration?) {
        val answer = CompletableDeferred<PulseResult>()
        @Volatile var cancelled = false
    }

    /** Each request waits until the test answers it. */
    private class Link : AgentLink {
        private val asked = mutableListOf<Asked>()

        override suspend fun pulse(endpoint: HttpUrl, after: String?, wait: Duration?): PulseResult {
            val call = Asked(endpoint.host, after, wait).also { synchronized(asked) { asked += it } }
            try {
                return call.answer.await()
            } catch (e: kotlin.coroutines.cancellation.CancellationException) {
                call.cancelled = true
                throw e
            }
        }

        fun to(host: String) = synchronized(asked) { asked.filter { it.host == host } }

        fun count() = synchronized(asked) { asked.size }

        fun all() = synchronized(asked) { asked.toList() }
    }

    private val link = Link()
    private val clock = TestTimeSource()
    private val owner = Owner().apply { registry.currentState = Lifecycle.State.RESUMED }

    private fun pulse(kind: String, cursor: String, times: List<Long>, cpu: Int) = PulseResult.Ok(checkNotNull(decodePulse(
        """{"schema":1,"kind":"$kind","cursor":"$cursor","cpu_pct":$cpu,
            "window":{"span_s":1800,"boottime_ms":$times,"cpu_pct":${times.map { cpu }}}}""",
    )))

    private fun show() = compose.setContent {
        CompositionLocalProvider(LocalLifecycleOwner provides owner) {
            AnshinTheme(Theme.Default) { AnshinApp(MachineBook(store, link), Watcher(link, clock)) }
        }
    }

    private fun waitFor(text: String) =
        compose.waitUntil(5_000) { compose.onAllNodesWithText(text, useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty() }

    private fun waitForCalls(host: String, count: Int) = compose.waitUntil(5_000) { link.to(host).size == count }

    private fun panel(name: String): SemanticsNodeInteraction = compose.onNode(hasClickAction() and hasText(name))

    private val laptop = MachineInput("laptop", "laptop")
    private val desktop = MachineInput("desktop", "100.64.0.2")

    /** Both Machines saved; desktop has answered a full Pulse and holds its long-poll, laptop holds its first request. */
    private fun bothWatched(): Pair<Machine, Machine> {
        val machines = seed(store, laptop) to seed(store, desktop)
        show()
        waitForCalls("laptop", 1)
        waitForCalls("100.64.0.2", 1)
        link.to("100.64.0.2")[0].answer.complete(pulse("full", "00000000000000ff.2", listOf(1_000, 3_000), 21))
        waitForCalls("100.64.0.2", 2)
        waitFor("21%")
        return machines
    }

    @Test
    fun anEmptyBookShowsNoMachinesYetAndAsksNothing() {
        show()
        waitFor("No Machines yet")
        compose.onNodeWithText("Add machine").assertExists()
        compose.onNodeWithText("No current Verdict", substring = true).assertDoesNotExist()
        assertEquals(0, link.count())
    }

    @Test
    fun aMachineThatNeverAnswersDoesNotHoldBackAnother() {
        bothWatched()
        val held = link.to("100.64.0.2")[1]
        assertEquals("00000000000000ff.2", held.after)
        held.answer.complete(pulse("delta", "00000000000000ff.3", listOf(5_000), 35))
        waitForCalls("100.64.0.2", 3)
        waitFor("35%")
        compose.onNodeWithText("Live", useUnmergedTree = true).assertExists()
        compose.onNodeWithText("Connecting", useUnmergedTree = true).assertExists()
        val waiting = link.to("laptop").single()
        assertFalse("laptop's request is still waiting, not retried", waiting.cancelled)
    }

    @Test
    fun editingAnAddressRestartsOnlyThatMachine() {
        val (first, _) = bothWatched()
        val laptopCall = link.to("laptop").single()
        val desktopCall = link.to("100.64.0.2")[1]
        runBlocking { MachineBook(store, { _, _, _ -> PulseResult.Forbidden }).save(MachineInput("laptop", "laptop.example.ts.net"), first.id) }
        compose.waitUntil(5_000) { laptopCall.cancelled }
        waitForCalls("laptop.example.ts.net", 1)
        assertNull(link.to("laptop.example.ts.net").single().after)
        assertFalse(desktopCall.cancelled)
        assertEquals(2, link.to("100.64.0.2").size)
    }

    @Test
    fun renamingKeepsTheConnection() {
        val (_, second) = bothWatched()
        val held = link.to("100.64.0.2")[1]
        runBlocking { MachineBook(store, { _, _, _ -> PulseResult.Forbidden }).save(MachineInput("office", "100.64.0.2"), second.id) }
        waitFor("office")
        held.answer.complete(pulse("delta", "00000000000000ff.3", listOf(5_000), 35))
        waitForCalls("100.64.0.2", 3)
        assertFalse(held.cancelled)
        assertEquals("00000000000000ff.3", link.to("100.64.0.2")[2].after)
        assertFalse(link.to("laptop").single().cancelled)
    }

    @Test
    fun removingAMachineStopsOnlyItsRequests() {
        val (first, _) = bothWatched()
        runBlocking { MachineBook(store, link).remove(first.id) }
        compose.waitUntil(5_000) { link.to("laptop").single().cancelled }
        compose.onNodeWithText("laptop").assertDoesNotExist()
        assertFalse(link.to("100.64.0.2")[1].cancelled)
    }

    @Test
    fun hiddenTheAppAsksNothingAndBackEveryMachineResumesWithItsOwnCursor() {
        bothWatched()
        link.to("laptop")[0].answer.complete(pulse("full", "00000000000000aa.4", listOf(1_000, 3_000), 12))
        waitForCalls("laptop", 2)
        val held = listOf(link.to("laptop")[1], link.to("100.64.0.2")[1])

        compose.runOnIdle { owner.registry.currentState = Lifecycle.State.CREATED }
        compose.waitUntil(5_000) { held.all { it.cancelled } }
        clock += 1.minutes
        compose.mainClock.advanceTimeBy(60_000)
        compose.waitForIdle()
        assertEquals("nothing is asked while hidden", 4, link.count())

        compose.runOnIdle { owner.registry.currentState = Lifecycle.State.RESUMED }
        waitForCalls("laptop", 3)
        waitForCalls("100.64.0.2", 3)
        assertEquals("00000000000000aa.4", link.to("laptop")[2].after)
        assertEquals("00000000000000ff.2", link.to("100.64.0.2")[2].after)
        waitFor("Last Sample 1 min ago. Not current.")
        compose.onAllNodesWithText("Stale", useUnmergedTree = true).fetchSemanticsNodes().let { assertEquals(2, it.size) }
        compose.onNodeWithText("Live", useUnmergedTree = true).assertDoesNotExist()
    }

    @Test
    fun openingAMachineKeepsItsCollectorAndBackReturns() {
        bothWatched()
        val before = link.count()
        panel("desktop").performClick()
        compose.onNodeWithText("All Machines").assertExists()
        compose.onNodeWithText("Edit").assertExists()
        compose.onNodeWithText("100.64.0.2").assertExists()
        compose.waitForIdle()
        assertEquals("opening a Machine asks nothing new", before, link.count())
        assertTrue(link.to("100.64.0.2").none { it.cancelled })

        Espresso.pressBack()
        waitFor("Add machine")
        compose.onNodeWithText("All Machines").assertDoesNotExist()
        assertEquals(before, link.count())
    }

    @Test
    fun theDetailOpensFromTheMachineScreenAndBackReturnsOneLevelAtATimeAskingNothing() {
        bothWatched()
        val before = link.count()
        fun assertNothingAsked(step: String) {
            compose.waitForIdle()
            assertEquals("$step asks nothing new", before, link.count())
            assertTrue("$step cancels no long-poll", link.all().none { it.cancelled })
        }
        fun assertOnMachineScreen() {
            compose.onNodeWithText("All Machines").assertExists()
            compose.onNodeWithText("Edit").assertExists()
            compose.onNodeWithText("Back to Machine").assertDoesNotExist()
        }

        panel("desktop").performClick()
        assertOnMachineScreen()
        assertNothingAsked("opening the Machine screen")

        compose.onNodeWithTag("machine-card").performClick()
        compose.onNodeWithText("Back to Machine").assertExists()
        compose.onNodeWithTag("machine-detail").assertExists()
        compose.onNodeWithText("All Machines").assertDoesNotExist()
        compose.onNodeWithText("Edit").assertDoesNotExist()
        assertNothingAsked("opening the detail")

        Espresso.pressBack()
        assertOnMachineScreen()
        assertNothingAsked("back from the detail")

        compose.onNodeWithTag("machine-card").performClick()
        compose.onNodeWithText("Back to Machine").performClick()
        assertOnMachineScreen()
        assertNothingAsked("Back to Machine")

        Espresso.pressBack()
        waitFor("Add machine")
        compose.onNodeWithText("All Machines").assertDoesNotExist()
        compose.onNodeWithText("Back to Machine").assertDoesNotExist()
        assertNothingAsked("back from the Machine screen")
    }
}
