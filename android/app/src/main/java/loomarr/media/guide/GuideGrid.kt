package loomarr.media.guide

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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import loomarr.media.design.Body
import loomarr.media.design.LoomarrTokens
import loomarr.media.design.MonoData
import loomarr.media.design.OverscanMargin
import loomarr.media.design.RemoteArtwork
import loomarr.media.design.SectionHeading
import loomarr.media.navigation.GuideCursor
import loomarr.media.navigation.GuideFocus
import loomarr.media.navigation.GuideFocusTarget
import loomarr.media.navigation.GuideMove

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
    artworkAuthorization: String? = null,
) {
    var filter by remember { mutableStateOf(GuideFilter.All) }
    val rows = filteredRows(window.channels, filter, favoriteChannelIds, recentChannelIds)
    val favoriteCount = window.channels.count { it.channelId in favoriteChannelIds }
    val recentCount = window.channels.count { it.channelId in recentChannelIds }
    val enabledFilterIndices =
        buildList {
            add(GuideFilter.All.ordinal)
            if (favoriteCount > 0) add(GuideFilter.Favorites.ordinal)
            if (recentCount > 0) add(GuideFilter.Recent.ordinal)
        }
    var guideFocus by remember { mutableStateOf(GuideFocus()) }
    val cursor = guideFocus.cursor
    val focus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        withFrameNanos { }
        focus.requestFocus()
    }
    LaunchedEffect(rows) {
        if (rows.isEmpty()) {
            filter = GuideFilter.All
            guideFocus = GuideFocus()
        } else {
            guideFocus =
                guideFocus.copy(
                    cursor =
                        cursor.copy(
                            row = cursor.row.coerceIn(0, rows.lastIndex),
                            airing = cursor.airing.coerceIn(
                                0,
                                rows[cursor.row.coerceIn(0, rows.lastIndex)].airings.lastIndex.coerceAtLeast(0),
                            ),
                        ),
                    filterIndex =
                        guideFocus.filterIndex.takeIf { it in enabledFilterIndices }
                            ?: filter.ordinal,
                )
        }
    }

    val focusedChannel = rows.getOrNull(cursor.row)
    val focusedAiring = focusedChannel?.airings?.getOrNull(cursor.airing)

    Column(
        modifier =
            modifier
                .testTag(GUIDE_GRID_TAG)
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    when (event.key) {
                        Key.DirectionUp -> {
                            guideFocus =
                                guideFocus.move(rows, GuideMove.Up, enabledFilterIndices, filter.ordinal)
                            true
                        }
                        Key.DirectionDown -> {
                            guideFocus =
                                guideFocus.move(rows, GuideMove.Down, enabledFilterIndices, filter.ordinal)
                            true
                        }
                        Key.DirectionLeft -> {
                            guideFocus =
                                guideFocus.move(rows, GuideMove.Left, enabledFilterIndices, filter.ordinal)
                            true
                        }
                        Key.DirectionRight -> {
                            guideFocus =
                                guideFocus.move(rows, GuideMove.Right, enabledFilterIndices, filter.ordinal)
                            true
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            if (guideFocus.target == GuideFocusTarget.Filters) {
                                filter = GuideFilter.entries[guideFocus.filterIndex]
                            } else {
                                focusedChannel?.let(onTune)
                            }
                            true
                        }
                        Key.Menu -> {
                            filter = nextAvailableFilter(filter, window.channels, favoriteChannelIds, recentChannelIds)
                            guideFocus = GuideFocus()
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
            focused =
                guideFocus.filterIndex
                    .takeIf { guideFocus.target == GuideFocusTarget.Filters }
                    ?.let(GuideFilter.entries::get),
            allCount = window.channels.size,
            favoriteCount = favoriteCount,
            recentCount = recentCount,
        )

        BoxWithConstraints(modifier = Modifier.fillMaxWidth().weight(1f)) {
            val timelineWidth = maxWidth - CHANNEL_COLUMN_WIDTH - POSITION_RAIL_WIDTH
            Column(modifier = Modifier.fillMaxSize().padding(end = POSITION_RAIL_WIDTH)) {
                Row(modifier = Modifier.fillMaxWidth().height(RULER_HEIGHT)) {
                    SectionHeading(
                        "Channel",
                        modifier =
                            Modifier
                                .width(CHANNEL_COLUMN_WIDTH)
                                .padding(start = OverscanMargin + LoomarrTokens.Space.S3),
                    )
                    TimeRuler(window = window, timelineWidth = timelineWidth)
                }
                ChannelRows(
                    rows = rows,
                    window = window,
                    nowMs = nowMs,
                    timelineWidth = timelineWidth,
                    cursor = cursor,
                    focusVisible = guideFocus.target == GuideFocusTarget.Grid,
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
            artworkAuthorization = artworkAuthorization,
        )
    }
}

@Composable
private fun GuideHeader(
    selected: GuideFilter,
    focused: GuideFilter?,
    allCount: Int,
    favoriteCount: Int,
    recentCount: Int,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(
                    start = OverscanMargin,
                    top = OverscanMargin,
                    end = OverscanMargin,
                    bottom = LoomarrTokens.Space.S3,
                ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Body(
            "Guide",
            color = LoomarrTokens.Color.Static0,
            fontSize = LoomarrTokens.Type.Lg,
            maxLines = 1,
        )
        FilterChip(
            "All · $allCount",
            active = selected == GuideFilter.All,
            focused = focused == GuideFilter.All,
            enabled = true,
        )
        FilterChip(
            "★ Favorites · $favoriteCount",
            active = selected == GuideFilter.Favorites,
            focused = focused == GuideFilter.Favorites,
            enabled = favoriteCount > 0,
        )
        FilterChip(
            "Recent · $recentCount",
            active = selected == GuideFilter.Recent,
            focused = focused == GuideFilter.Recent,
            enabled = recentCount > 0,
        )
        Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.CenterEnd) {
            MonoData(
                if (focused == null) "▲  Filters" else "◀▶ Choose · OK Apply · ▼ Grid",
                color = LoomarrTokens.Color.Static500,
                fontSize = LoomarrTokens.Type.Xs2,
                maxLines = 1,
            )
        }
    }
}

@Composable
private fun FilterChip(
    label: String,
    active: Boolean,
    focused: Boolean,
    enabled: Boolean,
) {
    MonoData(
        label,
        color =
            when {
                focused -> LoomarrTokens.Color.Signal
                !enabled -> LoomarrTokens.Color.Static700
                active -> LoomarrTokens.Color.Static0
                else -> LoomarrTokens.Color.Static400
            },
        fontSize = LoomarrTokens.Type.Xs2,
        maxLines = 1,
        modifier =
            Modifier
                .padding(start = LoomarrTokens.Space.S3)
                .then(if (focused) Modifier.testTag(GUIDE_FOCUSED_FILTER_TAG) else Modifier)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Lg))
                .background(
                    when {
                        focused -> LoomarrTokens.Color.Signal.copy(alpha = 0.18f)
                        active -> LoomarrTokens.Color.Signal.copy(alpha = 0.1f)
                        else -> LoomarrTokens.Color.Static950
                    },
                ).border(
                    if (focused) 2.dp else 1.dp,
                    when {
                        focused || active -> LoomarrTokens.Color.Signal
                        enabled -> LoomarrTokens.Color.Static700
                        else -> LoomarrTokens.Color.Static900
                    },
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
    focusVisible: Boolean,
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
                selectedRow = focusVisible && index == cursor.row,
                selectedAiring = if (focusVisible && index == cursor.row) cursor.airing else -1,
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
                .padding(
                    start = OverscanMargin + LoomarrTokens.Space.S3,
                    end = LoomarrTokens.Space.S3,
                ),
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
            fontSize = LoomarrTokens.Type.Xs2,
            maxLines = 1,
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
    artworkAuthorization: String?,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .height(DETAIL_HEIGHT)
                .border(1.dp, LoomarrTokens.Color.Static700)
                .background(LoomarrTokens.Color.Static900)
                .padding(
                    start = OverscanMargin + LoomarrTokens.Space.S2,
                    top = LoomarrTokens.Space.S2,
                    end = OverscanMargin + LoomarrTokens.Space.S2,
                    bottom = LoomarrTokens.Space.S2,
                ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RemoteArtwork(
            url = airing?.thumbUrl,
            title = airing?.heading ?: channel?.name ?: "Programme artwork",
            authorization = artworkAuthorization,
            modifier = Modifier.width(136.dp).height(76.dp),
        )
        Column(
            modifier = Modifier.padding(start = LoomarrTokens.Space.S3).weight(1f),
        ) {
            Body(
                airing?.heading ?: "Nothing scheduled",
                color = LoomarrTokens.Color.Static0,
                fontSize = LoomarrTokens.Type.Sm,
                maxLines = 1,
            )
            airing?.episodeFacts()?.takeIf { it.isNotEmpty() }?.let {
                MonoData(
                    it,
                    color = LoomarrTokens.Color.Static100,
                    fontSize = LoomarrTokens.Type.Xs2,
                    maxLines = 1,
                )
            }
            val facts = airing?.scheduleFacts(channel).orEmpty()
            if (facts.isNotEmpty()) {
                MonoData(
                    facts,
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Xs2,
                    maxLines = 1,
                )
            }
            airing?.description?.let {
                Body(
                    it,
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Xs2,
                    maxLines = 1,
                )
            }
        }
    }
}

private fun Airing.episodeFacts(): String =
    buildList {
        episodeTitle?.let { add("“$it”") }
        episodeLabel.takeIf { it.isNotEmpty() }?.let(::add)
        year.takeIf { it > 0 }?.toString()?.let(::add)
        rating?.let(::add)
    }.joinToString(" · ")

private fun Airing.scheduleFacts(channel: ChannelTimeline?): String =
    buildList {
        genres
            .take(2)
            .takeIf { it.isNotEmpty() }
            ?.joinToString(" / ")
            ?.let(::add)
        add("${clockLabel(startMs)}–${clockLabel(stopMs)}")
        channel?.let { add("CH ${it.number}") }
    }.joinToString(" · ")

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

private val CHANNEL_COLUMN_WIDTH = 250.dp + OverscanMargin
private val POSITION_RAIL_WIDTH = 12.dp
private val RULER_HEIGHT = 36.dp
private val ROW_HEIGHT = 48.dp
private val DETAIL_HEIGHT = 124.dp
private const val VISIBLE_ROWS = 5
internal const val GUIDE_GRID_TAG = "guide-grid"
internal const val GUIDE_FOCUSED_FILTER_TAG = "guide-focused-filter"
