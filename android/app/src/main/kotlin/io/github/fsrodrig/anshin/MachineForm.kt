package io.github.fsrodrig.anshin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import java.io.IOException
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch

internal const val NAME_FIELD_TAG = "machine-name"
internal const val ADDRESS_FIELD_TAG = "machine-address"
internal const val PORT_FIELD_TAG = "machine-port"

internal sealed interface FormPhase {
    data object Editing : FormPhase

    data class Testing(val address: String) : FormPhase

    data class Tested(val machine: Machine, val connection: Connection) : FormPhase

    data object Removed : FormPhase

    data object Failed : FormPhase

    data object RemovalFailed : FormPhase
}

/** Everything the form draws, so each state can be drawn on its own. */
internal data class MachineFormState(
    val name: String = "",
    val address: String = "",
    val port: String = "",
    val editing: Boolean = false,
    val saved: Boolean = editing,
    val problems: Problems = Problems(),
    val phase: FormPhase = FormPhase.Editing,
    val confirmingRemove: Boolean = false,
)

internal class MachineFormActions(
    val onName: (String) -> Unit = {},
    val onAddress: (String) -> Unit = {},
    val onPort: (String) -> Unit = {},
    val onSave: () -> Unit = {},
    val onDone: () -> Unit = {},
    val onRemove: () -> Unit = {},
    val onConfirmRemove: () -> Unit = {},
    val onKeep: () -> Unit = {},
)

/**
 * Add machine, or with [machine] Edit machine. Once saved, the form edits what it saved, so a second Save after a
 * correction changes the same Machine. Leaving cancels a test connection still running; the Machine stays saved.
 */
@Composable
internal fun MachineFormScreen(book: MachineBook, machine: Machine?, onDone: (MachineId?) -> Unit, onRemoved: () -> Unit) {
    var name by rememberSaveable { mutableStateOf(machine?.name.orEmpty()) }
    var address by rememberSaveable { mutableStateOf(machine?.address?.host.orEmpty()) }
    var port by rememberSaveable { mutableStateOf(machine?.address?.port?.takeIf { it != AGENT_PORT }?.toString().orEmpty()) }
    var savedId by rememberSaveable { mutableStateOf(machine?.id?.value) }
    var problems by remember { mutableStateOf(Problems()) }
    var phase by remember { mutableStateOf<FormPhase>(FormPhase.Editing) }
    var confirmingRemove by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var saveJob by remember { mutableStateOf<Job?>(null) }

    fun edited() {
        if (phase is FormPhase.Testing) {
            saveJob?.cancel()
            saveJob = null
        }
        phase = FormPhase.Editing
    }

    LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
        if (phase is FormPhase.Testing) edited()
    }

    fun save() {
        // Persistence must finish and set savedId before another Save can replace it.
        if (saveJob?.isActive == true && phase !is FormPhase.Testing) return
        edited()
        val input = MachineInput(name, address, port)
        val replacing = savedId?.let(::MachineId)
        val job = scope.launch(start = CoroutineStart.LAZY) {
            val current = currentCoroutineContext()[Job]
            val next = try {
                when (val result = book.save(input, replacing) { saved ->
                    savedId = saved.id.value
                    if (!lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) current?.cancel()
                    current?.ensureActive()
                    phase = FormPhase.Testing(saved.address.toString())
                }) {
                    is SaveResult.Saved -> {
                        if (saveJob === current) problems = Problems()
                        FormPhase.Tested(result.machine, result.connection)
                    }
                    is SaveResult.Rejected -> {
                        if (saveJob === current) problems = result.problems
                        FormPhase.Editing
                    }
                    SaveResult.NotFound -> FormPhase.Removed
                }
            } catch (_: IOException) {
                FormPhase.Failed
            }
            if (saveJob === current) phase = next
        }
        saveJob = job
        job.start()
    }

    MachineForm(
        MachineFormState(name, address, port, editing = machine != null, saved = savedId != null, problems, phase, confirmingRemove),
        MachineFormActions(
            onName = { name = it; problems = problems.copy(name = null); edited() },
            onAddress = { address = it; problems = problems.copy(address = null); edited() },
            onPort = { port = it; problems = problems.copy(port = null); edited() },
            onSave = ::save,
            onDone = { onDone(savedId?.let(::MachineId)) },
            onRemove = { confirmingRemove = true },
            onConfirmRemove = {
                val id = savedId?.let(::MachineId)
                scope.launch {
                    try {
                        if (id != null) book.remove(id)
                        onRemoved()
                    } catch (_: IOException) {
                        phase = FormPhase.RemovalFailed
                    }
                }
            },
            onKeep = { confirmingRemove = false },
        ),
    )
}

@Composable
internal fun MachineForm(state: MachineFormState, actions: MachineFormActions, topLine: () -> TopLine? = { null }) {
    val colors = AnshinTheme.colors
    val mono = LocalTextStyle.current.merge(AnshinTheme.type.mono)
    AppFrame(topLine) {
        Text(if (state.editing) "Edit machine" else "Add machine", color = colors.text, fontSize = 22.sp)
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = state.name,
                onValueChange = actions.onName,
                modifier = Modifier.fillMaxWidth().testTag(NAME_FIELD_TAG),
                label = { Text("Name") },
                placeholder = { Text("laptop") },
                isError = state.problems.name != null,
                supportingText = state.problems.name?.let { { Text(message(it)) } },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
            )
            OutlinedTextField(
                value = state.address,
                onValueChange = actions.onAddress,
                modifier = Modifier.fillMaxWidth().testTag(ADDRESS_FIELD_TAG),
                textStyle = mono,
                label = { Text("Address") },
                placeholder = { Text("MagicDNS name or tailnet IP") },
                isError = state.problems.address != null,
                supportingText = state.problems.address?.let { { Text(message(it)) } },
                singleLine = true,
                keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
            )
            OutlinedTextField(
                value = state.port,
                onValueChange = actions.onPort,
                modifier = Modifier.fillMaxWidth().testTag(PORT_FIELD_TAG),
                textStyle = mono,
                label = { Text("Port") },
                placeholder = { Text(AGENT_PORT.toString()) },
                isError = state.problems.port != null,
                supportingText = {
                    if (state.problems.port != null) Text("Enter a port from 1 to 65535.")
                    else Text(valueDetail("Leave empty for the Agent's port, ", AGENT_PORT.toString(), ""))
                },
                singleLine = true,
                keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { actions.onSave() }),
            )
        }
        Outcome(state.phase)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.phase != FormPhase.Removed) {
                Button(onClick = actions.onSave, enabled = state.phase !is FormPhase.Testing) { Text("Save") }
            }
            OutlinedButton(onClick = actions.onDone) {
                Text(if (state.saved || state.phase == FormPhase.Removed) "Done" else "Cancel")
            }
        }
        if (state.editing && state.phase != FormPhase.Removed) {
            if (state.confirmingRemove) {
                Message("Remove ${state.name.trim()}?", "Anshin stops watching it. Nothing changes on the Machine.")
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = actions.onConfirmRemove) { Text("Remove") }
                    OutlinedButton(onClick = actions.onKeep) { Text("Keep") }
                }
            } else {
                OutlinedButton(onClick = actions.onRemove) { Text("Remove machine") }
            }
        }
    }
}

@Composable
private fun Outcome(phase: FormPhase) {
    when (phase) {
        FormPhase.Editing -> Unit
        is FormPhase.Testing -> Text(valueDetail("Saved. Asking ", phase.address, " for a Pulse…"), color = AnshinTheme.colors.muted)
        is FormPhase.Tested -> {
            val address = phase.machine.address.toString()
            when (val connection = phase.connection) {
                Connection.Live -> Message("Live", valueDetail("", address, " answered with a Pulse. ${phase.machine.name} is on your dashboard."))
                Connection.Forbidden -> Message(
                    "Forbidden",
                    "Saved, but this Agent only answers its Owner's untagged devices. Anshin asks again about once a minute.",
                )
                Connection.Incompatible -> Message(
                    "Incompatible",
                    "Saved, but the Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.",
                )
                is Connection.Unreachable -> Message("Unreachable", when {
                    connection.outsideTailnet -> valueDetail("Saved, but ", address, " isn't a tailnet address, so Anshin won't connect to it.")
                    connection.httpStatus == 503 -> valueDetail("Saved, but the Agent on ", address, " can't ask tailscaled who this phone is.")
                    connection.httpStatus != null -> valueDetail("Saved, but ", address, " answered HTTP ${connection.httpStatus} instead of a Pulse.")
                    else -> valueDetail(
                        "Saved, but no Pulse came from ",
                        address,
                        ". The Machine may be off, its Agent stopped, or this phone off the tailnet. Anshin keeps trying.",
                    )
                })
            }
        }
        FormPhase.Removed -> Message("Removed", "This Machine was removed.")
        FormPhase.Failed -> Message("Not saved", "Anshin couldn't save this Machine. Try again.")
        FormPhase.RemovalFailed -> Message("Not removed", "Anshin couldn't remove this Machine. Try again.")
    }
}

@Composable
private fun message(problem: NameProblem): AnnotatedString = when (problem) {
    NameProblem.Blank -> AnnotatedString("Give this Machine a name.")
    NameProblem.TooLong -> AnnotatedString("Use at most $MAX_NAME_LENGTH characters.")
    NameProblem.NotOneLine -> AnnotatedString("Use one line of text.")
    is NameProblem.Taken -> AnnotatedString("Another Machine is already called ${problem.by.name}.")
}

@Composable
private fun message(problem: AddressProblem): AnnotatedString = when (problem) {
    AddressProblem.Blank -> AnnotatedString("Enter a MagicDNS name or a tailnet IP.")
    AddressProblem.Malformed -> AnnotatedString("Not a Machine address. Enter a MagicDNS name or a tailnet IP, without http:// or a path.")
    AddressProblem.HasPort -> AnnotatedString("Put the port in the Port field.")
    is AddressProblem.OutsideTailnet ->
        valueDetail("", problem.literal, " isn't a tailnet address. Tailnet IPs are in 100.64.0.0/10 or fd7a:115c:a1e0::/48.")
    is AddressProblem.Taken -> AnnotatedString("${problem.by.name} already uses this address and port.")
}
