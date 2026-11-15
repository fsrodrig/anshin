package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.LifecycleRegistry
import androidx.lifecycle.compose.LocalLifecycleOwner
import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes
import kotlin.time.TestTimeSource
import kotlinx.coroutines.CompletableDeferred
import okhttp3.HttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/** The screen's lifecycle wiring: hidden, it asks nothing; back, it resumes with its Cursor and shows its data's age. */
@RunWith(RobolectricTestRunner::class)
class PulseScreenLifecycleTest {
    @get:Rule val compose = createComposeRule()

    private class Owner : LifecycleOwner {
        val registry = LifecycleRegistry.createUnsafe(this)
        override val lifecycle: Lifecycle get() = registry
    }

    private class Asked(val after: String?, val wait: Duration?) {
        val answer = CompletableDeferred<PulseResult>()
        var cancelled = false
    }

    private class Link : AgentLink {
        val asked = mutableListOf<Asked>()

        override suspend fun pulse(endpoint: HttpUrl, after: String?, wait: Duration?): PulseResult {
            val call = Asked(after, wait).also { synchronized(asked) { asked += it } }
            try {
                return call.answer.await()
            } catch (e: kotlin.coroutines.cancellation.CancellationException) {
                call.cancelled = true
                throw e
            }
        }

        fun count() = synchronized(asked) { asked.size }

        fun last() = synchronized(asked) { asked.last() }
    }

    private fun pulse(kind: String, cursor: String, times: List<Long>) = PulseResult.Ok(checkNotNull(decodePulse(
        """{"schema":1,"kind":"$kind","cursor":"$cursor","hostname":"machine","cpu_pct":21,
            "window":{"span_s":1800,"boottime_ms":$times,"cpu_pct":${times.map { 21 }}}}""",
    )))

    @Test
    fun hiddenTheScreenAsksNothingAndBackItResumesWithItsCursorShowingStaleFirst() {
        val owner = Owner().apply { registry.currentState = Lifecycle.State.RESUMED }
        val clock = TestTimeSource()
        val link = Link()
        compose.setContent {
            CompositionLocalProvider(LocalLifecycleOwner provides owner) {
                AnshinTheme(Theme.Default) { PulseScreen(Watcher(link, clock)) }
            }
        }
        compose.onNode(hasSetTextAction()).performTextInput("machine")
        compose.onNodeWithText("Watch").performClick()
        compose.waitUntil(5_000) { link.count() == 1 }
        assertNull(link.last().after)
        link.last().answer.complete(pulse("full", "00000000000000ff.2", listOf(1_000, 3_000)))
        compose.waitUntil(5_000) { link.count() == 2 }
        compose.onNodeWithText("Live").assertExists()
        val held = link.last()
        assertEquals("00000000000000ff.2", held.after)

        compose.runOnIdle { owner.registry.currentState = Lifecycle.State.CREATED }
        compose.waitUntil(5_000) { held.cancelled }
        clock += 1.minutes
        compose.mainClock.advanceTimeBy(60_000)
        compose.waitForIdle()
        assertEquals("nothing is asked while hidden", 2, link.count())

        compose.runOnIdle { owner.registry.currentState = Lifecycle.State.RESUMED }
        compose.waitUntil(5_000) { link.count() == 3 }
        val resumed = link.last()
        assertEquals("the first request back carries the held Cursor", "00000000000000ff.2", resumed.after)
        compose.onNodeWithText("Stale").assertExists()
        compose.onNodeWithText("Last Sample 1 min ago. Nothing here is current.").assertExists()
        compose.onNodeWithText("Live").assertDoesNotExist()

        resumed.answer.complete(pulse("delta", "00000000000000ff.3", listOf(63_000)))
        compose.waitUntil(5_000) { link.count() == 4 }
        compose.onNodeWithText("Live").assertExists()
        compose.onNodeWithText("Stale").assertDoesNotExist()
        assertTrue(!link.last().cancelled)
    }
}
