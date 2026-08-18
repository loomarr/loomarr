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
    private val clientFor: (baseUrl: String, token: String) -> GuideClient = { url, token ->
        GuideClient(url, token)
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

            val window =
                try {
                    // The server's default four-hour window, the same span the web grid shows.
                    //
                    // An earlier draft narrowed this to two hours because a feature film could not
                    // hold its own time range at 4h. That was fixing the wrong end: the span is what
                    // the guide MEANS, and halving it halves what the viewer can see coming. The
                    // block was short of room, so the room is what changed — a narrower channel
                    // list and smaller block type, not less schedule.
                    clientFor(baseUrl, token).window()
                } catch (error: Exception) {
                    _state.value = GuideUiState.Failed(error.message ?: "Couldn't load the guide.")
                    return@launch
                }

            // Nothing scheduled is not a failure — it is dead air, and the test card says so.
            _state.value =
                if (window.channels.isEmpty()) GuideUiState.DeadAir else GuideUiState.Ready(window)
        }
    }
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
