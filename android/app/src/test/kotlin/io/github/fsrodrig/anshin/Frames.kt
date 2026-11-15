package io.github.fsrodrig.anshin

import android.view.View
import androidx.compose.ui.test.SemanticsNodeInteraction
import com.github.takahirom.roborazzi.captureRoboImage

/**
 * Captures [path] after drawing every window once more, with frame skipping off, and waiting for that frame.
 *
 * On API 37 Robolectric draws through real HWUI and stamps each frame's vsync with the wall clock. HWUI skips a frame
 * whose vsync is within 2 ms of the window's last one, as a second draw in the same vsync. A test that sets its
 * content fast enough after the activity's first, empty frame therefore never renders its content, nothing asks for
 * another frame, and PixelCopy copies the empty window. The frame is also rendered later, on the RenderThread, while
 * PixelCopy reads the window's buffer at once.
 */
internal fun SemanticsNodeInteraction.captureFrame(path: String) {
    fetchSemanticsNode()
    forEachWindow { view, renderer ->
        hardwareRenderer.getMethod("forceDrawNextFrame").invoke(renderer)
        view.invalidate()
    }
    fetchSemanticsNode()
    forEachWindow { _, renderer -> hardwareRenderer.getMethod("fence").invoke(renderer) }
    captureRoboImage(path)
}

private val hardwareRenderer = Class.forName("android.graphics.HardwareRenderer")

// WindowManagerGlobal, ViewRootImpl and the renderer are hidden API, so they are reached by reflection.
private fun forEachWindow(action: (View, Any) -> Unit) {
    val global = Class.forName("android.view.WindowManagerGlobal")
    val roots = global.getDeclaredField("mRoots").apply { isAccessible = true }
        .get(global.getMethod("getInstance").invoke(null)) as List<*>
    check(roots.isNotEmpty()) { "no window to capture" }
    for (root in roots.filterNotNull()) {
        val view = root.javaClass.getMethod("getView").invoke(root) as View
        val attachInfo = root.javaClass.getDeclaredField("mAttachInfo").apply { isAccessible = true }.get(root)
        val renderer = attachInfo.javaClass.getDeclaredField("mThreadedRenderer").apply { isAccessible = true }
            .get(attachInfo) ?: error("window draws without a hardware renderer")
        action(view, renderer)
    }
}
