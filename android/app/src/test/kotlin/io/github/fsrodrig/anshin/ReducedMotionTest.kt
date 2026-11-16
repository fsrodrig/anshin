package io.github.fsrodrig.anshin

import android.provider.Settings.Global
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.MotionDurationScale
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/** "Remove animations" (an animator duration scale of 0) makes every non-essential animation snap. */
@RunWith(RobolectricTestRunner::class)
class ReducedMotionTest {
    /**
     * Compose's own window-wide animation scale. On a device it follows the animator duration scale through a
     * notification that can arrive seconds after the app returns from Settings, so a test can leave it stale.
     */
    private var windowScale = 1f

    @get:Rule
    val compose = createComposeRule(
        object : MotionDurationScale {
            override val scaleFactor: Float get() = windowScale
        },
    )

    private val resolver = ApplicationProvider.getApplicationContext<android.content.Context>().contentResolver

    private var target by mutableFloatStateOf(0f)
    private var shown = -1f
    private var reduced: Boolean? = null

    private fun setScale(scale: Float) {
        Global.putFloat(resolver, Global.ANIMATOR_DURATION_SCALE, scale)
        // The settings provider notifies observers of a write; Robolectric's store does not, so the test does.
        resolver.notifyChange(Global.getUriFor(Global.ANIMATOR_DURATION_SCALE), null)
    }

    private fun show() = compose.setContent {
        AnshinTheme(Theme.Default) {
            val value = animateCalmly(target, label = "probe").value
            val motion = LocalReducedMotion.current
            SideEffect {
                shown = value
                reduced = motion
            }
        }
    }

    /** Moves the probe from 0 to 1 and returns what it shows about 100 ms later, well within a 250 ms tween. */
    private fun shownSoonAfterAChange(): Float {
        compose.waitForIdle()
        assertEquals(0f, shown)
        compose.mainClock.autoAdvance = false
        compose.runOnIdle { target = 1f }
        repeat(6) {
            compose.mainClock.advanceTimeByFrame()
            compose.waitForIdle()
        }
        val after = shown
        compose.mainClock.advanceTimeBy(1_000)
        compose.waitForIdle()
        assertEquals(1f, shown)
        return after
    }

    @Test
    fun withAnimationsOnABarFillTweens() {
        setScale(1f)
        show()
        val midway = shownSoonAfterAChange()
        assertEquals(false, reduced)
        assertTrue("100 ms in, the fill is on its way: $midway", midway > 0f && midway < 1f)
    }

    @Test
    fun withAnimationsRemovedABarFillSnaps() {
        setScale(0f)
        show()
        assertEquals(true, reduced)
        assertEquals(1f, shownSoonAfterAChange())
    }

    @Test
    fun turningAnimationsBackOnTweensEvenBeforeComposeHearsOfIt() {
        windowScale = 0f
        setScale(1f)
        show()
        val midway = shownSoonAfterAChange()
        assertEquals(false, reduced)
        assertTrue("100 ms in, the fill is on its way: $midway", midway > 0f && midway < 1f)
    }

    @Test
    fun turningAnimationsOffDuringATweenSnapsToItsTarget() {
        setScale(1f)
        show()
        compose.waitForIdle()
        compose.mainClock.autoAdvance = false
        compose.runOnIdle { target = 1f }
        repeat(6) {
            compose.mainClock.advanceTimeByFrame()
            compose.waitForIdle()
        }
        assertTrue("The fill is still tweening: $shown", shown > 0f && shown < 1f)

        compose.runOnIdle {
            windowScale = 0f
            setScale(0f)
        }
        compose.waitForIdle()
        repeat(2) {
            compose.mainClock.advanceTimeByFrame()
            compose.waitForIdle()
        }
        assertEquals(true, reduced)
        assertEquals(1f, shown)
    }

    @Test
    fun turningAnimationsOffWhileVisibleAppliesAtOnce() {
        setScale(1f)
        show()
        compose.waitForIdle()
        assertFalse(checkNotNull(reduced))
        setScale(0f)
        compose.waitForIdle()
        assertTrue(checkNotNull(reduced))
        setScale(1f)
        compose.waitForIdle()
        assertFalse(checkNotNull(reduced))
    }
}
