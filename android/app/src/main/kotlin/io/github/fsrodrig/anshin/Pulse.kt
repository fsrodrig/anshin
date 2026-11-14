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

/** The measurements currently available in a Pulse. A `null` measurement is unavailable on the Machine, never zero. */
@Serializable
data class Pulse(
    val schema: Int,
    @Serializable(with = PulseKindSerializer::class) val kind: PulseKind = PulseKind.Unknown,
    val hostname: String? = null,
    @SerialName("filesystems") val filesystems: List<Filesystem>? = null,
    @SerialName("primary_disk") val primaryDisk: String? = null,
    @SerialName("cpu_pct") val cpuPct: Double? = null,
    @SerialName("mem_total_bytes") val memTotalBytes: Long? = null,
    @SerialName("mem_available_bytes") val memAvailableBytes: Long? = null,
    @SerialName("mem_used_bytes") val memUsedBytes: Long? = null,
    @SerialName("mem_used_pct") val memUsedPct: Double? = null,
    @SerialName("swap_total_bytes") val swapTotalBytes: Long? = null,
    @SerialName("swap_used_bytes") val swapUsedBytes: Long? = null,
    @SerialName("swap_entries") val swapEntries: List<SwapEntry>? = null,
    @SerialName("load_1") val load1: Double? = null,
    @SerialName("load_5") val load5: Double? = null,
    @SerialName("load_15") val load15: Double? = null,
    @SerialName("uptime_s") val uptimeS: Double? = null,
    val distro: String? = null,
    val kernel: String? = null,
    val arch: String? = null,
    @SerialName("agent_version") val agentVersion: String? = null,
    @SerialName("cpu_temp_c") val cpuTempC: Double? = null,
    @SerialName("cpu_sensor") val cpuSensor: String? = null,
    @SerialName("temperatures") val temperatures: List<Sensor>? = null,
)

@Serializable
data class Sensor(
    @SerialName("sensor") val id: String? = null,
    @SerialName("temp_c") val tempC: Double? = null,
)

@Serializable
data class Filesystem(
    @SerialName("maj_min") val majMin: String? = null,
    @SerialName("mount_point") val mountPoint: String? = null,
    @SerialName("fs_type") val fsType: String? = null,
    @SerialName("total_bytes") val totalBytes: Long? = null,
    @SerialName("used_bytes") val usedBytes: Long? = null,
    @SerialName("available_bytes") val availableBytes: Long? = null,
    @SerialName("used_pct") val usedPct: Double? = null,
    @SerialName("age_s") val ageS: Double? = null,
)

@Serializable
data class SwapEntry(
    val name: String? = null,
    val zram: Boolean? = null,
    @SerialName("total_bytes") val totalBytes: Long? = null,
    @SerialName("used_bytes") val usedBytes: Long? = null,
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
