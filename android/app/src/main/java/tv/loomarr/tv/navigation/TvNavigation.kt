package tv.loomarr.tv.navigation

import tv.loomarr.tv.guide.ChannelTimeline
import tv.loomarr.tv.playback.Channel
import kotlin.math.abs

enum class TvSurface {
    Watching,
    Guide,
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

/** The bounded tune history needed by Surf and the remote's last-channel key. */
data class TuneHistory(
    val currentChannelId: String? = null,
    val lastChannelId: String? = null,
    val recentChannelIds: List<String> = emptyList(),
) {
    fun tuned(channelId: String): TuneHistory {
        if (channelId == currentChannelId) return this
        val previous = currentChannelId
        return copy(
            currentChannelId = channelId,
            lastChannelId = previous,
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
