package loomarr.media.diagnostics

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject

enum class ClientEvent(
    val wire: String,
) {
    PlayerAttached("player.attached"),
    PlayerDetached("player.detached"),
    PlayerReady("player.ready"),
    PlayerSourceReplaced("player.source_replaced"),
    PlayerBufferingStarted("player.buffering_started"),
    PlayerBufferingEnded("player.buffering_ended"),
    PlayerSeeking("player.seeking"),
    PlayerSeeked("player.seeked"),
    PlayerMediaError("player.media_error"),
    PlayerScheduleBlockChanged("player.schedule_block_changed"),
    PlayerPlayheadDrift("player.playhead_drift"),
}

data class ClientObservation(
    val event: ClientEvent,
    val occurredAt: Long = System.currentTimeMillis(),
    val playbackSessionId: String,
    val channelId: String,
    val scheduleBlockId: String? = null,
    val previousScheduleBlockId: String? = null,
    val previousChannelId: String? = null,
    val reason: String? = null,
    val errorCode: String? = null,
    val blockKind: String? = null,
    val fatal: Boolean? = null,
    val viewerTimeMs: Long? = null,
    val serverTimeMs: Long? = null,
    val driftMs: Long? = null,
    val bufferedMs: Long? = null,
) {
    fun toJson(): JSONObject =
        JSONObject()
            .put("event", event.wire)
            .put("occurredAt", occurredAt)
            .put("playbackSessionId", playbackSessionId)
            .put("channelId", channelId)
            .putOptional("scheduleBlockId", scheduleBlockId)
            .putOptional("previousScheduleBlockId", previousScheduleBlockId)
            .putOptional("previousChannelId", previousChannelId)
            .also { payload ->
                if (event !in setOf(ClientEvent.PlayerScheduleBlockChanged, ClientEvent.PlayerPlayheadDrift)) {
                    payload.put("transport", "media3")
                }
            }.putOptional("reason", reason)
            .putOptional("errorCode", errorCode)
            .putOptional("blockKind", blockKind)
            .putOptional("fatal", fatal)
            .putOptional("viewerTimeMs", viewerTimeMs)
            .putOptional("serverTimeMs", serverTimeMs)
            .putOptional("driftMs", driftMs)
            .putOptional("bufferedMs", bufferedMs)
}

private fun JSONObject.putOptional(
    name: String,
    value: Any?,
): JSONObject = if (value == null) this else put(name, value)

fun interface ClientDiagnosticSender {
    suspend fun send(events: List<ClientObservation>)
}

class HttpClientDiagnosticSender(
    private val baseUrl: String,
    private val token: String,
    private val clientVersion: String,
    private val platform: String,
    private val http: OkHttpClient = OkHttpClient(),
) : ClientDiagnosticSender {
    override suspend fun send(events: List<ClientObservation>) =
        withContext(Dispatchers.IO) {
            val body =
                JSONObject()
                    .put("source", "android_tv")
                    .put("clientVersion", clientVersion.take(64))
                    .put("platform", platform)
                    .put("events", JSONArray(events.map(ClientObservation::toJson)))
                    .toString()
                    .toRequestBody("application/json".toMediaType())
            val request =
                Request
                    .Builder()
                    .url("${baseUrl.trimEnd('/')}/v1/diagnostics/client-events")
                    .header("Authorization", "Bearer $token")
                    .post(body)
                    .build()
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) error("client diagnostics failed: ${response.code}")
            }
        }
}

/** Bounded, best-effort admission. Diagnostics never waits in the playback call path. */
class ClientDiagnosticsReporter(
    private val sender: ClientDiagnosticSender,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    private val queue = ArrayDeque<ClientObservation>()
    private var flushJob: Job? = null
    private var sending = false

    @Synchronized
    fun record(observation: ClientObservation) {
        if (queue.size >= QUEUE_LIMIT) {
            val routine = queue.indexOfFirst { it.event !in ERROR_EVENTS }
            if (routine >= 0) queue.removeAt(routine) else queue.removeFirst()
        }
        queue.addLast(observation)
        schedule()
    }

    @Synchronized
    private fun schedule() {
        if (flushJob?.isActive == true) return
        flushJob =
            scope.launch {
                delay(FLUSH_MS)
                flush()
            }
    }

    suspend fun flush() {
        val batch =
            synchronized(this) {
                if (sending || queue.isEmpty()) return
                sending = true
                buildList { repeat(minOf(BATCH_LIMIT, queue.size)) { add(queue.removeFirst()) } }
            }
        try {
            sender.send(batch)
        } catch (_: Exception) {
            synchronized(this) {
                batch.asReversed().forEach { if (queue.size < QUEUE_LIMIT) queue.addFirst(it) }
            }
        } finally {
            synchronized(this) {
                sending = false
                flushJob = null
                if (queue.isNotEmpty()) schedule()
            }
        }
    }

    fun close() = scope.cancel()

    private companion object {
        const val QUEUE_LIMIT = 100
        const val BATCH_LIMIT = 20
        const val FLUSH_MS = 2_000L
        val ERROR_EVENTS = setOf(ClientEvent.PlayerMediaError)
    }
}
