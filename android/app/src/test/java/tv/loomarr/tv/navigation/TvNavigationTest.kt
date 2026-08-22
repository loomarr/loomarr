package tv.loomarr.tv.navigation

import androidx.compose.ui.input.key.Key
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import tv.loomarr.tv.guide.Airing
import tv.loomarr.tv.guide.ChannelTimeline
import tv.loomarr.tv.playback.Channel
import tv.loomarr.tv.playback.remoteDigit

class TvNavigationTest {
    private val channels =
        listOf(
            Channel("sci-fi", "Late Night Sci-Fi", 7, true),
            Channel("nature", "Nature Documentaries", 21, true),
            Channel("horror", "Cozy Autumn Horror", 22, true),
        )

    @Test
    fun `paired shell starts watching and keeps surf over that surface`() {
        val initial = TvHomeState()
        assertEquals(TvSurface.Watching, initial.surface)
        assertEquals(false, initial.surfVisible)

        val surf = initial.openSurf()
        assertEquals(TvSurface.Watching, surf.surface)
        assertEquals(true, surf.surfVisible)
        assertEquals(TvSurface.Guide, surf.openGuide().surface)
        assertEquals(false, surf.openGuide().surfVisible)
    }

    @Test
    fun `remote digits map without treating unrelated keys as numbers`() {
        assertEquals('0', remoteDigit(Key.Zero))
        assertEquals('7', remoteDigit(Key.Seven))
        assertNull(remoteDigit(Key.DirectionUp))
    }

    @Test
    fun `tune history preserves last channel and newest-first recents`() {
        val history =
            TuneHistory()
                .tuned("sci-fi")
                .tuned("nature")
                .tuned("horror")

        assertEquals("horror", history.currentChannelId)
        assertEquals("nature", history.lastChannelId)
        assertEquals(listOf("nature", "sci-fi"), history.recentChannelIds)
        assertEquals(history.tuned("horror"), history)
    }

    @Test
    fun `channel number resolves exactly and never guesses`() {
        assertEquals(1, channelIndexForNumber(channels, "21"))
        assertNull(channelIndexForNumber(channels, "2"))
        assertNull(channelIndexForNumber(channels, "99"))
        assertNull(channelIndexForNumber(channels, ""))
    }

    @Test
    fun `surf sections keep optional groups honest`() {
        val sections =
            channelSections(
                channels = channels,
                favoriteIds = emptySet(),
                recentIds = listOf("nature", "missing", "sci-fi"),
            )

        assertEquals(listOf("Favorites", "Recent", "All channels"), sections.map { it.title })
        assertEquals(emptyList<Channel>(), sections[0].channels)
        assertEquals(listOf("nature", "sci-fi"), sections[1].channels.map { it.id })
        assertEquals(channels, sections[2].channels)
    }

    @Test
    fun `guide cursor follows rows and airings without escaping the grid`() {
        val rows =
            listOf(
                timeline("a", 1, 0L, 100L, 200L),
                timeline("b", 2, 0L, 90L, 210L),
            )

        var cursor = GuideCursor(row = 0, airing = 1)
        cursor = cursor.move(rows, GuideMove.Right)
        assertEquals(2, cursor.airing)
        assertEquals(cursor, cursor.move(rows, GuideMove.Right))

        cursor = cursor.move(rows, GuideMove.Down)
        assertEquals(1, cursor.row)
        assertEquals(2, cursor.airing)

        cursor = cursor.move(rows, GuideMove.Left).move(rows, GuideMove.Up)
        assertEquals(0, cursor.row)
        assertEquals(1, cursor.airing)
    }

    @Test
    fun `guide focus moves from the first row into enabled filters and back`() {
        val rows =
            listOf(
                timeline("a", 1, 0L, 100L),
                timeline("b", 2, 0L, 90L),
            )
        val enabledFilters = listOf(0, 2)

        var focus = GuideFocus(cursor = GuideCursor(row = 0, airing = 1))
        focus = focus.move(rows, GuideMove.Up, enabledFilters, activeFilterIndex = 0)
        assertEquals(GuideFocusTarget.Filters, focus.target)
        assertEquals(0, focus.filterIndex)

        focus = focus.move(rows, GuideMove.Right, enabledFilters, activeFilterIndex = 0)
        assertEquals(2, focus.filterIndex)

        focus = focus.move(rows, GuideMove.Down, enabledFilters, activeFilterIndex = 0)
        assertEquals(GuideFocusTarget.Grid, focus.target)
        assertEquals(GuideCursor(row = 0, airing = 1), focus.cursor)
    }

    private fun timeline(
        id: String,
        number: Int,
        vararg starts: Long,
    ) = ChannelTimeline(
        channelId = id,
        name = id,
        number = number,
        status = "live",
        pendingCount = 0,
        airings =
            starts.map { start ->
                Airing(
                    kind = "program",
                    title = "show-$start",
                    series = null,
                    season = 0,
                    episode = 0,
                    startMs = start,
                    stopMs = start + 80,
                    nominal = false,
                    provenance = null,
                )
            },
    )
}
