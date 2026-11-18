package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.launch

internal const val NAME_FIELD_TAG = "machine-name"
internal const val ADDRESS_FIELD_TAG = "machine-address"
internal const val PORT_FIELD_TAG = "machine-port"

internal sealed interface FormPhase {
    data object Editing : FormPhase

    data class Testing(val address: String) : FormPhase

    data class Tested(val machine: Machine, val connection: TestConnection) : FormPhase

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
    /** The name the Machine is stored under, which the Remove question names; [name] is the field's unsaved text. */
    val savedName: String = name.trim(),
    /** Counts Saves the book rejected, so each one brings its first rejected field into view once. */
    val rejections: Int = 0,
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
 * Add Machine, or with [machine] Edit Machine. [back] is where the bar's arrow and system Back go. Once saved, the form edits what it saved, so a second Save after a
 * correction changes the same Machine. Leaving cancels a test connection still running; the Machine stays saved.
 */
@Composable
internal fun MachineFormScreen(book: MachineBook, machine: Machine?, back: Back, onDone: (MachineId?) -> Unit, onRemoved: () -> Unit) {
    var name by rememberSaveable { mutableStateOf(machine?.name.orEmpty()) }
    var address by rememberSaveable { mutableStateOf(machine?.address?.host.orEmpty()) }
    var port by rememberSaveable { mutableStateOf(machine?.address?.port?.takeIf { it != AGENT_PORT }?.toString().orEmpty()) }
    var savedId by rememberSaveable { mutableStateOf(machine?.id?.value) }
    var savedName by rememberSaveable { mutableStateOf(machine?.name.orEmpty()) }
    var rejections by remember { mutableStateOf(0) }
    var problems by remember { mutableStateOf(Problems()) }
    var phase by remember { mutableStateOf<FormPhase>(FormPhase.Editing) }
    var confirmingRemove by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var writeJob by remember { mutableStateOf<Job?>(null) }

    var testingJob by remember { mutableStateOf<Job?>(null) }
    var outcomeGeneration by remember { mutableStateOf(0L) }

    fun invalidate() {
        outcomeGeneration++
        testingJob?.cancel()
        testingJob = null
    }

    fun edited() {
        invalidate()
        phase = FormPhase.Editing
    }

    LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
        invalidate()
        if (phase is FormPhase.Testing) phase = FormPhase.Editing
    }

    fun save() {
        edited()
        val requested = outcomeGeneration
        val previous = writeJob
        val input = MachineInput(name, address, port)
        val job = scope.launch(start = CoroutineStart.LAZY) {
            val current = currentCoroutineContext()[Job]
            // Accepted writes survive edits and ON_STOP; savedId must be published before the next write.
            previous?.join()
            val next = try {
                when (val result = book.save(input, savedId?.let(::MachineId)) { saved ->
                    savedId = saved.id.value
                    savedName = saved.name
                    if (outcomeGeneration != requested || !lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) {
                        throw CancellationException("Test connection superseded")
                    }
                    testingJob = current
                    phase = FormPhase.Testing(saved.address.toString())
                }) {
                    is SaveResult.Saved -> {
                        if (outcomeGeneration == requested && writeJob === current) problems = Problems()
                        FormPhase.Tested(result.machine, result.connection)
                    }
                    is SaveResult.Rejected -> {
                        if (outcomeGeneration == requested && writeJob === current) {
                            problems = result.problems
                            rejections++
                        }
                        FormPhase.Editing
                    }
                    SaveResult.NotFound -> FormPhase.Removed
                }
            } catch (_: IOException) {
                FormPhase.Failed
            } finally {
                if (testingJob === current) testingJob = null
            }
            if (outcomeGeneration == requested && writeJob === current) phase = next
        }
        writeJob = job
        job.start()
    }

    MachineForm(
        MachineFormState(
            name, address, port, editing = machine != null, saved = savedId != null, problems, phase, confirmingRemove,
            savedName = savedName, rejections = rejections,
        ),
        MachineFormActions(
            onName = { name = it; problems = problems.copy(name = null); edited() },
            onAddress = { address = it; problems = problems.copy(address = null); edited() },
            onPort = { port = it; problems = problems.copy(port = null); edited() },
            onSave = ::save,
            onDone = { onDone(savedId?.let(::MachineId)) },
            onRemove = { confirmingRemove = true },
            onConfirmRemove = {
                invalidate()
                val previous = writeJob
                val job = scope.launch(start = CoroutineStart.LAZY) {
                    val current = currentCoroutineContext()[Job]
                    previous?.join()
                    try {
                        savedId?.let { book.remove(MachineId(it)) }
                        onRemoved()
                    } catch (_: IOException) {
                        if (writeJob === current) phase = FormPhase.RemovalFailed
                    }
                }
                writeJob = job
                job.start()
            },
            onKeep = { confirmingRemove = false },
        ),
        back,
    )
}

@Composable
internal fun MachineForm(state: MachineFormState, actions: MachineFormActions, back: Back) {
    val mono = LocalTextStyle.current.merge(AnshinTheme.type.mono)
    val fields = anshinTextFieldColors()
    val nameField = remember { FieldFocus() }
    val addressField = remember { FieldFocus() }
    val portField = remember { FieldFocus() }
    // The field and its message can sit under the keyboard after a Save from the last field, so the first one rejected
    // is focused and scrolled to; only a new rejection does it, not a problem cleared by typing.
    LaunchedEffect(state.rejections) {
        if (state.rejections == 0) return@LaunchedEffect
        val first = when {
            state.problems.name != null -> nameField
            state.problems.address != null -> addressField
            state.problems.port != null -> portField
            else -> return@LaunchedEffect
        }
        first.focus.requestFocus()
        first.view.bringIntoView()
    }
    AnshinScreen(if (state.editing) "Edit Machine" else "Add Machine", back) {
        Column {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                OutlinedTextField(
                    value = state.name,
                    onValueChange = actions.onName,
                    modifier = Modifier.fillMaxWidth().testTag(NAME_FIELD_TAG).then(nameField.modifier),
                    shape = ControlShape,
                    colors = fields,
                    label = { Text("Machine name") },
                    placeholder = { Text("laptop") },
                    isError = state.problems.name != null,
                    supportingText = state.problems.name?.let { { Text(message(it)) } },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                )
                OutlinedTextField(
                    value = state.address,
                    onValueChange = actions.onAddress,
                    modifier = Modifier.fillMaxWidth().testTag(ADDRESS_FIELD_TAG).then(addressField.modifier),
                    shape = ControlShape,
                    colors = fields,
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
                    modifier = Modifier.fillMaxWidth().testTag(PORT_FIELD_TAG).then(portField.modifier),
                    shape = ControlShape,
                    colors = fields,
                    textStyle = mono,
                    label = { Text("Port") },
                    placeholder = { Text(AGENT_PORT.toString(), style = mono) },
                    isError = state.problems.port != null,
                    supportingText = {
                        if (state.problems.port != null) Text(valueDetail("Enter a port from ", "1", " to ") + valueDetail("", "65535", ""))
                        else Text(valueDetail("Leave empty for the Agent's port, ", AGENT_PORT.toString(), ""))
                    },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { actions.onSave() }),
                )
            }
            if (state.phase != FormPhase.Editing) {
                Spacer(Modifier.height(24.dp))
                Outcome(state.phase)
            }
            // While the Remove question is open its answers are the screen's one row of actions, so Save can't compete.
            if (!state.confirmingRemove) {
                Spacer(Modifier.height(24.dp))
                Actions(state.phase, actions)
            }
            if (state.editing && state.phase != FormPhase.Removed) {
                Spacer(Modifier.height(32.dp))
                Box(Modifier.fillMaxWidth().height(1.dp).background(AnshinTheme.colors.border))
                Spacer(Modifier.height(16.dp))
                if (state.confirmingRemove) {
                    Message("Remove ${state.savedName}?", "Anshin stops watching it. Nothing changes on the Machine.")
                    Spacer(Modifier.height(24.dp))
                    ButtonRow {
                        SecondaryButton("Remove", actions.onConfirmRemove)
                        QuietButton("Keep", actions.onKeep)
                    }
                } else {
                    SecondaryButton("Remove Machine", actions.onRemove)
                }
            }
        }
    }
}

private class FieldFocus {
    val focus = FocusRequester()
    val view = BringIntoViewRequester()
    val modifier = Modifier.focusRequester(focus).bringIntoViewRequester(view)
}

/**
 * At most one filled button. Before a save it is Save, with Cancel to leave; once a test connection has reported and
 * nothing was edited since, leaving is the likely next step, so Done is filled and Save stays for another try.
 * While the test runs the Machine is already saved, so leaving is Done, not Cancel.
 */
@Composable
private fun Actions(phase: FormPhase, actions: MachineFormActions) {
    ButtonRow {
        when (phase) {
            FormPhase.Removed -> PrimaryButton("Done", actions.onDone)
            is FormPhase.Tested -> {
                PrimaryButton("Done", actions.onDone)
                SecondaryButton("Save", actions.onSave)
            }
            is FormPhase.Testing -> {
                PrimaryButton("Save", actions.onSave, enabled = false)
                SecondaryButton("Done", actions.onDone)
            }
            FormPhase.Editing, FormPhase.Failed, FormPhase.RemovalFailed -> {
                PrimaryButton("Save", actions.onSave)
                SecondaryButton("Cancel", actions.onDone)
            }
        }
    }
}

@Composable
private fun ButtonRow(content: @Composable () -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
}

@Composable
private fun Outcome(phase: FormPhase) {
    when (phase) {
        FormPhase.Editing -> Unit
        is FormPhase.Testing -> Text(valueDetail("Saved. Asking ", phase.address, " for a Pulse…"), color = AnshinTheme.colors.muted)
        is FormPhase.Tested -> {
            val address = phase.machine.address.toString()
            when (val connection = phase.connection) {
                TestConnection.Live -> Message("Live", valueDetail("", address, " answered with a Pulse. ${phase.machine.name} is on your Dashboard."))
                TestConnection.Forbidden -> Message(
                    "Forbidden",
                    "Saved, but this Agent only answers its Owner's untagged devices. Anshin asks again about once a minute.",
                )
                TestConnection.Incompatible -> Message(
                    "Incompatible",
                    "Saved, but the Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.",
                )
                is TestConnection.Unreachable -> Message("Unreachable", when {
                    connection.outsideTailnet -> valueDetail("Saved, but ", address, " isn't a tailnet address, so Anshin won't connect to it.")
                    connection.httpStatus == 503 -> valueDetail("Saved, but the Agent on ", address, " can't ask tailscaled who this phone is.")
                    connection.httpStatus == 421 -> valueDetail("Saved, but the Agent doesn't answer to ", address, ". Use the Machine's tailnet address or MagicDNS name.")
                    connection.httpStatus != null -> valueDetail("Saved, but ", address, " answered HTTP ") + valueDetail("", connection.httpStatus.toString(), " instead of a Pulse.")
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
    NameProblem.Blank -> AnnotatedString("Give this Machine a name")
    NameProblem.TooLong -> valueDetail("Use at most ", MAX_NAME_LENGTH.toString(), " characters")
    NameProblem.NotOneLine -> AnnotatedString("Use one line of text")
    is NameProblem.Taken -> AnnotatedString("Another Machine is already called ${problem.by.name}")
}

@Composable
private fun message(problem: AddressProblem): AnnotatedString = when (problem) {
    AddressProblem.Blank -> AnnotatedString("Enter a MagicDNS name or a tailnet IP")
    AddressProblem.Malformed -> AnnotatedString("Not a Machine address. Enter a MagicDNS name or a tailnet IP, without http:// or a path.")
    AddressProblem.HasPort -> AnnotatedString("Put the port in the Port field")
    is AddressProblem.OutsideTailnet ->
        valueDetail("", problem.literal, " isn't a tailnet address. Tailnet IPs are in ") +
            valueDetail("", "100.64.0.0/10", " or ") + valueDetail("", "fd7a:115c:a1e0::/48", ".")
    is AddressProblem.Taken -> AnnotatedString("${problem.by.name} already uses this address and port")
}
