package io.github.fsrodrig.anshin

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/** Ink & bone, from `branding/palette.json`. Selectable themes arrive with #12. */
object Ink {
    val background = Color(0xFF141519)
    val surface = Color(0xFF1B1C22)
    val border = Color(0xFF2A2C35)
    val text = Color(0xFFE8E4DA)
    val muted = Color(0xFF9A978F)
    val accent = Color(0xFFA5A8F5)
    val cpu = Color(0xFF7DB7D6)
    val crit = Color(0xFFEC6A5E)
    val unavailable = Color(0xFF7F8396)
}

@Composable
fun AnshinTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Ink.accent,
            onPrimary = Ink.background,
            background = Ink.background,
            onBackground = Ink.text,
            surface = Ink.surface,
            onSurface = Ink.text,
            onSurfaceVariant = Ink.muted,
            outline = Ink.border,
            error = Ink.crit,
        ),
        content = content,
    )
}
