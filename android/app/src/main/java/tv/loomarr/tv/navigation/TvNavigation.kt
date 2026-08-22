package tv.loomarr.tv.navigation

import tv.loomarr.tv.guide.ChannelTimeline
import tv.loomarr.tv.playback.Channel
import kotlin.math.abs

enum class TvSurface {
    Watching,
    Guide,
}

enum class WatchingRemoteAction {
    ChannelUp,
    ChannelDown,
    OpenGuide,
    OpenSurf,
}

/** Back is intentionally absent: Android owns it and returns the viewer to the TV launcher. */
fun watchingRemoteAction(key: androidx.compose.ui.input.key.Key): WatchingRemoteAction? =
    when (key) {
        androidx.compose.ui.input.key.Key.DirectionUp,
        androidx.compose.ui.input.key.Key.ChannelUp,
        -> WatchingRemoteAction.ChannelUp
        androidx.compose.ui.input.key.Key.DirectionDown,
        androidx.compose.ui.input.key.Key.ChannelDown,
        -> WatchingRemoteAction.ChannelDown
        androidx.compose.ui.input.key.Key.DirectionCenter,
        androidx.compose.ui.input.key.Key.Enter,
        -> WatchingRemoteAction.OpenGuide
        androidx.compose.ui.input.key.Key.DirectionLeft,
        androidx.compose.ui.input.key.Key.Menu,
        -> WatchingRemoteAction.OpenSurf
        else -> null
    }

/** The paired app's three-state shell; Surf is an overlay on Watching, never another player. */
data class TvHomeState(
    val surface: TvSurface = TvSurface.Watching,
    val surfVisible: Boolean = false,
) {
    fun openGuide() = copy(surface = TvSurface.Guide, surfVisible = false)

    fun openSurf() = copy(surface = TvSurface.Watching, surfVisible = true)

    fun closeOverlay() = copy(surfVisible = false)

    fun watch() = copy(surface = TvSurface.Watching, surfVisible = false)
}

/** The bounded tune history used by Surf's recent-Channels group. */
data class TuneHistory(
    val currentChannelId: String? = null,
    val recentChannelIds: List<String> = emptyList(),
) {
    fun tuned(channelId: String): TuneHistory {
        if (channelId == currentChannelId) return this
        val previous = currentChannelId
        return copy(
            currentChannelId = channelId,
            recentChannelIds =
                if (previous == null) {
                    recentChannelIds
                } else {
                    (listOf(previous) + recentChannelIds)
                        .distinct()
                        .filterNot { it == channelId }
                        .take(RECENT_CHANNEL_LIMIT)
                },
        )
    }
}

/** Exact number matching: an incomplete or unknown number never tunes a different Channel. */
fun channelIndexForNumber(
    channels: List<Channel>,
    digits: String,
): Int? {
    val number = digits.toIntOrNull() ?: return null
    return channels.indexOfFirst { it.number == number }.takeIf { it >= 0 }
}

data class ChannelSection(
    val title: String,
    val channels: List<Channel>,
)

/** The Surf rail's stable group order. Empty optional groups stay explicit and honest. */
fun channelSections(
    channels: List<Channel>,
    favoriteIds: Set<String>,
    recentIds: List<String>,
): List<ChannelSection> {
    val byId = channels.associateBy { it.id }
    return listOf(
        ChannelSection("Favorites", channels.filter { it.id in favoriteIds }),
        ChannelSection("Recent", recentIds.mapNotNull(byId::get)),
        ChannelSection("All channels", channels),
    )
}

enum class GuideMove {
    Up,
    Down,
    Left,
    Right,
}

enum class GuideFocusTarget {
    Grid,
    Filters,
}

/** The Guide's virtual focus graph keeps filter controls reachable without relying on platform focus guesses. */
data class GuideFocus(
    val target: GuideFocusTarget = GuideFocusTarget.Grid,
    val cursor: GuideCursor = GuideCursor(),
    val filterIndex: Int = 0,
) {
    fun move(
        rows: List<ChannelTimeline>,
        direction: GuideMove,
        enabledFilterIndices: List<Int>,
        activeFilterIndex: Int,
    ): GuideFocus =
        when (target) {
            GuideFocusTarget.Grid -> {
                if (direction == GuideMove.Up && cursor.row <= 0) {
                    copy(
                        target = GuideFocusTarget.Filters,
                        filterIndex =
                            activeFilterIndex.takeIf { it in enabledFilterIndices }
                                ?: enabledFilterIndices.firstOrNull()
                                ?: 0,
                    )
                } else {
                    copy(cursor = cursor.move(rows, direction))
                }
            }
            GuideFocusTarget.Filters ->
                when (direction) {
                    GuideMove.Left -> copy(filterIndex = adjacentFilter(enabledFilterIndices, filterIndex, -1))
                    GuideMove.Right -> copy(filterIndex = adjacentFilter(enabledFilterIndices, filterIndex, 1))
                    GuideMove.Down -> copy(target = GuideFocusTarget.Grid)
                    GuideMove.Up -> this
                }
        }
}

private fun adjacentFilter(
    enabledFilterIndices: List<Int>,
    current: Int,
    offset: Int,
): Int {
    if (enabledFilterIndices.isEmpty()) return 0
    val position = enabledFilterIndices.indexOf(current).takeIf { it >= 0 } ?: 0
    return enabledFilterIndices[(position + offset).coerceIn(0, enabledFilterIndices.lastIndex)]
}

/** One explicit focus target in the guide; every move clamps to real data. */
data class GuideCursor(
    val row: Int = 0,
    val airing: Int = 0,
) {
    fun move(
        rows: List<ChannelTimeline>,
        direction: GuideMove,
    ): GuideCursor {
        if (rows.isEmpty()) return GuideCursor()
        val safeRow = row.coerceIn(0, rows.lastIndex)
        val safeAiring = airing.coerceIn(0, rows[safeRow].airings.lastIndex.coerceAtLeast(0))
        return when (direction) {
            GuideMove.Left -> copy(row = safeRow, airing = (safeAiring - 1).coerceAtLeast(0))
            GuideMove.Right ->
                copy(
                    row = safeRow,
                    airing = (safeAiring + 1).coerceAtMost(rows[safeRow].airings.lastIndex.coerceAtLeast(0)),
                )
            GuideMove.Up -> moveToRow(rows, safeRow - 1, safeAiring)
            GuideMove.Down -> moveToRow(rows, safeRow + 1, safeAiring)
        }
    }

    private fun moveToRow(
        rows: List<ChannelTimeline>,
        targetRow: Int,
        targetAiring: Int,
    ): GuideCursor {
        val nextRow = targetRow.coerceIn(0, rows.lastIndex)
        val currentStart = rows[row.coerceIn(0, rows.lastIndex)].airings.getOrNull(targetAiring)?.startMs
        val nextAiring =
            if (currentStart == null || rows[nextRow].airings.isEmpty()) {
                0
            } else {
                rows[nextRow].airings.indices.minBy { index ->
                    abs(rows[nextRow].airings[index].startMs - currentStart)
                }
            }
        return GuideCursor(row = nextRow, airing = nextAiring)
    }
}

private const val RECENT_CHANNEL_LIMIT = 6
