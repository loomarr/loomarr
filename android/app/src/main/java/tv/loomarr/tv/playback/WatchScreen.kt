package tv.loomarr.tv.playback

import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.Heading
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.Screen

/**
 * The watch surface: full-screen video with a now-playing banner, surfed with the D-pad.
 *
 * [Screen] supplies the overscan margin, so the banner stays readable on a television that crops
 * its own picture.
 */
@Composable
fun WatchScreen(model: WatchViewModel) {
    val state by model.state.collectAsStateWithLifecycle()
    val focus = remember { FocusRequester() }

    // The screen owns the D-pad, so it must hold focus from the moment it appears — otherwise the
    // first press goes nowhere and the remote feels broken.
    LaunchedEffect(Unit) { focus.requestFocus() }

    Screen(
        modifier =
            Modifier
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    when (event.key) {
                        // DPad and the dedicated channel keys both surf: a TV remote may send
                        // either, and a viewer pressing CHANNEL+ expects the same thing as up.
                        Key.DirectionUp, Key.ChannelUp -> {
                            model.channelUp()
                            true
                        }
                        Key.DirectionDown, Key.ChannelDown -> {
                            model.channelDown()
                            true
                        }
                        else -> false
                    }
                },
    ) {
        when (val current = state) {
            is WatchUiState.Loading ->
                Body("Loading channels…", modifier = Modifier.align(Alignment.Center))

            is WatchUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))

            is WatchUiState.Ready -> {
                val channel = current.channels[current.selected]
                if (current.playUrl != null) {
                    PlayerScreen(playUrl = current.playUrl)
                } else {
                    Body("Tuning ${channel.name}…", modifier = Modifier.align(Alignment.Center))
                }
                NowPlaying(
                    channel = channel,
                    modifier = Modifier.align(Alignment.BottomStart).fillMaxWidth(),
                )
            }
        }
    }
}

/** What is on, and how to change it. */
@Composable
private fun NowPlaying(
    channel: Channel,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        Heading("${channel.number}  ${channel.name}")
        Body(
            "Up and down to change channel",
            color = LoomarrTokens.Color.Static400,
        )
    }
}
