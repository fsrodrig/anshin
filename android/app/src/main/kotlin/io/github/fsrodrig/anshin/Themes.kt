package io.github.fsrodrig.anshin

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color

/**
 * The semantic color roles of `branding/palette.json`. Every color the app draws comes from one of these; this file
 * is the only place that spells a color out.
 */
@Immutable
data class AnshinColors(
    val background: Color,
    val surface: Color,
    val border: Color,
    val text: Color,
    val muted: Color,
    val accent: Color,
    val cpu: Color,
    val memory: Color,
    val storage: Color,
    val network: Color,
    val ok: Color,
    val warn: Color,
    val crit: Color,
    val unavailable: Color,
)

/**
 * The four dark themes. Each one's values are its JSON in `branding/` (a test keeps them equal), and
 * `branding/themes/README.md` records where the alternate themes come from.
 *
 * [key] is what the theme setting stores. Never change one: a stored key that no theme has falls back to Ink & bone.
 */
enum class Theme(val key: String, val label: String, val colors: AnshinColors) {
    InkAndBone(
        "ink-and-bone",
        "Ink & bone",
        AnshinColors(
            background = Color(0xFF141519),
            surface = Color(0xFF1B1C22),
            border = Color(0xFF2A2C35),
            text = Color(0xFFE8E4DA),
            muted = Color(0xFF9A978F),
            accent = Color(0xFFA5A8F5),
            cpu = Color(0xFF7DB7D6),
            memory = Color(0xFFC79BC9),
            storage = Color(0xFF74BFB0),
            network = Color(0xFFC4B998),
            ok = Color(0xFF93C47D),
            warn = Color(0xFFE8B04C),
            crit = Color(0xFFEC6A5E),
            unavailable = Color(0xFF7F8396),
        ),
    ),
    TokyoNight(
        "tokyo-night",
        "Tokyo Night",
        AnshinColors(
            background = Color(0xFF16161E),
            surface = Color(0xFF1A1B26),
            border = Color(0xFF363B54),
            text = Color(0xFFC0CAF5),
            muted = Color(0xFF9AA5CE),
            accent = Color(0xFF7AA2F7),
            cpu = Color(0xFF7DCFFF),
            memory = Color(0xFFBB9AF7),
            storage = Color(0xFF73DACA),
            network = Color(0xFFFF9E64),
            ok = Color(0xFF9ECE6A),
            warn = Color(0xFFE0AF68),
            crit = Color(0xFFF7768E),
            unavailable = Color(0xFF9699A8),
        ),
    ),
    CatppuccinMocha(
        "catppuccin-mocha",
        "Catppuccin Mocha",
        AnshinColors(
            background = Color(0xFF181825),
            surface = Color(0xFF1E1E2E),
            border = Color(0xFF313244),
            text = Color(0xFFCDD6F4),
            muted = Color(0xFFA6ADC8),
            accent = Color(0xFFCBA6F7),
            cpu = Color(0xFF74C7EC),
            memory = Color(0xFFF5C2E7),
            storage = Color(0xFF94E2D5),
            network = Color(0xFFFAB387),
            ok = Color(0xFFA6E3A1),
            warn = Color(0xFFF9E2AF),
            crit = Color(0xFFF38BA8),
            unavailable = Color(0xFF9399B2),
        ),
    ),
    MatteBlack(
        "matte-black",
        "Matte Black",
        AnshinColors(
            background = Color(0xFF121212),
            surface = Color(0xFF1E1E1E),
            border = Color(0xFF333333),
            text = Color(0xFFEAEAEA),
            muted = Color(0xFFBEBEBE),
            accent = Color(0xFF3B82F6),
            cpu = Color(0xFF1EA7A0),
            memory = Color(0xFFB027DE),
            storage = Color(0xFFEFBF04),
            network = Color(0xFFBF9903),
            ok = Color(0xFF10B981),
            warn = Color(0xFFD97706),
            crit = Color(0xFFF87171),
            unavailable = Color(0xFF8A8A8D),
        ),
    ),
    ;

    companion object {
        val Default = InkAndBone

        fun fromKey(key: String?): Theme = entries.firstOrNull { it.key == key } ?: Default
    }
}
