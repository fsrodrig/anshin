package io.github.fsrodrig.anshin

import androidx.compose.runtime.Composable
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

@Composable
internal fun PulseView(view: MachineView, machine: String) = PulseView(view, machine, now = { Ages.now })
