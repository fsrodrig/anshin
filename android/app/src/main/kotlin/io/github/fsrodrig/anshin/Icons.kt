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
import kotlin.math.asin
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
        cog()
        circle(12f, 12f, 3f)
    }),
    Chevron(autoMirror = true, { moveTo(9f, 6f); lineToRelative(6f, 6f); lineToRelative(-6f, 6f) }),
}

/** [icon] at 24 dp in [ink]. A null [contentDescription] makes it decoration, read by nothing. */
@Composable
internal fun Glyph(icon: AnshinIcon, ink: Color, contentDescription: String?, modifier: Modifier = Modifier) {
    val vector = remember(icon, ink) { glyph(icon, ink) }
    Image(rememberVectorPainter(vector), contentDescription, modifier.size(24.dp))
}

/**
 * A body ring of radius 7 with six square-ended teeth out to 9.5, one pointing up: drawn as one outline so the teeth
 * read as part of a wheel, where separate rays around a ring read as brightness.
 */
private fun PathBuilder.cog() {
    val body = 7f
    val tip = 9.5f
    val halfTooth = 1.75f
    val toothAngle = asin(halfTooth / body)
    fun at(angle: Double, radius: Float, side: Float = 0f) = Pair(
        12f + radius * cos(angle).toFloat() - side * sin(angle).toFloat(),
        12f + radius * sin(angle).toFloat() + side * cos(angle).toFloat(),
    )
    for (i in 0 until 6) {
        val axis = -PI / 2 + i * PI / 3
        val (sx, sy) = at(axis - toothAngle, body)
        if (i == 0) moveTo(sx, sy)
        val (ax, ay) = at(axis, tip, -halfTooth)
        val (bx, by) = at(axis, tip, halfTooth)
        val (ex, ey) = at(axis + toothAngle, body)
        lineTo(ax, ay)
        lineTo(bx, by)
        lineTo(ex, ey)
        val (nx, ny) = at(axis + PI / 3 - toothAngle, body)
        arcTo(body, body, 0f, isMoreThanHalf = false, isPositiveArc = true, x1 = nx, y1 = ny)
    }
    close()
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
