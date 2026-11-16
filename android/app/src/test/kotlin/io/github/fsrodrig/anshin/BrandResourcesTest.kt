package io.github.fsrodrig.anshin

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The app's `main` res source set includes `branding/android/res`, so the brand pack is never copied: a copy under
 * any `android/app/src/<source set>/res` would shadow or duplicate the generated resource and could drift from it.
 */
class BrandResourcesTest {
    private val brandRes = File(checkNotNull(System.getProperty("anshin.branding")) { "anshin.branding is unset" })
        .resolve("android/res")
    private val sources = checkNotNull(File(checkNotNull(System.getProperty("anshin.main")) { "anshin.main is unset" }).parentFile)

    @Test
    fun noSourceSetCopiesTheBrandPack() {
        assertTrue("the brand pack is empty", BrandCopies.pack(brandRes).isNotEmpty())
        assertEquals(emptyList<String>(), BrandCopies.find(brandRes, sources))
    }

    @Test
    fun aCopyUnderAnyQualifierOrSourceSetIsFound() {
        val dir = kotlin.io.path.createTempDirectory().toFile()
        try {
            dir.resolve("brand/drawable").apply { mkdirs() }.resolve("ic_anshin.xml").writeText("<vector/>")
            dir.resolve("brand/mipmap-anydpi").apply { mkdirs() }.resolve("ic_launcher.xml").writeText("<adaptive-icon/>")
            dir.resolve("src/main/res/drawable").apply { mkdirs() }.resolve("other.xml").writeText("<vector/>")
            dir.resolve("src/main/res/values").apply { mkdirs() }.resolve("ic_anshin.xml").writeText("<resources/>")
            assertEquals(emptyList<String>(), BrandCopies.find(dir.resolve("brand"), dir.resolve("src")))

            dir.resolve("src/debug/res/drawable-night").apply { mkdirs() }.resolve("ic_anshin.xml").writeText("<vector/>")
            dir.resolve("src/main/res/mipmap-anydpi-v33").apply { mkdirs() }.resolve("ic_launcher.xml").writeText("<x/>")
            assertEquals(
                listOf("debug/res/drawable-night/ic_anshin.xml", "main/res/mipmap-anydpi-v33/ic_launcher.xml"),
                BrandCopies.find(dir.resolve("brand"), dir.resolve("src")),
            )
        } finally {
            dir.deleteRecursively()
        }
    }
}

object BrandCopies {
    /** A resource's type is its directory name without qualifiers: `mipmap-anydpi-v33` holds `mipmap` resources. */
    private fun key(file: File) = checkNotNull(file.parentFile).name.substringBefore('-') to file.name

    fun pack(brandRes: File): Set<Pair<String, String>> = brandRes.walk().filter { it.isFile }.map(::key).toSet()

    fun find(brandRes: File, sources: File): List<String> {
        val pack = pack(brandRes)
        return sources.listFiles().orEmpty().map { it.resolve("res") }.filter { it.isDirectory }
            .flatMap { res -> res.walk().filter { it.isFile && it.parentFile?.parentFile == res && key(it) in pack } }
            .map { it.relativeTo(sources).invariantSeparatorsPath }
            .sorted()
    }
}
