package tv.loomarr.tv.playback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.IOException

/**
 * Which address the player fetches.
 *
 * This is one line of logic with a whole class of bug behind it: the server's absolute URL is built
 * from its own Host header, so it is only right when the client happens to share the server's idea
 * of where it lives. Every case here is a real deployment where it does not.
 */
class ResolveStreamUrlTest {
    private val relative = "/v1/playout/hls/ch_abc/master.m3u8?sig=abc%3A123%3Axyz"

    @Test
    fun `prefers the relative url against the paired base`() {
        // The bug this exists for: paired to the emulator's host alias, handed `localhost` by a
        // server that has no idea how it was reached. On Android `localhost` is the DEVICE, so the
        // absolute form fails to connect while the guide loads fine over the same pairing.
        val resolved =
            resolveStreamUrl(
                baseUrl = "http://10.0.2.2:18305",
                relativeUrl = relative,
                absoluteUrl = "http://localhost:18305$relative",
            )
        assertEquals("http://10.0.2.2:18305$relative", resolved)
    }

    @Test
    fun `keeps the signature intact`() {
        // The signature is a query parameter on the manifest AND on every segment URI the server
        // rewrites. Mangling it here fails at the first segment rather than at the manifest, which
        // is a far more confusing symptom.
        val resolved =
            resolveStreamUrl(
                baseUrl = "http://10.0.2.2:18305",
                relativeUrl = relative,
                absoluteUrl = "",
            )
        assertEquals(true, resolved.endsWith("?sig=abc%3A123%3Axyz"))
    }

    @Test
    fun `does not double the separating slash`() {
        val resolved =
            resolveStreamUrl(
                baseUrl = "http://10.0.2.2:18305/",
                relativeUrl = relative,
                absoluteUrl = "",
            )
        assertEquals("http://10.0.2.2:18305$relative", resolved)
    }

    @Test
    fun `tolerates a relative url with no leading slash`() {
        val resolved =
            resolveStreamUrl(
                baseUrl = "http://10.0.2.2:18305",
                relativeUrl = "v1/playout/hls/ch_abc/master.m3u8",
                absoluteUrl = "",
            )
        assertEquals("http://10.0.2.2:18305/v1/playout/hls/ch_abc/master.m3u8", resolved)
    }

    @Test
    fun `falls back to the absolute url only when there is no relative one`() {
        val resolved =
            resolveStreamUrl(
                baseUrl = "http://10.0.2.2:18305",
                relativeUrl = "",
                absoluteUrl = "https://loomarr.example.com$relative",
            )
        assertEquals("https://loomarr.example.com$relative", resolved)
    }

    @Test
    fun `names the failure when the server signed nothing`() {
        assertThrows(IOException::class.java) {
            resolveStreamUrl(baseUrl = "http://10.0.2.2:18305", relativeUrl = "", absoluteUrl = "")
        }
    }
}
