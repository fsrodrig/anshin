package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
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
            PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(live(current.value), "machine") }
        } }
        compose.onNodeWithText("0.0%").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("50.0%").assertDoesNotExist()
        compose.onRoot().captureFrame("disk-primary.${theme.key}.png")
        compose.runOnIdle { current.value = current.value.copy(filesystems = listOf(disks[1].copy(totalBytes = null, usedBytes = null, availableBytes = null, usedPct = null, ageS = 90.0))) }
        compose.onNodeWithText("0.0%").assertDoesNotExist()
        compose.onNodeWithText("Capacity age 90 s").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("disk-blocked.${theme.key}.png")
        compose.runOnIdle { current.value = Pulse(schema = 1, filesystems = disks) }
        compose.onNodeWithText("0.0%").assertDoesNotExist()
        compose.onNodeWithText("50.0%").assertDoesNotExist()
        compose.onNodeWithText("Capacity age 90 s").assertDoesNotExist()
    }

    @Test fun everyFilesystemScrollsAtLargeFontAndBackWorks() {
        compose.setContent { AnshinTheme(theme) {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
                PulseLayout("machine", onInputChange = {}, onWatch = {}) {
                    PulseView(live(Pulse(schema = 1, filesystems = disks, primaryDisk = "0:35")), "machine")
                }
            }
        } }
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        compose.onNodeWithText("/boot").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("50.0%").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("btrfs").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("0.0%").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("disk-detail-zero-large-font.${theme.key}.png")
        compose.onNodeWithText(longPath).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("disk-detail-long-path-large-font.${theme.key}.png")
        compose.onNodeWithText("90 s").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("disk-detail-blocked-large-font.${theme.key}.png")
        compose.onAllNodesWithText("Back to Machine")[1].performScrollTo().performClick()
        compose.onNodeWithText("Primary disk").performScrollTo().assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
