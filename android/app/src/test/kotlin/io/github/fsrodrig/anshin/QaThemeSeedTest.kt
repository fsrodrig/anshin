package io.github.fsrodrig.anshin

import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.emptyPreferences
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/**
 * Writes the theme preference files the agent-qa helper seeds into the installed release app. The real DataStore
 * writes each one and a fresh store reads it back through [ThemeSetting], so a seed is exactly what the app itself
 * would have saved. With `-Panshin.qaSeedDir` the round-tripped files are also copied there; without it nothing
 * leaves the temporary folder.
 */
class QaThemeSeedTest {
    @get:Rule val folder = TemporaryFolder()

    private fun <T> withSetting(file: File, block: suspend (ThemeSetting) -> T): T = runBlocking {
        val job = Job()
        val store = PreferenceDataStoreFactory.create(
            corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
            scope = CoroutineScope(Dispatchers.IO + job),
            produceFile = { file },
        )
        try {
            block(ThemeSetting(store))
        } finally {
            job.cancelAndJoin()
        }
    }

    @Test
    fun everyThemeSeedRoundTripsThroughThemeSetting() {
        val export = System.getProperty("anshin.qaSeedDir")?.takeIf { it.isNotBlank() }?.let(::File)
        for (theme in Theme.entries) {
            val file = folder.newFolder(theme.key).resolve("settings.preferences_pb")
            withSetting(file) { it.select(theme) }
            val bytes = file.readBytes()

            val copy = folder.newFolder("${theme.key}-copy").resolve("settings.preferences_pb")
            copy.writeBytes(bytes)
            assertEquals(theme, withSetting(copy) { it.theme.first() })

            export?.let { dir ->
                dir.mkdirs()
                val out = dir.resolve("${theme.key}.preferences_pb")
                out.writeBytes(bytes)
                assertArrayEquals(bytes, out.readBytes())
            }
        }
    }
}
