package io.github.fsrodrig.anshin

import android.content.Context
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.text.TextLayoutResult
import androidx.core.view.WindowInsetsControllerCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertFalse
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
    fun barIconsStayLightOnTheDarkThemesInLightAndDarkSystemMode() {
        for (theme in Theme.entries) {
            runBlocking { setting.select(theme) }
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                awaitTheme(theme)
                scenario.onActivity { assertLightBarIcons(it, "${theme.key}, system light mode") }

                RuntimeEnvironment.setQualifiers("+night")
                scenario.recreate()
                awaitTheme(theme)
                scenario.onActivity { assertLightBarIcons(it, "${theme.key}, system dark mode, after recreation") }

                RuntimeEnvironment.setQualifiers("+notnight")
                scenario.recreate()
                awaitTheme(theme)
                scenario.onActivity { assertLightBarIcons(it, "${theme.key}, system light mode, after recreation") }
            }
        }
    }

    @Test
    @Config(qualifiers = "notnight")
    fun changingTheSavedThemeKeepsBarIconsLightWithoutRecreation() {
        runBlocking { setting.select(Theme.Default) }
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            awaitTheme(Theme.Default)
            for (theme in Theme.entries.drop(1) + Theme.Default) {
                runBlocking { setting.select(theme) }
                awaitTheme(theme)
                scenario.onActivity { assertLightBarIcons(it, "${theme.key}, after live theme change") }
            }
        }
    }

    private fun awaitTheme(theme: Theme) {
        compose.waitUntil(timeoutMillis = 10_000) {
            val layouts = mutableListOf<TextLayoutResult>()
            compose.onNodeWithText("anshin").performSemanticsAction(SemanticsActions.GetTextLayoutResult) {
                it(layouts)
            }
            layouts.singleOrNull()?.layoutInput?.style?.color == theme.colors.text
        }
        compose.waitForIdle()
    }

    private fun assertLightBarIcons(activity: MainActivity, mode: String) {
        val bars = WindowInsetsControllerCompat(activity.window, activity.window.decorView)
        assertFalse("dark status bar icons in $mode", bars.isAppearanceLightStatusBars)
        assertFalse("dark navigation bar icons in $mode", bars.isAppearanceLightNavigationBars)
    }
}
