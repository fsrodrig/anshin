package io.github.fsrodrig.anshin

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
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
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
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
    // Each screen's saved state (its scroll, a form's fields) lives under its key here, so Back finds the parent as it
    // was left, while a screen opened afresh starts from the top.
    val holder = rememberSaveableStateHolder()
    fun go(to: Screen) {
        if (to.back == shown && to != shown) holder.removeState(to.savedKey) else holder.removeState(shown.savedKey)
        screen = to
    }
    val back = { go(shown.back) }
    fun backTo(to: Screen) = Back(name(to, ::panel), back)
    BackHandler(enabled = shown != Screen.Dashboard, onBack = back)
    holder.SaveableStateProvider(shown.savedKey) {
        when (shown) {
            Screen.Dashboard -> Dashboard(
                panels,
                now = { now.value },
                onOpen = { go(Screen.OneMachine(it)) },
                onAdd = { go(Screen.Add(fromSettings = false)) },
                onSettings = { go(Screen.Settings) },
                topLine = topLine,
            )
            is Screen.OneMachine -> {
                val panel = checkNotNull(panel(shown.id))
                key(panel.machine.id) {
                    OneMachineScreen(panel, now = { now.value }, back = backTo(shown.back),
                        onEdit = { go(Screen.Edit(panel.machine.id, fromSettings = false)) })
                }
            }
            is Screen.Add -> MachineFormScreen(book, null, backTo(shown.back), onDone = { back() }, onRemoved = { back() })
            is Screen.Edit -> key(shown.id) {
                MachineFormScreen(book, panel(shown.id)?.machine, backTo(shown.back), onDone = { back() }, onRemoved = { go(shown.origin) })
            }
            Screen.Settings -> {
                val context = LocalContext.current
                val version = remember(context) { context.versionName() }
                Settings(
                    machines,
                    AnshinTheme.theme,
                    version,
                    SettingsActions(
                        onBack = back,
                        onTheme = { theme ->
                            scope.launch {
                                // A failed write leaves the stored theme, which the picker keeps showing; there is nothing to retry.
                                try { themeSetting.select(theme) } catch (_: IOException) {}
                            }
                        },
                        onEdit = { go(Screen.Edit(it, fromSettings = true)) },
                        onAdd = { go(Screen.Add(fromSettings = true)) },
                        onLicences = { go(Screen.Licences) },
                    ),
                )
            }
            Screen.Licences -> Licences(onBack = back, onOpen = { go(Screen.Licence(it.key)) })
            is Screen.Licence -> LicenceText(checkNotNull(openSourceLicence(shown.key)), onBack = back)
        }
    }
}

/** [panel]'s view is read in this scope, so a Machine's update never recomposes the app around it. */
@Composable
private fun OneMachineScreen(panel: Panel, now: () -> ComparableTimeMark, back: Back, onEdit: () -> Unit) {
    val machine = panel.machine
    MachineScreen(machine.name, machine.address.toString(), panel.view(), now, back, onEdit)
}

/** What a back arrow that lands on [screen] calls it. */
private fun name(screen: Screen, panel: (MachineId) -> Panel?): String = when (screen) {
    Screen.Dashboard -> "Dashboard"
    is Screen.OneMachine -> panel(screen.id)?.machine?.name ?: "Dashboard"
    is Screen.Add -> "Add Machine"
    is Screen.Edit -> "Edit Machine"
    Screen.Settings -> "Settings"
    Screen.Licences -> "Open-source licences"
    is Screen.Licence -> openSourceLicence(screen.key)?.name ?: "Open-source licences"
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

private val Screen.savedKey: String get() = when (this) {
    Screen.Dashboard -> "dashboard"
    is Screen.Add -> if (fromSettings) "settings-add" else "add"
    is Screen.OneMachine -> "machine:${id.value}"
    is Screen.Edit -> "${if (fromSettings) "settings-edit" else "edit"}:${id.value}"
    Screen.Settings -> "settings"
    Screen.Licences -> "licences"
    is Screen.Licence -> "licence:${this.key}"
}

private val ScreenSaver = Saver<Screen, String>(
    save = { it.savedKey },
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
