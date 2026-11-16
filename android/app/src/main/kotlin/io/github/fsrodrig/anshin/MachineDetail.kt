package io.github.fsrodrig.anshin

import androidx.compose.runtime.Immutable

/** How current the phone's view of a Machine is, in the glossary's words. #10's Watcher will produce all three. */
internal sealed interface Connection {
    data object Live : Connection

    data object Stale : Connection

    /** [outsideTailnet]: Anshin refused the address itself, so it can say why; otherwise it doesn't claim to know. */
    data class Unreachable(val outsideTailnet: Boolean) : Connection
}

/** Everything the detail screen draws. Values always come from [last]; Live is the only state that shows a Verdict. */
@Immutable
internal data class MachineDetailState(
    /** The name the Owner typed, as the top line uses it. */
    val machine: String,
    val connection: Connection,
    val last: LastPulse,
    val window: HeldWindow?,
) {
    val live: Boolean get() = connection == Connection.Live

    /** A Verdict is current only while Live; Stale and Unreachable withhold it rather than quote it. */
    val currentVerdict: Verdict? get() = if (live) last.pulse.verdict else null
}
