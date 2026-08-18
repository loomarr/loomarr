package tv.loomarr.tv.guide

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.unit.dp
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.Heading
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.SectionHeading

/**
 * The guide: a channel list beside what is on it.
 *
 * ⚠ NOT a channel × time grid, and that is a deliberate departure from the web guide rather than an
 * omission. The web grid works because a pointer makes every cell one click away, so breadth is
 * nearly free. On a D-pad, distance IS time — every cell costs button presses — so a dense
 * multi-hour grid spends the viewer's effort on blocks nobody is choosing. Web is the design
 * inspiration; a television is a different input category.
 *
 * What a viewer actually decides at a television is "what do I watch now", so the layout gives that
 * decision the whole screen: move down the channels, read what is on, press Enter.
 */
@Composable
fun ChannelBrowser(
    window: GuideWindow,
    nowMs: Long,
    onTune: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
) {
    var focusedIndex by remember { mutableStateOf(0) }
    val focused = window.channels.getOrNull(focusedIndex)

    Row(modifier = modifier) {
        ChannelList(
            channels = window.channels,
            nowMs = nowMs,
            focusedIndex = focusedIndex,
            onFocus = { focusedIndex = it },
            onSelect = onTune,
            modifier = Modifier.width(ChannelListWidth).fillMaxHeight(),
        )

        if (focused != null) {
            ChannelDetail(
                channel = focused,
                window = window,
                nowMs = nowMs,
                modifier =
                    Modifier
                        .fillMaxHeight()
                        .padding(start = LoomarrTokens.Space.S6),
            )
        }
    }
}

/**
 * Wide enough for a channel name at TV scale without crowding the strip beside it.
 *
 * ⚠ 320dp, down from 420. On a 1080p television the row has 960dp to divide, and every dp the list
 * keeps is one the timeline cannot use: at 420 the strip had 412dp, which put an hour of a four-hour
 * window at 103dp and left a feature film too narrow to print its own time range. The list only ever
 * holds a number, a name and one line of what is on — 320 still fits all three.
 */
private val ChannelListWidth = 320.dp

@Composable
private fun ChannelList(
    channels: List<ChannelTimeline>,
    nowMs: Long,
    focusedIndex: Int,
    onFocus: (Int) -> Unit,
    onSelect: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
) {
    // The list owns the D-pad on arrival — without this the first press goes nowhere.
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        runCatching { first.requestFocus() }
    }

    val listState = rememberLazyListState()

    // ⚠ Keep the focused row on screen ourselves. A LazyColumn does NOT scroll to follow D-pad
    // focus: it disposes items that leave the viewport, so on a long channel list the focus walks
    // off the bottom and the viewer is steering something they cannot see — and the disposed row
    // loses its focus state on the way back. Invisible with three channels, unusable with thirty.
    //
    // `animateScrollToItem` rather than a jump, because on a television the viewer is tracking a
    // moving highlight rather than reading a scrollbar, and an instant jump loses them.
    LaunchedEffect(focusedIndex) {
        val visible = listState.layoutInfo.visibleItemsInfo
        val onScreen = visible.any { it.index == focusedIndex }
        if (!onScreen && channels.isNotEmpty()) {
            runCatching { listState.animateScrollToItem(focusedIndex) }
        }
    }

    LazyColumn(state = listState, modifier = modifier) {
        itemsIndexed(
            channels,
            // ⚠ Keyed by channel id, not position. Without a key a LazyColumn identifies items by
            // index, so a reordered or filtered list moves focus to whatever now occupies the
            // focused slot — the highlight jumps to a different channel than the one the viewer was
            // on. The id is stable across a refetch; the index is not.
            key = { _, channel -> channel.channelId },
        ) { index, channel ->
            ChannelRow(
                channel = channel,
                nowMs = nowMs,
                selected = index == focusedIndex,
                onFocus = { onFocus(index) },
                onSelect = { onSelect(channel) },
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .padding(bottom = LoomarrTokens.Space.S2)
                        .then(if (index == 0) Modifier.focusRequester(first) else Modifier),
            )
        }
    }
}

/**
 * One channel: its number, name, and what is on right now.
 *
 * The now-playing line is here rather than only in the detail pane because it is what a viewer
 * scans the list FOR — a list of bare channel names would make them focus each row in turn just to
 * discover what it carries.
 */
@Composable
private fun ChannelRow(
    channel: ChannelTimeline,
    nowMs: Long,
    /** Whether this is the row the browser considers current — see [highlighted]. */
    selected: Boolean,
    onFocus: () -> Unit,
    onSelect: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var focused by remember { mutableStateOf(false) }
    val now = channel.airingAt(nowMs)

    // ⚠ Either system focus OR the browser's own idea of the current row, and the second half is
    // not redundant. On arrival the list asks for focus before its rows are placed, so the request
    // can fail and `onFocusChanged` never fires — leaving every row unhighlighted while the detail
    // pane already describes the first one. The viewer's first D-pad press then LOOKS like it did
    // nothing, because it is spent establishing the focus that should have been there on arrival.
    //
    // `focusedIndex` is real state that starts at 0, so honouring it draws the ring correctly from
    // the first frame whether or not the focus request landed.
    val highlighted = focused || selected

    Row(
        modifier =
            modifier
                // ⚠ Order matters, and not only for looks. `clickable` is what installs the focus
                // target AND what makes D-pad centre select this row, so it has to sit below the
                // observer that reports focus and below the fill and border that draw it. Written
                // the other way round the row still highlights, but a frame late.
                .onFocusChanged {
                    focused = it.isFocused
                    if (it.isFocused) onFocus()
                }.clickable(onClick = onSelect)
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
                ).padding(LoomarrTokens.Space.S4),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        MonoData(
            text = channel.number.toString(),
            color = LoomarrTokens.Color.Signal,
            fontSize = LoomarrTokens.Type.Lg,
        )
        Column(modifier = Modifier.padding(start = LoomarrTokens.Space.S4)) {
            Body(
                text = channel.name,
                color = LoomarrTokens.Color.Static0,
                maxLines = 1,
            )
            Body(
                text = now?.heading ?: "Nothing scheduled",
                fontSize = LoomarrTokens.Type.Sm,
                maxLines = 1,
            )
        }
    }
}

/**
 * What is on the focused channel, and what follows — the headline above, the schedule below.
 *
 * The strip replaced a text list of "Up next" times. Both carry the same airings, but a list gives
 * a four-minute commercial break and a two-hour film the same single row, so a channel's shape was
 * unreadable: the one thing a viewer wants from a guide — how long until something else is on — had
 * to be worked out by subtracting clock times. Drawn against time, that is just a width.
 *
 * ⚠ Leads with what the server reliably sends. `description`, `year`, `rating` and `genres` are all
 * declared omitempty and are absent from real seeded data, so a pane built around them would be
 * mostly blank. Title, time, duration and `provenance` are what actually arrive.
 */
@Composable
private fun ChannelDetail(
    channel: ChannelTimeline,
    window: GuideWindow,
    nowMs: Long,
    modifier: Modifier = Modifier,
) {
    val now = channel.airingAt(nowMs)

    Column(modifier = modifier) {
        SectionHeading("On now")
        if (now == null) {
            Body(
                "Nothing scheduled on this channel.",
                modifier = Modifier.padding(top = LoomarrTokens.Space.S3),
            )
        } else {
            Heading(
                now.heading,
                modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
            )
            Row(
                modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
                horizontalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S4),
            ) {
                MonoData(
                    text = "${clockLabel(now.startMs)}–${clockLabel(now.stopMs)}",
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Md,
                )
                if (now.episodeLabel.isNotEmpty()) {
                    MonoData(
                        text = now.episodeLabel,
                        color = LoomarrTokens.Color.Static400,
                        fontSize = LoomarrTokens.Type.Md,
                    )
                }
            }
            now.provenance?.let {
                Body(it, modifier = Modifier.padding(top = LoomarrTokens.Space.S2))
            }
        }

        if (channel.airings.isNotEmpty()) {
            SectionHeading(
                "Schedule",
                // ⚠ S6, not S8. The strip is the pane's substance — the headline above it is a
                // caption for the block the now-line already points at — and a 32dp gap pushed the
                // schedule to the middle of a 1080-line screen with dead space above and below.
                modifier = Modifier.padding(top = LoomarrTokens.Space.S6),
            )
            ChannelTimelineStrip(
                channel = channel,
                window = window,
                nowMs = nowMs,
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .padding(top = LoomarrTokens.Space.S3),
            )
        }
    }
}

/**
 * "8:30" in the device's own timezone.
 *
 * ⚠ Converted through `ZoneId.systemDefault()`, not by arithmetic on the epoch. Dividing epoch ms
 * into hours and minutes yields UTC, which is right only in Britain in winter.
 *
 * The server sends absolute epoch ms deliberately: "a timezone is a formatting choice, and putting
 * instants on the wire in local time would invite a client to reinterpret rather than merely format
 * them." This is that formatting choice.
 */
internal fun clockLabel(epochMs: Long): String {
    val local =
        java.time.Instant
            .ofEpochMilli(epochMs)
            .atZone(java.time.ZoneId.systemDefault())
    return "%d:%02d".format(local.hour, local.minute)
}
