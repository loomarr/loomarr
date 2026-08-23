package loomarr.media.version

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.io.IOException

/** The two independently deployable identities a TV bug report must keep distinct. */
data class VersionIdentity(
    val clientVersion: String,
    val serverVersion: String,
) {
    val label: String
        get() = "Loomarr TV $clientVersion · Server $serverVersion"
}

data class ServerVersion(
    val version: String,
    val dirty: Boolean,
) {
    val displayName: String
        get() = if (dirty) "$version (modified)" else version
}

/** Small public-version client; visibility remains best-effort and never gates playback. */
class ServerVersionClient(
    private val baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    suspend fun fetch(): ServerVersion =
        withContext(Dispatchers.IO) {
            val request = Request
                .Builder()
                .url("${baseUrl.trimEnd('/')}/v1/system/version")
                .get()
                .build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("server version failed: ${response.code}")
                val payload = JSONObject(response.body?.string().orEmpty())
                ServerVersion(
                    version = payload.getString("version"),
                    dirty = payload.optBoolean("dirty", false),
                )
            }
        }
}
