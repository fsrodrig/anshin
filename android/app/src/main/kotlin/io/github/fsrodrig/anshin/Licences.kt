package io.github.fsrodrig.anshin

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

internal const val LICENCE_TEXT_TAG = "licence-text"

/**
 * One piece of work Anshin ships or links, and its licence. [asset] is the licence's full text in `assets/`, copied
 * verbatim from the upstream source. [font] is the bundled file it covers.
 */
internal class OpenSourceLicence(
    val key: String,
    val name: String,
    val covers: String,
    val licence: String,
    val asset: String,
    val font: String? = null,
    val themes: List<Theme> = emptyList(),
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
    OpenSourceLicence("tokyo-night", "Tokyo Night", "Colours of the Tokyo Night theme", MIT, "licences/tokyo-night.txt", themes = listOf(Theme.TokyoNight)),
    OpenSourceLicence("catppuccin", "Catppuccin", "Colours of the Catppuccin Mocha theme", MIT, "licences/catppuccin.txt", themes = listOf(Theme.CatppuccinMocha)),
    OpenSourceLicence(
        "omarchy", "Omarchy", "Colours of the Matte Black, Lupine and Flexoki Light themes", MIT, "licences/omarchy.txt",
        themes = listOf(Theme.MatteBlack, Theme.Lupine, Theme.FlexokiLight),
    ),
    OpenSourceLicence("matteblack-nvim", "matteblack.nvim", "Colours of the Matte Black theme", MIT, "licences/matteblack-nvim.txt", themes = listOf(Theme.MatteBlack)),
    OpenSourceLicence("flexoki", "Flexoki", "Colours of the Flexoki Light theme", MIT, "licences/flexoki.txt", themes = listOf(Theme.FlexokiLight)),
    OpenSourceLicence("androidx", "AndroidX", "Jetpack Compose, Activity, Lifecycle and DataStore", APACHE, APACHE_TEXT),
    OpenSourceLicence("okhttp", "OkHttp and Okio", "The HTTP client", APACHE, APACHE_TEXT),
    OpenSourceLicence("kotlin", "Kotlin", "The standard library, kotlinx.coroutines and kotlinx.serialization", APACHE, APACHE_TEXT),
)

internal fun openSourceLicence(key: String): OpenSourceLicence? = openSourceLicences.firstOrNull { it.key == key }

@Composable
internal fun Licences(onBack: () -> Unit, onOpen: (OpenSourceLicence) -> Unit) {
    AnshinScreen("Open-source licences", Back("Settings", onBack)) {
        ListGroup {
            openSourceLicences.forEachIndexed { i, entry ->
                ListRow(
                    first = i == 0,
                    title = entry.name,
                    meta = AnnotatedString("${entry.licence} · ${entry.covers}"),
                    onClickLabel = "Read the licence",
                    onClick = { onOpen(entry) },
                )
            }
        }
    }
}

@Composable
internal fun LicenceText(entry: OpenSourceLicence, onBack: () -> Unit) {
    val colors = AnshinTheme.colors
    val assets = LocalContext.current.assets
    // A few kilobytes from the APK, read once per licence; reading it off the main thread would only add a blank frame.
    val paragraphs = remember(entry.asset) { unwrapLicence(assets.open(entry.asset).bufferedReader().use { it.readText() }) }
    AnshinScreen(entry.name, Back("Open-source licences", onBack)) {
        Text("${entry.licence} · ${entry.covers}", color = colors.muted, style = MaterialTheme.typography.bodySmall)
        Column(Modifier.testTag(LICENCE_TEXT_TAG)) {
            paragraphs.forEachIndexed { i, paragraph ->
                Text(
                    paragraph.text,
                    color = colors.text,
                    fontSize = 13.sp,
                    lineHeight = 20.sp,
                    modifier = Modifier.padding(start = 4.dp * paragraph.indent, top = if (i > 0 && paragraph.opensBlock) 8.dp else 0.dp),
                )
            }
        }
    }
}

/**
 * One paragraph of a licence as drawn: its words on one line for the screen to wrap, the spaces it was indented by,
 * and whether a blank line stood before it.
 */
internal data class LicenceParagraph(val text: String, val indent: Int = 0, val opensBlock: Boolean = true)

/**
 * Licence files are hard-wrapped for an 80-column terminal, which a phone wraps again into ragged lines. This joins
 * each run of lines back into the paragraph it was, so the screen wraps it once; the asset itself stays verbatim.
 * Blank lines separate paragraphs. Inside a run, a line indented less than the one before starts a new paragraph (a
 * centred title), as does a line with no letter or digit (a rule) and a short all-capital heading such as `PREAMBLE`,
 * so neither is glued to the sentence under it. A hanging indent, deeper than the paragraph's first line, is joined.
 */
internal fun unwrapLicence(text: String): List<LicenceParagraph> {
    val paragraphs = mutableListOf<LicenceParagraph>()
    var words = StringBuilder()
    var indent = 0
    var lastIndent = 0
    var opensBlock = true
    var standsAlone = false
    fun close() {
        if (words.isNotEmpty()) paragraphs += LicenceParagraph(words.toString(), indent, opensBlock)
        words = StringBuilder()
        opensBlock = false
    }
    for (raw in text.lines()) {
        val line = raw.trimEnd()
        if (line.isBlank()) {
            close()
            opensBlock = true
            continue
        }
        val lineIndent = line.length - line.trimStart().length
        val alone = line.none { it.isLetterOrDigit() } || isHeading(line.trim())
        if (words.isNotEmpty() && (alone || standsAlone || lineIndent < lastIndent)) close()
        if (words.isEmpty()) indent = lineIndent else words.append(' ')
        words.append(line.trim())
        lastIndent = lineIndent
        standsAlone = alone
    }
    close()
    return paragraphs
}

private fun isHeading(line: String): Boolean =
    line.length <= 30 && line.last().isLetter() && line.any { it.isLetter() } && line.none { it.isLowerCase() }
