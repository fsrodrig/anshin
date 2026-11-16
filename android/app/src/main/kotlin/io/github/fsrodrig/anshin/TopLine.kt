package io.github.fsrodrig.anshin

/** One watched Machine, by its Machine name, and what the phone shows of it. */
data class WatchedMachine(val name: String, val view: MachineView)

/**
 * The Dashboard's first line: the worst current Verdict across Machines, and every Machine that has none, by name.
 * [headline] is `null` when no Machine has a known current Verdict.
 */
data class TopLine(val headline: Headline?, val withoutVerdict: List<String>, val unranked: List<Headline.Worst> = emptyList())

sealed interface Headline {
    /** Every watched Machine is Live with an ok Verdict. */
    data object AllOk : Headline

    /** The worst current Verdict and the Machine it belongs to. An ok here means some other Machine has no known current Verdict. */
    data class Worst(val machine: String, val verdict: Verdict) : Headline
}

fun topLine(machines: List<WatchedMachine>): TopLine? {
    if (machines.isEmpty()) return null
    val current = machines.mapNotNull { machine -> currentVerdict(machine.view)?.let { machine.name to it } }
    val withoutVerdict = machines.filter { currentVerdict(it.view) == null }.map { it.name }
    // maxByOrNull keeps the first of equals, so a tie names the Machine listed first.
    val unranked = current.filter { it.second.level is VerdictLevel.Other }.map { Headline.Worst(it.first, it.second) }
    val worst = current.filter { it.second.level !is VerdictLevel.Other }.maxByOrNull { (_, verdict) -> severity(verdict.level) }
    val headline = when {
        worst == null -> null
        worst.second.level == VerdictLevel.Ok && withoutVerdict.isEmpty() && unranked.isEmpty() -> Headline.AllOk
        else -> Headline.Worst(worst.first, worst.second)
    }
    return TopLine(headline, withoutVerdict, unranked)
}

/** A Verdict is current only while its Machine is Live; Stale, Unreachable, Forbidden and the rest withhold it. */
fun currentVerdict(view: MachineView): Verdict? = (view as? MachineView.Live)?.received?.pulse?.verdict

private fun severity(level: VerdictLevel): Int = when (level) {
    VerdictLevel.Ok -> 0
    is VerdictLevel.Other -> error("Unknown Verdicts cannot be ranked")
    VerdictLevel.Warn -> 2
    VerdictLevel.Crit -> 3
}
