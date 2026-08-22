package tv.loomarr.tv.guide

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.SectionHeading

/**
 * The guide: every channel against one shared timeline.
 *
 * ⚠ A GRID, and the earlier channel-list-plus-detail version was the wrong shape. That design
 * argued a dense grid spends a D-pad viewer's button presses on cells nobody is choosing — which
 * mistakes what breadth costs. Reading ACROSS rows costs nothing; the eye does it for free. Only
 * moving FOCUS costs presses, and focus still moves one channel row at a time here.
 *
 * What the grid buys back is the question a guide exists to answer: "what is on everywhere at nine
 * o'clock". The list version could only answer "what is on this one channel", and answering it for
 * a second channel meant focusing that row and re-reading the whole pane. Every set-top box ships
 * the grid on a D-pad for this reason.
 */
@Composable
fun GuideGrid(
    window: GuideWindow,
    nowMs: Long,
    onTune: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
) {
    var focusedRow by remember { mutableStateOf(0) }
    val focused = window.channels.getOrNull(focusedRow)

    Column(modifier = modifier) {
        // The detail pane sits ABOVE the grid rather than beside it: a row of channels needs the
        // full width to be worth having, and what is on the focused channel is one line of text.
        FocusedAiring(channel = focused, nowMs = nowMs)

        BoxWithConstraints(
            modifier = Modifier.fillMaxWidth().padding(top = LoomarrTokens.Space.S4),
        ) {
            // The timeline starts after the channel column and runs to the right edge.
            val timelineWidth = maxWidth - ChannelColumnWidth

            Column {
                TimeRuler(
                    window = window,
                    timelineWidth = timelineWidth,
                    modifier = Modifier.padding(start = ChannelColumnWidth),
                )

                ChannelRows(
                    window = window,
                    nowMs = nowMs,
                    timelineWidth = timelineWidth,
                    focusedRow = focusedRow,
                    onFocusRow = { focusedRow = it },
                    onTune = onTune,
                    modifier = Modifier.fillMaxWidth().fillMaxHeight(),
                )
            }

            // ⚠ Drawn LAST, so it crosses every row rather than being buried under them. It is the
            // one mark that spans the whole grid, and it is what turns a set of blocks into "here
            // is where you are".
            NowLine(
                nowMs = nowMs,
                window = window,
                timelineWidth = timelineWidth,
                modifier = Modifier.padding(start = ChannelColumnWidth),
            )
        }
    }
}

/** Wide enough for a number and a channel name; every dp beyond that is one the timeline loses. */
private val ChannelColumnWidth = 260.dp

/** One channel's row height — two lines of block text plus breathing room. */
private val RowHeight = 108.dp

/**
 * What is on the focused channel — the detail the grid's own cells are too small to carry.
 *
 * ⚠ Leads with what the server RELIABLY sends. `season`, `episode`, `year`, `rating` and `genres`
 * are all omitempty and absent from real data — a pane built around them would be mostly blank, so
 * this shows title, time, duration and provenance, which actually arrive.
 */
@Composable
private fun FocusedAiring(
    channel: ChannelTimeline?,
    nowMs: Long,
) {
    val now = channel?.airingAt(nowMs)

    Column {
        SectionHeading("On now")
        Row(
            modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Body(
                text = now?.heading ?: "Nothing scheduled",
                color = LoomarrTokens.Color.Static0,
                fontSize = LoomarrTokens.Type.Lg,
                maxLines = 1,
            )
            if (now != null) {
                MonoData(
                    text = "${clockLabel(now.startMs)}–${clockLabel(now.stopMs)}",
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Sm,
                    maxLines = 1,
                    modifier = Modifier.padding(start = LoomarrTokens.Space.S4),
                )
                if (now.episodeLabel.isNotEmpty()) {
                    MonoData(
                        text = now.episodeLabel,
                        color = LoomarrTokens.Color.Static400,
                        fontSize = LoomarrTokens.Type.Sm,
                        maxLines = 1,
                        modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
                    )
                }
                now.provenance?.takeIf { it.isNotBlank() }?.let {
                    Body(
                        text = it,
                        fontSize = LoomarrTokens.Type.Sm,
                        maxLines = 1,
                        modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
                    )
                }
            }
        }
    }
}

/** The scrolling stack of channel rows. */
@Composable
private fun ChannelRows(
    window: GuideWindow,
    nowMs: Long,
    timelineWidth: Dp,
    focusedRow: Int,
    onFocusRow: (Int) -> Unit,
    onTune: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
) {
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        // LaunchedEffect begins after composition, but that is still before the LazyColumn's first
        // row is guaranteed to be placed. Requesting immediately failed silently on the emulator:
        // the row LOOKED selected while Android reported no focused element, so the first D-pad
        // press merely established focus instead of moving. Waiting for the next frame puts the
        // target in the focus tree before asking for it.
        withFrameNanos { }
        runCatching { first.requestFocus() }
    }

    val listState = rememberLazyListState()

    // A LazyColumn does not reliably scroll to follow D-pad focus — it disposes items that leave
    // the viewport, so on a long channel list the focus can walk off the bottom and leave the viewer
    // steering something they cannot see.
    //
    // ⚠ Jump rather than animate. An animated scroll remains in flight for several remote-repeat
    // key events, and Compose drops those events while the next focus target is moving. Measured on
    // the 100-row emulator fixture, 99 Down presses only advanced 60 rows with animation. A direct
    // scroll keeps every repeat actionable and moves by only one row, so it still reads as normal
    // list navigation rather than a page jump.
    LaunchedEffect(focusedRow) {
        val onScreen = listState.layoutInfo.visibleItemsInfo.any { it.index == focusedRow }
        if (!onScreen && window.channels.isNotEmpty()) {
            runCatching { listState.scrollToItem(focusedRow) }
        }
    }

    LazyColumn(state = listState, modifier = modifier) {
        itemsIndexed(
            window.channels,
            // Keyed by id, not index: a refetch that reorders would otherwise move the highlight to
            // whatever channel now occupies the focused slot.
            key = { _, channel -> channel.channelId },
        ) { index, channel ->
            ChannelRow(
                channel = channel,
                window = window,
                nowMs = nowMs,
                timelineWidth = timelineWidth,
                selected = index == focusedRow,
                onFocus = { onFocusRow(index) },
                onSelect = { onTune(channel) },
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .then(if (index == 0) Modifier.focusRequester(first) else Modifier),
            )
        }
    }
}

/** One channel: its identity on the left, its schedule laid out against time on the right. */
@Composable
private fun ChannelRow(
    channel: ChannelTimeline,
    window: GuideWindow,
    nowMs: Long,
    timelineWidth: Dp,
    selected: Boolean,
    onFocus: () -> Unit,
    onSelect: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var focused by remember { mutableStateOf(false) }

    // Either system focus or the browser's own idea of the current row. The second half is not
    // redundant: the list asks for focus before its rows are placed, so the request can fail and
    // leave every row unhighlighted while the pane above already describes the first one.
    val highlighted = focused || selected

    Row(
        modifier =
            modifier
                .height(RowHeight)
                .onFocusChanged {
                    focused = it.isFocused
                    if (it.isFocused) onFocus()
                }.clickable(onClick = onSelect),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ChannelCell(
            channel = channel,
            highlighted = highlighted,
            modifier = Modifier.width(ChannelColumnWidth).fillMaxHeight(),
        )

        Box(modifier = Modifier.fillMaxHeight()) {
            HourGridlines(window = window, timelineWidth = timelineWidth, height = RowHeight)

            channel.airings.forEach { airing ->
                val width = airing.widthIn(window, timelineWidth)
                if (width > 0.dp) {
                    TimelineBlock(
                        airing = airing,
                        width = width,
                        onAir = nowMs >= airing.startMs && nowMs < airing.stopMs,
                        height = RowHeight,
                        modifier = Modifier.offset(x = airing.offsetIn(window, timelineWidth)),
                    )
                }
            }

            if (channel.airings.isEmpty()) {
                Body(
                    "Nothing scheduled",
                    fontSize = LoomarrTokens.Type.Sm,
                    maxLines = 1,
                    modifier =
                        Modifier
                            .align(Alignment.CenterStart)
                            .padding(start = LoomarrTokens.Space.S4),
                )
            }
        }
    }
}

/** The channel's number and name, in the fixed left column. */
@Composable
private fun ChannelCell(
    channel: ChannelTimeline,
    highlighted: Boolean,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier =
            modifier
                .padding(end = LoomarrTokens.Space.S2, bottom = LoomarrTokens.Space.S1)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Md))
                .background(
                    if (highlighted) LoomarrTokens.Color.Static800 else LoomarrTokens.Color.Static900,
                ).border(
                    width = if (highlighted) 3.dp else 1.dp,
                    color =
                        if (highlighted) {
                            LoomarrTokens.Color.Signal
                        } else {
                            LoomarrTokens.Color.Static700
                        },
                    shape = RoundedCornerShape(LoomarrTokens.Radius.Md),
                ).padding(LoomarrTokens.Space.S3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        MonoData(
            text = channel.number.toString(),
            color = LoomarrTokens.Color.Signal,
            fontSize = LoomarrTokens.Type.Lg,
            maxLines = 1,
        )
        Body(
            text = channel.name,
            color = LoomarrTokens.Color.Static0,
            fontSize = LoomarrTokens.Type.Sm,
            maxLines = 2,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
        )
    }
}
