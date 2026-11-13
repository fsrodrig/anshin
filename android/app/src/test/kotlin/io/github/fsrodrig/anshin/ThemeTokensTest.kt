package io.github.fsrodrig.anshin

import androidx.compose.ui.graphics.Color
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test

/** The JSON palettes in `branding/` are the source; the Kotlin tokens are a copy that must not drift. */
class ThemeTokensTest {
    private val branding = File(checkNotNull(System.getProperty("anshin.branding")) { "anshin.branding is unset" })

    private fun jsonFiles(): Map<String, File> =
        (branding.resolve("themes").listFiles { f -> f.extension == "json" }.orEmpty().toList() +
            branding.resolve("palette.json"))
            .associateBy { file -> if (file.name == "palette.json") Theme.Default.key else file.nameWithoutExtension }

    private fun read(file: File): JsonObject = Json.parseToJsonElement(file.readText()).jsonObject

    @Test
    fun everyJsonHasATheme_andEveryThemeHasAJson() {
        assertEquals(jsonFiles().keys.sorted(), Theme.entries.map { it.key }.sorted())
    }

    @Test
    fun onlyInkAndBoneIsTheDefault() {
        for ((key, file) in jsonFiles()) {
            assertEquals(file.path, key == Theme.Default.key, read(file).getValue("default").jsonPrimitive.boolean)
        }
    }

    @Test
    fun everyTokenMatchesItsJson() {
        for ((key, file) in jsonFiles()) {
            val theme = Theme.fromKey(key)
            val json = read(file)
            assertEquals(file.path, theme.label, json.getValue("name").jsonPrimitive.content)
            assertEquals(file.path, "dark", json.getValue("appearance").jsonPrimitive.content)
            val expected = json.getValue("colors").jsonObject.mapValues { (_, v) -> parseHex(v.jsonPrimitive.content) }
            assertEquals(file.path, expected, theme.colors.byRole())
        }
    }

    private fun parseHex(hex: String): Color {
        require(Regex("#[0-9A-Fa-f]{6}").matches(hex)) { "not #RRGGBB: $hex" }
        return Color(0xFF000000 or hex.substring(1).toLong(16))
    }
}

/** The roles by their `palette.json` names, in its order. */
fun AnshinColors.byRole(): Map<String, Color> = linkedMapOf(
    "background" to background,
    "surface" to surface,
    "border" to border,
    "text" to text,
    "muted" to muted,
    "accent" to accent,
    "cpu" to cpu,
    "memory" to memory,
    "storage" to storage,
    "network" to network,
    "ok" to ok,
    "warn" to warn,
    "crit" to crit,
    "unavailable" to unavailable,
)
