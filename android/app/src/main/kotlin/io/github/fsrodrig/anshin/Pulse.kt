package io.github.fsrodrig.anshin

import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.nullable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonEncoder
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive

/** The measurements currently available in a Pulse. A `null` measurement is unavailable on the Machine, never zero. */
@Serializable
data class Pulse(
    val schema: Int,
    @Serializable(with = PulseKindSerializer::class) val kind: PulseKind = PulseKind.Unknown,
    val cursor: String? = null,
    val hostname: String? = null,
    @SerialName("filesystems") val filesystems: List<Filesystem>? = null,
    @SerialName("primary_disk") val primaryDisk: String? = null,
    @SerialName("cpu_pct") val cpuPct: Double? = null,
    @SerialName("cpu_core_pct") val cpuCorePct: List<Double?>? = null,
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
    @Serializable(with = BatteryReportSerializer::class) val battery: BatteryReport = BatteryReport.NotReported,
    // Absent (an Agent from before Verdicts) and null (the Agent had no input to judge) both mean no current Verdict.
    @Serializable(with = VerdictReportSerializer::class) val verdict: Verdict? = null,
    @SerialName("disk_read_bps") val diskReadBps: Long? = null,
    @SerialName("disk_write_bps") val diskWriteBps: Long? = null,
    val window: Window? = null,
)

/** The filesystem the Agent named as the primary disk. Never inferred from list order. */
fun Pulse.primaryFilesystem(): Filesystem? = primaryDisk?.let { disk -> filesystems?.firstOrNull { it.majMin == disk } }

/** The Agent's judgement of its Machine. The phone renders it and never recomputes it. */
@Serializable
data class Verdict(
    @Serializable(with = VerdictLevelSerializer::class) val level: VerdictLevel,
    val reasons: List<Reason>? = null,
)

internal object VerdictReportSerializer : KSerializer<Verdict?> {
    override val descriptor: SerialDescriptor = Verdict.serializer().descriptor

    override fun deserialize(decoder: Decoder): Verdict? {
        // Streaming objects lets ignoreUnknownKeys skip future fields without the JSON tree reader's depth limit.
        val fields = decoder.decodeSerializableValue(VerdictFields.serializer().nullable) ?: return null
        val level = fields.level as? JsonPrimitive ?: return null
        if (!level.isString) return null
        val reasons = fields.reasons?.map { reason ->
            if (listOf(reason.code, reason.level, reason.message).any { !it.isNullableString() }) return null
            Reason(reason.code.stringOrNull(), reason.level.stringOrNull()?.let(VerdictLevel::of), reason.message.stringOrNull())
        }
        return Verdict(VerdictLevel.of(level.content), reasons)
    }

    override fun serialize(encoder: Encoder, value: Verdict?) {
        val output = encoder as JsonEncoder
        output.encodeJsonElement(value?.let { output.json.encodeToJsonElement(Verdict.serializer(), it) } ?: JsonNull)
    }
}

// Only known string fields need trees for type validation; Verdict and Reason objects must stay streaming.
@Serializable
private data class VerdictFields(
    val level: JsonElement? = null,
    val reasons: List<ReasonFields>? = null,
)

@Serializable
private data class ReasonFields(
    val code: JsonElement? = null,
    val level: JsonElement? = null,
    val message: JsonElement? = null,
)

private fun JsonElement?.isNullableString(): Boolean =
    this == null || this == JsonNull || this is JsonPrimitive && isString

private fun JsonElement?.stringOrNull(): String? = (this as? JsonPrimitive)?.takeIf { it.isString }?.content

/** One cause behind a [Verdict]. [code] is an open string; the phone shows [message], never a meaning it guessed from the code. */
@Serializable
data class Reason(
    val code: String? = null,
    @Serializable(with = VerdictLevelSerializer::class) val level: VerdictLevel? = null,
    val message: String? = null,
)

/** A Verdict word. A word this app doesn't know keeps its text in [Other] and is never given a severity. */
sealed interface VerdictLevel {
    val word: String

    data object Ok : VerdictLevel { override val word = "ok" }
    data object Warn : VerdictLevel { override val word = "warn" }
    data object Crit : VerdictLevel { override val word = "crit" }
    data class Other(override val word: String) : VerdictLevel

    companion object {
        fun of(word: String): VerdictLevel = when (word) {
            Ok.word -> Ok
            Warn.word -> Warn
            Crit.word -> Crit
            else -> Other(word)
        }
    }
}

internal object VerdictLevelSerializer : KSerializer<VerdictLevel> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("io.github.fsrodrig.anshin.VerdictLevel", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): VerdictLevel = VerdictLevel.of(decoder.decodeString())

    override fun serialize(encoder: Encoder, value: VerdictLevel) = encoder.encodeString(value.word)
}

@Serializable
data class Sensor(
    @SerialName("sensor") val id: String? = null,
    @SerialName("temp_c") val tempC: Double? = null,
    @SerialName("age_s") val ageS: Long? = null,
)

/**
 * The one place the phone must not collapse an absent key into `null`: an Agent from before batteries were measured
 * omits the key, which is unavailable, while `null` is a Machine with no system battery.
 */
sealed interface BatteryReport {
    data object NotReported : BatteryReport
    data object NoBattery : BatteryReport
    data class Present(val battery: Battery) : BatteryReport
}

/** The Machine's system batteries. Every summary field is `null` when the Agent couldn't classify a power supply. */
@Serializable
data class Battery(
    val pct: Double? = null,
    @Serializable(with = BatteryStatusSerializer::class) val status: BatteryStatus? = null,
    @SerialName("power_w") val powerW: Double? = null,
    // A Double, like age_s, so a fractional value can never make the whole Pulse unreadable.
    @SerialName("time_left_s") val timeLeftS: Double? = null,
    val batteries: List<BatteryRow>? = null,
)

@Serializable
data class BatteryRow(
    val name: String? = null,
    val pct: Double? = null,
    @Serializable(with = BatteryStatusSerializer::class) val status: BatteryStatus? = null,
    @SerialName("power_w") val powerW: Double? = null,
    @SerialName("health_pct") val healthPct: Double? = null,
)

/** The kernel's power_supply status. Watts are a magnitude; this is the direction. */
enum class BatteryStatus { Charging, Discharging, NotCharging, Full, Unknown }

internal object BatteryStatusSerializer : KSerializer<BatteryStatus> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("io.github.fsrodrig.anshin.BatteryStatus", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): BatteryStatus = when (decoder.decodeString()) {
        "Charging" -> BatteryStatus.Charging
        "Discharging" -> BatteryStatus.Discharging
        "Not charging" -> BatteryStatus.NotCharging
        "Full" -> BatteryStatus.Full
        else -> BatteryStatus.Unknown
    }

    override fun serialize(encoder: Encoder, value: BatteryStatus) = encoder.encodeString(
        when (value) {
            BatteryStatus.NotCharging -> "Not charging"
            else -> value.name
        },
    )
}

internal object BatteryReportSerializer : KSerializer<BatteryReport> {
    override val descriptor: SerialDescriptor = JsonElement.serializer().descriptor

    override fun deserialize(decoder: Decoder): BatteryReport {
        val json = decoder as? JsonDecoder ?: throw SerializationException("battery decodes only from JSON")
        val element = json.decodeJsonElement()
        return if (element is JsonNull) {
            BatteryReport.NoBattery
        } else {
            BatteryReport.Present(json.json.decodeFromJsonElement(Battery.serializer(), element))
        }
    }

    override fun serialize(encoder: Encoder, value: BatteryReport) {
        val json = encoder as? JsonEncoder ?: throw SerializationException("battery encodes only to JSON")
        json.encodeJsonElement(
            when (value) {
                is BatteryReport.Present -> json.json.encodeToJsonElement(Battery.serializer(), value.battery)
                else -> JsonNull
            },
        )
    }
}

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
 * [PulseKind.Unknown] instead of failing the whole Pulse. Verdict levels and Reason codes follow the same pattern,
 * and battery status already does in [BatteryStatusSerializer].
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
