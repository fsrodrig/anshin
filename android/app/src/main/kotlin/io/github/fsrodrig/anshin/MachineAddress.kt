package io.github.fsrodrig.anshin

import okhttp3.HttpUrl

const val AGENT_PORT = 7247

/**
 * The Pulse URL for what the Owner typed: a MagicDNS name or tailnet IP, optionally with `:port` (`[v6]:port` for
 * IPv6). Returns `null` for anything else. Whether the address is on the tailnet is the socket's call, not this one.
 */
fun pulseUrl(machine: String): HttpUrl? {
    val input = machine.trim()
    val (host, portText) = when {
        input.startsWith("[") -> {
            val close = input.indexOf(']')
            if (close < 0) return null
            val rest = input.substring(close + 1)
            if (rest.isNotEmpty() && !rest.startsWith(":")) return null
            input.substring(1, close) to rest.removePrefix(":").ifEmpty { null }
        }
        input.count { it == ':' } > 1 -> input to null
        ':' in input -> input.substringBefore(':') to input.substringAfter(':')
        else -> input to null
    }
    if (host.isEmpty()) return null
    val port = if (portText == null) AGENT_PORT else portText.toIntOrNull()?.takeIf { it in 1..65535 } ?: return null
    return try {
        HttpUrl.Builder().scheme("http").host(host).port(port).addPathSegments("v1/pulse").build()
    } catch (_: IllegalArgumentException) {
        null
    }
}
