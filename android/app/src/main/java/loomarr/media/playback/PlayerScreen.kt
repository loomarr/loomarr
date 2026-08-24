package loomarr.media.playback

import android.view.ViewGroup
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.Timeline
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.hls.HlsMediaSource
import androidx.media3.ui.PlayerView
import kotlinx.coroutines.delay
import loomarr.media.diagnostics.ClientEvent

data class PlayerDiagnostic(
    val event: ClientEvent,
    val reason: String? = null,
    val errorCode: String? = null,
    val fatal: Boolean? = null,
    val viewerTimeMs: Long? = null,
    val bufferedMs: Long? = null,
)

/** Convert Media3's corrected origin clock and decoded-frame live offset into programme wall time. */
internal fun frameUnixTimeMs(
    originNowMs: Long,
    liveOffsetMs: Long,
): Long? =
    if (originNowMs == C.TIME_UNSET || liveOffsetMs == C.TIME_UNSET) {
        null
    } else {
        originNowMs - liveOffsetMs
    }

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
private fun Player.frameUnixTimeMs(window: Timeline.Window): Long? {
    if (currentTimeline.isEmpty || currentMediaItemIndex == C.INDEX_UNSET) return null
    currentTimeline.getWindow(currentMediaItemIndex, window)
    return frameUnixTimeMs(window.currentUnixTimeMs, currentLiveOffset)
}

/**
 * Full-screen playback of one channel.
 *
 * The URL is already signed and the server has already appended its credential to every segment URI
 * and to `#EXT-X-MAP:URI` — so no custom DataSource header or query injection is needed here. That
 * is deliberate on the server side (`rewritePlaylistAuth`): a native player will not re-append a
 * query parameter the way hls.js can be made to.
 *
 * ⚠ `@OptIn(UnstableApi)` is required: HlsMediaSource, DefaultHttpDataSource and PlayerView are all
 * marked unstable in Media3, which flags API that may change between minor releases rather than API
 * that is unfinished. Every Media3-based player opts in, Jellyfin included. It is annotated on this
 * one composable rather than suppressed project-wide so the opt-in stays visible where it applies.
 */
@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
@Composable
fun PlayerScreen(
    playUrl: String,
    onError: (String) -> Unit = {},
    onViewerTimeChanged: (Long) -> Unit = {},
    onDiagnostic: (PlayerDiagnostic) -> Unit = {},
) {
    val context = LocalContext.current

    val player =
        remember(playUrl) {
            ExoPlayer.Builder(context).build().apply {
                // Explicit HLS source rather than inferring from the URL: the signed URL carries a
                // `?sig=` query, and extension sniffing on `master.m3u8?sig=…` is one more thing to
                // get wrong for no benefit.
                val source =
                    HlsMediaSource
                        .Factory(
                            androidx.media3.datasource.DefaultHttpDataSource
                                .Factory(),
                        ).createMediaSource(MediaItem.fromUri(playUrl))
                setMediaSource(source)
                playWhenReady = true
                prepare()
            }
        }
    val currentViewerTimeChanged by rememberUpdatedState(onViewerTimeChanged)
    val currentDiagnostic by rememberUpdatedState(onDiagnostic)

    // Media3 already maps HLS PROGRAM-DATE-TIME onto the origin server clock. Subtracting the
    // player's current live offset reports the frame actually being rendered, including the normal
    // safety buffer; a device clock is never consulted. Poll alongside the visible progress bar —
    // Player.Listener has no continuous-position callback.
    LaunchedEffect(player) {
        val window = Timeline.Window()
        while (true) {
            player.frameUnixTimeMs(window)?.let(currentViewerTimeChanged)
            delay(250)
        }
    }

    DisposableEffect(player) {
        val listener =
            object : Player.Listener {
                override fun onPlaybackStateChanged(playbackState: Int) {
                    val viewerTime = player.frameUnixTimeMs(Timeline.Window())
                    when (playbackState) {
                        Player.STATE_BUFFERING ->
                            viewerTime?.let {
                                currentDiagnostic(
                                    PlayerDiagnostic(
                                        event = ClientEvent.PlayerBufferingStarted,
                                        viewerTimeMs = it,
                                        bufferedMs = player.totalBufferedDuration.coerceAtLeast(0),
                                    ),
                                )
                            }
                        Player.STATE_READY -> {
                            viewerTime?.let {
                                currentDiagnostic(
                                    PlayerDiagnostic(
                                        ClientEvent.PlayerBufferingEnded,
                                        viewerTimeMs = it,
                                        bufferedMs = player.totalBufferedDuration.coerceAtLeast(0),
                                    ),
                                )
                            }
                            currentDiagnostic(PlayerDiagnostic(ClientEvent.PlayerReady))
                        }
                        Player.STATE_IDLE, Player.STATE_ENDED -> Unit
                    }
                }

                override fun onPositionDiscontinuity(
                    oldPosition: Player.PositionInfo,
                    newPosition: Player.PositionInfo,
                    reason: Int,
                ) {
                    if (reason != Player.DISCONTINUITY_REASON_SEEK) return
                    val viewerTime = player.frameUnixTimeMs(Timeline.Window()) ?: return
                    currentDiagnostic(PlayerDiagnostic(ClientEvent.PlayerSeeking, viewerTimeMs = viewerTime))
                    currentDiagnostic(PlayerDiagnostic(ClientEvent.PlayerSeeked, viewerTimeMs = viewerTime))
                }

                override fun onPlayerError(error: androidx.media3.common.PlaybackException) {
                    // ⚠ Report and STOP. Jellyfin shipped the same bug twice (#5422, #5703): an
                    // unmapped track restarted playback, which failed the same way, which restarted
                    // again — roughly once a second, reporting a play to the server every time.
                    // Whatever recovery is added later must be bounded and must not loop on the
                    // same failure.
                    onError(error.errorCodeName)
                    currentDiagnostic(
                        PlayerDiagnostic(
                            event = ClientEvent.PlayerMediaError,
                            errorCode = error.errorCodeName.take(64),
                            fatal = true,
                        ),
                    )
                }
            }
        player.addListener(listener)
        currentDiagnostic(PlayerDiagnostic(ClientEvent.PlayerAttached, reason = "mount"))
        onDispose {
            currentDiagnostic(PlayerDiagnostic(ClientEvent.PlayerDetached, reason = "unmount"))
            player.removeListener(listener)
            player.release()
        }
    }

    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = {
            PlayerView(it).apply {
                this.player = player
                useController = false
                layoutParams =
                    ViewGroup.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT,
                    )
            }
        },
    )
}
