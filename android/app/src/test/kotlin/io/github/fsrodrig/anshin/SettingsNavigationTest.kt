package io.github.fsrodrig.anshin

import androidx.compose.runtime.SideEffect
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.assertTextContains
import androidx.test.espresso.Espresso
import kotlin.time.TestTimeSource
import kotlinx.coroutines.Job
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/** Settings as [AnshinApp] wires it: the theme it stores, the forms it opens and where every way back leads. */
@RunWith(RobolectricTestRunner::class)
class SettingsNavigationTest {
    @get:Rule val compose = createComposeRule()
    @get:Rule val folder = TemporaryFolder()

    private val job = Job()
    private val store by lazy { machineStore(folder, job) }
    private val settings by lazy { themeSetting(folder, job) }
    private val book by lazy { MachineBook(store, { _, _, _ -> awaitCancellation() }) }
    private val watcher = Watcher({ _, _, _ -> awaitCancellation() }, TestTimeSource())
    private val restoration = StateRestorationTester(compose)
    private var drawn: AnshinColors? = null

    @After
    fun closeStore() = job.cancel()

    private fun show() {
        restoration.setContent {
            AnshinTheme(settings) {
                val colors = AnshinTheme.colors
                SideEffect { drawn = colors }
                AnshinApp(book, watcher, settings)
            }
        }
    }

    private fun waitFor(text: String) =
        compose.waitUntil(5_000) { compose.onAllNodesWithText(text).fetchSemanticsNodes().isNotEmpty() }

    private fun click(text: String) = compose.onNodeWithText(text).performScrollTo().performClick()

    private fun assertOnSettings() {
        waitFor("About Anshin")
        compose.onNode(hasText("Settings") and isHeading()).assertExists()
    }

    private fun assertOnDashboard() {
        waitFor("Add machine")
        compose.onNodeWithTag(ADD_MACHINE_TAG).assertExists()
        compose.onNodeWithText("About Anshin").assertDoesNotExist()
    }

    private fun openSettings() {
        waitFor("Settings")
        click("Settings")
        assertOnSettings()
    }

    private fun editLaptop() {
        click("laptop")
        waitFor("Edit machine")
    }

    @Test fun choosingAThemeStoresItAndRecoloursWithoutARestart() {
        show()
        openSettings()
        compose.waitUntil(5_000) { drawn == Theme.InkAndBone.colors }
        compose.onNode(hasText("Ink & bone") and isSelectable()).assertIsSelected()

        compose.onNode(hasText("Matte Black") and isSelectable()).performScrollTo().performClick()

        compose.waitUntil(5_000) { drawn == Theme.MatteBlack.colors }
        assertEquals(Theme.MatteBlack, runBlocking { settings.theme.first() })
        compose.onNode(hasText("Matte Black") and isSelectable()).assertIsSelected()
        assertOnSettings()
    }

    @Test fun editAndAddOpenedFromSettingsReturnToSettings() {
        seed(store, MachineInput("laptop", "laptop.example.ts.net"))
        seed(store, MachineInput("desktop", "100.64.0.2"))
        show()
        openSettings()

        editLaptop()
        Espresso.pressBack()
        assertOnSettings()

        editLaptop()
        click("Done")
        assertOnSettings()

        click("Add machine")
        waitFor("Cancel")
        Espresso.pressBack()
        assertOnSettings()
        click("Add machine")
        click("Cancel")
        assertOnSettings()

        editLaptop()
        click("Remove machine")
        click("Remove")
        assertOnSettings()
        compose.waitUntil(5_000) { compose.onAllNodesWithText("laptop").fetchSemanticsNodes().isEmpty() }
        compose.onNodeWithText("desktop").assertExists()
    }

    @Test fun backAndAllMachinesLeaveSettingsForTheDashboard() {
        show()
        openSettings()
        Espresso.pressBack()
        assertOnDashboard()
        openSettings()
        click("All Machines")
        assertOnDashboard()
    }

    @Test fun theLicencesOpenFromSettingsAndBackReturnsOneLevelAtATime() {
        show()
        openSettings()
        click("Show licences")
        waitFor("JetBrains Mono NL")
        click("JetBrains Mono NL")
        compose.onNodeWithTag(LICENCE_TEXT_TAG).assertExists()
        Espresso.pressBack()
        waitFor("matteblack.nvim")
        compose.onNodeWithTag(LICENCE_TEXT_TAG).assertDoesNotExist()
        click("Tokyo Night")
        click("Licences")
        waitFor("matteblack.nvim")
        click("Settings")
        assertOnSettings()
    }

    @Test fun everySettingsScreenSurvivesRecreation() {
        seed(store, MachineInput("laptop", "laptop.example.ts.net"))
        show()
        openSettings()
        restoration.emulateSavedInstanceStateRestore()
        assertOnSettings()

        editLaptop()
        compose.onNodeWithTag(NAME_FIELD_TAG).performTextReplacement("laptop-renamed")
        restoration.emulateSavedInstanceStateRestore()
        waitFor("Edit machine")
        compose.onNodeWithTag(NAME_FIELD_TAG).assertTextContains("laptop-renamed")
        Espresso.pressBack()
        assertOnSettings()

        click("Add machine")
        restoration.emulateSavedInstanceStateRestore()
        waitFor("Cancel")
        click("Cancel")
        assertOnSettings()

        click("Show licences")
        restoration.emulateSavedInstanceStateRestore()
        waitFor("matteblack.nvim")
        click("Catppuccin")
        restoration.emulateSavedInstanceStateRestore()
        compose.onNodeWithTag(LICENCE_TEXT_TAG).assertExists()
        Espresso.pressBack()
        waitFor("matteblack.nvim")
        Espresso.pressBack()
        assertOnSettings()
        Espresso.pressBack()
        assertOnDashboard()
    }
}
