package loomarr.media.playback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * How the probe combines what several decoders report about picture height.
 *
 * ⚠ These pin a bug that shipped: `maxHeight` was seeded to 1080 and combined with `maxOf`, so the
 * probe could never report anything BELOW 1080 — a device whose HEVC decoder tops out at 720 still
 * claimed 1080, because the seed always won. A floor dressed as a measurement, in the one function
 * whose whole job is to measure rather than assume.
 *
 * It survived because it lived inside a loop over `MediaCodecList`, a static platform call no unit
 * test reaches. The arithmetic is the part worth pinning.
 */
class DeviceProfileHeightTest {
    @Test
    fun `two silences stay silent`() {
        // "No decoder would tell us" must not become a number. It travels as maxResolution = 0,
        // which the wire contract defines as "no cap" — a device that declines to answer says
        // nothing rather than guessing.
        assertNull(DeviceProfile.tallerOf(null, null))
    }

    @Test
    fun `a single answer is the answer`() {
        assertEquals(720, DeviceProfile.tallerOf(null, 720))
        assertEquals(720, DeviceProfile.tallerOf(720, null))
    }

    @Test
    fun `the taller decoder wins`() {
        // A device may expose several hardware HEVC decoders; what it can play is the best of them.
        assertEquals(2160, DeviceProfile.tallerOf(1080, 2160))
        assertEquals(2160, DeviceProfile.tallerOf(2160, 1080))
    }

    @Test
    fun `a decoder that declines does not drag the answer down`() {
        assertEquals(2160, DeviceProfile.tallerOf(2160, null))
    }

    @Test
    fun `a device below 1080 reports what it actually is`() {
        // THE regression. Under the old seeding this combination produced 1080 no matter what the
        // hardware said, which would hand a 720 device a stream it cannot decode the moment the
        // server starts reading the field.
        assertEquals(720, DeviceProfile.tallerOf(null, 720))
        assertEquals(480, DeviceProfile.tallerOf(480, null))
        assertEquals(720, DeviceProfile.tallerOf(480, 720))
    }
}
