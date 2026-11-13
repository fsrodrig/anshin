package io.github.fsrodrig.anshin

import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** Against a real Preferences DataStore in a temporary file, opened the way the app opens it. */
class ThemeSettingTest {
    @get:Rule val folder = TemporaryFolder()

    private fun <T> withStore(file: File, block: suspend (ThemeSetting) -> T): T = runBlocking {
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

    private fun storeFile() = folder.root.resolve("settings.preferences_pb")

    @Test
    fun anEmptyStoreIsInkAndBone() {
        assertEquals(Theme.InkAndBone, withStore(storeFile()) { it.theme.first() })
    }

    @Test
    fun aRecreatedStoreRestoresTheSavedTheme() {
        val file = storeFile()
        withStore(file) { it.select(Theme.CatppuccinMocha) }
        assertEquals(Theme.CatppuccinMocha, withStore(file) { it.theme.first() })
    }

    @Test
    fun anUnknownStoredThemeIsInkAndBone() {
        val file = storeFile()
        withStore(file) { setting ->
            setting.select(Theme.TokyoNight)
        }
        // A key from a future version, or a hand-edited file.
        runBlocking {
            val job = Job()
            val store = PreferenceDataStoreFactory.create(scope = CoroutineScope(Dispatchers.IO + job), produceFile = { file })
            store.edit { it[stringPreferencesKey("theme")] = "solarized" }
            job.cancelAndJoin()
        }
        assertEquals(Theme.InkAndBone, withStore(file) { it.theme.first() })
    }

    @Test
    fun aCorruptStoreIsInkAndBone() {
        val file = storeFile().apply { writeBytes(byteArrayOf(0x7f, 0x00, 0x13, 0x37)) }
        assertEquals(Theme.InkAndBone, withStore(file) { it.theme.first() })
    }
}
