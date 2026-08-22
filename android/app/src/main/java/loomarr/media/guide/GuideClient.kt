package loomarr.media.guide

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.io.IOException
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter

/**
 * Reads the guide — the same endpoint the web grid and XMLTV are built from.
 *
 * ⚠ `GET /v1/guide`, NOT `/v1/playout/guide.xml`. The XMLTV route is authed by a device token and
 * exists for media servers; a native client holds a user credential and should never carry one.
 */
class GuideClient(
    private val baseUrl: String,
    private val token: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    /**
     * Reads the server clock without computing the guide just to learn what time it is.
     *
     * `/v1/healthz` is deliberately public and cheap. Its HTTP `Date` header is server-authored,
     * unlike the TV clock, and avoids the previous two-guide-request path doubling 100 channel
     * timeline computation before the grid could render.
     */
    suspend fun serverNowMs(): Long =
        withContext(Dispatchers.IO) {
            val request = Request.Builder().url("$baseUrl/v1/healthz").build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("server clock failed: ${response.code}")
                response
                    .header("Date")
                    ?.let { value ->
                        runCatching {
                            ZonedDateTime
                                .parse(value, DateTimeFormatter.RFC_1123_DATE_TIME)
                                .toInstant()
                                .toEpochMilli()
                        }.getOrNull()
                    } ?: throw IOException("server clock response had no valid Date header")
            }
        }

    /**
     * Fetch the window between two epoch-ms instants.
     *
     * Both bounds are optional — the server defaults to now through now+4h — but the grid passes
     * them explicitly so a scroll can ask for a later window without re-deriving "now" on a device
     * whose clock may be wrong.
     */
    suspend fun window(
        fromMs: Long? = null,
        toMs: Long? = null,
    ): GuideWindow =
        withContext(Dispatchers.IO) {
            val query =
                buildList {
                    fromMs?.let { add("from=$it") }
                    toMs?.let { add("to=$it") }
                }.joinToString("&").let { if (it.isEmpty()) "" else "?$it" }

            val request =
                Request
                    .Builder()
                    .url("$baseUrl/v1/guide$query")
                    .header("Authorization", "Bearer $token")
                    .build()

            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw IOException("guide failed: ${response.code}")
                parse(JSONObject(response.body?.string().orEmpty()))
            }
        }

    private fun parse(payload: JSONObject): GuideWindow {
        val channelsJson = payload.optJSONArray("channels")
        val channels =
            buildList {
                for (i in 0 until (channelsJson?.length() ?: 0)) {
                    val c = channelsJson!!.getJSONObject(i)
                    val airingsJson = c.optJSONArray("airings")
                    val airings =
                        buildList {
                            for (j in 0 until (airingsJson?.length() ?: 0)) {
                                val a = airingsJson!!.getJSONObject(j)
                                add(
                                    Airing(
                                        kind = a.optString("kind", "program"),
                                        title = a.optString("title"),
                                        // `optString` returns "" for a missing key, but this field is
                                        // genuinely absent for a film — so the empty case is mapped
                                        // back to null rather than letting "" mean "a series named
                                        // nothing".
                                        series = a.optString("series").takeIf { it.isNotBlank() },
                                        season = a.optInt("season"),
                                        episode = a.optInt("episode"),
                                        description = a.optString("description").takeIf { it.isNotBlank() },
                                        genres = a.optJSONArray("genres").stringValues(),
                                        year = a.optInt("year"),
                                        rating = a.optString("rating").takeIf { it.isNotBlank() },
                                        thumbUrl = sameOriginUrl(a.optString("thumbUrl")),
                                        runtimeMs = a.optLong("runtimeMs"),
                                        startMs = a.optLong("startMs"),
                                        stopMs = a.optLong("stopMs"),
                                        nominal = a.optBoolean("nominal", false),
                                        provenance = a.optString("provenance").takeIf { it.isNotBlank() },
                                    ),
                                )
                            }
                        }
                    add(
                        ChannelTimeline(
                            channelId = c.optString("channelId"),
                            name = c.optString("name", "Channel"),
                            number = c.optInt("number"),
                            logoUrl = absoluteUrl(c.optString("logo")),
                            status = c.optString("status", "live"),
                            pendingCount = c.optInt("pendingCount"),
                            airings = airings,
                        ),
                    )
                }
            }

        return GuideWindow(
            // The window the server SERVED, after clamping — not what was asked for. Laying the grid
            // out against the request would draw every block at the wrong offset whenever the server
            // narrowed it.
            fromMs = payload.optLong("fromMs"),
            toMs = payload.optLong("toMs"),
            channels = channels,
        )
    }

    private fun absoluteUrl(value: String): String? =
        value
            .takeIf { it.isNotBlank() }
            ?.let { baseUrl.toHttpUrlOrNull()?.resolve(it)?.toString() }

    /** Guide previews are an explicit same-instance contract; reject a drifting third-party URL. */
    private fun sameOriginUrl(value: String): String? {
        val origin = baseUrl.toHttpUrlOrNull() ?: return null
        val resolved = value.takeIf { it.isNotBlank() }?.let(origin::resolve) ?: return null
        return resolved
            .takeIf { it.scheme == origin.scheme && it.host == origin.host && it.port == origin.port }
            ?.toString()
    }

    private fun org.json.JSONArray?.stringValues(): List<String> =
        buildList {
            val values = this@stringValues ?: return@buildList
            for (index in 0 until values.length()) {
                values.optString(index).takeIf { it.isNotBlank() }?.let(::add)
            }
        }
}
