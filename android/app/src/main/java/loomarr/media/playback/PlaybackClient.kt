package loomarr.media.playback

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.isActive
import kotlinx.coroutines.job
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.util.concurrent.TimeUnit

/** One channel, as the tuner list shows it. */
data class Channel(
    val id: String,
    val name: String,
    val number: Int,
    val inAppPlayable: Boolean,
)

/** A signed, time-limited URL for one channel's HLS stream. */
data class PlayUrl(
    val url: String,
    val expiresAt: String,
)

/**
 * The playback half of the server contract (§9.1).
 *
 * Mirrors Jellyfin's shape: the client posts what it can decode, the server decides copy vs
 * transcode and hands back a ready-to-play URL. The client never reasons about codecs beyond
 * reporting its own capability.
 */
class PlaybackClient(
    private val baseUrl: String,
    private val token: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val json = "application/json".toMediaType()

    private fun authed(builder: Request.Builder) = builder.header("Authorization", "Bearer $token")

    /** Channels this device may play, newest server state. */
    suspend fun channels(): List<Channel> =
        withContext(Dispatchers.IO) {
            val request = authed(Request.Builder().url("$baseUrl/v1/channels")).build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("channels failed: ${response.code}")
                val payload = JSONObject(response.body?.string().orEmpty())
                val array = payload.optJSONArray("channels") ?: return@use emptyList()
                buildList {
                    for (i in 0 until array.length()) {
                        val c = array.getJSONObject(i)
                        add(
                            Channel(
                                id = c.getString("id"),
                                name = c.optString("name", "Channel"),
                                number = c.optInt("number", 0),
                                // The server refuses a play-url for a Tunarr-backed channel with a
                                // 409, and this is the same predicate — so filtering on it here
                                // turns a tune-time error into a channel that never appears.
                                inAppPlayable = c.optBoolean("inAppPlayable", false),
                            ),
                        )
                    }
                }
            }
        }

    /**
     * Open the authenticated state-change stream and expose only catalog-relevant signals.
     *
     * The payload is intentionally ignored. `/v1/events` is lossy, so constructing a lineup from
     * frames would eventually drift; callers reconcile through [channels] after every signal.
     * [ChannelStreamEvent.Connected] is emitted only after the server accepts the stream, allowing
     * every reconnect to trigger the same authoritative read as a foreground start.
     */
    internal fun channelEvents(): Flow<ChannelStreamEvent> =
        flow {
            val request =
                authed(Request.Builder().url("$baseUrl/v1/events"))
                    .header("Accept", "text/event-stream")
                    .build()
            // SSE is intentionally quiet between changes. OkHttp's normal 10-second read timeout
            // would turn silence into a reconnect loop (and therefore an accidental GET poll), so
            // this one derived client has no read timeout while retaining the shared pool/settings.
            val call =
                http
                    .newBuilder()
                    .readTimeout(0, TimeUnit.MILLISECONDS)
                    .build()
                    .newCall(request)
            // OkHttp's blocking read does not observe coroutine cancellation by itself. Cancelling
            // the call is what lets a backgrounded TV close a quiet, long-lived stream immediately.
            val cancellation = currentCoroutineContext().job.invokeOnCompletion { call.cancel() }
            try {
                call.execute().use { response ->
                    if (!response.isSuccessful) throw IOException("events failed: ${response.code}")
                    val source = response.body?.source() ?: throw IOException("events returned no body")
                    emit(ChannelStreamEvent.Connected)

                    var eventName: String? = null
                    while (currentCoroutineContext().isActive) {
                        val line = source.readUtf8Line() ?: break
                        when {
                            line.startsWith("event:") -> eventName = line.substringAfter(':').trim()
                            // The payload is irrelevant, but its first data line completes the SSE
                            // frame. Emitting here also tolerates a proxy closing immediately after
                            // the data bytes without forwarding the conventional trailing blank.
                            line.startsWith("data:") && eventName == "channel" -> {
                                emit(ChannelStreamEvent.ChannelChanged)
                                eventName = null
                            }
                            line.isEmpty() -> eventName = null
                        }
                    }
                    // EOF is a normal reconnect boundary. Returning (rather than throwing) lets
                    // flowOn deliver any invalidation already read before the proxy closed.
                }
            } finally {
                cancellation.dispose()
                call.cancel()
            }
        }.flowOn(Dispatchers.IO)

    /**
     * Mint a signed HLS URL for one channel.
     *
     * The profile is posted per playback rather than registered once, because capability is a
     * property of this device and the server keys its copy/transcode decision on it.
     */
    suspend fun playUrl(
        channelId: String,
        profile: DeviceProfile,
    ): PlayUrl =
        withContext(Dispatchers.IO) {
            val body = profile.toJson().toString().toRequestBody(json)
            val request =
                authed(Request.Builder().url("$baseUrl/v1/channels/$channelId/play-url"))
                    .post(body)
                    .build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("play-url failed: ${response.code}")
                val payload = JSONObject(response.body?.string().orEmpty())

                val url =
                    resolveStreamUrl(
                        baseUrl = baseUrl,
                        relativeUrl = payload.optString("relativeUrl"),
                        absoluteUrl = payload.optString("url"),
                    )

                PlayUrl(url = url, expiresAt = payload.optString("expiresAt"))
            }
        }
}

/** Connection lifecycle plus the one SSE frame that invalidates the Channel catalog. */
internal sealed interface ChannelStreamEvent {
    data object Connected : ChannelStreamEvent

    data object ChannelChanged : ChannelStreamEvent
}

/**
 * Which of the server's two stream addresses this device should actually fetch.
 *
 * ⚠ The RELATIVE one, resolved against the address this device paired to — not the absolute `url`.
 *
 * The server builds that absolute URL from its own Host header, which is its idea of where it lives
 * rather than the address this client reached it by. The two differ whenever anything sits between.
 * The emulator is the cheapest demonstration: it pairs to `10.0.2.2` and is handed
 * `http://localhost:18305/…`, and on Android `localhost` is the DEVICE — so playback dies with a
 * ConnectException while the guide loads perfectly over the very same pairing. A Shield behind a
 * reverse proxy, or reached over Tailscale, breaks in exactly the same way.
 *
 * An earlier comment here held that a native client "cannot use the relative form a browser falls
 * back to". It can, and by the same rule a browser does: [baseUrl] came from pairing, so it is the
 * one address this device is known to be able to reach.
 */
internal fun resolveStreamUrl(
    baseUrl: String,
    relativeUrl: String,
    absoluteUrl: String,
): String =
    when {
        relativeUrl.isNotBlank() -> baseUrl.trimEnd('/') + "/" + relativeUrl.trimStart('/')
        // Only when the server sent no relative form at all. Better than failing outright, but it
        // carries the Host-header assumption above and will strand any client that does not share
        // the server's own notion of its address.
        absoluteUrl.isNotBlank() -> absoluteUrl
        else -> throw IOException("this Loomarr returned no stream address for the channel")
    }
