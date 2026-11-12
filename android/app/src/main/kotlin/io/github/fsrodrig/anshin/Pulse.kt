package io.github.fsrodrig.anshin

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** The part of a Pulse the tracer bullet shows. A `null` measurement is unavailable on the Machine, never zero. */
@Serializable
data class Pulse(
    val schema: Int,
    val hostname: String? = null,
    @SerialName("cpu_pct") val cpuPct: Double? = null,
)

const val PULSE_SCHEMA = 1

// Within schema 1 the Agent may add fields at any time, so unknown keys are expected, not an error.
private val pulseJson = Json { ignoreUnknownKeys = true }

/** Returns `null` when [text] is not a Pulse this app can read: another schema, or not a Pulse at all. */
fun decodePulse(text: String): Pulse? = try {
    pulseJson.decodeFromString<Pulse>(text).takeIf { it.schema == PULSE_SCHEMA }
} catch (_: IllegalArgumentException) {
    null
}
