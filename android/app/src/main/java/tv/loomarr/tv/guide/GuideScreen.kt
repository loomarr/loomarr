package tv.loomarr.tv.guide

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.loomarr.tv.design.DeadAir
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.Screen
import tv.loomarr.tv.design.TuningText

/**
 * The guide: what is on, across every channel, laid out against time.
 *
 * [Screen] supplies the overscan margin, so the leftmost channel names and the rightmost blocks stay
 * on a television that crops its own picture.
 */
@Composable
fun GuideScreen(
    model: GuideViewModel,
    onTune: (ChannelTimeline) -> Unit,
) {
    val state by model.state.collectAsStateWithLifecycle()

    Screen {
        when (val current = state) {
            is GuideUiState.Loading ->
                TuningText("Loading the guide…", modifier = Modifier.align(Alignment.Center))

            is GuideUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))

            is GuideUiState.DeadAir ->
                DeadAir(
                    title = "Dead air",
                    modifier = Modifier.align(Alignment.Center),
                    description = "No channels are scheduled yet. Create one in Loomarr and it will appear here.",
                )

            is GuideUiState.Ready ->
                GuideGrid(
                    window = current.window,
                    onTune = onTune,
                    modifier = Modifier.fillMaxSize(),
                )
        }
    }
}
