package loomarr.media.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import com.github.takahirom.roborazzi.captureRoboImage
import loomarr.media.PairingOffer
import loomarr.media.guide.Airing
import loomarr.media.guide.ChannelTimeline
import loomarr.media.guide.GUIDE_FOCUSED_FILTER_TAG
import loomarr.media.guide.GUIDE_GRID_TAG
import loomarr.media.guide.GuideGrid
import loomarr.media.guide.GuideSurface
import loomarr.media.guide.GuideUiState
import loomarr.media.guide.GuideWindow
import loomarr.media.pairing.PairingUiState
import loomarr.media.playback.Channel
import loomarr.media.playback.NOW_PLAYING_BAR_TAG
import loomarr.media.playback.SurfRail
import loomarr.media.playback.WatchUiState
import loomarr.media.playback.WatchingChrome
import loomarr.media.playback.watchingChromeContainer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.util.TimeZone

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

    private lateinit var originalTimeZone: TimeZone

    @Before
    fun useDeterministicTimeZone() {
        originalTimeZone = TimeZone.getDefault()
        // Production intentionally uses the television's zone. Pin only the screenshot fixture so
        // a developer workstation and GitHub's UTC runner render the same schedule labels.
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"))
    }

    @After
    fun restoreTimeZone() {
        TimeZone.setDefault(originalTimeZone)
    }

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
                    modifier = Modifier.watchingChromeContainer(),
                )
            }
        }
        compose.onRoot().captureRoboImage()
    }

    @Test
    fun `watching chrome clears after inactivity`() {
        compose.mainClock.autoAdvance = false
        compose.setContent {
            Box(modifier = Modifier.fillMaxSize().background(LoomarrTokens.Color.Static800)) {
                WatchingChrome(
                    channel = sampleChannels[2],
                    guide = sampleGuide,
                    numberEntry = "",
                    visibleNonce = 1,
                    modifier = Modifier.watchingChromeContainer(),
                )
            }
        }

        compose.onNodeWithText("NATURE DOCUMENTARIES").assertIsDisplayed()
        compose.mainClock.advanceTimeBy(6_000)
        compose.waitForIdle()
        compose.onAllNodesWithText("NATURE DOCUMENTARIES").assertCountEquals(0)
    }

    @Test
    fun `watching programme bar spans the screen and reaches the bottom edge`() {
        compose.setContent {
            Box(modifier = Modifier.fillMaxSize().background(LoomarrTokens.Color.Static800)) {
                WatchingChrome(
                    channel = sampleChannels[2],
                    guide = sampleGuide,
                    numberEntry = "",
                    visibleNonce = 1,
                    modifier = Modifier.watchingChromeContainer(),
                )
            }
        }

        val screen = compose.onRoot().fetchSemanticsNode().boundsInRoot
        val bar = compose.onNodeWithTag(NOW_PLAYING_BAR_TAG).fetchSemanticsNode().boundsInRoot
        assertEquals(screen.left, bar.left, 0f)
        assertEquals(screen.right, bar.right, 0f)
        assertEquals(screen.bottom, bar.bottom, 0f)
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
                    modifier = Modifier.watchingChromeContainer(),
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
    @Config(qualifiers = "w960dp-h540dp-television-xxxhdpi")
    fun `android tv surf at 4k density`() {
        compose.setContent {
            Box(modifier = Modifier.fillMaxSize().background(LoomarrTokens.Color.Static800)) {
                SurfRail(
                    state =
                        WatchUiState.Ready(
                            channels = sampleChannels,
                            selected = 2,
                            playUrl = "preview",
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
            GuideSurface {
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

    @Test
    @Config(qualifiers = "w960dp-h540dp-television-xxxhdpi")
    fun `android tv guide at 4k density`() {
        compose.setContent {
            GuideSurface {
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

    @Test
    fun `android tv guide spans the screen`() {
        compose.setContent {
            GuideSurface {
                GuideGrid(
                    window = (sampleGuide as GuideUiState.Ready).window,
                    nowMs = sampleNow,
                    onTune = {},
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }

        val screen = compose.onRoot().fetchSemanticsNode().boundsInRoot
        val guide = compose.onNodeWithTag(GUIDE_GRID_TAG).fetchSemanticsNode().boundsInRoot
        assertEquals(screen.left, guide.left, 0f)
        assertEquals(screen.top, guide.top, 0f)
        assertEquals(screen.right, guide.right, 0f)
        assertEquals(screen.bottom, guide.bottom, 0f)
    }

    @Test
    @OptIn(ExperimentalTestApi::class)
    fun `android tv guide filter row is reachable with the d-pad`() {
        compose.setContent {
            GuideSurface {
                GuideGrid(
                    window = (sampleGuide as GuideUiState.Ready).window,
                    nowMs = sampleNow,
                    onTune = {},
                    recentChannelIds = listOf("nature"),
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }

        compose.onNodeWithTag(GUIDE_GRID_TAG).performKeyInput { pressKey(Key.DirectionUp) }
        compose.onNodeWithTag(GUIDE_FOCUSED_FILTER_TAG).assertTextContains("All · 6")

        compose.onNodeWithTag(GUIDE_GRID_TAG).performKeyInput { pressKey(Key.DirectionRight) }
        compose.onNodeWithTag(GUIDE_FOCUSED_FILTER_TAG).assertTextContains("Recent · 1")
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
                                        sampleAiring(
                                            title = "Pilot episode",
                                            startMs = 7_200_000L,
                                            stopMs = 9_000_000L,
                                            series = channel.name,
                                        ),
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
        series: String? = null,
    ) = Airing(
        kind = "program",
        title = title,
        series = series,
        season = if (series == null) 0 else 2,
        episode = if (series == null) 0 else 4,
        description = if (series == null) null else "A focused episode description that belongs in the detail card.",
        genres = if (series == null) emptyList() else listOf("Drama", "Adventure"),
        year = if (series == null) 0 else 1996,
        rating = if (series == null) null else "TV-PG",
        startMs = startMs,
        stopMs = stopMs,
        nominal = false,
        provenance = "in library",
    )
}
