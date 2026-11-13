package io.github.fsrodrig.anshin

import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json

/** The part of a Pulse the tracer bullet shows. A `null` measurement is unavailable on the Machine, never zero. */
@Serializable
data class Pulse(
    val schema: Int,
    @Serializable(with = PulseKindSerializer::class) val kind: PulseKind = PulseKind.Unknown,
    val hostname: String? = null,
    @SerialName("cpu_pct") val cpuPct: Double? = null,
)

/** Whether a Pulse replaces what the phone has ([Full]) or adds to it ([Delta]). */
enum class PulseKind { Full, Delta, Unknown }

/**
 * Enum-like Pulse values are open strings (docs/design.md § Field rules): a value this app doesn't know decodes as
 * [PulseKind.Unknown] instead of failing the whole Pulse. Verdict levels, Reason codes and battery status follow
 * the same pattern.
 */
internal object PulseKindSerializer : KSerializer<PulseKind> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("io.github.fsrodrig.anshin.PulseKind", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): PulseKind = when (decoder.decodeString()) {
        "full" -> PulseKind.Full
        "delta" -> PulseKind.Delta
        else -> PulseKind.Unknown
    }

    override fun serialize(encoder: Encoder, value: PulseKind) = encoder.encodeString(value.name.lowercase())
}

const val PULSE_SCHEMA = 1

// Within schema 1 the Agent may add fields at any time, so unknown keys are expected, not an error.
private val pulseJson = Json { ignoreUnknownKeys = true }

/** Returns `null` when [text] is not a Pulse this app can read: another schema, or not a Pulse at all. */
fun decodePulse(text: String): Pulse? = try {
    pulseJson.decodeFromString<Pulse>(text).takeIf { it.schema == PULSE_SCHEMA }
} catch (_: IllegalArgumentException) {
    null
}
