package io.github.fsrodrig.anshin

import java.net.InetAddress
import java.util.Locale
import okhttp3.HttpUrl

const val AGENT_PORT = 7247

/**
 * How the phone reaches a Machine's Agent. [host] is canonical: a lowercase MagicDNS name or FQDN without a trailing
 * dot, a dotted IPv4 literal, or an IPv6 literal in RFC 5952 form without brackets. Two addresses that reach the same
 * Agent by the same spelling are therefore equal.
 */
data class MachineAddress(val host: String, val port: Int) {
    val pulseUrl: HttpUrl
        get() = HttpUrl.Builder().scheme("http").host(host).port(port).addPathSegments("v1/pulse").build()

    /** `laptop`, `100.64.0.2`, `fd7a:115c:a1e0::1`; with another port `laptop:9000` or `[fd7a:115c:a1e0::1]:9000`. */
    override fun toString(): String = when {
        port == AGENT_PORT -> host
        ':' in host -> "[$host]:$port"
        else -> "$host:$port"
    }
}

sealed interface AddressProblem {
    data object Blank : AddressProblem
    data object Malformed : AddressProblem

    /** The port was typed into the address; it belongs in its own field. */
    data object HasPort : AddressProblem

    /** An IP literal outside `100.64.0.0/10` and `fd7a:115c:a1e0::/48`, in its canonical form. */
    data class OutsideTailnet(val literal: String) : AddressProblem

    data class Taken(val by: Machine) : AddressProblem
}

/** Anything but a decimal number from 1 to 65535. */
data object PortProblem

/** What the address and port fields hold: a [MachineAddress], or the problem with each field. */
internal data class ParsedAddress(
    val address: MachineAddress?,
    val addressProblem: AddressProblem?,
    val portProblem: PortProblem?,
)

internal fun parseAddress(address: String, port: String): ParsedAddress {
    val host = parseHost(address.trim())
    val number = parsePort(port.trim())
    return ParsedAddress(
        address = if (host is Host.Valid && number != null) MachineAddress(host.canonical, number) else null,
        addressProblem = (host as? Host.Invalid)?.problem,
        portProblem = if (number == null) PortProblem else null,
    )
}

private fun parsePort(text: String): Int? = when {
    text.isEmpty() -> AGENT_PORT
    !PORT.matches(text) -> null
    else -> text.toInt().takeIf { it in 1..65535 }
}

private sealed interface Host {
    data class Valid(val canonical: String) : Host
    data class Invalid(val problem: AddressProblem) : Host
}

private val malformed = Host.Invalid(AddressProblem.Malformed)

private fun parseHost(text: String): Host = when {
    text.isEmpty() -> Host.Invalid(AddressProblem.Blank)
    // A URL, a path, a user, an IPv6 zone or two words: none is a host the Owner can mean.
    text.any { it.isWhitespace() || it in "/?#@%" } -> malformed
    text.startsWith("[") -> {
        val close = text.indexOf(']')
        val rest = if (close < 0) null else text.substring(close + 1)
        when {
            rest == null -> malformed
            rest.isEmpty() -> text.substring(1, close).let { if (':' in it) ipv6(it) else malformed }
            rest.startsWith(":") && DIGITS.matches(rest.substring(1)) -> Host.Invalid(AddressProblem.HasPort)
            else -> malformed
        }
    }
    text.count { it == ':' } == 1 ->
        if (DIGITS.matches(text.substringAfter(':'))) Host.Invalid(AddressProblem.HasPort) else malformed
    ':' in text -> ipv6(text)
    text.all { it.isDigit() || it == '.' } -> ipv4(text)
    else -> name(text)
}

private fun ipv6(text: String): Host {
    // OkHttp's strict decoder, so the canonical form is the one the request will carry.
    val canonical = try {
        HttpUrl.Builder().scheme("http").host(text).build().host
    } catch (_: IllegalArgumentException) {
        return malformed
    }
    // OkHttp writes an IPv4-mapped literal as dotted IPv4.
    if (':' !in canonical) return ipv4(canonical)
    // A validated numeric literal, so this makes no DNS lookup.
    val address = InetAddress.getByName(canonical)
    return if (TailnetAddresses.contains(address)) Host.Valid(canonical) else Host.Invalid(AddressProblem.OutsideTailnet(canonical))
}

private fun ipv4(text: String): Host {
    val octets = text.split('.')
    // A leading zero reads as octal to some resolvers, so 100.064.0.1 has no single meaning.
    if (octets.size != 4 || octets.any { !OCTET.matches(it) || it.toInt() > 255 }) return malformed
    val address = InetAddress.getByAddress(ByteArray(4) { octets[it].toInt().toByte() })
    return if (TailnetAddresses.contains(address)) Host.Valid(text) else Host.Invalid(AddressProblem.OutsideTailnet(text))
}

/**
 * A MagicDNS short name or an FQDN. It is not range-checked here: the socket checks the address it resolves to, and
 * the test connection reports it. ASCII only, so a lookalike letter can't pass for another Machine's name.
 */
private fun name(text: String): Host {
    val name = text.lowercase(Locale.ROOT).removeSuffix(".")
    val labels = name.split('.')
    val valid = name.all { it.code < 0x80 } &&
        name.length in 1..253 &&
        labels.all { it.length in 1..63 && LABEL.matches(it) } &&
        !labels.last().all { it.isDigit() }
    return if (valid) Host.Valid(name) else malformed
}

private val DIGITS = Regex("[0-9]+")
private val PORT = Regex("[0-9]{1,5}")
private val OCTET = Regex("0|[1-9][0-9]{0,2}")
private val LABEL = Regex("[a-z0-9]([a-z0-9-]*[a-z0-9])?")

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
