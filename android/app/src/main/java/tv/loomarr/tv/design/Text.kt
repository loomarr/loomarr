package tv.loomarr.tv.design

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign

/**
 * Loomarr's text styles for a 10-foot screen.
 *
 * These exist so a screen never writes `fontSize = 32.sp` again. Inline sizes were how the first TV
 * screens were built, and the result is the same drift a hand-copied hex causes: nothing relates the
 * number to the design system, so nothing can tell a considered choice from a guess.
 *
 * The set is deliberately small. Web has twenty primitives, most of which — checkbox, switch,
 * dropdown, tooltip — are pointer idioms a D-pad television has no use for. Porting them would be
 * cargo-cult, not consistency.
 */
@Composable
fun Display(
    text: String,
    modifier: Modifier = Modifier,
    color: androidx.compose.ui.graphics.Color = LoomarrTokens.Color.Static0,
    align: TextAlign? = null,
) = Text(
    text = text,
    modifier = modifier,
    color = color,
    fontSize = LoomarrTokens.Type.Xl2,
    textAlign = align,
)

/** A screen's title. */
@Composable
fun Heading(
    text: String,
    modifier: Modifier = Modifier,
    color: androidx.compose.ui.graphics.Color = LoomarrTokens.Color.Static0,
    align: TextAlign? = null,
) = Text(
    text = text,
    modifier = modifier,
    color = color,
    fontSize = LoomarrTokens.Type.Xl,
    textAlign = align,
)

/** Ordinary reading text. */
@Composable
fun Body(
    text: String,
    modifier: Modifier = Modifier,
    color: androidx.compose.ui.graphics.Color = LoomarrTokens.Color.Static400,
    align: TextAlign? = null,
) = Text(
    text = text,
    modifier = modifier,
    color = color,
    fontSize = LoomarrTokens.Type.Md,
    textAlign = align,
)

/**
 * A pairing code, or anything else read aloud across a room.
 *
 * The largest style in the set on purpose: a code is the one thing a viewer must carry from the
 * screen to a phone, and it is read from further away than anything else on it.
 */
@Composable
fun CodeDisplay(
    text: String,
    modifier: Modifier = Modifier,
) = Text(
    text = text,
    modifier = modifier,
    color = LoomarrTokens.Color.Signal,
    fontSize = LoomarrTokens.Type.Code,
)

/** An error, in the same red the web client uses for destructive states. */
@Composable
fun ErrorText(
    text: String,
    modifier: Modifier = Modifier,
    align: TextAlign? = TextAlign.Center,
) = Text(
    text = text,
    modifier = modifier,
    color = LoomarrTokens.Color.Onair,
    fontSize = LoomarrTokens.Type.Lg,
    textAlign = align,
)
