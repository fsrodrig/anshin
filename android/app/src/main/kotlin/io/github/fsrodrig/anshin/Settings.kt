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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

internal const val SETTINGS_TAG = "settings"
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
        Section("Theme")
        Column(Modifier.selectableGroup().testTag(THEME_GROUP_TAG), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Theme.entries.forEach { option -> ThemeOption(option, selected = option == theme) { actions.onTheme(option) } }
        }

        Section("Machines")
        if (machines.isEmpty()) Text("No Machines yet", color = colors.muted)
        Column {
            machines.forEach { machine ->
                Column(
                    Modifier
                        .fillMaxWidth()
                        .clickable(role = Role.Button, onClickLabel = "Edit ${machine.name}") { actions.onEdit(machine.id) }
                        .heightIn(min = 48.dp)
                        .padding(vertical = 6.dp),
                ) {
                    Text(machine.name, color = colors.text, fontSize = 17.sp)
                    Text(machine.address.toString(), color = colors.muted, style = AnshinTheme.type.mono, fontSize = 13.sp)
                }
            }
        }
        SecondaryButton("Add Machine", actions.onAdd)

        Section("About Anshin")
        Text("Your systems. At peace.", color = colors.text, fontSize = 17.sp)
        Text(
            "No accounts, no cloud, no telemetry. The phone talks only to your own Machines' Agents, over your tailnet, " +
                "and only while Anshin is on screen.",
            color = colors.muted,
        )

        Section("Version")
        if (version != null) Text(version, color = colors.text, style = AnshinTheme.type.mono)
        else Text("Development build", color = colors.text)

        Section("Open-source licences")
        Text("Anshin, its font, its theme colors and the libraries it links.", color = colors.muted)
        SecondaryButton("Show licences", actions.onLicences)
    }
}

@Composable
private fun Section(title: String) {
    Box(Modifier.padding(top = 8.dp).testTag("section-$title")) { Title(title) }
}

@Composable
private fun ThemeOption(theme: Theme, selected: Boolean, onSelect: () -> Unit) {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(8.dp)
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .border(1.dp, if (selected) colors.accent else colors.border, shape)
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .heightIn(min = 48.dp)
            .padding(horizontal = 4.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null, modifier = Modifier.padding(horizontal = 8.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(theme.label, color = colors.text)
            AnshinTheme(theme) { ThemePreview() }
        }
    }
}

/** The theme's own tokens: a panel on its background, its text, then one chip per color role. Decoration only. */
@Composable
private fun ThemePreview() {
    val colors = AnshinTheme.colors
    val shape = RoundedCornerShape(6.dp)
    Box(
        Modifier
            .padding(end = 12.dp)
            .fillMaxWidth()
            .clip(shape)
            .background(colors.background)
            .padding(6.dp)
            .clearAndSetSemantics {},
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .clip(shape)
                .background(colors.surface)
                .border(1.dp, colors.border, shape)
                .padding(horizontal = 8.dp, vertical = 6.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("desktop", color = colors.text, fontSize = 13.sp)
                Text("Live", color = colors.accent, fontSize = 13.sp)
                Text("42%", color = colors.muted, style = AnshinTheme.type.mono, fontSize = 13.sp)
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
    Box(Modifier.size(width = 18.dp, height = 8.dp).clip(RoundedCornerShape(2.dp)).background(color))
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
