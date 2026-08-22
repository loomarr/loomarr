package tv.loomarr.tv.guide

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import tv.loomarr.tv.pairing.DeviceStore

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
 * Deliberately does not poll. A schedule changes on the order of minutes and a viewer is looking at
 * a grid for seconds; a background refresh that redraws under a moving focus ring costs more than
 * the freshness is worth. The window is re-fetched when the surface is entered.
 */
class GuideViewModel(
    private val store: DeviceStore,
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
    private val _state = MutableStateFlow<GuideUiState>(GuideUiState.Loading)
    val state: StateFlow<GuideUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            val baseUrl = store.serverUrl()
            val token = store.token()
            if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
                _state.value = GuideUiState.Failed("This device is not paired yet.")
                return@launch
            }

            val loaded =
                try {
                    loadGuideGridWindow(
                        serverNowMs = { serverNowFor(baseUrl, token) },
                        window = { fromMs, toMs -> windowFor(baseUrl, token, fromMs, toMs) },
                    )
                } catch (error: Exception) {
                    _state.value = GuideUiState.Failed(error.message ?: "Couldn't load the guide.")
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
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(GuideViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return GuideViewModel(store) as T
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
 * How much schedule the grid shows at once.
 *
 * Four hours, the same span the web grid defaults to. An earlier draft halved it because a feature
 * film could not hold its own time range — that was fixing the wrong end. The span is what the guide
 * MEANS, and halving it halves what the viewer can see coming; the block was short of room, so the
 * room is what changed.
 */
private const val GRID_SPAN_MS = 4 * 60 * 60 * 1000L
