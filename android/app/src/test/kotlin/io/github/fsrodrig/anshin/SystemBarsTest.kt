package io.github.fsrodrig.anshin

import android.content.Context
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.text.TextLayoutResult
import androidx.core.view.WindowInsetsControllerCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
class SystemBarsTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val setting by lazy {
        ThemeSetting(ApplicationProvider.getApplicationContext<Context>().settingsStore)
    }

    @Test
    @Config(qualifiers = "notnight")
    fun barIconsFollowTheThemesAppearanceInLightAndDarkSystemMode() {
        assertTrue(Theme.entries.any { it.appearance == Appearance.Dark } && Theme.entries.any { it.appearance == Appearance.Light })
        for (theme in Theme.entries) {
            runBlocking { setting.select(theme) }
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                awaitTheme(theme)
                scenario.onActivity { assertBarIcons(it, theme, "${theme.key}, system light mode") }

                RuntimeEnvironment.setQualifiers("+night")
                scenario.recreate()
                awaitTheme(theme)
                scenario.onActivity { assertBarIcons(it, theme, "${theme.key}, system dark mode, after recreation") }

                RuntimeEnvironment.setQualifiers("+notnight")
                scenario.recreate()
                awaitTheme(theme)
                scenario.onActivity { assertBarIcons(it, theme, "${theme.key}, system light mode, after recreation") }
            }
        }
    }

    @Test
    @Config(qualifiers = "notnight")
    fun changingTheSavedThemeRestylesTheBarIconsWithoutRecreation() {
        runBlocking { setting.select(Theme.Default) }
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            awaitTheme(Theme.Default)
            for (theme in Theme.entries.drop(1) + Theme.Default) {
                runBlocking { setting.select(theme) }
                awaitTheme(theme)
                scenario.onActivity { assertBarIcons(it, theme, "${theme.key}, after live theme change") }
            }
        }
    }

    private fun awaitTheme(theme: Theme) {
        compose.waitUntil(timeoutMillis = 10_000) {
            // The app composes nothing until the saved Machines are read, so on a slow runner the name isn't there yet.
            if (compose.onAllNodesWithText("anshin").fetchSemanticsNodes().size != 1) return@waitUntil false
            val layouts = mutableListOf<TextLayoutResult>()
            compose.onNodeWithText("anshin").performSemanticsAction(SemanticsActions.GetTextLayoutResult) {
                it(layouts)
            }
            layouts.singleOrNull()?.layoutInput?.style?.color == theme.colors.text
        }
        compose.waitForIdle()
    }

    /** A "light appearance" bar is one with dark icons, drawn over a light background. */
    private fun assertBarIcons(activity: MainActivity, theme: Theme, mode: String) {
        val bars = WindowInsetsControllerCompat(activity.window, activity.window.decorView)
        val darkIcons = theme.appearance == Appearance.Light
        val expected = if (darkIcons) "dark" else "light"
        assertEquals("$expected status bar icons in $mode", darkIcons, bars.isAppearanceLightStatusBars)
        assertEquals("$expected navigation bar icons in $mode", darkIcons, bars.isAppearanceLightNavigationBars)
    }
}
