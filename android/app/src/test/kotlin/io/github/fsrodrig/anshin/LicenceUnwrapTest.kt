package io.github.fsrodrig.anshin

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Test

class LicenceUnwrapTest {
    private val main = File(checkNotNull(System.getProperty("anshin.main")) { "anshin.main is unset" })

    @Test fun aHardWrappedParagraphBecomesOneLine() = assertEquals(
        listOf(LicenceParagraph("Permission is hereby granted, free of charge, to any person obtaining a copy")),
        unwrapLicence("Permission is hereby granted, free of charge,\nto any person obtaining\na copy\n"),
    )

    @Test fun blankLinesSeparateParagraphs() = assertEquals(
        listOf(LicenceParagraph("The MIT License (MIT)"), LicenceParagraph("Copyright (c) the authors")),
        unwrapLicence("The MIT License (MIT)\n\n\nCopyright (c) the authors"),
    )

    @Test fun aBlocksLeadingIndentIsKeptAndAHangingIndentIsJoined() = assertEquals(
        listOf(
            LicenceParagraph("1. Definitions.", indent = 3),
            LicenceParagraph("2. Grant of Copyright License. Subject to the terms of this License", indent = 3),
        ),
        unwrapLicence("   1. Definitions.\n\n   2. Grant of Copyright License. Subject to the terms of\n      this License"),
    )

    @Test fun aCentredTitleKeepsItsLines() = assertEquals(
        listOf(
            LicenceParagraph("Apache License", indent = 33),
            LicenceParagraph("Version 2.0, January 2004", indent = 27, opensBlock = false),
        ),
        unwrapLicence("                                 Apache License\n                           Version 2.0, January 2004\n"),
    )

    @Test fun rulesAndShortCapitalHeadingsStandAlone() = assertEquals(
        listOf(
            LicenceParagraph("-----"),
            LicenceParagraph("SIL OPEN FONT LICENSE Version 1.1", opensBlock = false),
            LicenceParagraph("-----", opensBlock = false),
            LicenceParagraph("PREAMBLE"),
            LicenceParagraph("The goals of the Open Font License", opensBlock = false),
        ),
        unwrapLicence("-----\nSIL OPEN FONT LICENSE Version 1.1\n-----\n\nPREAMBLE\nThe goals of\nthe Open Font License\n"),
    )

    @Test fun aWarrantyInCapitalsIsStillOneParagraph() = assertEquals(
        listOf(LicenceParagraph("THE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED.")),
        unwrapLicence("THE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND,\nEXPRESS OR IMPLIED.\n"),
    )

    @Test fun everyShippedLicenceKeepsEveryWordInOrder() {
        for (entry in openSourceLicences) {
            val text = main.resolve("assets/${entry.asset}").readText()
            fun words(text: String) = text.split(Regex("\\s+")).filter { it.isNotEmpty() }
            assertEquals(entry.asset, words(text), unwrapLicence(text).flatMap { words(it.text) })
        }
    }
}
