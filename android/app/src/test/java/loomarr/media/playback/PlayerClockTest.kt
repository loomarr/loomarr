package loomarr.media.playback

import androidx.media3.common.C
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PlayerClockTest {
    @Test
    fun `programme clock follows the displayed frame behind the live edge`() {
        assertEquals(992_000L, frameUnixTimeMs(originNowMs = 1_000_000L, liveOffsetMs = 8_000L))
    }

    @Test
    fun `programme clock waits for Media3 live timing instead of guessing`() {
        assertNull(frameUnixTimeMs(originNowMs = C.TIME_UNSET, liveOffsetMs = 8_000L))
        assertNull(frameUnixTimeMs(originNowMs = 1_000_000L, liveOffsetMs = C.TIME_UNSET))
    }
}
