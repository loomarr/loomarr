package tv.loomarr.tv.playback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The posted profile is what the server keys its copy-vs-transcode decision on, so its wire shape is
 * a contract rather than an implementation detail. A renamed field is a silent downgrade to
 * baseline — the server sees no claim and assumes none.
 */
class DeviceProfileJsonTest {
    private val profile =
        DeviceProfile(
            video = listOf("hevc", "h264"),
            audio = listOf("eac3", "aac"),
            video10Bit = true,
            hdr = false,
            maxResolution = 2160,
        )

    @Test
    fun `posts the field names the server reads`() {
        val json = profile.toJson()

        assertTrue(json.has("video"))
        assertTrue(json.has("audio"))
        // ⚠ Lowercase `video10bit`, not `video10Bit`. The Go DTO's json tag is lowercase, and an
        // unknown field is dropped rather than rejected — so this typo would cost 10-bit playback
        // with nothing in any log to explain it.
        assertTrue(json.has("video10bit"))
        assertTrue(json.has("hdr"))
        assertTrue(json.has("maxResolution"))
    }

    @Test
    fun `carries the declared capabilities`() {
        val json = profile.toJson()

        assertEquals(2160, json.getInt("maxResolution"))
        assertTrue(json.getBoolean("video10bit"))
        assertFalse(json.getBoolean("hdr"))
        assertEquals("hevc", json.getJSONArray("video").getString(0))
    }

    /**
     * The server floors an absent or empty profile at h264/aac, so an empty one is safe rather than
     * broken — it costs a transcode, never a black screen.
     */
    @Test
    fun `an empty profile still serialises`() {
        val empty =
            DeviceProfile(
                video = emptyList(),
                audio = emptyList(),
                video10Bit = false,
                hdr = false,
                maxResolution = 0,
            ).toJson()

        assertEquals(0, empty.getJSONArray("video").length())
        assertFalse(empty.getBoolean("video10bit"))
    }
}
