package tv.loomarr.tv.pairing

import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/** A pending pairing: what to show on screen while waiting for a human. */
data class Pairing(
    /** Secret the device keeps and polls with. Never displayed. */
    val deviceCode: String,
    /** Short code shown on the television, e.g. "BCDF-GHJK". */
    val userCode: String,
    /** Seconds to wait between polls, as advised by the server. */
    val intervalSeconds: Int,
)

/** The outcome of one poll. Modelled as a type because "keep waiting" is not an error. */
sealed interface PollResult {
    /** A human approved it; [token] is the durable credential. */
    data class Paired(val token: String, val deviceName: String) : PollResult

    /**
     * Nobody has approved it yet — keep polling.
     *
     * ⚠ Distinct from [Expired] on purpose. The server answers 428 here and 404 there, and the
     * device does opposite things: wait, or throw the code away and start over. Collapsing them
     * would send the TV back to a fresh code every few seconds, which no one could ever type in
     * time.
     */
    data object Pending : PollResult

    /** The code is dead (expired, consumed, or wrong). Start a new pairing. */
    data object Expired : PollResult
}

/**
 * The device half of Loomarr's pairing handshake (§11) — RFC 8628 in shape.
 *
 * Deliberately tiny and dependency-light: pairing is the one thing that must work before anything
 * else does, so it does not pull in a serialization framework or a DI graph to run.
 */
class PairingClient(
    private val baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val json = "application/json".toMediaType()

    /** Begin pairing. The caller shows [Pairing.userCode] and the server's /pair address. */
    suspend fun start(deviceName: String): Pairing = withContext(Dispatchers.IO) {
        val body = JSONObject().put("deviceName", deviceName).toString().toRequestBody(json)
        val request = Request.Builder().url("$baseUrl/v1/auth/device/start").post(body).build()
        http.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw IOException("pairing start failed: ${response.code}")
            val payload = JSONObject(response.body?.string().orEmpty())
            Pairing(
                deviceCode = payload.getString("deviceCode"),
                userCode = payload.getString("userCode"),
                // Fall back rather than fail: a server that omits the hint is not broken, and a
                // device that refuses to poll over a missing number would be.
                intervalSeconds = payload.optInt("interval", 5),
            )
        }
    }

    /** Ask once whether the pairing has been approved. */
    suspend fun poll(deviceCode: String): PollResult = withContext(Dispatchers.IO) {
        val body = JSONObject().put("deviceCode", deviceCode).toString().toRequestBody(json)
        val request = Request.Builder().url("$baseUrl/v1/auth/device/poll").post(body).build()
        http.newCall(request).execute().use { response ->
            when (response.code) {
                200 -> {
                    val payload = JSONObject(response.body?.string().orEmpty())
                    PollResult.Paired(
                        token = payload.getString("token"),
                        deviceName = payload.optString("deviceName", "Device"),
                    )
                }
                // 428 Precondition Required — the pairing exists, a human has not acted yet.
                428 -> PollResult.Pending
                // 404 — expired, already redeemed, or never existed. All three mean "start over",
                // and the server deliberately does not distinguish them.
                404 -> PollResult.Expired
                else -> throw IOException("pairing poll failed: ${response.code}")
            }
        }
    }
}
