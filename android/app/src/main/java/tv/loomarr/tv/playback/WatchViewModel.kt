package tv.loomarr.tv.playback

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import tv.loomarr.tv.pairing.DeviceStore

/** What the watch surface is showing. */
sealed interface WatchUiState {
    data object Loading : WatchUiState

    /** Channels are loaded; [playUrl] is set once one has been tuned. */
    data class Ready(
        val channels: List<Channel>,
        val selected: Int,
        val playUrl: String?,
    ) : WatchUiState

    data class Failed(
        val message: String,
    ) : WatchUiState
}

/**
 * Channel list plus tuning.
 *
 * Tuning mints a fresh signed URL per channel rather than caching one: the URL is scoped to a single
 * channel and expires, so there is nothing useful to reuse across a channel change.
 */
class WatchViewModel(
    private val store: DeviceStore,
    private val clientFor: (baseUrl: String, token: String) -> PlaybackClient = { url, token ->
        PlaybackClient(url, token)
    },
    private val profile: DeviceProfile = DeviceProfile.probe(),
) : ViewModel() {
    private val _state = MutableStateFlow<WatchUiState>(WatchUiState.Loading)
    val state: StateFlow<WatchUiState> = _state.asStateFlow()

    private var client: PlaybackClient? = null

    init {
        load()
    }

    private fun load() {
        viewModelScope.launch {
            val baseUrl = store.serverUrl()
            val token = store.token()
            if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
                _state.value = WatchUiState.Failed("This device is not paired yet.")
                return@launch
            }
            val playback = clientFor(baseUrl, token)
            client = playback

            val channels =
                try {
                    // Only channels the server will actually mint a URL for. `inAppPlayable` is the
                    // same predicate behind its 409, so filtering here turns a tune-time error into
                    // a channel that never appears in the list.
                    playback.channels().filter { it.inAppPlayable }
                } catch (error: Exception) {
                    _state.value = WatchUiState.Failed(error.message ?: "Couldn't load channels.")
                    return@launch
                }

            if (channels.isEmpty()) {
                _state.value = WatchUiState.Failed("No channels are available to play on this device yet.")
                return@launch
            }

            _state.value = WatchUiState.Ready(channels = channels, selected = 0, playUrl = null)
            tune(0)
        }
    }

    /** Tune the channel at [index], wrapping at both ends so surfing is continuous. */
    fun tune(index: Int) {
        val current = _state.value as? WatchUiState.Ready ?: return
        val playback = client ?: return
        val wrapped = ((index % current.channels.size) + current.channels.size) % current.channels.size

        // Clear the URL first so the player tears down the old stream rather than showing the
        // previous channel while the next one is still being minted.
        _state.value = current.copy(selected = wrapped, playUrl = null)

        viewModelScope.launch {
            try {
                val url = playback.playUrl(current.channels[wrapped].id, profile)
                val latest = _state.value as? WatchUiState.Ready ?: return@launch
                // ⚠ Latest-request-wins. Surfing fires tunes faster than they resolve, and a slow
                // response for a channel the viewer has already left would otherwise replace the one
                // they are on now.
                if (latest.selected == wrapped) {
                    _state.value = latest.copy(playUrl = url.url)
                }
            } catch (error: Exception) {
                _state.value = WatchUiState.Failed(error.message ?: "Couldn't tune that channel.")
            }
        }
    }

    fun channelUp() {
        (_state.value as? WatchUiState.Ready)?.let { tune(it.selected + 1) }
    }

    fun channelDown() {
        (_state.value as? WatchUiState.Ready)?.let { tune(it.selected - 1) }
    }
}

/** Builds [WatchViewModel] — see PairingViewModelFactory for why a factory is required. */
class WatchViewModelFactory(
    private val store: DeviceStore,
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(WatchViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return WatchViewModel(store) as T
    }
}
