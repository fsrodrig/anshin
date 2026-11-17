package io.github.fsrodrig.anshin

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.espresso.Espresso
import kotlin.time.TestTimeSource
import kotlinx.coroutines.Job
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancel
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Every screen's way back: the bar's arrow and system Back go to the same parent, the arrow names it, and the parent
 * comes back where it was left.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class NavigationTest {
    @get:Rule val compose = createComposeRule()
    @get:Rule val folder = TemporaryFolder()

    private val job = Job()
    private val store by lazy { machineStore(folder, job) }
    private val settings by lazy { themeSetting(folder, job) }
    private val book by lazy { MachineBook(store, { _, _, _ -> awaitCancellation() }) }
    private val watcher = Watcher({ _, _, _ -> awaitCancellation() }, TestTimeSource())

    @After
    fun closeStore() = job.cancel()

    private fun show() {
        seed(store, MachineInput("laptop", "laptop.example.ts.net"))
        seed(store, MachineInput("desktop", "100.64.0.2"))
        compose.setContent { AnshinTheme(Theme.Default) { AnshinApp(book, watcher, settings) } }
        onDashboard()
    }

    private fun waitFor(matcher: SemanticsMatcher) =
        compose.waitUntil(5_000) { compose.onAllNodes(matcher).fetchSemanticsNodes().isNotEmpty() }

    private fun titled(title: String) = hasText(title) and isHeading()

    private fun onDashboard() {
        waitFor(hasTestTag(ADD_MACHINE_TAG))
        compose.onNodeWithContentDescription("Back to", substring = true).assertDoesNotExist()
    }

    private fun on(title: String) {
        waitFor(titled(title))
        compose.onNode(titled(title)).assertExists()
    }

    private fun action(label: String) = compose.onNodeWithContentDescription(label).performClick()

    private fun click(text: String) = compose.onNodeWithText(text).performScrollTo().performClick()

    private fun openLaptop() {
        compose.onNode(hasClickAction() and hasContentDescription("laptop.", substring = true)).performClick()
        on("laptop")
    }

    /** Opens a screen with [open], then leaves it once by system Back and once by the arrow, landing on [parent] both times. */
    private fun assertBothBacksLand(open: () -> Unit, arrow: String, parent: () -> Unit) {
        open()
        Espresso.pressBack()
        parent()
        open()
        action(arrow)
        parent()
    }

    @Test fun everyScreensArrowAndSystemBackLandOnTheSameParent() {
        show()
        val dashboard = ::onDashboard
        val settings = { on("Settings") }
        val licences = { on("Open-source licences") }

        assertBothBacksLand({ openLaptop() }, "Back to Dashboard", dashboard)
        assertBothBacksLand({ action("Add Machine"); on("Add Machine") }, "Back to Dashboard", dashboard)
        assertBothBacksLand({ action("Settings"); settings() }, "Back to Dashboard", dashboard)

        openLaptop()
        assertBothBacksLand({ action("Edit laptop"); on("Edit Machine") }, "Back to laptop", { on("laptop") })
        Espresso.pressBack()
        onDashboard()

        action("Settings")
        settings()
        assertBothBacksLand({ click("laptop"); on("Edit Machine") }, "Back to Settings", settings)
        assertBothBacksLand({ click("Add Machine"); on("Add Machine") }, "Back to Settings", settings)
        assertBothBacksLand({ click("Open-source licences"); licences() }, "Back to Settings", settings)

        click("Open-source licences")
        licences()
        assertBothBacksLand({ click("Catppuccin"); on("Catppuccin") }, "Back to Open-source licences", licences)
        Espresso.pressBack()
        settings()
        Espresso.pressBack()
        onDashboard()
    }

    @Test fun editOpenedFromAMachineReturnsToItAndARemovalReturnsToTheDashboard() {
        show()
        openLaptop()
        action("Edit laptop")
        on("Edit Machine")
        click("Cancel")
        on("laptop")

        action("Edit laptop")
        on("Edit Machine")
        click("Remove Machine")
        click("Remove")
        onDashboard()
        compose.waitUntil(5_000) { compose.onAllNodes(hasContentDescription("laptop.", substring = true)).fetchSemanticsNodes().isEmpty() }
    }

    @Test fun editOpenedFromSettingsReturnsToSettingsAndARemovalStaysThere() {
        show()
        action("Settings")
        on("Settings")
        click("laptop")
        on("Edit Machine")
        click("Cancel")
        on("Settings")

        click("laptop")
        on("Edit Machine")
        click("Remove Machine")
        click("Remove")
        on("Settings")
        compose.waitUntil(5_000) { compose.onAllNodes(hasText("laptop")).fetchSemanticsNodes().isEmpty() }
    }

    private fun scrolled(): Float =
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).fetchSemanticsNode().config[SemanticsProperties.VerticalScrollAxisRange].value()

    @Test fun backFindsTheParentWhereItWasLeftAndAScreenOpenedAgainStartsAtTheTop() {
        show()
        action("Settings")
        on("Settings")
        compose.onNodeWithText("Open-source licences").performScrollTo()
        val left = scrolled()
        assertTrue("Settings scrolled before leaving", left > 0f)

        click("Open-source licences")
        on("Open-source licences")
        Espresso.pressBack()
        on("Settings")
        compose.waitForIdle()
        assertEquals(left, scrolled(), 0.5f)

        Espresso.pressBack()
        onDashboard()
        action("Settings")
        on("Settings")
        compose.waitForIdle()
        assertEquals(0f, scrolled(), 0f)
    }
}
