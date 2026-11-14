package io.github.fsrodrig.anshin

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.lifecycle.compose.collectAsStateWithLifecycle

/** In `assets/` rather than `res/font/`, so its OFL licence can sit beside it and ship with it. */
private const val MONO_FONT = "fonts/JetBrainsMonoNL-Regular.ttf"

/** UI copy uses the system sans through Material's default typography; values use [mono]. */
@Immutable
class AnshinType internal constructor(monoFamily: FontFamily) {
    /** For numbers, units, paths, hostnames, addresses, timestamps and Verdict words. */
    val mono = TextStyle(fontFamily = monoFamily, fontFeatureSettings = "tnum")
}

private val LocalAnshinColors = staticCompositionLocalOf { Theme.Default.colors }
private val LocalAnshinType = staticCompositionLocalOf<AnshinType> { error("No AnshinTheme around this composable") }

object AnshinTheme {
    val colors: AnshinColors
        @Composable @ReadOnlyComposable get() = LocalAnshinColors.current

    val type: AnshinType
        @Composable @ReadOnlyComposable get() = LocalAnshinType.current
}

@Composable
fun AnshinTheme(theme: Theme, content: @Composable () -> Unit) {
    val assets = LocalContext.current.assets
    val type = remember(assets) { AnshinType(FontFamily(Font(MONO_FONT, assets))) }
    val colors = theme.colors
    val scheme = remember(colors) { colors.toColorScheme() }
    CompositionLocalProvider(LocalAnshinColors provides colors, LocalAnshinType provides type) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}

@Composable
fun AnshinTheme(setting: ThemeSetting, content: @Composable () -> Unit) {
    val theme by setting.theme.collectAsStateWithLifecycle(initialValue = null)
    // Until the setting is read, the window keeps its launch background instead of flashing another theme.
    theme?.let { AnshinTheme(it, content) }
}

/**
 * Every Material role comes from a token, so no Material baseline color reaches a component. Containers are the
 * surface and tints are no-ops because panels have borders and zero elevation.
 */
internal fun AnshinColors.toColorScheme(): ColorScheme = darkColorScheme(
    primary = accent,
    onPrimary = background,
    primaryContainer = surface,
    onPrimaryContainer = text,
    inversePrimary = accent,
    secondary = accent,
    onSecondary = background,
    secondaryContainer = surface,
    onSecondaryContainer = text,
    tertiary = accent,
    onTertiary = background,
    tertiaryContainer = surface,
    onTertiaryContainer = text,
    background = background,
    onBackground = text,
    surface = surface,
    onSurface = text,
    surfaceVariant = surface,
    onSurfaceVariant = muted,
    surfaceTint = surface,
    inverseSurface = text,
    inverseOnSurface = background,
    error = crit,
    onError = background,
    errorContainer = surface,
    onErrorContainer = crit,
    outline = border,
    outlineVariant = border,
    scrim = background,
    surfaceBright = surface,
    surfaceContainer = surface,
    surfaceContainerHigh = surface,
    surfaceContainerHighest = surface,
    surfaceContainerLow = surface,
    surfaceContainerLowest = background,
    surfaceDim = background,
    primaryFixed = accent,
    primaryFixedDim = accent,
    onPrimaryFixed = background,
    onPrimaryFixedVariant = background,
    secondaryFixed = accent,
    secondaryFixedDim = accent,
    onSecondaryFixed = background,
    onSecondaryFixedVariant = background,
    tertiaryFixed = accent,
    tertiaryFixedDim = accent,
    onTertiaryFixed = background,
    onTertiaryFixedVariant = background,
)
