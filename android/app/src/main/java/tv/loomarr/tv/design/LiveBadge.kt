package tv.loomarr.tv.design

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * "● LIVE" — that this channel is playing at the broadcast edge rather than from a buffer.
 *
 * ⚠ `onair-300` on an `onair` TINT, not white on solid `onair`. Web measured the obvious version at
 * 3.91:1 and rejected it: below WCAG-AA's 4.5:1 for small text. The tinted chip with the lighter
 * text clears it, and this is the same badge on a screen read from three metres rather than thirty
 * centimetres — the case where contrast matters more, not less.
 */
@Composable
fun LiveBadge(modifier: Modifier = Modifier) {
    // ⚠ Guarded by the platform's animation scale. `motion-safe:` on web is the same guard: a
    // viewer who has asked for reduced motion should get a steady dot, not a pulsing one. When
    // animations are switched off the scale is 0 and the transition never advances, which leaves
    // the dot at full opacity rather than mid-fade.
    val transition = rememberInfiniteTransition(label = "live-pulse")
    val pulse by transition.animateFloat(
        initialValue = 1f,
        targetValue = 0.35f,
        animationSpec =
            infiniteRepeatable(
                animation = tween(durationMillis = 1_200),
                repeatMode = RepeatMode.Reverse,
            ),
        label = "live-pulse-alpha",
    )

    Row(
        modifier =
            modifier
                .clip(CircleShape)
                // ⚠ Derived from the base token, not a new constant. Web's `onair-tint-15` is
                // `color-mix(in srgb, var(--color-onair) 15%, transparent)` computed in CSS at
                // render time, so tints never enter tokens.json and never reach the Kotlin
                // generator. `copy(alpha = …)` is the same derivation; a hand-written 0x26E5484D
                // would be precisely the hardcoded hex the design system exists to prevent.
                .background(LoomarrTokens.Color.Onair.copy(alpha = 0.15f))
                .border(1.dp, LoomarrTokens.Color.Onair300.copy(alpha = 0.25f), CircleShape)
                .padding(horizontal = LoomarrTokens.Space.S3, vertical = LoomarrTokens.Space.S1),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier =
                Modifier
                    .size(LiveDotSize)
                    .alpha(pulse)
                    .clip(CircleShape)
                    .background(LoomarrTokens.Color.Onair300),
        )
        Text(
            text = "LIVE",
            color = LoomarrTokens.Color.Onair300,
            fontSize = LoomarrTokens.Type.Xs,
            fontFamily = FontFamily.Monospace,
            // The wide tracking is what makes four upper-case letters read as a status chip rather
            // than as a word someone typed. Web sets 0.12em; this is the same ratio.
            letterSpacing = (LoomarrTokens.Type.Xs.value * 0.12f).sp,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S2),
        )
    }
}

/** Small enough to read as a status dot rather than a bullet. */
private val LiveDotSize = 10.dp
