package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollToKey
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Every Pulse in contract/fixtures, old ones included, draws a detail that says which sections it lacks. */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class DetailFixturesTest {
    @get:Rule val compose = createComposeRule()

    private val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })

    private fun state(name: String): MachineDetailState {
        val pulse = checkNotNull(decodePulse(File(dir, name).readText())) { "$name does not decode" }
        return DetailSamples.state(pulse = pulse, window = null.merge(pulse))
    }

    private fun assertInRow(key: String, text: String) {
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)
        compose.onNode(hasText(text) and (hasTestTag(key) or hasAnyAncestor(hasTestTag(key)))).assertIsDisplayed()
    }

    @Test fun everyFixtureRendersTheDetail() {
        val names = dir.walkTopDown().filter { it.isFile && it.extension == "json" }.map { it.relativeTo(dir).path }.sorted().toList()
        assertTrue("no fixtures in $dir", "desktop.json" in names)
        val current = mutableStateOf(state(names.first()))
        compose.setContent { AnshinTheme(Theme.Default) { MachineScreen(current.value) } }
        for (name in names) {
            compose.runOnIdle { current.value = state(name) }
            val items = detailItems(current.value)
            for (stop in DETAIL_SECTIONS) {
                compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(stop)
                compose.onNodeWithTag(stop).assertIsDisplayed()
            }
            for (unavailable in items.map { it.key }.filter { it.endsWith("-unavailable") }) {
                assertInRow(unavailable, UNAVAILABLE)
            }
        }

        compose.runOnIdle { current.value = state("desktop.json") }
        assertInRow("no-battery", "no battery")
        assertInRow("cpu-bars", "16 CPUs")
        val items = detailItems(current.value)
        val ifKey = items.first { it is DetailItem.ListRow && it.name.text == "enp2s0" }.key
        assertInRow(ifKey, "enp2s0")
        assertInRow("disk:0", "nvme0n1")

        val beforeMemory = detailItems(state("kept/2026-11-14/desktop.json")).map { it.key }
        assertTrue("memory-unavailable" in beforeMemory)
        assertTrue("network-unavailable" in beforeMemory)
        assertEquals(1, beforeMemory.count { it == "memory-unavailable" })
    }
}
