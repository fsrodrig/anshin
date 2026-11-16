package io.github.fsrodrig.anshin

import androidx.activity.compose.BackHandler
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
import androidx.compose.runtime.State
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import java.io.IOException
import kotlin.time.ComparableTimeMark
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeSource
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.launch
import okhttp3.HttpUrl

/**
 * The app: every saved Machine watched at once, above navigation, so each Machine has exactly one collector whichever
 * screen shows, and moving between the Dashboard and a Machine never restarts a long-poll. Each Machine is collected on
 * its own, so one that never answers can't hold back another.
 */
@Composable
fun AnshinApp(book: MachineBook, watcher: Watcher, themeSetting: ThemeSetting) {
    val saved by book.machines.collectAsStateWithLifecycle(initialValue = null)
    // As in AnshinTheme: the launch background stays until the store is read, rather than flashing "No Machines yet".
    val machines = saved ?: return
    // A new address starts a new Flow, so a Window never shows under another address; a new name keeps the connection.
    val panels = machines.map { machine ->
        key(machine.id, machine.address) {
            val view = watch(watcher, machine.address.pulseUrl)
            Panel(machine) { view.value }
        }
    }
    val now = ticker(watcher.clock)
    val topLine = { topLine(panels.map { WatchedMachine(it.machine.name, it.view()) }) }
    var screen by rememberSaveable(stateSaver = ScreenSaver) { mutableStateOf<Screen>(Screen.Dashboard) }
    fun panel(id: MachineId) = panels.firstOrNull { it.machine.id == id }
    // A Machine removed under its screen falls back to where that screen was opened from.
    val shown = when (val wanted = screen) {
        is Screen.OneMachine -> wanted.takeIf { panel(it.id) != null } ?: Screen.Dashboard
        is Screen.Edit -> wanted.takeIf { panel(it.id) != null } ?: wanted.origin
        is Screen.Licence -> wanted.takeIf { openSourceLicence(it.key) != null } ?: Screen.Licences
        Screen.Dashboard, is Screen.Add, Screen.Settings, Screen.Licences -> wanted
    }

    val scope = rememberCoroutineScope()
    BackHandler(enabled = shown != Screen.Dashboard) { screen = shown.back }
    when (shown) {
        Screen.Dashboard -> Dashboard(
            panels,
            now = { now.value },
            onOpen = { screen = Screen.OneMachine(it) },
            onAdd = { screen = Screen.Add(fromSettings = false) },
            onSettings = { screen = Screen.Settings },
            topLine = topLine,
        )
        is Screen.OneMachine -> {
            val panel = checkNotNull(panel(shown.id))
            key(panel.machine.id) {
                OneMachineScreen(panel, now = { now.value }, onBack = { screen = Screen.Dashboard },
                    onEdit = { screen = Screen.Edit(panel.machine.id, fromSettings = false) }, topLine = topLine)
            }
        }
        is Screen.Add -> MachineFormScreen(book, null, onDone = { screen = shown.back }, onRemoved = { screen = shown.back })
        is Screen.Edit -> key(shown.id) {
            MachineFormScreen(book, panel(shown.id)?.machine, onDone = { screen = shown.back }, onRemoved = { screen = shown.origin })
        }
        Screen.Settings -> {
            val context = LocalContext.current
            val version = remember(context) { context.versionName() }
            Settings(
                machines,
                AnshinTheme.theme,
                version,
                SettingsActions(
                    onBack = { screen = Screen.Dashboard },
                    onTheme = { theme ->
                        scope.launch {
                            // A failed write leaves the stored theme, which the picker keeps showing; there is nothing to retry.
                            try { themeSetting.select(theme) } catch (_: IOException) {}
                        }
                    },
                    onEdit = { screen = Screen.Edit(it, fromSettings = true) },
                    onAdd = { screen = Screen.Add(fromSettings = true) },
                    onLicences = { screen = Screen.Licences },
                ),
            )
        }
        Screen.Licences -> Licences(onBack = { screen = Screen.Settings }, onOpen = { screen = Screen.Licence(it.key) })
        is Screen.Licence -> LicenceText(checkNotNull(openSourceLicence(shown.key)), onBack = { screen = Screen.Licences })
    }
}

/**
 * One Machine's screen, and from its card the detail. The detail's BackHandler, composed after the app's, takes Back
 * first, so Back from the detail returns here and Back from here returns to the Dashboard. [panel]'s view is read in
 * this scope, so a Machine's update never recomposes the app around it.
 */
@Composable
private fun OneMachineScreen(
    panel: Panel,
    now: () -> ComparableTimeMark,
    onBack: () -> Unit,
    onEdit: () -> Unit,
    topLine: () -> TopLine?,
) {
    val machine = panel.machine
    val address = machine.address.toString()
    val view = panel.view()
    MachineScreens(machine.name, view, address, now) { openDetail ->
        MachinePage(machine.name, address, onBack, onEdit, topLine) {
            PulseView(view, machine.name, address, onDetail = openDetail, now = now)
        }
    }
}

private sealed interface Screen {
    val back: Screen

    data object Dashboard : Screen {
        override val back get() = Dashboard
    }

    data class OneMachine(val id: MachineId) : Screen {
        override val back get() = Dashboard
    }

    data class Add(val fromSettings: Boolean) : Screen {
        override val back get() = if (fromSettings) Settings else Dashboard
    }

    /** Done goes [back]; removing the Machine goes to [origin], since its Machine screen is gone. */
    data class Edit(val id: MachineId, val fromSettings: Boolean) : Screen {
        val origin: Screen get() = if (fromSettings) Settings else Dashboard
        override val back get() = if (fromSettings) Settings else OneMachine(id)
    }

    data object Settings : Screen {
        override val back get() = Dashboard
    }

    data object Licences : Screen {
        override val back get() = Settings
    }

    data class Licence(val key: String) : Screen {
        override val back get() = Licences
    }
}

private val ScreenSaver = Saver<Screen, String>(
    save = {
        when (it) {
            Screen.Dashboard -> "dashboard"
            is Screen.Add -> if (it.fromSettings) "settings-add" else "add"
            is Screen.OneMachine -> "machine:${it.id.value}"
            is Screen.Edit -> "${if (it.fromSettings) "settings-edit" else "edit"}:${it.id.value}"
            Screen.Settings -> "settings"
            Screen.Licences -> "licences"
            is Screen.Licence -> "licence:${it.key}"
        }
    },
    restore = {
        when {
            it == "add" -> Screen.Add(fromSettings = false)
            it == "settings-add" -> Screen.Add(fromSettings = true)
            it == "settings" -> Screen.Settings
            it == "licences" -> Screen.Licences
            it.startsWith("machine:") -> Screen.OneMachine(MachineId(it.substringAfter(':')))
            it.startsWith("edit:") -> Screen.Edit(MachineId(it.substringAfter(':')), fromSettings = false)
            it.startsWith("settings-edit:") -> Screen.Edit(MachineId(it.substringAfter(':')), fromSettings = true)
            it.startsWith("licence:") -> Screen.Licence(it.substringAfter(':'))
            else -> Screen.Dashboard
        }
    },
)

/**
 * Collection, and with it every request, stops whenever the app is not visible. The Flow is remembered, so coming
 * back resumes with the held Window and Cursor, and the Watcher shows Stale rather than an old Live. Merging the
 * Window copies every series, so the Watcher runs off the UI thread.
 */
@Composable
private fun watch(watcher: Watcher, endpoint: HttpUrl): State<MachineView> {
    val flow = remember(endpoint) { watcher.watch(endpoint).flowOn(Dispatchers.Default) }
    return flow.collectAsStateWithLifecycle(initialValue = MachineView.Connecting)
}

/** The Watcher's clock, read once a second while the app is visible, so ages count up and stop in the background. */
@Composable
private fun ticker(clock: TimeSource.WithComparableMarks): State<ComparableTimeMark> {
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    return produceState(clock.markNow(), clock, lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                value = clock.markNow()
                delay(1.seconds)
            }
        }
    }
}

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
        // The window title already names the app, so TalkBack starts at the top line rather than the lockup.
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AnshinMark()
            Text("anshin", color = colors.text, fontSize = 20.sp, modifier = Modifier.semantics { hideFromAccessibility() })
        }
        TopLineSlot(topLine)
        content()
    }
}

@Composable
private fun TopLineSlot(topLine: () -> TopLine?) {
    topLine()?.let { TopLineView(it, Modifier.semantics(mergeDescendants = true) { heading() }) }
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
