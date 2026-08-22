package tv.loomarr.tv.guide

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.loomarr.tv.design.DeadAir
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.Screen
import tv.loomarr.tv.design.TuningText

/**
 * The guide: what is on, across every channel, laid out against time.
 *
 * [Screen] supplies the overscan margin, so the leftmost channel names and the rightmost blocks stay
 * on a television that crops its own picture.
 */
@Composable
fun GuideScreen(
    model: GuideViewModel,
    onTune: (ChannelTimeline) -> Unit,
    onBack: () -> Unit = {},
    favoriteChannelIds: Set<String> = emptySet(),
    recentChannelIds: List<String> = emptyList(),
    playableChannelIds: Set<String>? = null,
) {
    val state by model.state.collectAsStateWithLifecycle()

    Screen {
        when (val current = state) {
            is GuideUiState.Loading ->
                TuningText("Loading the guide…", modifier = Modifier.align(Alignment.Center))

            is GuideUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))

            is GuideUiState.DeadAir ->
                DeadAir(
                    title = "Dead air",
                    modifier = Modifier.align(Alignment.Center),
                    description = "No channels are scheduled yet. Create one in Loomarr and it will appear here.",
                )

            is GuideUiState.Ready -> {
                val liveNowMs = rememberServerNow(current.nowMs)
                val playableWindow =
                    current.window.copy(
                        channels =
                            current.window.channels.filter { channel ->
                                playableChannelIds == null || channel.channelId in playableChannelIds
                            },
                    )
                GuideGrid(
                    window = playableWindow,
                    // ⚠ "Now" comes from the SERVER, not System.currentTimeMillis().
                    //
                    // The device clock cannot be trusted — the emulator's sits hours behind its
                    // host, and a television is exactly the hardware where that happens: some boxes
                    // have no RTC and NTP may not have synced. The first build of this screen read
                    // "Nothing scheduled" for a channel airing a film, because the device's idea of
                    // now fell outside the window the server had just sent.
                    //
                    // ⚠ And no longer `window.fromMs`. That was true only while the window started
                    // at now; it now opens deliberately EARLIER so the on-air block has room to its
                    // left, so the window's start and the current instant are different facts. The
                    // ViewModel carries the server's now separately.
                    nowMs = liveNowMs,
                    onTune = onTune,
                    onBack = onBack,
                    favoriteChannelIds = favoriteChannelIds,
                    recentChannelIds = recentChannelIds,
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }
    }
}
