package loomarr.media.playback

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import loomarr.media.pairing.DeviceStore
import java.io.Closeable
import java.io.IOException
import kotlinx.coroutines.channels.Channel as CoroutineChannel

/** One complete, server-authored view of the playable Channel lineup. */
sealed interface ChannelCatalogState {
    data object Loading : ChannelCatalogState

    data class Ready(
        val channels: List<Channel>,
        /** Advances after every successful reconciliation, even when membership is unchanged. */
        val revision: Long,
        /** A refresh failure leaves this last-known-good snapshot usable. */
        val refreshError: String? = null,
    ) : ChannelCatalogState

    data class Failed(
        val message: String,
    ) : ChannelCatalogState
}

/** The narrow server seam owned by [ChannelCatalog]. */
internal interface ChannelCatalogClient {
    suspend fun channels(): List<Channel>

    fun channelEvents(): Flow<ChannelStreamEvent>
}

/**
 * App-scoped source of truth for every TV surface's playable Channel lineup.
 *
 * SSE never mutates this state. It merely requests a complete GET reconciliation; the same request
 * happens on foreground entry and after every successful stream connection. A conflated request
 * queue absorbs event bursts, while the last complete snapshot survives transient read failures.
 */
class ChannelCatalog internal constructor(
    private val client: ChannelCatalogClient,
    private val scope: CoroutineScope =
        CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val coalesceMillis: Long = 250,
    private val initialReconnectMillis: Long = 1_000,
    private val maximumReconnectMillis: Long = 30_000,
    private val safetyRefreshMillis: Long = 5 * 60 * 1_000,
) : Closeable {
    constructor(store: DeviceStore) : this(StoredChannelCatalogClient(store))

    private val _state = MutableStateFlow<ChannelCatalogState>(ChannelCatalogState.Loading)
    val state: StateFlow<ChannelCatalogState> = _state.asStateFlow()

    private val refreshRequests = CoroutineChannel<Unit>(capacity = CoroutineChannel.CONFLATED)
    private var eventsJob: Job? = null
    private var safetyRefreshJob: Job? = null
    private var revision = 0L

    init {
        scope.launch {
            for (ignored in refreshRequests) {
                if (coalesceMillis > 0) delay(coalesceMillis)
                // Drain anything that arrived during the debounce. One authoritative GET satisfies
                // every duplicate/out-of-order frame in that burst.
                while (refreshRequests.tryReceive().isSuccess) {
                    // Deliberately empty.
                }
                reconcile()
            }
        }
    }

    /** Enter the foreground. Idempotent because lifecycle callbacks can be repeated. */
    fun start() {
        requestRefresh()
        if (eventsJob?.isActive == true) return
        eventsJob =
            scope.launch {
                var reconnectMillis = initialReconnectMillis
                while (isActive) {
                    var connected = false
                    try {
                        client.channelEvents().collect { event ->
                            if (event == ChannelStreamEvent.Connected) {
                                connected = true
                                reconnectMillis = initialReconnectMillis
                            }
                            // Connected and ChannelChanged both reconcile through GET. The event
                            // payload never becomes catalog state.
                            requestRefresh()
                        }
                    } catch (cancelled: CancellationException) {
                        throw cancelled
                    } catch (_: Exception) {
                        // Stream availability affects freshness, not the last valid lineup.
                    }

                    if (!isActive) break
                    delay(reconnectMillis)
                    reconnectMillis =
                        if (connected) {
                            initialReconnectMillis
                        } else {
                            (reconnectMillis * 2).coerceAtMost(maximumReconnectMillis)
                        }
                }
            }
        safetyRefreshJob =
            scope.launch {
                while (isActive) {
                    delay(safetyRefreshMillis)
                    requestRefresh()
                }
            }
    }

    /** Leave the foreground: close the long-lived socket but retain the last complete snapshot. */
    fun stop() {
        eventsJob?.cancel()
        eventsJob = null
        safetyRefreshJob?.cancel()
        safetyRefreshJob = null
    }

    /** Explicit invalidation for tests and rare callers that already know state changed. */
    fun requestRefresh() {
        refreshRequests.trySend(Unit)
    }

    private suspend fun reconcile() {
        try {
            val channels = client.channels().filter { it.inAppPlayable }
            revision++
            _state.value = ChannelCatalogState.Ready(channels = channels, revision = revision)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: Exception) {
            val message = error.message ?: "Couldn't refresh channels."
            _state.value =
                when (val current = _state.value) {
                    is ChannelCatalogState.Ready -> current.copy(refreshError = message)
                    else -> ChannelCatalogState.Failed(message)
                }
        }
    }

    override fun close() {
        refreshRequests.close()
        scope.coroutineContext[Job]?.cancel()
    }
}

/** Reads the current pairing for every operation so re-pairing cannot leave a stale credential. */
private class StoredChannelCatalogClient(
    private val store: DeviceStore,
) : ChannelCatalogClient {
    override suspend fun channels(): List<Channel> = client().channels()

    override fun channelEvents(): Flow<ChannelStreamEvent> =
        flow {
            emitAll(client().channelEvents())
        }

    private suspend fun client(): PlaybackClient {
        val baseUrl = store.serverUrl()
        val token = store.token()
        if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
            throw IOException("This device is not paired yet.")
        }
        return PlaybackClient(baseUrl, token)
    }
}

/** Retains the one catalog across Activity recreation and owns its socket/coroutine lifetime. */
class ChannelCatalogViewModel(
    store: DeviceStore,
) : ViewModel() {
    val catalog = ChannelCatalog(store)

    override fun onCleared() {
        catalog.close()
    }
}

class ChannelCatalogViewModelFactory(
    private val store: DeviceStore,
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(ChannelCatalogViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return ChannelCatalogViewModel(store) as T
    }
}
