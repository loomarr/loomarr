package tv.loomarr.tv.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * ⚠ Overscan. Older televisions crop the edges of the picture — historically up to 5% per side —
 * and the amount is not knowable from the app. Android TV's convention is a 48dp margin on every
 * side, which is why this is a constant here rather than a number each screen picks.
 *
 * It is NOT a design token: the web has no equivalent, because no monitor crops its own output.
 */
val OverscanMargin = 48.dp

/**
 * The frame every Loomarr TV screen sits in: the app background, and the overscan margin.
 *
 * Screens compose this instead of building their own Box/Column so the margin cannot be forgotten
 * on one screen and present on the rest.
 */
@Composable
fun Screen(
    modifier: Modifier = Modifier,
    content: @Composable BoxScope.() -> Unit,
) {
    Box(
        modifier =
            modifier
                .fillMaxSize()
                .background(LoomarrTokens.Color.Static950)
                .padding(OverscanMargin),
        content = content,
    )
}

/**
 * A screen whose content is a centred column — the shape of every "one message, one action" surface
 * (pairing, errors, empty states).
 */
@Composable
fun CenteredScreen(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Screen(modifier = modifier) {
        Column(
            modifier = Modifier.fillMaxSize(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
            content = content,
        )
    }
}
