package io.github.fsrodrig.anshin

import androidx.compose.ui.graphics.Color
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

/** The JSON palettes in `branding/` are the source; the Kotlin tokens are a copy that must not drift. */
class ThemeTokensTest {
    private val branding = File(checkNotNull(System.getProperty("anshin.branding")) { "anshin.branding is unset" })
    private val rootPalette = branding.resolve("palette.json")

    private fun jsonFiles(): Map<String, File> =
        palettesByKey(branding.resolve("themes").listFiles { f -> f.extension == "json" }.orEmpty().toList() + rootPalette)

    private fun palettesByKey(files: List<File>): Map<String, File> {
        val grouped = files.groupBy { file -> if (file == rootPalette) "ink-and-bone" else file.nameWithoutExtension }
        for ((key, palettes) in grouped) {
            assertEquals("Duplicate palette key '$key' in $palettes", 1, palettes.size)
        }
        return grouped.mapValues { (_, palettes) -> palettes.single() }
    }

    private fun read(file: File): JsonObject = Json.parseToJsonElement(file.readText()).jsonObject

    @Test
    fun onlyRootPaletteGetsTheInkAndBoneKey() {
        val alternate = branding.resolve("themes/palette.json")
        assertEquals(mapOf("ink-and-bone" to rootPalette, "palette" to alternate), palettesByKey(listOf(rootPalette, alternate)))
    }

    @Test
    fun duplicatePaletteKeysFail() {
        val duplicate = branding.resolve("themes/ink-and-bone.json")
        assertThrows(AssertionError::class.java) { palettesByKey(listOf(rootPalette, duplicate)) }
    }

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
            assertEquals(file.path, theme.appearance.name.lowercase(), json.getValue("appearance").jsonPrimitive.content)
            val expected = json.getValue("colors").jsonObject.mapValues { (_, v) -> parseHex(v.jsonPrimitive.content) }
            assertEquals(file.path, expected, theme.colors.byRole())
        }
    }

    private fun parseHex(hex: String): Color {
        require(Regex("#[0-9A-Fa-f]{6}").matches(hex)) { "not #RRGGBB: $hex" }
        return Color(0xFF000000 or hex.substring(1).toLong(16))
    }
}

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
