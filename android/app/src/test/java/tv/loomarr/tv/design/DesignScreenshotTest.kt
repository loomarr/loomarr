package tv.loomarr.tv.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
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

/**
 * Screenshots of the design system's pieces.
 *
 * These render real Compose on the JVM through Robolectric, so a styling regression fails the build
 * on an ordinary CI runner with no emulator attached — the same bargain web's visual suite makes.
 *
 * ⚠ What a screenshot test is FOR here is the thing a unit test cannot state: that a badge is the
 * right red, that a chip's text clears its background, that type has not started overlapping. It is
 * not a substitute for asserting behaviour — [ChannelTimelineStripTest] still owns the geometry,
 * because "is this block 234dp wide" deserves an assertion with a number in it, not a picture.
 */
@RunWith(RobolectricTestRunner::class)
// ⚠ NATIVE graphics, not the default. Robolectric's legacy renderer draws nothing — every canvas
// operation is a no-op — so a screenshot taken under it is a blank image that matches its own blank
// baseline forever. A test that cannot fail is worse than no test.
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w1920dp-h1080dp-television-xhdpi")
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
}
