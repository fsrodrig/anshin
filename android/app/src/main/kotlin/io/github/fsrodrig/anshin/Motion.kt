package io.github.fsrodrig.anshin

import android.content.ContentResolver
import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.provider.Settings.Global
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.VectorConverter
import androidx.compose.animation.core.tween
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.State
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.MotionDurationScale
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.withContext

/** True when the Owner asked Android to remove animations; every non-essential animation then snaps. */
internal val LocalReducedMotion = compositionLocalOf { false }

/**
 * Follows [target]: a 250 ms tween to each new value, or a snap when [LocalReducedMotion] is on.
 *
 * Compose also scales every animation by the window's animator duration scale, but it learns of a change only from a
 * settings notification that Android delivered several seconds after the app came back from Settings, so for those
 * seconds turning "Remove animations" back off still snapped. The tween runs under a scale of its own, which leaves
 * [LocalReducedMotion], re-read on every start, the only thing that decides.
 */
@Composable
internal fun animateCalmly(target: Float, label: String): State<Float> {
    val animatable = remember { Animatable(target, Float.VectorConverter, label = label) }
    val reduced = LocalReducedMotion.current
    LaunchedEffect(animatable, target, reduced) {
        when {
            animatable.value == target -> Unit
            reduced -> animatable.snapTo(target)
            else -> withContext(UnscaledMotion) { animatable.animateTo(target, tween(durationMillis = 250)) }
        }
    }
    return animatable.asState()
}

private object UnscaledMotion : MotionDurationScale {
    override val scaleFactor = 1f
}

/**
 * Android has no separate reduced-motion switch: "Remove animations" sets the animator duration scale to 0. It is read
 * here only, and watched while the app is visible, so turning it on in quick settings applies without a restart.
 */
@Composable
internal fun ProvideReducedMotion(content: @Composable () -> Unit) {
    val resolver = LocalContext.current.contentResolver
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val reduced by produceState(animationsRemoved(resolver), resolver, lifecycle) {
        val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean) {
                value = animationsRemoved(resolver)
            }
        }
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            value = animationsRemoved(resolver)
            resolver.registerContentObserver(Global.getUriFor(Global.ANIMATOR_DURATION_SCALE), false, observer)
            try {
                awaitCancellation()
            } finally {
                resolver.unregisterContentObserver(observer)
            }
        }
    }
    CompositionLocalProvider(LocalReducedMotion provides reduced, content = content)
}

private fun animationsRemoved(resolver: ContentResolver): Boolean = Global.getFloat(resolver, Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
