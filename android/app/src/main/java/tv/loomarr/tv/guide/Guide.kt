package tv.loomarr.tv.guide

/**
 * One block on a channel's timeline.
 *
 * A subset of the server's GuideAiring: the fields a 10-foot grid cell can actually show. The wire
 * type also carries genres, ratings, pod composition and image records, which belong to the web
 * hover card rather than a cell read from three metres away.
 */
data class Airing(
    /** `program`, `filler`, `pending`, or `flex`. */
    val kind: String,
    val title: String,
    val series: String?,
    val season: Int,
    val episode: Int,
    val startMs: Long,
    val stopMs: Long,
    /**
     * ⚠ The block's times are a DISPLAY ESTIMATE, not scheduled airtime.
     *
     * Today that means a pending acquisition: it has no known duration, so the grid draws it at a
     * nominal width to hold its place. A client must never present these times as a promise that
     * something airs then — the server says so explicitly, and honouring it is the difference
     * between a guide and a guess.
     */
    val nominal: Boolean,
    /** Pre-assembled "why is this here" — "in library", "acquiring · 62% · 8m left". */
    val provenance: String?,
) {
    /** How long this block occupies the schedule. */
    val durationMs: Long get() = stopMs - startMs

    /** "S2E4" for an episode, empty for a film. */
    val episodeLabel: String
        get() = if (season > 0 && episode > 0) "S${season}E$episode" else ""

    /**
     * What to show as the block's primary line: the series for an episode, the title otherwise.
     *
     * ⚠ A filler block carries an EMPTY title — verified against live data, not assumed. It is a
     * commercial break rather than a programme, so it is named rather than rendered as a blank.
     */
    val heading: String
        get() =
            series?.takeIf { it.isNotBlank() }
                ?: title.takeIf { it.isNotBlank() }
                ?: when (kind) {
                    "filler" -> "Commercial break"
                    "flex" -> "Off air"
                    else -> "Coming up"
                }
}

/** One channel's row in the grid. */
data class ChannelTimeline(
    val channelId: String,
    val name: String,
    val number: Int,
    /** `building`, `live`, `empty`, `drifted`, `detached`, or `paused`. */
    val status: String,
    /** Titles still being acquired — drives the "filling in" chip. */
    val pendingCount: Int,
    val airings: List<Airing>,
) {
    /** What is on at [atMs], or null in a gap. */
    fun airingAt(atMs: Long): Airing? = airings.firstOrNull { atMs >= it.startMs && atMs < it.stopMs }
}

/**
 * The guide window the server actually served.
 *
 * ⚠ [fromMs] and [toMs] are the CLAMPED window, not what was requested. The grid must lay out
 * against these rather than its own request, or a clamp turns into blocks drawn at the wrong offset.
 */
data class GuideWindow(
    val fromMs: Long,
    val toMs: Long,
    val channels: List<ChannelTimeline>,
) {
    val durationMs: Long get() = toMs - fromMs
}
