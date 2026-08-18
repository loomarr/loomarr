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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
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
                nowMs = nowMs,
                modifier =
                    Modifier
                        .fillMaxHeight()
                        .padding(start = LoomarrTokens.Space.S8),
            )
        }
    }
}

/** Wide enough for a channel name at TV scale without crowding the detail beside it. */
private val ChannelListWidth = 420.dp

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
    androidx.compose.runtime.LaunchedEffect(Unit) {
        runCatching { first.requestFocus() }
    }

    LazyColumn(modifier = modifier) {
        itemsIndexed(channels) { index, channel ->
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
    selected: Boolean,
    onFocus: () -> Unit,
    onSelect: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var focused by remember { mutableStateOf(false) }
    val now = channel.airingAt(nowMs)

    Row(
        modifier =
            modifier
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Md))
                .background(
                    if (focused) LoomarrTokens.Color.Static800 else LoomarrTokens.Color.Static900,
                ).border(
                    width = if (focused) 3.dp else 1.dp,
                    color = if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static700,
                    shape = RoundedCornerShape(LoomarrTokens.Radius.Md),
                ).onFocusChanged {
                    focused = it.isFocused
                    if (it.isFocused) onFocus()
                }.clickable(onClick = onSelect)
                .padding(LoomarrTokens.Space.S4),
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
 * What is on the focused channel, and what follows.
 *
 * ⚠ Leads with what the server reliably sends. `description`, `year`, `rating` and `genres` are all
 * declared omitempty and are absent from real seeded data, so a pane built around them would be
 * mostly blank. Title, time, duration and `provenance` are what actually arrive.
 */
@Composable
private fun ChannelDetail(
    channel: ChannelTimeline,
    nowMs: Long,
    modifier: Modifier = Modifier,
) {
    val now = channel.airingAt(nowMs)
    val upcoming = channel.airings.filter { it.startMs > nowMs }.take(4)

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

        if (upcoming.isNotEmpty()) {
            SectionHeading(
                "Up next",
                modifier = Modifier.padding(top = LoomarrTokens.Space.S8),
            )
            upcoming.forEach { airing ->
                Row(
                    modifier = Modifier.padding(top = LoomarrTokens.Space.S3),
                    horizontalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S4),
                ) {
                    MonoData(
                        text = clockLabel(airing.startMs),
                        color = LoomarrTokens.Color.Static400,
                        fontSize = LoomarrTokens.Type.Md,
                    )
                    Body(airing.heading, maxLines = 1)
                }
            }
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
