package io.github.fsrodrig.anshin

@JvmInline value class MachineId(val value: String)

/** A Machine the Owner saved: their name for it and how the phone reaches its Agent. */
data class Machine(val id: MachineId, val name: String, val address: MachineAddress)
