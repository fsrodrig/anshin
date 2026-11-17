package io.github.fsrodrig.anshin

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class SettingsTest {
    @get:Rule val compose = createComposeRule()

    private val main = File(checkNotNull(System.getProperty("anshin.main")) { "anshin.main is unset" })
    private val machines = listOf(Machines.laptop, Machines.desktop)

    private fun showSettings(theme: Theme = Theme.Default, actions: SettingsActions = SettingsActions()) =
        compose.setContent { AnshinTheme(theme) { Settings(machines, theme, version = null, actions) } }

    private fun spoken(node: SemanticsNode): String =
        (node.config.getOrNull(SemanticsProperties.ContentDescription).orEmpty() +
            node.config.getOrNull(SemanticsProperties.Text).orEmpty().map { it.text }).joinToString(" ")

    private fun assertEveryControlReachable(): List<String> {
        val controls = compose.onAllNodes(hasClickAction())
        val count = controls.fetchSemanticsNodes().size
        assertTrue("found controls", count > 0)
        return (0 until count).map { i ->
            // The bar's controls never scroll; everything under it does.
            val control = controls[i]
            if (hasAnyAncestor(hasScrollAction()).matches(control.fetchSemanticsNode())) control.performScrollTo()
            val node = control.fetchSemanticsNode()
            val label = spoken(node)
            assertTrue("control $i has a spoken label", label.isNotBlank())
            val bounds = node.touchBoundsInRoot
            with(compose.density) {
                assertTrue("$label is ${bounds.width.toDp()} wide", bounds.width.toDp() >= 48.dp)
                assertTrue("$label is ${bounds.height.toDp()} tall", bounds.height.toDp() >= 48.dp)
            }
            label
        }
    }

    @Test fun everySettingsControlIsLabelledAndLargeEnough() {
        showSettings()
        val labels = assertEveryControlReachable()
        val expected = listOf("Back to Dashboard") + Theme.entries.map { it.label } +
            listOf("laptop", "desktop", "Add Machine", "Open-source licences")
        for (label in expected) assertTrue("$label in $labels", labels.any { it.startsWith(label) })
        assertEquals(expected.size, labels.size)
    }

    @Test fun sectionTitlesAreHeadingsInOrder() {
        showSettings()
        val titles = listOf("Settings", "Theme", "Machines", "About")
        val tops = titles.map { title ->
            compose.onNode(hasText(title) and isHeading()).fetchSemanticsNode().positionInRoot.y
        }
        assertEquals(tops.sorted(), tops)
    }

    @Test fun themesAreOneSelectableGroupWithTheCurrentThemeSelected() {
        showSettings(Theme.CatppuccinMocha)
        val group = compose.onNodeWithTag(THEME_GROUP_TAG).fetchSemanticsNode()
        assertNotNull(group.config.getOrNull(SemanticsProperties.SelectableGroup))
        for (theme in Theme.entries) {
            val option = compose.onNode(hasText(theme.label) and isSelectable())
            assertEquals(Role.RadioButton, option.fetchSemanticsNode().config[SemanticsProperties.Role])
            if (theme == Theme.CatppuccinMocha) option.assertIsSelected() else option.assertIsNotSelected()
        }
    }

    @Test fun choosingAThemeAsksForIt() {
        var chosen: Theme? = null
        showSettings(actions = SettingsActions(onTheme = { chosen = it }))
        compose.onNode(hasText("Tokyo Night") and isSelectable()).performScrollTo().performClick()
        assertEquals(Theme.TokyoNight, chosen)
    }

    @Test fun aBuildWithoutAVersionSaysSoAndAReleaseShowsItsVersion() {
        var version by mutableStateOf<String?>(null)
        compose.setContent { AnshinTheme(Theme.Default) { Settings(machines, Theme.Default, version, SettingsActions()) } }
        compose.onNodeWithText("Development build").performScrollTo()
        version = "1.2.3"
        compose.onNodeWithText("1.2.3").performScrollTo()
        compose.onNodeWithText("Development build").assertDoesNotExist()
    }

    @Test fun licencesAndALicenceTextAreLabelledAndLargeEnough() {
        var reading by mutableStateOf(false)
        compose.setContent {
            AnshinTheme(Theme.Default) {
                if (reading) LicenceText(openSourceLicences.first(), onBack = {}) else Licences(onBack = {}, onOpen = {})
            }
        }
        compose.onNode(hasText("Open-source licences") and isHeading()).assertExists()
        val labels = assertEveryControlReachable()
        val expected = listOf("Back to Settings") + openSourceLicences.map { it.name }
        assertEquals(expected.size, labels.size)
        expected.zip(labels).forEach { (name, label) -> assertTrue("$label starts with $name", label.startsWith(name)) }

        reading = true
        assertEquals(listOf("Back to Open-source licences"), assertEveryControlReachable())
    }

    @Test fun theDashboardsBarActionsAndEmptyStateAreLabelledAndLargeEnough() {
        compose.setContent { AnshinTheme(Theme.Default) { Dashboard(emptyList(), now = { Ages.now }, onOpen = {}, onAdd = {}, onSettings = {}) } }
        assertEquals(listOf("Add Machine", "Settings", "Add Machine"), assertEveryControlReachable())
    }

    @Test fun theLicencesCoverTheBundledFontAndEveryBorrowedPalette() {
        val fonts = main.resolve("assets/fonts").listFiles { file -> file.extension == "ttf" }.orEmpty().map { "fonts/${it.name}" }
        assertTrue(fonts.isNotEmpty())
        for (font in fonts) assertTrue("$font has a licence", openSourceLicences.any { it.font == font })
        assertTrue(fonts.contains(MONO_FONT))
        for (theme in Theme.entries - Theme.InkAndBone) {
            assertTrue("${theme.label} has a licence", openSourceLicences.any { it.theme == theme })
        }
        for (entry in openSourceLicences) assertTrue(entry.asset, main.resolve("assets/${entry.asset}").isFile)
        assertEquals(main.resolve("../../../../LICENSE").readText(), main.resolve("assets/licences/anshin.txt").readText())
        assertEquals(openSourceLicences.size, openSourceLicences.map { it.key }.distinct().size)
    }

    @Test fun theFontsLicenceShowsEveryWordOfTheBundledOflTextUnwrapped() {
        val font = openSourceLicences.single { it.font == MONO_FONT }
        compose.setContent { AnshinTheme(Theme.Default) { LicenceText(font, onBack = {}) } }
        val shown = compose.onAllNodes(hasAnyAncestor(hasTestTag(LICENCE_TEXT_TAG)) and SemanticsMatcher.keyIsDefined(SemanticsProperties.Text), useUnmergedTree = true)
            .fetchSemanticsNodes().map { node -> node.config[SemanticsProperties.Text].joinToString("") { it.text } }
        val asset = main.resolve("assets/fonts/OFL.txt").readText()
        assertEquals(unwrapLicence(asset).map { it.text }, shown)
        fun words(text: String) = text.split(Regex("\\s+")).filter { it.isNotEmpty() }
        assertEquals("no word is lost or added", words(asset), shown.flatMap(::words))
    }
}
