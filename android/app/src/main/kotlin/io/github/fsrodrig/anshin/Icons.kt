package io.github.fsrodrig.anshin

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathBuilder
import androidx.compose.ui.graphics.vector.path
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.unit.dp
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin

/**
 * The app's five glyphs, drawn here because Material Icons are not on the classpath (ADR 0001). Each is a 2-unit
 * stroke on a 24-unit grid with round caps and joins and no fill, in the theme token [Glyph] is given.
 */
internal enum class AnshinIcon(val autoMirror: Boolean = false, val path: PathBuilder.() -> Unit) {
    Back(autoMirror = true, { moveTo(19f, 12f); horizontalLineTo(5f); moveTo(11f, 6f); lineToRelative(-6f, 6f); lineToRelative(6f, 6f) }),
    Add(path = { moveTo(12f, 5f); verticalLineToRelative(14f); moveTo(5f, 12f); horizontalLineToRelative(14f) }),
    Edit(path = {
        moveTo(4f, 20f); horizontalLineToRelative(4f); lineTo(19f, 9f); lineToRelative(-4f, -4f); lineTo(4f, 16f); close()
        moveTo(13f, 7f); lineToRelative(4f, 4f)
    }),
    Settings(path = {
        circle(12f, 12f, 6.5f)
        circle(12f, 12f, 2.5f)
        for (i in 0 until 8) {
            val angle = i * PI / 4
            moveTo(12f + 6.5f * cos(angle).toFloat(), 12f + 6.5f * sin(angle).toFloat())
            lineTo(12f + 9.5f * cos(angle).toFloat(), 12f + 9.5f * sin(angle).toFloat())
        }
    }),
    Chevron(autoMirror = true, { moveTo(9f, 6f); lineToRelative(6f, 6f); lineToRelative(-6f, 6f) }),
}

/** [icon] at 24 dp in [ink]. A null [contentDescription] makes it decoration, read by nothing. */
@Composable
internal fun Glyph(icon: AnshinIcon, ink: Color, contentDescription: String?, modifier: Modifier = Modifier) {
    val vector = remember(icon, ink) { glyph(icon, ink) }
    Image(rememberVectorPainter(vector), contentDescription, modifier.size(24.dp))
}

private fun PathBuilder.circle(cx: Float, cy: Float, r: Float) {
    moveTo(cx - r, cy)
    arcToRelative(r, r, 0f, isMoreThanHalf = true, isPositiveArc = true, dx1 = 2 * r, dy1 = 0f)
    arcToRelative(r, r, 0f, isMoreThanHalf = true, isPositiveArc = true, dx1 = -2 * r, dy1 = 0f)
    close()
}

private fun glyph(icon: AnshinIcon, ink: Color): ImageVector =
    ImageVector.Builder(icon.name, 24.dp, 24.dp, 24f, 24f, autoMirror = icon.autoMirror).path(
        fill = null,
        stroke = SolidColor(ink),
        strokeLineWidth = 2f,
        strokeLineCap = StrokeCap.Round,
        strokeLineJoin = StrokeJoin.Round,
        pathBuilder = icon.path,
    ).build()
