package io.github.fsrodrig.anshin

import androidx.compose.ui.graphics.Color
import kotlin.math.abs
import kotlin.math.atan2
import kotlin.math.cbrt
import kotlin.math.cos
import kotlin.math.exp
import kotlin.math.hypot
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/** WCAG 2.2 contrast and CIEDE2000 color difference, for the theme checks and the token sheet. */
object ColorMath {
    fun channels(c: Color) = doubleArrayOf(c.red.toDouble(), c.green.toDouble(), c.blue.toDouble())

    fun linear(v: Double) = if (v <= 0.04045) v / 12.92 else ((v + 0.055) / 1.055).pow(2.4)

    fun luminance(c: Color): Double {
        val (r, g, b) = channels(c).map(::linear)
        return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }

    fun contrast(a: Color, b: Color): Double {
        val (hi, lo) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (hi + 0.05) / (lo + 0.05)
    }

    /** sRGB to CIELAB under D65. */
    fun lab(c: Color): DoubleArray {
        val (r, g, b) = channels(c).map(::linear)
        val x = (0.4124564 * r + 0.3575761 * g + 0.1804375 * b) / 0.95047
        val y = 0.2126729 * r + 0.7151522 * g + 0.0721750 * b
        val z = (0.0193339 * r + 0.1191920 * g + 0.9503041 * b) / 1.08883
        fun f(t: Double) = if (t > 216.0 / 24389) cbrt(t) else t * 24389 / 27 / 116 + 16.0 / 116
        return doubleArrayOf(116 * f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z)))
    }

    fun hueDifference(a: DoubleArray, b: DoubleArray): Double {
        val d = abs(Math.toDegrees(atan2(a[2], a[1])) - Math.toDegrees(atan2(b[2], b[1]))) % 360
        return if (d > 180) 360 - d else d
    }

    fun deltaE2000(lab1: DoubleArray, lab2: DoubleArray): Double {
        val (l1, a1, b1) = lab1.toList()
        val (l2, a2, b2) = lab2.toList()
        val cBar = (hypot(a1, b1) + hypot(a2, b2)) / 2
        val g = 0.5 * (1 - sqrt(cBar.pow(7) / (cBar.pow(7) + 25.0.pow(7))))
        val a1p = (1 + g) * a1
        val a2p = (1 + g) * a2
        val c1p = hypot(a1p, b1)
        val c2p = hypot(a2p, b2)
        fun hue(b: Double, a: Double) = if (a == 0.0 && b == 0.0) 0.0 else (Math.toDegrees(atan2(b, a)) + 360) % 360
        val h1p = hue(b1, a1p)
        val h2p = hue(b2, a2p)
        val dhp = when {
            c1p * c2p == 0.0 -> 0.0
            abs(h2p - h1p) <= 180 -> h2p - h1p
            h2p - h1p > 180 -> h2p - h1p - 360
            else -> h2p - h1p + 360
        }
        val dLp = l2 - l1
        val dCp = c2p - c1p
        val dHp = 2 * sqrt(c1p * c2p) * sin(Math.toRadians(dhp / 2))
        val lBarP = (l1 + l2) / 2
        val cBarP = (c1p + c2p) / 2
        val hBarP = when {
            c1p * c2p == 0.0 -> h1p + h2p
            abs(h1p - h2p) <= 180 -> (h1p + h2p) / 2
            h1p + h2p < 360 -> (h1p + h2p + 360) / 2
            else -> (h1p + h2p - 360) / 2
        }
        val t = 1 - 0.17 * cos(Math.toRadians(hBarP - 30)) + 0.24 * cos(Math.toRadians(2 * hBarP)) +
            0.32 * cos(Math.toRadians(3 * hBarP + 6)) - 0.20 * cos(Math.toRadians(4 * hBarP - 63))
        val dTheta = 30 * exp(-((hBarP - 275) / 25).pow(2))
        val rC = 2 * sqrt(cBarP.pow(7) / (cBarP.pow(7) + 25.0.pow(7)))
        val sL = 1 + 0.015 * (lBarP - 50).pow(2) / sqrt(20 + (lBarP - 50).pow(2))
        val sC = 1 + 0.045 * cBarP
        val sH = 1 + 0.015 * cBarP * t
        val rT = -sin(Math.toRadians(2 * dTheta)) * rC
        return sqrt(
            (dLp / sL).pow(2) + (dCp / sC).pow(2) + (dHp / sH).pow(2) + rT * (dCp / sC) * (dHp / sH),
        )
    }

    fun deltaE2000(a: Color, b: Color) = deltaE2000(lab(a), lab(b))

    fun hueDifference(a: Color, b: Color) = hueDifference(lab(a), lab(b))

    fun chroma(c: Color) = lab(c).let { hypot(it[1], it[2]) }
}
