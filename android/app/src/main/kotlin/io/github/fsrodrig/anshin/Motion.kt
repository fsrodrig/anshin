package io.github.fsrodrig.anshin

import android.content.ContentResolver
import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.provider.Settings.Global
import androidx.compose.animation.core.AnimationSpec
import androidx.compose.animation.core.snap
import androidx.compose.animation.core.tween
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.awaitCancellation

/** True when the Owner asked Android to remove animations; every non-essential animation then snaps. */
internal val LocalReducedMotion = compositionLocalOf { false }

@Composable
@ReadOnlyComposable
internal fun <T> calmMotion(): AnimationSpec<T> = if (LocalReducedMotion.current) snap() else tween(durationMillis = 250)

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
