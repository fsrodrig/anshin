package io.github.fsrodrig.anshin

import android.net.ConnectivityManager
import android.net.NetworkCapabilities

/**
 * True when the phone's active network is not a VPN. Advisory only: any VPN counts, since Android can't tell
 * Tailscale from another without querying packages, and a Pulse that arrives is Live whatever this says.
 */
fun ConnectivityManager.vpnLooksOff(): Boolean = vpnLooksOff(activeNetwork?.let(::getNetworkCapabilities))

internal fun vpnLooksOff(capabilities: NetworkCapabilities?): Boolean =
    capabilities?.hasTransport(NetworkCapabilities.TRANSPORT_VPN) != true
