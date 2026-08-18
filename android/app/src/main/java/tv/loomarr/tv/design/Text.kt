package tv.loomarr.tv.design

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
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
    lineHeight = LoomarrTokens.Type.Xl2 * 1.5f,
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
    lineHeight = LoomarrTokens.Type.Xl * 1.5f,
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
    lineHeight = LoomarrTokens.Type.Md * 1.5f,
    textAlign = align,
)

/**
 * A pairing code, or anything else read aloud across a room.
 *
 * The largest style in the set on purpose: a code is the one thing a viewer must carry from the
 * screen to a phone, and it is read from further away than anything else on it.
 *
 * ⚠ Monospaced, per frontend-design §2.2: "if it came from a machine, it's set in mono". A pairing
 * code is machine-generated, like channel numbers and durations, so it takes the data face rather
 * than the UI one. It is also `signal` amber — the brand/primary token — because this is the one
 * thing on screen the viewer must act on.
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
    fontFamily = FontFamily.Monospace,
)

/**
 * A channel number, duration, or any other machine-produced value.
 *
 * frontend-design §2.2 makes mono a signature rather than a garnish: channel numbers, EPG times,
 * state badges, external ids and durations are always mono on web, and a TV showing them in the UI
 * face would read as a different product.
 */
@Composable
fun MonoData(
    text: String,
    modifier: Modifier = Modifier,
    color: androidx.compose.ui.graphics.Color = LoomarrTokens.Color.Static0,
) = Text(
    text = text,
    modifier = modifier,
    color = color,
    fontSize = LoomarrTokens.Type.Lg,
    fontFamily = FontFamily.Monospace,
)

/**
 * An in-progress state — tuning, loading, connecting.
 *
 * `tune` cyan is the semantic token for exactly this (frontend-design §2.1: "links, informational
 * states, in-progress tuning"), so a grey "Tuning…" was throwing away meaning the palette encodes.
 */
@Composable
fun TuningText(
    text: String,
    modifier: Modifier = Modifier,
) = Text(
    text = text,
    modifier = modifier,
    color = LoomarrTokens.Color.Tune,
    fontSize = LoomarrTokens.Type.Lg,
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
    // ⚠ Compose does NOT derive line height from font size — left unset, wrapped lines render on
    // top of one another. Invisible until a string is long enough to wrap, which is why the first
    // multi-line error on screen was an unreadable overlap. 1.5 is the design's body leading.
    lineHeight = LoomarrTokens.Type.Lg * 1.5f,
    textAlign = align,
)
