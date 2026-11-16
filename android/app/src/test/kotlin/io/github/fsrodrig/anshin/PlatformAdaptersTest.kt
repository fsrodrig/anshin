package io.github.fsrodrig.anshin

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import androidx.test.core.app.ApplicationProvider
import java.time.Duration
import kotlin.time.Duration.Companion.hours
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.shadows.ShadowNetworkCapabilities
import org.robolectric.shadows.ShadowSystemClock

/** The two platform reads behind the Watcher, in one class to spare a JVM fork. */
@RunWith(RobolectricTestRunner::class)
class PlatformAdaptersTest {
    private val connectivity = ApplicationProvider.getApplicationContext<Context>().getSystemService(ConnectivityManager::class.java)

    @Test
    fun elapsedRealtimeCountsTimeThePhoneSpentAsleep() {
        val before = ElapsedRealtime.markNow()
        // Robolectric's SystemClock moves elapsedRealtime the way deep sleep does, without System.nanoTime.
        ShadowSystemClock.advanceBy(Duration.ofHours(1))
        assertEquals(1.hours, before.elapsedNow())
    }

    @Test
    fun noActiveNetworkLooksLikeTheVpnIsOff() {
        shadowOf(connectivity).setActiveNetworkInfo(null)
        assertTrue(connectivity.vpnLooksOff())
    }

    @Test
    fun anActiveNetworkWithoutAVpnLooksOff() {
        val wifi = ShadowNetworkCapabilities.newInstance().also { shadowOf(it).addTransportType(NetworkCapabilities.TRANSPORT_WIFI) }
        shadowOf(connectivity).setNetworkCapabilities(connectivity.activeNetwork, wifi)
        assertTrue(connectivity.vpnLooksOff())
    }

    @Test
    fun anActiveVpnDoesNotLookOff() {
        val vpn = ShadowNetworkCapabilities.newInstance().also {
            shadowOf(it).addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            shadowOf(it).addTransportType(NetworkCapabilities.TRANSPORT_VPN)
        }
        shadowOf(connectivity).setNetworkCapabilities(connectivity.activeNetwork, vpn)
        assertFalse(connectivity.vpnLooksOff())
    }
}
