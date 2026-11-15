package io.github.fsrodrig.anshin

import android.content.Context
import android.graphics.Path
import android.graphics.PorterDuff
import android.graphics.PorterDuffColorFilter
import android.graphics.drawable.AdaptiveIconDrawable
import android.graphics.drawable.ColorDrawable
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.test.core.app.ApplicationProvider
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.util.ReflectionHelpers
import org.robolectric.util.ReflectionHelpers.ClassParameter

// Stand-ins for a launcher's wallpaper and its Android 13 themed-icon colours; only the icon's own layers are real.
private val WALLPAPER = Color(0xFF5B6170)
private val LABEL = Color(0xFFF2F2F2)
private val SOURCE = Color(0xFFB8BCC8)
private val THEMED_BACKGROUND = Color(0xFF2B2F3D)
private val THEMED_INK = Color(0xFFD7DBF5)

private class Mask(val label: String, val studioName: String, val pathData: String)

// Android Studio's adaptive-icon preview shapes, copied verbatim from AdaptiveIconShape.java in JetBrains/android
// render-resources at 9acf2bf8 (same strings in platform/tools/adt/idea studio-main at 9bb2edb8).
private val CIRCLE = Mask(
    "circle",
    "Circle",
    "M50 0C77.6 0 100 22.4 100 50C100 77.6 77.6 100 50 100C22.4 100 0 77.6 0 50C0 22.4 22.4 0 50 0Z",
)
private val SQUIRCLE = Mask(
    "squircle",
    "Squircle",
    "M50,0 C10,0 0,10 0,50 0,90 10,100 50,100 90,100 100,90 100,50 100,10 90,0 50,0 Z",
)
private val ROUNDED_SQUARE = Mask(
    "rounded square",
    "Rounded Corners",
    "M50,0L92,0C96.42,0 100,4.58 100 8L100,92C100, 96.42 96.42 100 92 100L8 100C4.58, 100 0 96.42 0 92L0 8 C 0 4.42 4.42 0 8 0L50 0Z",
)

/**
 * The installed launcher icon, its real `R.mipmap.ic_launcher`, masked by the framework's own [AdaptiveIconDrawable]
 * with Android Studio's circle, squircle and rounded-square preview paths, and its monochrome layer as an Android 13
 * themed icon. This is the deterministic adaptive-icon preview.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h140dp-xhdpi")
class LauncherIconScreenshotTest {
    @get:Rule val compose = createComposeRule()

    private val context = ApplicationProvider.getApplicationContext<Context>()

    private fun installedIcon() = context.getDrawable(R.mipmap.ic_launcher) as AdaptiveIconDrawable

    private fun themedIcon(): AdaptiveIconDrawable {
        val monochrome = checkNotNull(installedIcon().monochrome).mutate().apply {
            colorFilter = PorterDuffColorFilter(THEMED_INK.toArgb(), PorterDuff.Mode.SRC_IN)
        }
        return AdaptiveIconDrawable(ColorDrawable(THEMED_BACKGROUND.toArgb()), monochrome)
    }

    @Test
    fun launcherIconUnderEachMaskAndAsAThemedIcon() {
        compose.setContent {
            Row(
                Modifier.fillMaxSize().background(WALLPAPER).padding(horizontal = 8.dp, vertical = 16.dp),
                horizontalArrangement = Arrangement.SpaceEvenly,
            ) {
                Cell(CIRCLE.label, CIRCLE, ::installedIcon)
                Cell(SQUIRCLE.label, SQUIRCLE, ::installedIcon)
                Cell(ROUNDED_SQUARE.label, ROUNDED_SQUARE, ::installedIcon)
                Cell("themed", CIRCLE, ::themedIcon)
            }
        }
        compose.onRoot().captureRoboImage("launcher-icon.png")
    }

    @Composable
    private fun Cell(label: String, mask: Mask, icon: () -> AdaptiveIconDrawable) {
        val px = with(LocalDensity.current) { ICON_SIZE.roundToPx() }
        val drawable = remember(mask, px) { masked(icon(), mask, px) }
        Column(Modifier.width(80.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Canvas(Modifier.size(ICON_SIZE)) { drawIntoCanvas { drawable.draw(it.nativeCanvas) } }
            Text(label, color = LABEL, fontSize = 11.sp, modifier = Modifier.padding(top = 6.dp))
            Text("Studio ${mask.studioName}", color = SOURCE, fontSize = 9.sp, textAlign = TextAlign.Center)
        }
    }

    /**
     * Every constructor reloads the static `sMask` from `config_icon_mask`, but `setBounds` scales whatever `sMask`
     * holds then, so the shape is swapped in between and restored once the drawable has its own scaled copy.
     */
    private fun masked(drawable: AdaptiveIconDrawable, mask: Mask, px: Int): AdaptiveIconDrawable {
        val platformMask = ReflectionHelpers.getStaticField<Path>(AdaptiveIconDrawable::class.java, "sMask")
        try {
            ReflectionHelpers.setStaticField(AdaptiveIconDrawable::class.java, "sMask", parse(mask.pathData))
            drawable.setBounds(0, 0, px, px)
        } finally {
            ReflectionHelpers.setStaticField(AdaptiveIconDrawable::class.java, "sMask", platformMask)
        }
        return drawable
    }

    private fun parse(pathData: String): Path = ReflectionHelpers.callStaticMethod(
        Class.forName("android.util.PathParser"),
        "createPathFromPathData",
        ClassParameter.from(String::class.java, pathData),
    )

    private companion object {
        val ICON_SIZE = 64.dp
    }
}
