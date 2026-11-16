package io.github.fsrodrig.anshin

import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineFormScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun capture(name: String, state: MachineFormState) {
        compose.setContent { AnshinTheme(theme) { MachineForm(state, MachineFormActions()) } }
        compose.onRoot().captureFrame("machine-form-$name.${theme.key}.png")
    }

    private val typed = MachineFormState(name = "desktop", address = "100.64.0.2")

    private fun tested(connection: Connection) =
        typed.copy(saved = true, phase = FormPhase.Tested(Machines.desktop, connection))

    @Test fun add() = capture("add", MachineFormState())

    @Test fun rejected() = capture("rejected", MachineFormState(
        name = " ", address = "192.0.2.1", port = "0",
        problems = Problems(NameProblem.Blank, AddressProblem.OutsideTailnet("192.0.2.1"), PortProblem),
    ))

    @Test fun duplicate() = capture("duplicate", MachineFormState(
        name = "Laptop", address = "laptop.example.ts.net",
        problems = Problems(name = NameProblem.Taken(Machines.laptop), address = AddressProblem.Taken(Machines.laptop)),
    ))

    @Test fun testing() = capture("testing", typed.copy(saved = true, phase = FormPhase.Testing("100.64.0.2")))

    @Test fun live() = capture("live", tested(Connection.Live))

    @Test fun forbidden() = capture("forbidden", tested(Connection.Forbidden))

    @Test fun unreachable() = capture("unreachable", tested(Connection.Unreachable()))

    @Test fun incompatible() = capture("incompatible", tested(Connection.Incompatible))

    @Test fun edit() = capture("edit", MachineFormState(name = "laptop", address = "laptop.example.ts.net", port = "9000", editing = true))

    @Test fun remove() = capture("remove", MachineFormState(name = "laptop", address = "laptop.example.ts.net", editing = true, confirmingRemove = true))

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
