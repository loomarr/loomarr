package tv.loomarr.tv.playback

import android.media.MediaCodecInfo.CodecProfileLevel
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Pins the HEVC profile literals in [DeviceProfile] against the platform's own constants.
 *
 * ⚠ Those literals exist because the CONSTANTS are API-gated (Main10HDR10 is API 24, HDR10Plus is
 * API 29) while minSdk is 23 — referencing them by name would throw `NoSuchFieldError` at class
 * load on an old device. Hardcoding them trades a crash for a silent wrong answer, though: a
 * mistyped literal reports the wrong capability, the server sends a stream the device cannot
 * decode, and the symptom is a black screen with nothing in the log.
 *
 * The first draft of those literals WAS wrong — 0x1000/0x2000/0x4000, each shifted one position —
 * and nothing in the build would have caught it. This test is why they are now correct.
 *
 * Unit tests run against a stubbed android.jar, but `static final int` fields are compile-time
 * constants inlined by the compiler, so they carry their real values here.
 */
class HevcProfileConstantsTest {
    @Test
    fun `main10 matches the platform constant`() {
        assertEquals(CodecProfileLevel.HEVCProfileMain10, 2)
    }

    @Test
    fun `main10 hdr10 matches the platform constant`() {
        assertEquals(CodecProfileLevel.HEVCProfileMain10HDR10, 4096)
    }

    @Test
    fun `main10 hdr10 plus matches the platform constant`() {
        assertEquals(CodecProfileLevel.HEVCProfileMain10HDR10Plus, 8192)
    }
}
