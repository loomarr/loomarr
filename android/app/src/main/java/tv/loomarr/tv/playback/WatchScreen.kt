package tv.loomarr.tv.playback

import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
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
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.Panel
import tv.loomarr.tv.design.Screen
import tv.loomarr.tv.design.TuningText

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
                TuningText("Tuning in…", modifier = Modifier.align(Alignment.Center))

            is WatchUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))

            is WatchUiState.Ready -> {
                val channel = current.channels[current.selected]
                if (current.playUrl != null) {
                    PlayerScreen(playUrl = current.playUrl)
                } else {
                    // `tune` cyan is the token for an in-progress state, so the colour carries the
                    // meaning rather than the word alone.
                    TuningText(
                        "Tuning ${channel.name}…",
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                NowPlaying(
                    channel = channel,
                    modifier = Modifier.align(Alignment.BottomStart).fillMaxWidth(),
                )
            }
        }
    }
}

/**
 * What is on, and how to change it — the TV counterpart of web's now-playing strip.
 *
 * Sits on a [Panel] so it stays legible over video: a bordered `static-900` surface rather than
 * text floating on a moving picture, which is unreadable the moment a bright frame passes under it.
 */
@Composable
private fun NowPlaying(
    channel: Channel,
    modifier: Modifier = Modifier,
) {
    Panel(modifier = modifier) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            // ⚠ The channel number is mono. frontend-design §2.2 names channel numbers explicitly:
            // "if it came from a machine, it's set in mono", and the web guide sets them the same
            // way — a proportional number here would read as a different product.
            MonoData(
                channel.number.toString(),
                color = LoomarrTokens.Color.Signal,
            )
            Heading(
                channel.name,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S4),
            )
        }
        Body(
            "Up and down to change channel",
            modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
        )
    }
}
