package io.github.fsrodrig.anshin

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/** The Owner's chosen theme. A missing, unknown or unreadable value is Ink & bone. */
class ThemeSetting(private val store: DataStore<Preferences>) {
    val theme: Flow<Theme> = store.data
        .catch { if (it is IOException) emit(emptyPreferences()) else throw it }
        .map { Theme.fromKey(it[KEY]) }
        .distinctUntilChanged()

    suspend fun select(theme: Theme) {
        store.edit { it[KEY] = theme.key }
    }

    private companion object {
        val KEY = stringPreferencesKey("theme")
    }
}

/**
 * The app's settings, in `files/datastore/settings.preferences_pb`. The delegate keeps one store per process, which
 * DataStore requires; a corrupt file is replaced with an empty one rather than failing every read.
 */
val Context.settingsStore: DataStore<Preferences> by preferencesDataStore(
    name = "settings",
    corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
)
