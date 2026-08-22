package loomarr.media.pairing

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException

/** A pending pairing: what to show on screen while waiting for a human. */
data class Pairing(
    /** Secret the device keeps and polls with. Never displayed. */
    val deviceCode: String,
    /** Short code shown on the television, e.g. "BCDF-GHJK". */
    val userCode: String,
    /** Seconds to wait between polls, as advised by the server. */
    val intervalSeconds: Int,
    /**
     * Seconds this code remains valid, derived from the server's `expiresAt`.
     *
     * ⚠ Stored as a DURATION, not the server's timestamp. A television's clock is frequently wrong
     * — no RTC on some boxes, NTP not yet synced at first boot — so comparing a server instant
     * against local `now` can show a code as long expired or valid for hours. A duration measured
     * from the moment the response arrived only depends on elapsed time, which the device does
     * track correctly.
     */
    val expiresInSeconds: Long,
)

/** The outcome of one poll. Modelled as a type because "keep waiting" is not an error. */
sealed interface PollResult {
    /** A human approved it; [token] is the durable credential. */
    data class Paired(
        val token: String,
        val deviceName: String,
    ) : PollResult

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
 * How long a code lives when the server does not say, or says something unparseable.
 *
 * Matches the server's own PairingTTL. A countdown is a promise to the viewer, so guessing SHORT is
 * the safe direction: a code that outlives its displayed timer is merely surprising, while one that
 * dies before the timer reaches zero looks broken.
 */
private const val DEFAULT_PAIRING_TTL_SECONDS = 10L * 60

/**
 * How long the code lasts, measured against the SERVER's clock rather than this device's.
 *
 * ⚠ The device clock cannot be trusted, and this is not hypothetical: the first build of this
 * countdown read "Expires in 133:37" for a ten-minute code, because the emulator's clock sat two
 * hours behind the host. A television is exactly the class of device this happens on — some boxes
 * have no RTC, and NTP may not have synced at first boot.
 *
 * So the expiry instant is compared against the server's own `Date` response header, which arrived
 * in the same exchange and shares its clock. Both fall back to the known TTL, because a countdown
 * is a nicety and failing a working handshake over one unreadable header would be the wrong trade.
 */
private fun secondsUntil(
    rfc3339: String,
    serverDateHeader: String?,
): Long {
    if (rfc3339.isBlank()) return DEFAULT_PAIRING_TTL_SECONDS
    return runCatching {
        val expiry = java.time.Instant.parse(rfc3339)
        val serverNow =
            serverDateHeader
                ?.let {
                    runCatching {
                        java.time.ZonedDateTime
                            .parse(it, java.time.format.DateTimeFormatter.RFC_1123_DATE_TIME)
                            .toInstant()
                    }.getOrNull()
                }
                // No usable Date header: the TTL is a better guess than a clock known to drift.
                ?: return DEFAULT_PAIRING_TTL_SECONDS
        java.time.Duration
            .between(serverNow, expiry)
            .seconds
            .coerceAtLeast(0)
    }.getOrDefault(DEFAULT_PAIRING_TTL_SECONDS)
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
    suspend fun start(deviceName: String): Pairing =
        withContext(Dispatchers.IO) {
            val body = JSONObject().put("deviceName", deviceName).toString().toRequestBody(json)
            val request = Request
                .Builder()
                .url("$baseUrl/v1/auth/device/start")
                .post(body)
                .build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("pairing start failed: ${response.code}")
                val payload = JSONObject(response.body?.string().orEmpty())
                Pairing(
                    deviceCode = payload.getString("deviceCode"),
                    userCode = payload.getString("userCode"),
                    // Fall back rather than fail: a server that omits the hint is not broken, and a
                    // device that refuses to poll over a missing number would be.
                    intervalSeconds = payload.optInt("interval", 5),
                    expiresInSeconds =
                        secondsUntil(
                            payload.optString("expiresAt"),
                            response.header("Date"),
                        ),
                )
            }
        }

    /** Ask once whether the pairing has been approved. */
    suspend fun poll(deviceCode: String): PollResult =
        withContext(Dispatchers.IO) {
            val body = JSONObject().put("deviceCode", deviceCode).toString().toRequestBody(json)
            val request = Request
                .Builder()
                .url("$baseUrl/v1/auth/device/poll")
                .post(body)
                .build()
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
