package media.loomarr.tv.legacypairing

import android.content.Context
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import expo.modules.kotlin.exception.Exceptions
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import kotlinx.coroutines.flow.first

private val Context.legacyLoomarrDataStore by preferencesDataStore(name = "loomarr")

/** Read-only bridge for the Compose-to-React-Native in-place update. */
class LegacyPairingModule : Module() {
  private val context: Context
    get() = appContext.reactContext ?: throw Exceptions.ReactContextLost()

  override fun definition() = ModuleDefinition {
    Name("LoomarrLegacyPairing")

    AsyncFunction("read").SuspendBody<Map<String, String>?> {
      val preferences = context.legacyLoomarrDataStore.data.first()
      val serverUrl = preferences[SERVER_URL]?.trimEnd('/')
      val token = preferences[DEVICE_TOKEN]
      if (serverUrl.isNullOrBlank() || token.isNullOrBlank()) {
        null
      } else {
        mapOf("serverUrl" to serverUrl, "token" to token)
      }
    }
  }

  private companion object {
    val SERVER_URL = stringPreferencesKey("server_url")
    val DEVICE_TOKEN = stringPreferencesKey("device_token")
  }
}
