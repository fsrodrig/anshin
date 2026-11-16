package io.github.fsrodrig.anshin

import android.content.Context
import android.graphics.drawable.AdaptiveIconDrawable
import android.graphics.drawable.ColorDrawable
import android.os.Build
import androidx.test.core.app.ApplicationProvider
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * The installed launcher icon is the brand pack's adaptive icon, with the themed-icon layer Android 13 uses. It runs on
 * API 33, the first level that reads the monochrome layer; LauncherIconScreenshotTest draws it on the default API.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [Build.VERSION_CODES.TIRAMISU])
class LauncherIconTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Test
    fun theLauncherIconIsAdaptiveWithAMonochromeLayerOnThePaletteBackground() {
        assertEquals(R.mipmap.ic_launcher, context.applicationInfo.icon)
        val icon = context.getDrawable(R.mipmap.ic_launcher)
        assertTrue("not adaptive: $icon", icon is AdaptiveIconDrawable)
        icon as AdaptiveIconDrawable
        assertNotNull("no foreground layer", icon.foreground)
        assertNotNull("no monochrome layer", icon.monochrome)

        val palette = File(checkNotNull(System.getProperty("anshin.branding")) { "anshin.branding is unset" })
            .resolve("palette.json")
        val hex = Json.parseToJsonElement(palette.readText()).jsonObject.getValue("colors").jsonObject
            .getValue("background").jsonPrimitive.content
        val background = icon.background
        assertTrue("background is not a colour: $background", background is ColorDrawable)
        assertEquals(hex, "#%06X".format((background as ColorDrawable).color and 0xFFFFFF))
        assertEquals(0xFF, background.color ushr 24)
    }
}
