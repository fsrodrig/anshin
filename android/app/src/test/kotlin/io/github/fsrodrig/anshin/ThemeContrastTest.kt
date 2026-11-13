package io.github.fsrodrig.anshin

import androidx.compose.ui.graphics.Color
import io.github.fsrodrig.anshin.ColorMath.chroma
import io.github.fsrodrig.anshin.ColorMath.contrast
import io.github.fsrodrig.anshin.ColorMath.deltaE2000
import io.github.fsrodrig.anshin.ColorMath.hueDifference
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * WCAG 2.2 AA for every theme, plus the brand rule that the accent never reads as a Verdict color
 * (`branding/README.md`): far enough in CIEDE2000 and in hue, and a color rather than a grey.
 */
class ThemeContrastTest {
    @Test
    fun textAndVerdictRolesMeetFourPointFiveToOne() {
        assertContrast(4.5) { listOf("text" to text, "muted" to muted, "accent" to accent, "ok" to ok, "warn" to warn, "crit" to crit, "unavailable" to unavailable) }
    }

    @Test
    fun seriesMeetThreeToOne() {
        assertContrast(3.0) { listOf("cpu" to cpu, "memory" to memory, "storage" to storage, "network" to network) }
    }

    @Test
    fun accentIsDistinctFromEveryVerdictColor() {
        val failures = Theme.entries.flatMap { theme ->
            val c = theme.colors
            val accentChroma = chroma(c.accent)
            listOfNotNull("${theme.label}: accent chroma ${"%.1f".format(accentChroma)} < $MIN_ACCENT_CHROMA"
                .takeIf { accentChroma < MIN_ACCENT_CHROMA }) +
                listOf("ok" to c.ok, "warn" to c.warn, "crit" to c.crit).mapNotNull { (role, color) ->
                    val de = deltaE2000(c.accent, color)
                    val dh = hueDifference(c.accent, color)
                    "${theme.label}: accent vs $role ΔE00 ${"%.1f".format(de)}, Δh ${"%.0f".format(dh)}°"
                        .takeIf { de < MIN_DELTA_E || dh < MIN_HUE_DEGREES }
                }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }

    /** Sharma, Wu and Dalal's published CIEDE2000 test data, so a broken formula can't pass the check above. */
    @Test
    fun deltaE2000MatchesReferenceData() {
        val pairs = listOf(
            Triple(doubleArrayOf(50.0, 2.6772, -79.7751), doubleArrayOf(50.0, 0.0, -82.7485), 2.0425),
            Triple(doubleArrayOf(50.0, 0.0, 0.0), doubleArrayOf(50.0, -1.0, 2.0), 2.3669),
            Triple(doubleArrayOf(50.0, 2.5, 0.0), doubleArrayOf(73.0, 25.0, -18.0), 27.1492),
            Triple(doubleArrayOf(60.2574, -34.0099, 36.2677), doubleArrayOf(60.4626, -34.1751, 39.4387), 1.2644),
        )
        for ((a, b, expected) in pairs) assertEquals(expected, deltaE2000(a, b), 1e-4)
    }

    private fun assertContrast(minimum: Double, roles: AnshinColors.() -> List<Pair<String, Color>>) {
        val failures = Theme.entries.flatMap { theme ->
            roles(theme.colors).flatMap { (role, color) ->
                listOf("background" to theme.colors.background, "surface" to theme.colors.surface)
                    .mapNotNull { (ground, behind) ->
                        val ratio = contrast(color, behind)
                        "${theme.label}: $role on $ground is ${"%.2f".format(ratio)}:1".takeIf { ratio < minimum }
                    }
            }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }

    private companion object {
        const val MIN_DELTA_E = 20.0
        const val MIN_HUE_DEGREES = 45.0
        const val MIN_ACCENT_CHROMA = 30.0
    }
}
