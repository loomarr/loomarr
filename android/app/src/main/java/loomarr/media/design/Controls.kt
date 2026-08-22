package loomarr.media.design

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp

/** A hairline rule, in the design's border token. */
@Composable
fun VerticalDivider(
    modifier: Modifier = Modifier,
    height: androidx.compose.ui.unit.Dp = 200.dp,
) {
    Box(
        modifier =
            modifier
                .width(1.dp)
                .height(height)
                .background(LoomarrTokens.Color.Static700),
    )
}

/** A horizontal hairline, for separating a panel's rows. */
@Composable
fun HorizontalDivider(modifier: Modifier = Modifier) {
    Box(
        modifier =
            modifier
                .fillMaxWidth()
                .height(1.dp)
                .background(LoomarrTokens.Color.Static700),
    )
}

/**
 * A countdown, in the register machine-produced values take.
 *
 * ⚠ Colour carries the urgency, using the tokens' own meanings: `static-400` while there is plenty
 * of time, `caution` under a minute, `onair` in the last ten seconds. A viewer glancing up from a
 * phone should be able to tell whether they still have time without reading the digits.
 */
@Composable
fun Countdown(
    secondsRemaining: Long,
    modifier: Modifier = Modifier,
) {
    val colour =
        when {
            secondsRemaining <= 10 -> LoomarrTokens.Color.Onair
            secondsRemaining <= 60 -> LoomarrTokens.Color.Caution
            else -> LoomarrTokens.Color.Static400
        }
    val minutes = secondsRemaining / 60
    val seconds = secondsRemaining % 60

    Text(
        text =
            if (secondsRemaining <= 0) {
                "Code expired"
            } else {
                "Expires in %d:%02d".format(minutes, seconds)
            },
        modifier = modifier,
        color = colour,
        fontSize = LoomarrTokens.Type.Md,
        fontFamily = FontFamily.Monospace,
    )
}

/**
 * A focusable button, sized for a remote.
 *
 * ⚠ The focus ring is not decoration — it is the ONLY thing telling a viewer what the D-pad will
 * activate, and with a single button on screen an unfocused-looking control reads as disabled. It
 * uses `signal`, the design's focus-ring token.
 */
@Composable
fun TvButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    focusRequester: FocusRequester? = null,
) {
    var focused by remember { mutableStateOf(false) }

    Button(
        onClick = onClick,
        modifier =
            modifier
                .then(focusRequester?.let { Modifier.focusRequester(it) } ?: Modifier)
                .onFocusChanged { focused = it.isFocused },
        shape = RoundedCornerShape(LoomarrTokens.Radius.Md),
        colors =
            ButtonDefaults.buttonColors(
                containerColor = if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static800,
                contentColor = if (focused) LoomarrTokens.Color.Static950 else LoomarrTokens.Color.Static100,
            ),
        border =
            BorderStroke(
                width = if (focused) 3.dp else 1.dp,
                color = if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static700,
            ),
        contentPadding =
            androidx.compose.foundation.layout.PaddingValues(
                horizontal = LoomarrTokens.Space.S6,
                vertical = LoomarrTokens.Space.S3,
            ),
    ) {
        Text(text = text, fontSize = LoomarrTokens.Type.Md)
    }
}

/** Centres content in a full-height box — used to align a divider against its neighbours. */
@Composable
fun CenteredRow(
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Box(
        modifier = modifier.fillMaxHeight().padding(horizontal = LoomarrTokens.Space.S2),
        contentAlignment = Alignment.Center,
    ) {
        content()
    }
}
