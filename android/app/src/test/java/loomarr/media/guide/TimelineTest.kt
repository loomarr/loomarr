package loomarr.media.guide

import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate
import java.time.ZoneId

/**
 * The guide timeline's geometry and per-kind styling.
 *
 * These are plain JVM tests because the interesting behaviour is arithmetic on the served window and
 * a lookup by block kind — neither needs a device, a view to inflate, or a screenshot. Only the
 * painting does, and painting is not what breaks here.
 */
class TimelineTest {
    private val hour = 3_600_000L
    private val windowStart = 1_700_000_000_000L

    /** A four-hour window across the grid's real timeline pane on a 1080p television. */
    private fun window(vararg airings: Airing) =
        GuideWindow(
            fromMs = windowStart,
            toMs = windowStart + 4 * hour,
            channels =
                listOf(
                    ChannelTimeline(
                        channelId = "c1",
                        name = "90s Action",
                        number = 42,
                        status = "live",
                        pendingCount = 0,
                        airings = airings.toList(),
                    ),
                ),
        )

    private fun airing(
        startMinutes: Long,
        durationMinutes: Long,
        kind: String = "program",
        title: String = "Terminator 2",
    ) = Airing(
        kind = kind,
        title = title,
        series = null,
        season = 0,
        episode = 0,
        startMs = windowStart + startMinutes * 60_000L,
        stopMs = windowStart + (startMinutes + durationMinutes) * 60_000L,
        nominal = false,
        provenance = null,
    )

    /**
     * The timeline width on the shared 960dp TV canvas: 96dp overscan, 210dp Channel column, and
     * the 12dp position rail. The same canvas maps to either 1080p/xhdpi or 4K/xxxhdpi.
     */
    private val pane = 642.dp

    @Test
    fun `clock labels use twelve hour time`() {
        fun localEpoch(
            hour: Int,
            minute: Int,
        ) = LocalDate
            .of(2026, 8, 22)
            .atTime(hour, minute)
            .atZone(ZoneId.systemDefault())
            .toInstant()
            .toEpochMilli()

        assertEquals("12:05 AM", clockLabel(localEpoch(0, 5)))
        assertEquals("7:24 PM", clockLabel(localEpoch(19, 24)))
    }

    @Test
    fun `block width is proportional to duration`() {
        val w = window()
        // A two-hour film occupies half of the four-hour pane.
        val twoHours = airing(startMinutes = 0, durationMinutes = 120)
        assertEquals(321f, twoHours.widthIn(w, pane).value, 0.5f)

        val fourMinutes = airing(startMinutes = 0, durationMinutes = 4)
        assertEquals(10.7f, fourMinutes.widthIn(w, pane).value, 0.5f)
    }

    @Test
    fun `a feature film shows its time range at the four-hour span`() {
        // The reason the channel list narrowed and the block's type dropped a step. At the original
        // 420dp list and `Sm` type a 108-minute film measured 185dp against a 198dp threshold, so
        // essentially nothing on a real channel printed its own times.
        val w = window()
        val film = airing(startMinutes = 0, durationMinutes = 108)
        val width = film.widthIn(w, pane).value
        assertTrue("a 108-minute film measured only ${width}dp", width >= 200f)
        assertTrue(blockTreatment(width, kind = "program", airing = false).showsTime)
    }

    @Test
    fun `a block that began before the window is clipped to the visible portion`() {
        val w = window()
        // Started an hour before the window opened and runs an hour into it. Only the hour INSIDE
        // the window may be drawn — sizing it by full duration overhangs the strip.
        val overhanging = airing(startMinutes = -60, durationMinutes = 120)
        assertEquals(160.5f, overhanging.widthIn(w, pane).value, 0.5f)
        assertEquals(0f, overhanging.offsetIn(w, pane).value, 0.01f)
    }

    @Test
    fun `a block entirely outside the window has no width`() {
        val w = window()
        val after = airing(startMinutes = 300, durationMinutes = 30)
        assertEquals(0f, after.widthIn(w, pane).value, 0.01f)
    }

    @Test
    fun `offsets are measured against the served window`() {
        val w = window()
        // Halfway through a four-hour window is halfway across the pane.
        assertEquals(321f, w.offsetOf(windowStart + 2 * hour, pane).value, 0.5f)
        assertEquals(0f, w.offsetOf(windowStart, pane).value, 0.01f)
        assertEquals(642f, w.offsetOf(windowStart + 4 * hour, pane).value, 0.5f)
    }

    @Test
    fun `filler is never labelled at any width`() {
        // The web grid lets WIDTH carry filler's meaning. Labelling a commercial break would give
        // it the same visual weight as a film, so a channel with five breaks an hour would read as
        // five programmes.
        listOf(4f, 50f, 200f, 900f).forEach { w ->
            val t = blockTreatment(w, kind = "filler", airing = false)
            assertFalse("filler showed text at ${w}dp", t.showsText)
            assertFalse("filler showed a time range at ${w}dp", t.showsTime)
        }
    }

    @Test
    fun `a narrow programme drops its label rather than truncating to glyphs`() {
        val narrow = blockTreatment(60f, kind = "program", airing = false)
        assertFalse(narrow.showsText)

        val wide = blockTreatment(300f, kind = "program", airing = false)
        assertTrue(wide.showsText)
        assertTrue(wide.showsTime)
    }

    @Test
    fun `the time range needs more room than the heading`() {
        // 150dp holds a heading but not a second line beneath it.
        val medium = blockTreatment(150f, kind = "program", airing = false)
        assertTrue("a 150dp block should hold a heading", medium.showsText)
        assertFalse("a 150dp block should not hold a time range", medium.showsTime)
    }

    @Test
    fun `amber marks what is airing, not what is filler`() {
        // Easy to get backwards from a screenshot, where the only visible amber happens to be
        // commercial breaks. An on-air programme and a filler block must not share a fill.
        val onAir = blockTreatment(300f, kind = "program", airing = true)
        val offAir = blockTreatment(300f, kind = "program", airing = false)
        val filler = blockTreatment(300f, kind = "filler", airing = false)

        assertTrue("an on-air programme should not look like an idle one", onAir.fill != offAir.fill)
        assertTrue("an on-air programme should not look like filler", onAir.fill != filler.fill)
        // Filler's wash is the heavier of the two ambers; on-air is a faint tint under its stroke.
        assertTrue(filler.fill.alpha > onAir.fill.alpha)
    }

    @Test
    fun `pending blocks never claim a time range`() {
        // A pending block's times are an ESTIMATE — it has no known duration and is drawn at a
        // nominal width to hold its place. Showing a range would turn a placeholder into a promise.
        val pending = blockTreatment(900f, kind = "pending", airing = false)
        assertFalse(pending.showsTime)
    }

    @Test
    fun `a zero-length window does not divide by zero`() {
        val degenerate = GuideWindow(fromMs = windowStart, toMs = windowStart, channels = emptyList())
        assertEquals(0f, degenerate.offsetOf(windowStart, pane).value, 0.01f)
    }
}
