package tv.loomarr.tv.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tv.loomarr.tv.PairingOffer
import tv.loomarr.tv.guide.Airing
import tv.loomarr.tv.guide.ChannelTimeline
import tv.loomarr.tv.guide.GuideGrid
import tv.loomarr.tv.guide.GuideUiState
import tv.loomarr.tv.guide.GuideWindow
import tv.loomarr.tv.pairing.PairingUiState
import tv.loomarr.tv.playback.Channel
import tv.loomarr.tv.playback.SurfRail
import tv.loomarr.tv.playback.WatchUiState
import tv.loomarr.tv.playback.WatchingChrome

/**
 * Screenshots of the design system's pieces.
 *
 * These render real Compose on the JVM through Robolectric, so a styling regression fails the build
 * on an ordinary CI runner with no emulator attached — the same bargain web's visual suite makes.
 *
 * ⚠ What a screenshot test is FOR here is the thing a unit test cannot state: that a badge is the
 * right red, that a chip's text clears its background, that type has not started overlapping. It is
 * not a substitute for asserting behaviour — the guide's `TimelineTest` still owns the geometry,
 * because "is this block 234dp wide" deserves an assertion with a number in it, not a picture.
 */
@RunWith(RobolectricTestRunner::class)
// ⚠ NATIVE graphics, not the default. Robolectric's legacy renderer draws nothing — every canvas
// operation is a no-op — so a screenshot taken under it is a blank image that matches its own blank
// baseline forever. A test that cannot fail is worse than no test.
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w960dp-h540dp-television-xhdpi")
class DesignScreenshotTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `live badge`() {
        compose.setContent {
            // On the app background, because that is where it appears and because a badge that
            // relies on a tint has no contrast to measure against a white default.
            Box(
                modifier =
                    Modifier
                        .background(LoomarrTokens.Color.Static950)
                        .padding(LoomarrTokens.Space.S6),
            ) {
                LiveBadge()
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `color bars`() {
        compose.setContent {
            Box(modifier = Modifier.background(LoomarrTokens.Color.Static950)) {
                ColorBars()
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `pairing with a real hostname stays on one line`() {
        compose.setContent {
            CenteredScreen {
                ColorBars(modifier = Modifier.padding(bottom = LoomarrTokens.Space.S6))
                PairingOffer(
                    state =
                        PairingUiState.AwaitingApproval(
                            userCode = "WMQJ-QVFJ",
                            verificationUri = "loomarr.projectguacamole.com/pair",
                            verificationUriComplete =
                                "https://loomarr.projectguacamole.com/pair?code=WMQJ-QVFJ",
                            secondsRemaining = 593,
                        ),
                    onRefresh = {},
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `android tv watching`() {
        compose.setContent {
            Box(
                modifier =
                    Modifier
                        .fillMaxSize()
                        .background(LoomarrTokens.Color.Static800),
            ) {
                WatchingChrome(
                    channel = sampleChannels[2],
                    guide = sampleGuide,
                    numberEntry = "21",
                    numberEntryChannelName = "Nature Documentaries",
                    visibleNonce = 1,
                    modifier = Modifier.fillMaxSize().padding(OverscanMargin),
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    @Config(qualifiers = "w960dp-h540dp-television-xxxhdpi")
    fun `android tv watching at 4k density`() {
        compose.setContent {
            Box(
                modifier =
                    Modifier
                        .fillMaxSize()
                        .background(LoomarrTokens.Color.Static800),
            ) {
                WatchingChrome(
                    channel = sampleChannels[2],
                    guide = sampleGuide,
                    numberEntry = "21",
                    numberEntryChannelName = "Nature Documentaries",
                    visibleNonce = 1,
                    modifier = Modifier.fillMaxSize().padding(OverscanMargin),
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `android tv surf`() {
        compose.setContent {
            Box(
                modifier =
                    Modifier
                        .fillMaxSize()
                        .background(LoomarrTokens.Color.Static800),
            ) {
                SurfRail(
                    state =
                        WatchUiState.Ready(
                            channels = sampleChannels,
                            selected = 2,
                            playUrl = "preview",
                            lastChannelId = sampleChannels[0].id,
                            recentChannelIds = listOf(sampleChannels[1].id),
                        ),
                    guide = sampleGuide,
                    onTune = {},
                    onCancel = {},
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `android tv guide`() {
        compose.setContent {
            Screen {
                GuideGrid(
                    window = (sampleGuide as GuideUiState.Ready).window,
                    nowMs = sampleNow,
                    onTune = {},
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    private val sampleChannels =
        listOf(
            Channel("noir", "Noir Nights", 19, true),
            Channel("games", "Game Show Vault", 20, true),
            Channel("nature", "Nature Documentaries", 21, true),
            Channel("horror", "Cozy Autumn Horror", 22, true),
            Channel("kung-fu", "Kung Fu Theater", 23, true),
            Channel("anime", "Midnight Anime", 24, true),
        )

    private val sampleNow = 8_700_000L

    private val sampleGuide: GuideUiState =
        GuideUiState.Ready(
            nowMs = sampleNow,
            window =
                GuideWindow(
                    fromMs = 7_200_000L,
                    toMs = 14_400_000L,
                    channels =
                        sampleChannels.mapIndexed { index, channel ->
                            ChannelTimeline(
                                channelId = channel.id,
                                name = channel.name,
                                number = channel.number,
                                status = "live",
                                pendingCount = 0,
                                airings =
                                    listOf(
                                        sampleAiring("${channel.name} Premiere", 7_200_000L, 9_000_000L),
                                        sampleAiring(
                                            if (index == 2) "Blue Planet — The Deep" else "${channel.name} Late",
                                            9_000_000L,
                                            11_100_000L,
                                        ),
                                        sampleAiring("After Hours", 11_100_000L, 14_400_000L),
                                    ),
                            )
                        },
                ),
        )

    private fun sampleAiring(
        title: String,
        startMs: Long,
        stopMs: Long,
    ) = Airing(
        kind = "program",
        title = title,
        series = null,
        season = 0,
        episode = 0,
        startMs = startMs,
        stopMs = stopMs,
        nominal = false,
        provenance = "in library",
    )
}
