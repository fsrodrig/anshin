package io.github.fsrodrig.anshin

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

internal const val THEME_GROUP_TAG = "theme-group"

internal class SettingsActions(
    val onBack: () -> Unit = {},
    val onTheme: (Theme) -> Unit = {},
    val onEdit: (MachineId) -> Unit = {},
    val onAdd: () -> Unit = {},
    val onLicences: () -> Unit = {},
)

@Composable
internal fun Settings(machines: List<Machine>, theme: Theme, version: String?, actions: SettingsActions) {
    val colors = AnshinTheme.colors
    AnshinScreen("Settings", Back("Dashboard", actions.onBack)) {
        Column {
            Section("Theme", first = true) {
                Column(Modifier.selectableGroup().testTag(THEME_GROUP_TAG), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Theme.entries.forEach { option -> ThemeOption(option, selected = option == theme) { actions.onTheme(option) } }
                }
            }
            Section("Machines") {
                Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    if (machines.isEmpty()) {
                        Text("No Machines yet", color = colors.muted)
                    } else {
                        ListGroup {
                            machines.forEachIndexed { i, machine ->
                                ListRow(
                                    first = i == 0,
                                    title = machine.name,
                                    meta = AnnotatedString(machine.address.toString(), AnshinTheme.type.mono.toSpanStyle()),
                                    onClickLabel = "Edit ${machine.name}",
                                    onClick = { actions.onEdit(machine.id) },
                                )
                            }
                        }
                    }
                    SecondaryButton("Add Machine", actions.onAdd)
                }
            }
            Section("About") {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("Your systems. At peace.", color = colors.text)
                    Text(
                        "No accounts, no cloud, no telemetry. The phone talks only to your own Machines' Agents, over your tailnet, " +
                            "and only while Anshin is on screen.",
                        color = colors.muted,
                    )
                }
                Spacer(Modifier.height(16.dp))
                ListGroup {
                    VersionRow(version)
                    ListRow(
                        first = false,
                        title = "Open-source licences",
                        meta = AnnotatedString("Anshin, its font, theme colours and libraries"),
                        onClickLabel = "Open-source licences",
                        onClick = actions.onLicences,
                    )
                }
            }
        }
    }
}

/** A rule over every section but the first, then its heading, as on the Machine screen. */
@Composable
private fun Section(title: String, first: Boolean = false, content: @Composable () -> Unit) {
    Column {
        if (!first) {
            Spacer(Modifier.height(24.dp))
            Box(Modifier.fillMaxWidth().height(1.dp).background(AnshinTheme.colors.border))
            Spacer(Modifier.height(16.dp))
        }
        Text(
            title,
            color = AnshinTheme.colors.text,
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.testTag("section-$title").semantics { heading() },
        )
        Spacer(Modifier.height(12.dp))
        content()
    }
}

/** Rows on one surface panel, 1 dp rules between them. */
@Composable
internal fun ListGroup(content: @Composable () -> Unit) {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(8.dp)
    Column(Modifier.fillMaxWidth().clip(shape).background(colors.surface).border(1.dp, colors.border, shape)) { content() }
}

/** A row of a [ListGroup] that opens another screen, which its chevron says. [first] has no rule above it. */
@Composable
internal fun ListRow(first: Boolean, title: String, meta: AnnotatedString?, onClickLabel: String, onClick: () -> Unit) {
    val colors = AnshinTheme.colors
    RowRule(first)
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(role = Role.Button, onClickLabel = onClickLabel, onClick = onClick)
            .heightIn(min = 56.dp)
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f).padding(end = 8.dp)) {
            Text(title, color = colors.text)
            meta?.let { Text(it, color = colors.muted, style = MaterialTheme.typography.bodySmall) }
        }
        Glyph(AnshinIcon.Chevron, colors.muted, contentDescription = null)
    }
}

@Composable
private fun RowRule(first: Boolean) {
    if (!first) Box(Modifier.fillMaxWidth().height(1.dp).background(AnshinTheme.colors.border))
}

/** Read-only, so no chevron and no click. A build without a release number says so instead of a version. */
@Composable
private fun VersionRow(version: String?) {
    val colors = AnshinTheme.colors
    FlowRow(
        Modifier.fillMaxWidth().heightIn(min = 56.dp).padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalArrangement = Arrangement.Center,
    ) {
        Text("Version", color = colors.text, modifier = Modifier.padding(end = 8.dp).align(Alignment.CenterVertically))
        if (version != null) {
            Text(version, color = colors.text, style = AnshinTheme.type.mono, modifier = Modifier.align(Alignment.CenterVertically))
        } else {
            Text("Development build", color = colors.text, modifier = Modifier.align(Alignment.CenterVertically))
        }
    }
}

@Composable
private fun ThemeOption(theme: Theme, selected: Boolean, onSelect: () -> Unit) {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(8.dp)
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(colors.surface)
            .border(1.dp, if (selected) colors.accent else colors.border, shape)
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .heightIn(min = 48.dp)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(theme.label, color = colors.text)
            AnshinTheme(theme) { ThemePreview() }
        }
    }
}

/** The theme's own tokens: a panel on its background, its text, then one chip per colour role. Decoration only. */
@Composable
private fun ThemePreview() {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(4.dp)
    Box(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(colors.background)
            .padding(8.dp)
            .clearAndSetSemantics {},
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .clip(shape)
                .background(colors.surface)
                .border(1.dp, colors.border, shape)
                .padding(horizontal = 8.dp, vertical = 4.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("desktop", color = colors.text, fontSize = 13.sp, lineHeight = 18.sp)
                Text("Live", color = colors.accent, fontSize = 13.sp, lineHeight = 18.sp)
                Text("42 %", color = colors.muted, style = AnshinTheme.type.mono, fontSize = 13.sp, lineHeight = 18.sp)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                listOf(colors.cpu, colors.memory, colors.storage, colors.network).forEach { Chip(it) }
                Box(Modifier.size(4.dp))
                listOf(colors.ok, colors.warn, colors.crit, colors.unavailable).forEach { Chip(it) }
            }
        }
    }
}

@Composable
private fun Chip(color: Color) {
    Box(Modifier.size(width = 16.dp, height = 8.dp).clip(RoundedCornerShape(2.dp)).background(color))
}

internal fun Context.versionName(): String? {
    val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        packageManager.getPackageInfo(packageName, PackageManager.PackageInfoFlags.of(0))
    } else {
        @Suppress("DEPRECATION")
        packageManager.getPackageInfo(packageName, 0)
    }
    return info.versionName?.takeIf { it.isNotBlank() }
}
