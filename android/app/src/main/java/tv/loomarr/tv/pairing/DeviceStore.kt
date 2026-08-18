package tv.loomarr.tv.pairing

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val Context.dataStore by preferencesDataStore(name = "loomarr")

/**
 * Where the device's server address and credential live between launches.
 *
 * DataStore rather than EncryptedSharedPreferences: `androidx.security:security-crypto` is fully
 * deprecated as of 1.1.0, so its guidance is stale. A TV is a shared appliance with no per-user
 * lock screen, and the token grants exactly what the approving member already has — so the value
 * of encrypting at rest here is small, while a Keystore-backed key that cannot be unlocked without
 * user authentication would be actively wrong on a device nobody signs into.
 */
class DeviceStore(
    private val context: Context,
) {
    suspend fun serverUrl(): String? = context.dataStore.data.first()[SERVER_URL]

    suspend fun setServerUrl(url: String) {
        context.dataStore.edit { it[SERVER_URL] = url.trimEnd('/') }
    }

    suspend fun token(): String? = context.dataStore.data.first()[TOKEN]

    suspend fun setToken(token: String) {
        context.dataStore.edit { it[TOKEN] = token }
    }

    /** Drop the credential — used when the server rejects it, i.e. the device was revoked. */
    suspend fun clearToken() {
        context.dataStore.edit { it.remove(TOKEN) }
    }

    private companion object {
        val SERVER_URL = stringPreferencesKey("server_url")
        val TOKEN = stringPreferencesKey("device_token")
    }
}
