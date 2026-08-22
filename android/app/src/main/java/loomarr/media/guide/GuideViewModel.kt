package loomarr.media.guide

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import loomarr.media.pairing.DeviceStore
import loomarr.media.playback.ChannelCatalog
import loomarr.media.playback.ChannelCatalogState

/** What the guide surface is showing. */
sealed interface GuideUiState {
    data object Loading : GuideUiState

    data class Ready(
        val window: GuideWindow,
        /**
         * The server's "now", carried separately from the window.
         *
         * ⚠ NOT `window.fromMs`, and not the device clock either. The window deliberately opens
         * before now so the on-air programme has room, so its start is no longer the current
         * instant — and a television's clock cannot be trusted to supply one. This comes from the
         * server-authored HTTP `Date` header on a cheap health request.
         */
        val nowMs: Long,
    ) : GuideUiState

    /** Reachable and authenticated, but nothing is scheduled. */
    data object DeadAir : GuideUiState

    data class Failed(
        val message: String,
    ) : GuideUiState
}

/**
 * Loads the guide window.
 *
 * Refreshes from the shared Channel catalog's reconciliation boundary rather than running its own
 * timer. It also reloads when Guide is opened. SSE normally supplies a catalog boundary immediately;
 * foreground/reconnect and the catalog's low-frequency loss-recovery read keep Guide membership
 * correct when a frame is missed.
 */
class GuideViewModel(
    private val store: DeviceStore,
    private val catalog: ChannelCatalog,
    private val serverNowFor: suspend (baseUrl: String, token: String) -> Long = { url, token ->
        GuideClient(url, token).serverNowMs()
    },
    private val windowFor:
        suspend (
            baseUrl: String,
            token: String,
            fromMs: Long?,
            toMs: Long?,
        ) -> GuideWindow = { url, token, fromMs, toMs ->
        GuideClient(url, token).window(fromMs = fromMs, toMs = toMs)
    },
) : ViewModel() {
    private var artworkAuthorization: String? = null
    private val _state = MutableStateFlow<GuideUiState>(GuideUiState.Loading)
    val state: StateFlow<GuideUiState> = _state.asStateFlow()
    private var loadJob: Job? = null

    init {
        // A catalog revision means an authoritative Channel GET completed. Refreshing the guide
        // from the same revision boundary prevents a new Channel appearing in Surf while its Guide
        // row remains absent. Revisions advance on reconnect/foreground even if ids did not change.
        viewModelScope.launch {
            catalog.state
                .filterIsInstance<ChannelCatalogState.Ready>()
                .map { it.revision }
                .distinctUntilChanged()
                .collect { load() }
        }
    }

    fun load() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            val baseUrl = store.serverUrl()
            val token = store.token()
            if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
                _state.value = GuideUiState.Failed("This device is not paired yet.")
                return@launch
            }
            artworkAuthorization = "Bearer $token"

            val loaded =
                try {
                    loadGuideGridWindow(
                        serverNowMs = { serverNowFor(baseUrl, token) },
                        window = { fromMs, toMs -> windowFor(baseUrl, token, fromMs, toMs) },
                    )
                } catch (error: Exception) {
                    // A latency refresh must not erase the programme data already on screen.
                    if (_state.value !is GuideUiState.Ready) {
                        _state.value = GuideUiState.Failed(error.message ?: "Couldn't load the guide.")
                    }
                    return@launch
                }

            // Nothing scheduled is not a failure — it is dead air, and the test card says so.
            _state.value =
                if (loaded.window.channels.isEmpty()) {
                    GuideUiState.DeadAir
                } else {
                    GuideUiState.Ready(window = loaded.window, nowMs = loaded.nowMs)
                }
        }
    }

    /** Member-visible image renditions use the same paired credential as the Guide request. */
    internal fun artworkAuthorization(): String? = artworkAuthorization
}

internal data class LoadedGuideWindow(
    val window: GuideWindow,
    val nowMs: Long,
)

/**
 * Loads the four-hour grid around the server's current instant.
 *
 * A window starting at now clips the on-air programme against the left edge, while deriving an
 * earlier bound from the TV clock makes the whole grid wrong when that clock has drifted. Server
 * time comes from a cheap health request rather than an unbounded guide request: computing every
 * channel timeline twice would make the clock probe the most expensive part of loading the grid.
 */
internal suspend fun loadGuideGridWindow(
    serverNowMs: suspend () -> Long,
    window: suspend (fromMs: Long?, toMs: Long?) -> GuideWindow,
): LoadedGuideWindow {
    val nowMs = serverNowMs()
    val visible =
        window(
            nowMs - GRID_LOOKBACK_MS,
            nowMs - GRID_LOOKBACK_MS + GRID_SPAN_MS,
        )
    return LoadedGuideWindow(window = visible, nowMs = nowMs)
}

/** Builds [GuideViewModel] — see PairingViewModelFactory for why a factory is required. */
class GuideViewModelFactory(
    private val store: DeviceStore,
    private val catalog: ChannelCatalog,
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(GuideViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return GuideViewModel(store, catalog) as T
    }
}

/**
 * How far before "now" the grid opens.
 *
 * Thirty minutes gives the on-air programme visible room to its left instead of pinning it to the
 * edge — enough that its title and times fit, without spending much of the window on schedule the
 * viewer has already missed.
 */
private const val GRID_LOOKBACK_MS = 30 * 60 * 1000L

/**
 * How much schedule the 10-foot grid shows at once.
 *
 * Two hours matches the supplied TV composition and keeps a normal 22-minute episode wide enough
 * to carry its series name. The web can show four hours because it has a wider pointer-driven
 * viewport; copying that span into the overscan-safe TV canvas produced an almost blank grid of
 * unlabeled slivers on the real Simpsons schedule.
 */
private const val GRID_SPAN_MS = 2 * 60 * 60 * 1000L
