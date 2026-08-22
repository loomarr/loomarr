package loomarr.media.playback

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch
import loomarr.media.navigation.TuneHistory
import loomarr.media.navigation.channelIndexForNumber
import loomarr.media.pairing.DeviceStore

/** What the watch surface is showing. */
sealed interface WatchUiState {
    data object Loading : WatchUiState

    /** Channels are loaded; [playUrl] is set once one has been tuned. */
    data class Ready(
        val channels: List<Channel>,
        val selected: Int,
        val playUrl: String?,
        val recentChannelIds: List<String> = emptyList(),
    ) : WatchUiState

    data class Failed(
        val message: String,
    ) : WatchUiState

    /**
     * Reachable and authenticated, but nothing is scheduled to play.
     *
     * Distinct from [Failed] because it is not a failure: nothing is wrong, there is simply nothing
     * on. That is the difference between an error message and a test card, and a viewer should see
     * the second.
     */
    data object DeadAir : WatchUiState
}

/**
 * Channel list plus tuning.
 *
 * Tuning mints a fresh signed URL per channel rather than caching one: the URL is scoped to a single
 * channel and expires, so there is nothing useful to reuse across a channel change.
 */
class WatchViewModel(
    private val store: DeviceStore,
    private val catalog: ChannelCatalog,
    private val clientFor: (baseUrl: String, token: String) -> PlaybackClient = { url, token ->
        PlaybackClient(url, token)
    },
    private val profile: DeviceProfile = DeviceProfile.probe(),
) : ViewModel() {
    private val _state = MutableStateFlow<WatchUiState>(WatchUiState.Loading)
    val state: StateFlow<WatchUiState> = _state.asStateFlow()

    private var client: PlaybackClient? = null
    private var history = TuneHistory()

    init {
        observeCatalog()
    }

    private fun observeCatalog() {
        viewModelScope.launch {
            val baseUrl = store.serverUrl()
            val token = store.token()
            if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
                _state.value = WatchUiState.Failed("This device is not paired yet.")
                return@launch
            }
            client = clientFor(baseUrl, token)

            catalog.state.collect(::applyCatalog)
        }
    }

    /** Reconcile playback by Channel identity, never by a stale list index. */
    private fun applyCatalog(catalogState: ChannelCatalogState) {
        when (catalogState) {
            ChannelCatalogState.Loading -> {
                if (_state.value !is WatchUiState.Ready) _state.value = WatchUiState.Loading
            }

            is ChannelCatalogState.Failed -> {
                // This is only reachable before a complete snapshot exists. Refresh failures after
                // that retain Ready in ChannelCatalog and therefore cannot tear down live video.
                if (_state.value !is WatchUiState.Ready) {
                    _state.value = WatchUiState.Failed(catalogState.message)
                }
            }

            is ChannelCatalogState.Ready -> reconcileChannels(catalogState.channels)
        }
    }

    private fun reconcileChannels(channels: List<Channel>) {
        if (channels.isEmpty()) {
            _state.value = WatchUiState.DeadAir
            return
        }

        val previous = _state.value as? WatchUiState.Ready
        val previousChannelId = previous?.channels?.getOrNull(previous.selected)?.id
        val selected = channels.indexOfFirst { it.id == previousChannelId }.takeIf { it >= 0 } ?: 0
        val selectedChannelId = channels[selected].id
        val keptCurrentChannel = previousChannelId == selectedChannelId

        _state.value =
            WatchUiState.Ready(
                channels = channels,
                selected = selected,
                playUrl = previous?.playUrl.takeIf { keptCurrentChannel },
                recentChannelIds = history.recentChannelIds,
            )

        // Additions and metadata edits do not disturb playback. Initial load or removal of the
        // tuned Channel selects the deterministic first playable Channel and mints a fresh URL.
        if (!keptCurrentChannel) tune(selected)
    }

    /** Tune the channel at [index], wrapping at both ends so surfing is continuous. */
    fun tune(index: Int) {
        val current = _state.value as? WatchUiState.Ready ?: return
        val playback = client ?: return
        val wrapped = ((index % current.channels.size) + current.channels.size) % current.channels.size
        history = history.tuned(current.channels[wrapped].id)

        // Clear the URL first so the player tears down the old stream rather than showing the
        // previous channel while the next one is still being minted.
        _state.value =
            current.copy(
                selected = wrapped,
                playUrl = null,
                recentChannelIds = history.recentChannelIds,
            )

        viewModelScope.launch {
            try {
                val url = playback.playUrl(current.channels[wrapped].id, profile)
                val latest = _state.value as? WatchUiState.Ready ?: return@launch
                // ⚠ Latest-request-wins. Surfing fires tunes faster than they resolve, and a slow
                // response for a channel the viewer has already left would otherwise replace the one
                // they are on now.
                if (latest.channels.getOrNull(latest.selected)?.id == current.channels[wrapped].id) {
                    _state.value = latest.copy(playUrl = url.url)
                }
            } catch (error: Exception) {
                _state.value = WatchUiState.Failed(error.message ?: "Couldn't tune that channel.")
            }
        }
    }

    /**
     * Tune the channel with this id — how the guide hands a selection to playback.
     *
     * Falls back to tuning nothing rather than guessing when the id is absent: the guide and the
     * watch surface load their channel lists independently, so a channel present in one and not the
     * other is a real possibility, and tuning "whatever was at that index" would put a viewer on a
     * programme they did not choose.
     */
    fun tuneChannelId(channelId: String) {
        val current = _state.value as? WatchUiState.Ready ?: return
        val index = current.channels.indexOfFirst { it.id == channelId }
        if (index >= 0) tune(index)
    }

    /** Tune an exact Channel number after the remote's bounded digit-entry timeout. */
    fun tuneChannelNumber(digits: String) {
        val current = _state.value as? WatchUiState.Ready ?: return
        channelIndexForNumber(current.channels, digits)?.let(::tune)
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
    private val catalog: ChannelCatalog,
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(WatchViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return WatchViewModel(store, catalog) as T
    }
}
