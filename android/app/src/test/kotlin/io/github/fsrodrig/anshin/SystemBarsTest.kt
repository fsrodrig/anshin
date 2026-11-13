package io.github.fsrodrig.anshin

import androidx.core.view.WindowInsetsControllerCompat
import androidx.test.core.app.ActivityScenario
import org.junit.Assert.assertFalse
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
class SystemBarsTest {
    @Test
    @Config(qualifiers = "notnight")
    fun barIconsStayLightOnTheDarkThemesInLightAndDarkSystemMode() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { assertLightBarIcons(it, "system light mode") }

            RuntimeEnvironment.setQualifiers("+night")
            scenario.recreate()
            scenario.onActivity { assertLightBarIcons(it, "system dark mode, after recreation") }

            RuntimeEnvironment.setQualifiers("+notnight")
            scenario.recreate()
            scenario.onActivity { assertLightBarIcons(it, "system light mode, after recreation") }
        }
    }

    private fun assertLightBarIcons(activity: MainActivity, mode: String) {
        val bars = WindowInsetsControllerCompat(activity.window, activity.window.decorView)
        assertFalse("dark status bar icons in $mode", bars.isAppearanceLightStatusBars)
        assertFalse("dark navigation bar icons in $mode", bars.isAppearanceLightNavigationBars)
    }
}
