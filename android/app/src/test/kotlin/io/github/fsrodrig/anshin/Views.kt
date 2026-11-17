package io.github.fsrodrig.anshin

import androidx.compose.runtime.Composable
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import kotlin.time.ComparableTimeMark
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TestTimeSource

internal object Ages {
    val AGE = 42.seconds
    private val clock = TestTimeSource()
    val at = clock.markNow()
    val now = at + AGE

    fun ago(age: Duration) = now - age
}

internal fun live(pulse: Pulse, window: HeldWindow? = null) = MachineView.Live(Received(pulse, window, Ages.now))

internal fun received(pulse: Pulse, window: HeldWindow? = null, age: Duration = Ages.AGE) = Received(pulse, window, Ages.ago(age))

internal fun stale(pulse: Pulse, window: HeldWindow? = null, age: Duration = Ages.AGE) = MachineView.Stale(received(pulse, window, age))

internal fun unreachable(last: Received? = null, retryIn: Duration = 4.seconds, vpnHint: Boolean = false, outsideTailnet: Boolean = false, httpStatus: Int? = null) =
    MachineView.Unreachable(last, Ages.now + retryIn, vpnHint, outsideTailnet, httpStatus)

/** The Machine screen as the app draws it at [Ages.now], for a Machine whose Machine name and address are both [name]. */
@Composable
internal fun MachineScreen(view: MachineView, name: String = "machine", now: () -> ComparableTimeMark = { Ages.now }) =
    MachineScreen(name, name, view, now, Back("Dashboard") {}, onEdit = {})

/** The Dashboard as the app draws it at [Ages.now], with its top line read from [panels]. */
@Composable
internal fun Dashboard(panels: List<Panel>) =
    Dashboard(panels, { Ages.now }, onOpen = {}, onAdd = {}, onSettings = {}, topLine = { topLine(panels.map { WatchedMachine(it.machine.name, it.view()) }) })

/** Text inside the Machine screen's item [key], the test tag each of its items carries. */
internal fun inItem(key: String, text: String, substring: Boolean = false): SemanticsMatcher =
    hasText(text, substring = substring) and (hasTestTag(key) or hasAnyAncestor(hasTestTag(key)))
