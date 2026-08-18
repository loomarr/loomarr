package tv.loomarr.tv.guide

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.SectionHeading

/**
 * How wide an hour of schedule is drawn.
 *
 * The grid is a TIME axis, so a block's width is its duration — a 22-minute episode and a 90-minute
 * film must look different at a glance, which is the whole point of a grid over a list.
 */
private val HourWidth = 360.dp

/** Channel names occupy a fixed gutter so every row's timeline starts on the same vertical line. */
private val ChannelGutter = 220.dp

private val RowHeight = 96.dp

/**
 * The channel × time guide.
 *
 * ⚠ Standard `LazyColumn`, NOT `TvLazyColumn`. The `androidx.tv:tv-foundation` variants were REMOVED
 * in 1.0.0-alpha12 (Jan 2025), and standard Foundation has carried TV focus handling since 1.7.0 —
 * most Compose-TV grid tutorials online are written against the deleted API.
 *
 * Vertical is lazy because channel count is unbounded; horizontal is not, because one window is at
 * most a few hours of blocks per row and nesting lazy scrollers costs more than it saves at this
 * size.
 */
@Composable
fun GuideGrid(
    window: GuideWindow,
    onTune: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
) {
    val listState = rememberLazyListState()

    Column(modifier = modifier) {
        TimeRuler(window = window)

        LazyColumn(
            state = listState,
            modifier = Modifier.fillMaxWidth(),
        ) {
            items(window.channels, key = { it.channelId }) { channel ->
                ChannelRow(
                    channel = channel,
                    window = window,
                    onTune = { onTune(channel) },
                )
            }
        }
    }
}

/** Hour marks across the top, aligned to the same gutter every row uses. */
@Composable
private fun TimeRuler(window: GuideWindow) {
    Row(modifier = Modifier.padding(bottom = LoomarrTokens.Space.S2)) {
        Box(modifier = Modifier.width(ChannelGutter)) {
            SectionHeading("Channel")
        }
        // Marks every half hour, which is the granularity a schedule is actually built on.
        val halfHours = (window.durationMs / (30 * 60_000L)).toInt()
        repeat(halfHours) { index ->
            Box(modifier = Modifier.width(HourWidth / 2)) {
                MonoData(
                    text = clockLabel(window.fromMs + index * 30 * 60_000L),
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Sm,
                )
            }
        }
    }
}

@Composable
private fun ChannelRow(
    channel: ChannelTimeline,
    window: GuideWindow,
    onTune: () -> Unit,
) {
    Row(
        modifier = Modifier.height(RowHeight).padding(bottom = LoomarrTokens.Space.S1),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier.width(ChannelGutter),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            MonoData(
                text = channel.number.toString(),
                color = LoomarrTokens.Color.Signal,
                fontSize = LoomarrTokens.Type.Md,
            )
            Body(
                text = channel.name,
                color = LoomarrTokens.Color.Static0,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
            )
        }

        channel.airings.forEach { airing ->
            AiringCell(
                airing = airing,
                width = widthFor(airing, window),
                onSelect = onTune,
            )
        }
    }
}

/**
 * One block, sized by its duration.
 *
 * Focus is drawn with a `signal` border — the design's focus-ring token. On a D-pad surface the ring
 * is the only thing telling a viewer what Enter will do, so it is not decoration.
 */
@Composable
private fun AiringCell(
    airing: Airing,
    width: Dp,
    onSelect: () -> Unit,
) {
    var focused by remember { mutableStateOf(false) }

    Box(
        modifier =
            Modifier
                .width(width)
                .height(RowHeight)
                .padding(end = LoomarrTokens.Space.S1)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                .background(backgroundFor(airing, focused))
                .border(
                    width = if (focused) 3.dp else 1.dp,
                    color = if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static700,
                    shape = RoundedCornerShape(LoomarrTokens.Radius.Sm),
                ).onFocusChanged { focused = it.isFocused }
                // ⚠ `clickable` supplies focusability AND the Enter/D-pad-centre handler together.
                // A `focusable()` cell without it draws a focus ring and then does nothing when
                // selected, which reads as a broken remote rather than a missing feature.
                .clickable(onClick = onSelect)
                .padding(LoomarrTokens.Space.S2),
    ) {
        Column {
            Body(
                text = airing.heading,
                color = LoomarrTokens.Color.Static0,
                fontSize = LoomarrTokens.Type.Sm,
                maxLines = 1,
            )
            // ⚠ A nominal block's times are an ESTIMATE, so it says so rather than printing a clock
            // range the schedule does not promise. The server flags this precisely because a client
            // showing "8:00–8:30" for an unacquired title is asserting something untrue.
            val detail =
                when {
                    airing.nominal -> airing.provenance ?: "Not yet scheduled"
                    airing.episodeLabel.isNotEmpty() -> airing.episodeLabel
                    else -> clockLabel(airing.startMs)
                }
            Body(
                text = detail,
                color = LoomarrTokens.Color.Static400,
                fontSize = LoomarrTokens.Type.Xs,
                maxLines = 1,
            )
        }
    }
}

/**
 * A block's colour carries its kind, using the palette's own meanings rather than inventing any:
 * filler is dimmer because a commercial break is not the programme, and a pending acquisition takes
 * `caution` because it is a block that may not air as drawn.
 */
private fun backgroundFor(
    airing: Airing,
    focused: Boolean,
): androidx.compose.ui.graphics.Color =
    when {
        focused -> LoomarrTokens.Color.Static700
        airing.kind == "filler" -> LoomarrTokens.Color.Static950
        airing.kind == "pending" -> LoomarrTokens.Color.Static800
        else -> LoomarrTokens.Color.Static900
    }

/** A block's width is its share of the window — the grid's x-axis is time. */
private fun widthFor(
    airing: Airing,
    window: GuideWindow,
): Dp {
    val hours = airing.durationMs.toDouble() / 3_600_000.0
    // Floored so a very short block stays selectable: a two-minute filler clip would otherwise be a
    // few pixels wide and impossible to focus with a D-pad.
    return (HourWidth * hours.toFloat()).coerceAtLeast(72.dp)
}

/**
 * "8:30" in the device's own timezone.
 *
 * ⚠ Converted through `ZoneId.systemDefault()`, not by arithmetic on the epoch. Dividing epoch ms
 * into hours and minutes yields UTC, which is right only in Britain in winter — a guide that shows
 * every programme an hour or five off is worse than no guide.
 *
 * The server sends absolute epoch ms deliberately and says so: "a timezone is a formatting choice,
 * and putting instants on the wire in local time would invite a client to reinterpret rather than
 * merely format them." This is that formatting choice.
 */
private fun clockLabel(epochMs: Long): String {
    val local =
        java.time.Instant
            .ofEpochMilli(epochMs)
            .atZone(java.time.ZoneId.systemDefault())
    return "%d:%02d".format(local.hour, local.minute)
}
