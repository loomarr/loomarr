package loomarr.media.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * The SMPTE test-card strip that is the whole design's namesake.
 *
 * Seven hard-edged segments in the broadcast accents, in the same order the web client uses:
 * signal · caution · lock · tune · suggest · onair · static-400. The colours come from the shared
 * tokens, so the brand mark IS the palette rather than a picture of it — change a token and both
 * clients move together.
 *
 * Purely decorative, so it carries no content description.
 */
@Composable
fun ColorBars(
    modifier: Modifier = Modifier,
    width: androidx.compose.ui.unit.Dp = 320.dp,
    height: androidx.compose.ui.unit.Dp = 24.dp,
) {
    val segments =
        listOf(
            LoomarrTokens.Color.Signal,
            LoomarrTokens.Color.Caution,
            LoomarrTokens.Color.Lock,
            LoomarrTokens.Color.Tune,
            LoomarrTokens.Color.Suggest,
            LoomarrTokens.Color.Onair,
            LoomarrTokens.Color.Static400,
        )

    Row(
        modifier =
            modifier
                .width(width)
                .height(height)
                // 2px on web; the TV strip is larger, so the same near-square corner reads as the
                // hard edge a test card is supposed to have.
                .clip(RoundedCornerShape(2.dp)),
    ) {
        segments.forEach { segment ->
            androidx.compose.foundation.layout.Spacer(
                modifier =
                    Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .background(segment),
            )
        }
    }
}

/**
 * The empty state, as a test card.
 *
 * ⚠ A test card is literally what a set showed when nothing was broadcasting, so on this screen the
 * motif IS the state rather than decoration applied to it. The web guide uses the same treatment for
 * "Dead air", and reuses the same bars rather than a separate asset.
 *
 * A test-card BLOCK, not the lockup's thin strip: presence here comes from size. It is deliberately
 * STILL — a test card that jitters reads as a fault, while one that simply holds reads as standing
 * by, which is what dead air means.
 *
 * The title takes web's empty-state treatment: mono, upper-case, wide-tracked. That register says
 * "no signal" in a way sentence-case body text does not.
 */
@Composable
fun DeadAir(
    title: String,
    modifier: Modifier = Modifier,
    description: String? = null,
) {
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S5),
    ) {
        ColorBars(width = 400.dp, height = 120.dp)
        Text(
            text = title.uppercase(),
            color = LoomarrTokens.Color.Static400,
            fontSize = LoomarrTokens.Type.Lg,
            fontFamily = FontFamily.Monospace,
            letterSpacing = (LoomarrTokens.Type.Lg.value * 0.08f).sp,
        )
        if (description != null) {
            Text(
                text = description,
                color = LoomarrTokens.Color.Static400,
                fontSize = LoomarrTokens.Type.Md,
                lineHeight = LoomarrTokens.Type.Md * 1.5f,
                textAlign = TextAlign.Center,
            )
        }
    }
}

/**
 * The LOOMARR mark: the test-card strip above the wordmark, with the tagline beneath.
 *
 * This is the web client's `hero` lockup, scaled for a television. The wordmark is all-caps and
 * wide-tracked; the tagline is monospaced, in the same register the rest of the product uses.
 *
 * ⚠ The tracking is the mark. "LOOMARR" set without letter-spacing is a different logo — web pins
 * it at 0.08em on the hero, and this matches rather than approximates it.
 */
@Composable
fun BrandLockup(
    modifier: Modifier = Modifier,
    tagline: Boolean = true,
) {
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S3),
    ) {
        ColorBars()
        Text(
            text = "LOOMARR",
            color = LoomarrTokens.Color.Static0,
            fontSize = LoomarrTokens.Type.Xl2,
            fontWeight = FontWeight.Bold,
            // 0.08em at this size. Compose takes letterSpacing in sp rather than em, so it is
            // computed from the font size instead of copied as a magic number.
            letterSpacing = (LoomarrTokens.Type.Xl2.value * 0.08f).sp,
        )
        if (tagline) {
            Text(
                text = "always something on",
                color = LoomarrTokens.Color.Static400,
                fontSize = LoomarrTokens.Type.Md,
                fontFamily = FontFamily.Monospace,
            )
        }
    }
}
