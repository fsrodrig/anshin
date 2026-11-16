package io.github.fsrodrig.anshin

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

internal const val LICENCE_TEXT_TAG = "licence-text"

/**
 * One piece of work Anshin ships or links, and its licence. [asset] is the licence's full text in `assets/`, copied
 * verbatim from the upstream source; [font] is the bundled file it covers, and [theme] the theme whose colors it gave.
 */
internal class OpenSourceLicence(
    val key: String,
    val name: String,
    val covers: String,
    val licence: String,
    val asset: String,
    val font: String? = null,
    val theme: Theme? = null,
)

private const val MIT = "MIT License"
private const val APACHE = "Apache License 2.0"
private const val APACHE_TEXT = "licences/apache-2.0.txt"

internal val openSourceLicences = listOf(
    OpenSourceLicence("anshin", "Anshin", "This app", MIT, "licences/anshin.txt"),
    OpenSourceLicence(
        "jetbrains-mono", "JetBrains Mono NL", "The bundled monospaced font", "SIL Open Font License 1.1", "fonts/OFL.txt",
        font = MONO_FONT,
    ),
    OpenSourceLicence("tokyo-night", "Tokyo Night", "Colors of the Tokyo Night theme", MIT, "licences/tokyo-night.txt", theme = Theme.TokyoNight),
    OpenSourceLicence("catppuccin", "Catppuccin", "Colors of the Catppuccin Mocha theme", MIT, "licences/catppuccin.txt", theme = Theme.CatppuccinMocha),
    OpenSourceLicence("omarchy", "Omarchy", "Colors of the Matte Black theme", MIT, "licences/omarchy.txt", theme = Theme.MatteBlack),
    OpenSourceLicence("matteblack-nvim", "matteblack.nvim", "Colors of the Matte Black theme", MIT, "licences/matteblack-nvim.txt", theme = Theme.MatteBlack),
    OpenSourceLicence("androidx", "AndroidX", "Jetpack Compose, Activity, Lifecycle and DataStore", APACHE, APACHE_TEXT),
    OpenSourceLicence("okhttp", "OkHttp and Okio", "The HTTP client", APACHE, APACHE_TEXT),
    OpenSourceLicence("kotlin", "Kotlin", "The standard library, kotlinx.coroutines and kotlinx.serialization", APACHE, APACHE_TEXT),
)

internal fun openSourceLicence(key: String): OpenSourceLicence? = openSourceLicences.firstOrNull { it.key == key }

/** Every licence, each opening its full text. */
@Composable
internal fun Licences(onBack: () -> Unit, onOpen: (OpenSourceLicence) -> Unit) {
    val colors = AnshinTheme.colors
    AppFrame(topLine = { null }) {
        Column {
            TextButton(onClick = onBack) { Text("Settings") }
            Title("Open-source licences")
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            openSourceLicences.forEach { entry ->
                Column(
                    Modifier
                        .fillMaxWidth()
                        .clickable(role = Role.Button, onClickLabel = "Read the licence") { onOpen(entry) }
                        .heightIn(min = 48.dp)
                        .padding(vertical = 8.dp),
                ) {
                    Text(entry.name, color = colors.text, fontSize = 17.sp)
                    Text("${entry.licence} · ${entry.covers}", color = colors.muted, fontSize = 13.sp, lineHeight = 18.sp)
                }
            }
        }
    }
}

/** One licence's full text, read from its asset. */
@Composable
internal fun LicenceText(entry: OpenSourceLicence, onBack: () -> Unit) {
    val colors = AnshinTheme.colors
    val assets = LocalContext.current.assets
    // A few kilobytes from the APK, read once per licence; reading it off the main thread would only add a blank frame.
    val text = remember(entry.asset) { assets.open(entry.asset).bufferedReader().use { it.readText() } }
    AppFrame(topLine = { null }) {
        Column {
            TextButton(onClick = onBack) { Text("Licences") }
            Title(entry.name)
            Text("${entry.licence} · ${entry.covers}", color = colors.muted, fontSize = 13.sp, lineHeight = 18.sp)
        }
        Text(text, color = colors.text, fontSize = 13.sp, lineHeight = 18.sp, modifier = Modifier.testTag(LICENCE_TEXT_TAG))
    }
}

@Composable
internal fun Title(text: String) {
    Text(text, color = AnshinTheme.colors.text, fontSize = 22.sp, modifier = Modifier.semantics { heading() })
}
