package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * Every screen's frame: the mark, the name and the fleet's top line, then [content]. [topLine] is read here, so a
 * Machine's update recomposes the line without recomposing the screen under it.
 */
@Composable
internal fun AppFrame(topLine: () -> TopLine?, content: @Composable ColumnScope.() -> Unit) {
    val colors = AnshinTheme.colors
    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(colors.background)
            .safeDrawingPadding()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AnshinMark()
            Text("anshin", color = colors.text, fontSize = 20.sp)
        }
        TopLineSlot(topLine)
        content()
    }
}

@Composable
private fun TopLineSlot(topLine: () -> TopLine?) {
    topLine()?.let { TopLineView(it) }
}

/** One Machine's screen: its Machine name and address, a way back and to Edit, then [content]. */
@Composable
internal fun MachinePage(
    name: String,
    address: String,
    onBack: () -> Unit,
    onEdit: () -> Unit,
    topLine: () -> TopLine? = { null },
    content: @Composable () -> Unit,
) {
    val colors = AnshinTheme.colors
    AppFrame(topLine) {
        Column {
            TextButton(onClick = onBack) { Text("All Machines") }
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(name, color = colors.text, fontSize = 17.sp)
                    Text(address, color = colors.muted, style = AnshinTheme.type.mono, fontSize = 13.sp)
                }
                OutlinedButton(onClick = onEdit) { Text("Edit") }
            }
        }
        content()
    }
}
