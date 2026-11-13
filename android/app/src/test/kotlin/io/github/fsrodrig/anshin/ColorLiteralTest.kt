package io.github.fsrodrig.anshin

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import org.w3c.dom.Node

/**
 * No color is spelled out in the app's production code or resources except the theme tokens (`Themes.kt`) and the
 * unchanged copies of the shared brand resources in `branding/android/res`.
 */
class ColorLiteralTest {
    private val main = File(checkNotNull(System.getProperty("anshin.main")) { "anshin.main is unset" })
    private val brandRes = File(checkNotNull(System.getProperty("anshin.branding"))).resolve("android/res")

    @Test
    fun productionSourcesHaveNoColorLiterals() {
        val scanned = ColorLiterals.scan(main, brandRes)
        assertTrue("scanned ${scanned.files} files", scanned.files.any { it.endsWith("PulseScreen.kt") })
        assertTrue("scanned ${scanned.files} files", scanned.files.any { it.endsWith("themes.xml") })
        assertEquals(emptyList<String>(), scanned.findings)
    }

    @Test
    fun catchesKotlinColorLiterals() {
        for (code in listOf(
            "val c = Color(0xFF141519)",
            "val c = Color(0x141519)",
            "val c = Color(20, 21, 25)",
            "val c = Color (1f, 0f, 0f)",
            "val c = Color \n (red = 0.1f, green = 0.2f, blue = 0.3f)",
            "window.setBackgroundDrawable(ColorDrawable (1))",
            "val c = Color(red = 0.1f, green = 0.2f, blue = 0.3f)",
            "val c = Color.Red",
            "val c = Color.Transparent",
            "val c = android.graphics.Color.BLACK",
            "val c = Color.parseColor(\"#141519\")",
            "val c = Color.rgb(1, 2, 3)",
            "val c = \"#A5A8F5\"",
            "val c = 0xFF141519.toInt()",
            "window.setBackgroundDrawable(ColorDrawable(0x141519))",
        )) {
            assertTrue(code, ColorLiterals.kotlin(code, tokensFile = false).isNotEmpty())
        }
    }

    @Test
    fun tokensFileMayOnlyHoldOpaqueHexTokens() {
        assertEquals(emptyList<String>(), ColorLiterals.kotlin("background = Color(0xFF141519),", tokensFile = true))
        assertTrue(ColorLiterals.kotlin("val x = Color.Red", tokensFile = true).isNotEmpty())
        assertTrue(ColorLiterals.kotlin("val x = Color (0xFF141519)", tokensFile = true).isNotEmpty())
        assertTrue(ColorLiterals.kotlin("val x = Color(1, 2, 3)", tokensFile = true).isNotEmpty())
    }

    @Test
    fun ignoresCommentsAndProse() {
        for (code in listOf(
            "// Selectable themes arrive with #123",
            "/* see #ABC and Color.Red */ val x = 1",
            "val s = \"Issue #123 is fixed\"",
            "val c = AnshinTheme.colors.accent",
            "val h = 0x7f",
            "val code = \"http://example.com/#abc\"",
        )) {
            assertEquals(code, emptyList<String>(), ColorLiterals.kotlin(code, tokensFile = false))
        }
    }

    @Test
    fun catchesXmlColorLiterals() {
        for (xml in listOf(
            """<resources><color name="x">#141519</color></resources>""",
            """<resources><color name="x">@color/y</color></resources>""",
            """<vector android:strokeColor="#FFF" xmlns:android="http://schemas.android.com/apk/res/android"/>""",
            """<style xmlns:android="http://schemas.android.com/apk/res/android"><item name="android:windowBackground">@android:color/black</item></style>""",
            """<shape xmlns:android="http://schemas.android.com/apk/res/android"><solid android:color="#80141519"/></shape>""",
        )) {
            assertTrue(xml, ColorLiterals.xml(xml).isNotEmpty())
        }
    }

    @Test
    fun allowsXmlReferencesAndProse() {
        for (xml in listOf(
            """<resources><string name="x">Fixed in #123</string></resources>""",
            """<style xmlns:android="http://schemas.android.com/apk/res/android"><item name="android:windowBackground">@color/anshin_icon_background</item></style>""",
            """<!-- #141519 --><resources/>""",
        )) {
            assertEquals(xml, emptyList<String>(), ColorLiterals.xml(xml))
        }
    }

    @Test
    fun aChangedBrandCopyIsNoLongerAllowed() {
        val dir = kotlin.io.path.createTempDirectory().toFile()
        try {
            val res = dir.resolve("main/res/values").apply { mkdirs() }
            val brand = dir.resolve("brand/values").apply { mkdirs() }
            brand.resolve("anshin_colors.xml").writeText("""<resources><color name="a">#A5A8F5</color></resources>""")
            res.resolve("anshin_colors.xml").writeText("""<resources><color name="a">#A5A8F5</color></resources>""")
            assertEquals(emptyList<String>(), ColorLiterals.scan(dir.resolve("main"), dir.resolve("brand")).findings)
            res.resolve("anshin_colors.xml").writeText("""<resources><color name="a">#FF0000</color></resources>""")
            assertTrue(ColorLiterals.scan(dir.resolve("main"), dir.resolve("brand")).findings.isNotEmpty())
        } finally {
            dir.deleteRecursively()
        }
    }
}

object ColorLiterals {
    class Scan(val files: List<String>, val findings: List<String>)

    private const val TOKENS_FILE = "kotlin/io/github/fsrodrig/anshin/Themes.kt"
    private val hexColor = Regex("#([0-9A-Fa-f]{3,4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})")
    private val tokenLiteral = Regex("""Color\(0xFF[0-9A-F]{6}\)""")
    private val kotlinColorCode = listOf(
        // A Compose Color built from numbers, or any 24- or 32-bit hex number, which can only be a color here.
        Regex("""\bColor\s*\(\s*(0x|[0-9]|red\s*=|alpha\s*=)"""),
        Regex("""\b0x[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?\b"""),
        // Named colors: Compose's Color.Red, Android's Color.BLACK, Color.parseColor, Color.rgb, Color.valueOf…
        Regex("""\bColor\.(Black|DarkGray|Gray|LightGray|White|Red|Green|Blue|Yellow|Cyan|Magenta|Transparent)\b"""),
        Regex("""\bColor\.([A-Z]+|parseColor|rgb|argb|valueOf)\b"""),
        Regex("""\bColorDrawable\s*\("""),
    )

    fun scan(main: File, brandRes: File): Scan {
        val brandCopies = brandRes.walk().filter { it.isFile }.map { it.name to it.readBytes().toList() }.toSet()
        val files = main.walk().filter { it.isFile && it.extension in setOf("kt", "xml") }.sortedBy { it.path }.toList()
        val findings = files.flatMap { file ->
            val relative = file.relativeTo(main).invariantSeparatorsPath
            when {
                file.extension == "kt" -> kotlin(file.readText(), tokensFile = relative == TOKENS_FILE)
                (file.name to file.readBytes().toList()) in brandCopies -> emptyList()
                else -> xml(file.readText())
            }.map { "$relative: $it" }
        }
        return Scan(files.map { it.relativeTo(main).invariantSeparatorsPath }, findings)
    }

    /** Kotlin source, read as code and string literals with comments removed. */
    fun kotlin(source: String, tokensFile: Boolean): List<String> {
        val (code, strings) = splitKotlin(source)
        val tokens = if (tokensFile) tokenLiteral.findAll(code).map { it.range }.toList() else emptyList()
        val inCode = kotlinColorCode.flatMap { pattern -> pattern.findAll(code).toList() }
            .filterNot { match -> tokens.any { match.range.first >= it.first && match.range.last <= it.last } }
            .map { it.value }
        val inStrings = strings.filter { hexColor.matches(it.trim()) }
        return (inCode + inStrings).distinct()
    }

    /** XML attribute values and text, without comments. A color resource is a literal wherever it is defined. */
    fun xml(source: String): List<String> {
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        val document = factory.newDocumentBuilder().parse(source.byteInputStream())
        val found = mutableListOf<String>()
        fun visit(node: Node) {
            if (node is Element) {
                if (node.tagName == "color") found += "<color name=\"${node.getAttribute("name")}\">"
                val attributes = node.attributes
                for (i in 0 until attributes.length) check(attributes.item(i).nodeValue, found)
            }
            if (node.nodeType == Node.TEXT_NODE) check(node.nodeValue, found)
            val children = node.childNodes
            for (i in 0 until children.length) visit(children.item(i))
        }
        visit(document)
        return found
    }

    private fun check(value: String, found: MutableList<String>) {
        val v = value.trim()
        if (hexColor.matches(v) || v.startsWith("@android:color/")) found += v
    }

    /** Splits Kotlin source into code (string contents blanked) and the string literals' contents. */
    private fun splitKotlin(source: String): Pair<String, List<String>> {
        val code = StringBuilder()
        val strings = mutableListOf<String>()
        var i = 0
        while (i < source.length) {
            when {
                source.startsWith("//", i) -> i = source.indexOf('\n', i).let { if (it < 0) source.length else it }
                source.startsWith("/*", i) -> i = source.indexOf("*/", i + 2).let { if (it < 0) source.length else it + 2 }
                source.startsWith("\"\"\"", i) -> {
                    val end = source.indexOf("\"\"\"", i + 3).let { if (it < 0) source.length else it }
                    strings += source.substring(i + 3, end)
                    code.append("\"\"")
                    i = end + 3
                }
                source[i] == '"' -> {
                    var j = i + 1
                    while (j < source.length && source[j] != '"') j += if (source[j] == '\\') 2 else 1
                    strings += source.substring(i + 1, minOf(j, source.length))
                    code.append("\"\"")
                    i = j + 1
                }
                source[i] == '\'' && i + 2 < source.length -> {
                    val from = if (source[i + 1] == '\\') i + 3 else i + 2
                    val end = source.indexOf('\'', from).let { if (it < 0) source.length else it }
                    code.append("' '")
                    i = end + 1
                }
                else -> code.append(source[i++])
            }
        }
        return code.toString() to strings
    }
}
