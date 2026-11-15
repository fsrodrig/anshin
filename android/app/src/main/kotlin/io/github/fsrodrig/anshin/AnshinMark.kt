package io.github.fsrodrig.anshin

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp

internal const val ANSHIN_MARK_TAG = "anshin-mark"

/**
 * The paired-frame mark in the active theme's accent. It identifies Anshin, so it takes no Verdict or connection
 * state: nothing a Machine reports can change it. Decorative, because the "anshin" text beside it names the app.
 */
@Composable
fun AnshinMark(modifier: Modifier = Modifier) {
    Image(
        painterResource(R.drawable.ic_anshin),
        contentDescription = null,
        modifier = modifier.testTag(ANSHIN_MARK_TAG).size(24.dp),
        colorFilter = ColorFilter.tint(AnshinTheme.colors.accent),
    )
}
