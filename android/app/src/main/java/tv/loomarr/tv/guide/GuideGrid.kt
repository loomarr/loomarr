package tv.loomarr.tv.guide

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.Heading
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.SectionHeading
import tv.loomarr.tv.navigation.GuideCursor
import tv.loomarr.tv.navigation.GuideMove

/** The mock's Channel-by-time Guide with one explicit, never-lost remote cursor. */
@Composable
fun GuideGrid(
    window: GuideWindow,
    nowMs: Long,
    onTune: (ChannelTimeline) -> Unit,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    favoriteChannelIds: Set<String> = emptySet(),
    recentChannelIds: List<String> = emptyList(),
) {
    var filter by remember { mutableStateOf(GuideFilter.All) }
    val rows = filteredRows(window.channels, filter, favoriteChannelIds, recentChannelIds)
    var cursor by remember { mutableStateOf(GuideCursor()) }
    val focus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        withFrameNanos { }
        focus.requestFocus()
    }
    LaunchedEffect(rows) {
        if (rows.isEmpty()) {
            filter = GuideFilter.All
            cursor = GuideCursor()
        } else {
            cursor =
                cursor.copy(
                    row = cursor.row.coerceIn(0, rows.lastIndex),
                    airing = cursor.airing.coerceIn(
                        0,
                        rows[cursor.row.coerceIn(0, rows.lastIndex)].airings.lastIndex.coerceAtLeast(0),
                    ),
                )
        }
    }

    val focusedChannel = rows.getOrNull(cursor.row)
    val focusedAiring = focusedChannel?.airings?.getOrNull(cursor.airing)

    Column(
        modifier =
            modifier
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    when (event.key) {
                        Key.DirectionUp -> {
                            cursor = cursor.move(rows, GuideMove.Up)
                            true
                        }
                        Key.DirectionDown -> {
                            cursor = cursor.move(rows, GuideMove.Down)
                            true
                        }
                        Key.DirectionLeft -> {
                            cursor = cursor.move(rows, GuideMove.Left)
                            true
                        }
                        Key.DirectionRight -> {
                            cursor = cursor.move(rows, GuideMove.Right)
                            true
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            focusedChannel?.let(onTune)
                            true
                        }
                        Key.Menu -> {
                            filter = nextAvailableFilter(filter, window.channels, favoriteChannelIds, recentChannelIds)
                            cursor = GuideCursor()
                            true
                        }
                        Key.Back -> {
                            onBack()
                            true
                        }
                        else -> false
                    }
                },
    ) {
        GuideHeader(
            selected = filter,
            allCount = window.channels.size,
            favoriteCount = window.channels.count { it.channelId in favoriteChannelIds },
            recentCount = window.channels.count { it.channelId in recentChannelIds },
        )

        BoxWithConstraints(modifier = Modifier.fillMaxWidth().weight(1f)) {
            val timelineWidth = maxWidth - CHANNEL_COLUMN_WIDTH - POSITION_RAIL_WIDTH
            Column(modifier = Modifier.fillMaxSize().padding(end = POSITION_RAIL_WIDTH)) {
                Row(modifier = Modifier.fillMaxWidth().height(RULER_HEIGHT)) {
                    SectionHeading(
                        "Channel",
                        modifier = Modifier.width(CHANNEL_COLUMN_WIDTH).padding(start = LoomarrTokens.Space.S3),
                    )
                    TimeRuler(window = window, timelineWidth = timelineWidth)
                }
                ChannelRows(
                    rows = rows,
                    window = window,
                    nowMs = nowMs,
                    timelineWidth = timelineWidth,
                    cursor = cursor,
                    modifier = Modifier.fillMaxSize(),
                )
            }
            NowLine(
                nowMs = nowMs,
                window = window,
                timelineWidth = timelineWidth,
                modifier = Modifier.padding(
                    start = CHANNEL_COLUMN_WIDTH,
                    top = RULER_HEIGHT,
                    end = POSITION_RAIL_WIDTH,
                ),
            )
            PositionRail(
                row = cursor.row,
                count = rows.size,
                modifier = Modifier.align(Alignment.CenterEnd).fillMaxHeight().width(POSITION_RAIL_WIDTH),
            )
        }

        FocusedDetail(
            channel = focusedChannel,
            airing = focusedAiring,
            row = cursor.row,
            count = rows.size,
        )
    }
}

@Composable
private fun GuideHeader(
    selected: GuideFilter,
    allCount: Int,
    favoriteCount: Int,
    recentCount: Int,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(bottom = LoomarrTokens.Space.S3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Heading("Guide")
        FilterChip("All · $allCount", selected == GuideFilter.All)
        FilterChip("★ Favorites · $favoriteCount", selected == GuideFilter.Favorites)
        FilterChip("Recent · $recentCount", selected == GuideFilter.Recent)
        MonoData(
            "◀▶ time · ▲▼ channel · OK tune · MENU filter",
            color = LoomarrTokens.Color.Static500,
            fontSize = LoomarrTokens.Type.Xs2,
            maxLines = 1,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S6),
        )
    }
}

@Composable
private fun FilterChip(
    label: String,
    active: Boolean,
) {
    MonoData(
        label,
        color = if (active) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static400,
        fontSize = LoomarrTokens.Type.Xs2,
        maxLines = 1,
        modifier =
            Modifier
                .padding(start = LoomarrTokens.Space.S3)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Lg))
                .background(
                    if (active) LoomarrTokens.Color.Signal.copy(alpha = 0.1f) else LoomarrTokens.Color.Static950,
                ).border(
                    1.dp,
                    if (active) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static700,
                    RoundedCornerShape(LoomarrTokens.Radius.Lg),
                ).padding(horizontal = LoomarrTokens.Space.S3, vertical = LoomarrTokens.Space.S1),
    )
}

@Composable
private fun ChannelRows(
    rows: List<ChannelTimeline>,
    window: GuideWindow,
    nowMs: Long,
    timelineWidth: Dp,
    cursor: GuideCursor,
    modifier: Modifier = Modifier,
) {
    val list = rememberLazyListState()
    LaunchedEffect(cursor.row) {
        if (rows.isNotEmpty() && list.layoutInfo.visibleItemsInfo.none { it.index == cursor.row }) {
            list.scrollToItem(cursor.row)
        }
    }
    LazyColumn(state = list, modifier = modifier) {
        itemsIndexed(rows, key = { _, row -> row.channelId }) { index, channel ->
            ChannelRow(
                channel = channel,
                window = window,
                nowMs = nowMs,
                timelineWidth = timelineWidth,
                selectedRow = index == cursor.row,
                selectedAiring = if (index == cursor.row) cursor.airing else -1,
            )
        }
    }
}

@Composable
private fun ChannelRow(
    channel: ChannelTimeline,
    window: GuideWindow,
    nowMs: Long,
    timelineWidth: Dp,
    selectedRow: Boolean,
    selectedAiring: Int,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .height(ROW_HEIGHT)
                .background(
                    if (selectedRow) LoomarrTokens.Color.Signal.copy(alpha = 0.03f) else LoomarrTokens.Color.Static950,
                ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ChannelCell(channel = channel, selected = selectedRow)
        Box(modifier = Modifier.fillMaxHeight().width(timelineWidth)) {
            HourGridlines(window = window, timelineWidth = timelineWidth, height = ROW_HEIGHT)
            channel.airings.forEachIndexed { index, airing ->
                val width = airing.widthIn(window, timelineWidth)
                if (width > 0.dp) {
                    TimelineBlock(
                        airing = airing,
                        width = width,
                        onAir = nowMs >= airing.startMs && nowMs < airing.stopMs,
                        height = ROW_HEIGHT,
                        focused = selectedAiring == index,
                        modifier = Modifier.offset(x = airing.offsetIn(window, timelineWidth)),
                    )
                }
            }
            if (channel.airings.isEmpty()) {
                Body(
                    "Nothing scheduled",
                    fontSize = LoomarrTokens.Type.Xs,
                    modifier = Modifier.align(Alignment.CenterStart).padding(start = LoomarrTokens.Space.S3),
                )
            }
        }
    }
}

@Composable
private fun ChannelCell(
    channel: ChannelTimeline,
    selected: Boolean,
) {
    Row(
        modifier =
            Modifier
                .width(CHANNEL_COLUMN_WIDTH)
                .fillMaxHeight()
                .padding(end = LoomarrTokens.Space.S2, bottom = LoomarrTokens.Space.S1)
                .background(if (selected) LoomarrTokens.Color.Static900 else LoomarrTokens.Color.Static950)
                .padding(horizontal = LoomarrTokens.Space.S3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        MonoData(
            channel.number.toString().padStart(2, '0'),
            color = if (selected) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static400,
            fontSize = LoomarrTokens.Type.Sm,
        )
        Body(
            channel.name,
            color = if (selected) LoomarrTokens.Color.Static0 else LoomarrTokens.Color.Static100,
            fontSize = LoomarrTokens.Type.Xs,
            maxLines = 2,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
        )
    }
}

@Composable
private fun PositionRail(
    row: Int,
    count: Int,
    modifier: Modifier = Modifier,
) {
    BoxWithConstraints(modifier = modifier.background(LoomarrTokens.Color.Static950)) {
        if (count > 0) {
            val thumbFraction = (VISIBLE_ROWS.toFloat() / count).coerceIn(0.08f, 1f)
            val thumbHeight = maxHeight * thumbFraction
            val travel = maxHeight - thumbHeight
            val position = if (count <= 1) 0f else row.toFloat() / (count - 1).toFloat()
            Box(
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .height(thumbHeight)
                        .offset(y = travel * position.coerceIn(0f, 1f))
                        .padding(horizontal = LoomarrTokens.Space.S1)
                        .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                        .background(LoomarrTokens.Color.Static700),
            )
        }
    }
}

@Composable
private fun FocusedDetail(
    channel: ChannelTimeline?,
    airing: Airing?,
    row: Int,
    count: Int,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .height(DETAIL_HEIGHT)
                .border(1.dp, LoomarrTokens.Color.Static700)
                .background(LoomarrTokens.Color.Static900)
                .padding(horizontal = LoomarrTokens.Space.S4),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Body(
            airing?.heading ?: "Nothing scheduled",
            color = LoomarrTokens.Color.Static0,
            fontSize = LoomarrTokens.Type.Sm,
            maxLines = 1,
        )
        if (airing != null && channel != null) {
            MonoData(
                "${clockLabel(airing.startMs)}–${clockLabel(airing.stopMs)} · CH ${channel.number}",
                color = LoomarrTokens.Color.Static400,
                fontSize = LoomarrTokens.Type.Xs2,
                maxLines = 1,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S4),
            )
        }
        MonoData(
            "row ${(row + 1).coerceAtMost(count)} of $count · OK to tune",
            color = LoomarrTokens.Color.Signal,
            fontSize = LoomarrTokens.Type.Xs2,
            maxLines = 1,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S6),
        )
    }
}

private enum class GuideFilter {
    All,
    Favorites,
    Recent,
}

private fun filteredRows(
    channels: List<ChannelTimeline>,
    filter: GuideFilter,
    favorites: Set<String>,
    recents: List<String>,
): List<ChannelTimeline> =
    when (filter) {
        GuideFilter.All -> channels
        GuideFilter.Favorites -> channels.filter { it.channelId in favorites }
        GuideFilter.Recent -> {
            val byId = channels.associateBy { it.channelId }
            recents.mapNotNull(byId::get)
        }
    }

private fun nextAvailableFilter(
    current: GuideFilter,
    channels: List<ChannelTimeline>,
    favorites: Set<String>,
    recents: List<String>,
): GuideFilter {
    val order = GuideFilter.entries
    for (offset in 1..order.size) {
        val candidate = order[(current.ordinal + offset) % order.size]
        if (filteredRows(channels, candidate, favorites, recents).isNotEmpty()) return candidate
    }
    return GuideFilter.All
}

private val CHANNEL_COLUMN_WIDTH = 210.dp
private val POSITION_RAIL_WIDTH = 12.dp
private val RULER_HEIGHT = 36.dp
private val ROW_HEIGHT = 48.dp
private val DETAIL_HEIGHT = 52.dp
private const val VISIBLE_ROWS = 6
