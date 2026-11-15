package io.github.fsrodrig.anshin

import android.content.Context
import android.graphics.PorterDuff
import android.graphics.PorterDuffColorFilter
import android.graphics.drawable.AdaptiveIconDrawable
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.GenericShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.test.core.app.ApplicationProvider
import com.github.takahirom.roborazzi.captureRoboImage
import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.roundToInt
import kotlin.math.sign
import kotlin.math.sin
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Stand-ins for a launcher's wallpaper and its Android 13 themed-icon colours; only the icon's own layers are real.
private val WALLPAPER = Color(0xFF5B6170)
private val LABEL = Color(0xFFF2F2F2)
private val THEMED_BACKGROUND = Color(0xFF2B2F3D)
private val THEMED_INK = Color(0xFFD7DBF5)

private val Squircle = GenericShape { size, _ ->
    val steps = 96
    for (i in 0..steps) {
        val angle = 2 * Math.PI * i / steps
        // A superellipse with exponent 5, close to the squircle masks launchers draw.
        fun axis(v: Double) = (sign(v) * abs(v).pow(2.0 / 5)).toFloat()
        val x = size.width / 2 * (1 + axis(cos(angle)))
        val y = size.height / 2 * (1 + axis(sin(angle)))
        if (i == 0) moveTo(x, y) else lineTo(x, y)
    }
    close()
}

/**
 * The installed launcher icon, its real `R.mipmap.ic_launcher` layers, under circle, squircle and rounded-square masks,
 * and its monochrome layer as an Android 13 themed icon. This is the deterministic adaptive-icon preview.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h140dp-xhdpi")
class LauncherIconScreenshotTest {
    @get:Rule val compose = createComposeRule()

    private val icon = ApplicationProvider.getApplicationContext<Context>().getDrawable(R.mipmap.ic_launcher)
        as AdaptiveIconDrawable

    @Test
    fun launcherIconUnderEachMaskAndAsAThemedIcon() {
        val monochrome = checkNotNull(icon.monochrome).mutate().apply {
            colorFilter = PorterDuffColorFilter(THEMED_INK.toArgb(), PorterDuff.Mode.SRC_IN)
        }
        compose.setContent {
            Row(
                Modifier.fillMaxSize().background(WALLPAPER).padding(16.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Masked("circle", CircleShape) { layer(icon.background); layer(icon.foreground) }
                Masked("squircle", Squircle) { layer(icon.background); layer(icon.foreground) }
                Masked("rounded square", RoundedCornerShape(percent = 20)) { layer(icon.background); layer(icon.foreground) }
                Masked("themed", CircleShape) { drawRect(THEMED_BACKGROUND); layer(monochrome) }
            }
        }
        compose.onRoot().captureRoboImage("launcher-icon.png")
    }

    @Composable
    private fun Masked(label: String, shape: Shape, draw: DrawScope.() -> Unit) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Box(Modifier.size(64.dp).clip(shape)) { Canvas(Modifier.fillMaxSize(), onDraw = draw) }
            Text(label, color = LABEL, fontSize = 11.sp, modifier = Modifier.padding(top = 6.dp))
        }
    }

    /** Adaptive-icon layers are 108 units and the mask shows the central 72, so a layer overhangs by a quarter. */
    private fun DrawScope.layer(drawable: Drawable?) {
        drawable ?: return
        val overhang = (size.width / 4).roundToInt()
        drawable.setBounds(-overhang, -overhang, size.width.roundToInt() + overhang, size.height.roundToInt() + overhang)
        drawIntoCanvas { drawable.draw(it.nativeCanvas) }
    }
}
