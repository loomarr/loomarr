package tv.loomarr.tv.playback

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.loomarr.tv.design.LoomarrTokens

/**
 * The watch surface: full-screen video with a now-playing banner, surfed with the D-pad.
 *
 * ⚠ Nothing sits within 48dp of the edge. Older televisions crop the picture (overscan), and the
 * banner is the one thing that must stay readable when they do.
 */
@Composable
fun WatchScreen(model: WatchViewModel) {
    val state by model.state.collectAsStateWithLifecycle()
    val focus = remember { FocusRequester() }

    // The screen owns the D-pad, so it must hold focus from the moment it appears — otherwise the
    // first press goes nowhere and the remote feels broken.
    LaunchedEffect(Unit) { focus.requestFocus() }

    Box(
        modifier =
            Modifier
                .fillMaxSize()
                .background(LoomarrTokens.Static950)
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    when (event.key) {
                        // DPadUp/Down and the dedicated channel keys both surf: a TV remote may send
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
                Text(
                    text = "Loading channels…",
                    fontSize = 28.sp,
                    color = LoomarrTokens.Static0,
                    modifier = Modifier.align(Alignment.Center),
                )

            is WatchUiState.Failed ->
                Text(
                    text = current.message,
                    fontSize = 26.sp,
                    color = LoomarrTokens.Onair,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.align(Alignment.Center).padding(48.dp),
                )

            is WatchUiState.Ready -> {
                val channel = current.channels[current.selected]
                if (current.playUrl != null) {
                    PlayerScreen(playUrl = current.playUrl)
                } else {
                    Text(
                        text = "Tuning ${channel.name}…",
                        fontSize = 28.sp,
                        color = LoomarrTokens.Static400,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                NowPlaying(
                    channel = channel,
                    modifier = Modifier.align(Alignment.BottomStart).padding(48.dp).fillMaxWidth(),
                )
            }
        }
    }
}

@Composable
private fun NowPlaying(
    channel: Channel,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        Text(
            text = "${channel.number}  ${channel.name}",
            fontSize = 32.sp,
            color = LoomarrTokens.Static0,
        )
        Text(
            text = "Up and down to change channel",
            fontSize = 20.sp,
            color = LoomarrTokens.Static400,
        )
    }
}
