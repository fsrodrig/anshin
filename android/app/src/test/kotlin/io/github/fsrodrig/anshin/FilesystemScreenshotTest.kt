package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollTo
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

    @Test fun primarySelectionAndReplacement() {
        val current = mutableStateOf(Pulse(schema = 1, hostname = "machine", filesystems = disks, primaryDisk = "0:35"))
        compose.setContent { AnshinTheme(theme) {
            MachineScreen() { PulseView(live(current.value), "machine") }
        } }
        compose.onNodeWithText("0.0%", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("50.0%", useUnmergedTree = true).assertDoesNotExist()
        compose.onRoot().captureFrame("disk-primary.${theme.key}.png")
        compose.runOnIdle { current.value = current.value.copy(filesystems = listOf(disks[1].copy(totalBytes = null, usedBytes = null, availableBytes = null, usedPct = null, ageS = 90.0))) }
        compose.onNodeWithText("0.0%", useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithText("Capacity age 90 s", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("disk-blocked.${theme.key}.png")
        compose.runOnIdle { current.value = Pulse(schema = 1, filesystems = disks) }
        compose.onNodeWithText("0.0%", useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithText("50.0%", useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithText("Capacity age 90 s", useUnmergedTree = true).assertDoesNotExist()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
