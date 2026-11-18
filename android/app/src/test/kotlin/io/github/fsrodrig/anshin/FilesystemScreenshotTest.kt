package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollToKey
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class FilesystemScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private val longPath = "/mnt/archive/very-long-filesystem-mount-point/with-more-components"
    private val disks = listOf(
        Filesystem("8:1", "/boot", "ext4", 1024, 512, 512, 50.0, 0.0),
        Filesystem("0:35", "/", "btrfs", 4096, 0, 4096, 0.0, 0.0),
        Filesystem("8:2", longPath, "xfs", ageS = 90.0),
    )

    // A pinned section heading covers the item a scroll brings to the top, so each shot scrolls to the item above it.
    private fun scrollTo(key: String) = compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)

    @Test fun primarySelectionAndReplacement() {
        val current = mutableStateOf(Pulse(schema = 1, hostname = "machine", filesystems = disks, primaryDisk = "0:35"))
        compose.setContent { AnshinTheme(theme) { MachineScreen(live(current.value)) } }
        scrollTo("storage")
        compose.onNode(inItem("fs:1", "0.0 %")).assertIsDisplayed()
        compose.onNode(inItem("fs:1", "primary", substring = true)).assertIsDisplayed()
        compose.onAllNodes(hasText("primary", substring = true)).assertCountEquals(1)
        compose.onRoot().captureFrame("disk-primary.${theme.key}.png")
        compose.runOnIdle { current.value = current.value.copy(filesystems = listOf(disks[1].copy(totalBytes = null, usedBytes = null, availableBytes = null, usedPct = null, ageS = 90.0))) }
        scrollTo("storage")
        compose.onNode(inItem("fs:0", UNAVAILABLE)).assertIsDisplayed()
        compose.onNode(inItem("fs:0", "Capacity age 90 s")).assertIsDisplayed()
        compose.onAllNodes(hasText("0.0 %")).assertCountEquals(0)
        compose.onRoot().captureFrame("disk-blocked.${theme.key}.png")
        compose.runOnIdle { current.value = Pulse(schema = 1, filesystems = disks) }
        scrollTo("storage")
        compose.onAllNodes(hasText("primary", substring = true)).assertCountEquals(0)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
