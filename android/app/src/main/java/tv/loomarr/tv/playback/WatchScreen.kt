package tv.loomarr.tv.playback

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
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
import kotlinx.coroutines.delay
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.DeadAir
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.Heading
import tv.loomarr.tv.design.LiveBadge
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
fun WatchScreen(
    model: WatchViewModel,
    onOpenGuide: () -> Unit = {},
) {
    val state by model.state.collectAsStateWithLifecycle()
    val focus = remember { FocusRequester() }

    // Bumped by any key the screen handles, to re-show the now-playing banner. A counter rather
    // than a boolean because the banner hides itself on a timer: setting a flag that is already
    // true would not restart it, so pressing a key while the banner was fading would do nothing.
    var bannerNonce by remember { mutableIntStateOf(0) }

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

                    // Any handled key re-shows the banner, including DPad centre — which is
                    // otherwise inert here. On a television "where am I?" is the commonest
                    // question during playback, and pressing something to find out is the
                    // universal answer; a viewer should not have to change channel to see the
                    // channel they are on.
                    bannerNonce++

                    when (event.key) {
                        // DPad and the dedicated channel keys both surf: a TV remote may send
                        // either, and a viewer pressing CHANNEL+ expects the same thing as up.
                        // ⚠ Up opens the GUIDE rather than surfing. On a television that is the
                        // near-universal convention, and the dedicated CHANNEL+ key still surfs —
                        // so a viewer gets both behaviours without either being hidden.
                        Key.DirectionUp, Key.Menu -> {
                            onOpenGuide()
                            true
                        }
                        Key.ChannelUp -> {
                            model.channelUp()
                            true
                        }
                        Key.DirectionDown, Key.ChannelDown -> {
                            model.channelDown()
                            true
                        }
                        // Consumed so it does not travel on and trigger something else; the reveal
                        // above is the whole effect.
                        Key.DirectionCenter, Key.Enter -> true
                        else -> false
                    }
                },
    ) {
        when (val current = state) {
            is WatchUiState.Loading ->
                TuningText("Tuning in…", modifier = Modifier.align(Alignment.Center))

            is WatchUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))

            // Nothing is wrong — there is simply nothing on, which is what a test card is for.
            is WatchUiState.DeadAir ->
                DeadAir(
                    title = "Dead air",
                    description = "No channels are scheduled yet. Create one in Loomarr and it will appear here.",
                    modifier = Modifier.align(Alignment.Center),
                )

            is WatchUiState.Ready -> {
                val channel = current.channels[current.selected]
                val playing = current.playUrl != null

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

                // Re-shown whenever the channel changes — `selected` is the key, so surfing brings
                // the banner back without the viewer asking. `bannerNonce` lets a plain key press
                // do the same without a channel change, so a viewer who has forgotten where they
                // are can check.
                var banner by remember { mutableStateOf(true) }
                LaunchedEffect(current.selected, bannerNonce) {
                    banner = true
                    delay(BANNER_VISIBLE_MS)
                    banner = false
                }

                AnimatedVisibility(
                    visible = banner,
                    // Slides from below rather than fading: over moving video a cross-fade reads as
                    // a compression artefact, while motion from off-screen is unambiguous.
                    enter = slideInVertically { it } + fadeIn(),
                    exit = slideOutVertically { it } + fadeOut(),
                    modifier = Modifier.align(Alignment.BottomStart),
                ) {
                    NowPlaying(
                        channel = channel,
                        // Only claim LIVE while something is actually playing. A tuning channel is
                        // not live yet, and a badge that is always on is decoration rather than
                        // status.
                        live = playing,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }
            }
        }
    }
}

/**
 * What is on, and how to change it — the TV counterpart of web's now-playing strip.
 *
 * Sits on a [Panel] so it stays legible over video: a bordered `static-900` surface rather than
 * text floating on a moving picture, which is unreadable the moment a bright frame passes under it.
 *
 * ⚠ It HIDES. A now-playing bar that never goes away is not information, it is an obstruction: it
 * covers the bottom of every frame for as long as the viewer watches, and the one thing a
 * television is for is the picture. It appears on a channel change, which is when a viewer wants
 * confirmation of where they landed, and withdraws once that question is answered.
 */
@Composable
private fun NowPlaying(
    channel: Channel,
    live: Boolean,
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
                // Explicit, because the default dropped to `Md` for the pairing screen's long
                // server address. A channel number is two or three glyphs and is the thing a
                // viewer reads while surfing, so it keeps the larger size.
                fontSize = LoomarrTokens.Type.Lg,
            )
            Heading(
                channel.name,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S4),
            )
            if (live) {
                LiveBadge(modifier = Modifier.padding(start = LoomarrTokens.Space.S4))
            }
        }
        Body(
            "Up and down to change channel",
            modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
        )
    }
}

/**
 * How long the now-playing banner stays up after a channel change or a key press.
 *
 * Five seconds, matching the convention every set-top box has trained viewers on. Long enough to
 * read a channel name and number without hurrying, short enough that it is gone before it starts
 * feeling like part of the picture.
 */
private const val BANNER_VISIBLE_MS = 5_000L
