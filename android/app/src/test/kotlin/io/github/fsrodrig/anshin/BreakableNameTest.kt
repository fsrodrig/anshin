package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Test

class BreakableNameTest {
    @Test fun aNameBreaksAfterItsSeparatorsAndReadsTheSameWithoutTheHints() {
        val shown = breakableName("workstation-in_the.rack")
        assertEquals("workstation-\u200Bin_\u200Bthe.\u200Brack", shown)
        assertEquals("workstation-in_the.rack", shown.replace("\u200B", ""))
    }

    @Test fun aNameWithoutSeparatorsIsUnchanged() = assertEquals("desktop", breakableName("desktop"))
}
