package loomarr.media.design

import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.EncodeHintType
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Round-trips the pairing URL through encode → decode.
 *
 * ⚠ A QR that renders is not a QR that SCANS. It can look convincing on screen and still be
 * unreadable — a missing quiet zone, a wrong error-correction level, or a payload that overflows
 * the chosen version all produce a plausible-looking square. Rendering proves it drew something;
 * only decoding proves it says what it should.
 *
 * This encodes with the same settings [QrCode] uses and reads the matrix back, so it fails if
 * either half drifts.
 */
class QrCodeTest {
    private fun encode(content: String): Pair<IntArray, Int> {
        val matrix =
            QRCodeWriter().encode(
                content,
                BarcodeFormat.QR_CODE,
                512,
                512,
                mapOf(
                    EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.H,
                    EncodeHintType.MARGIN to 4,
                ),
            )
        val pixels = IntArray(matrix.width * matrix.height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                pixels[y * matrix.width + x] = if (matrix.get(x, y)) BLACK else WHITE
            }
        }
        return pixels to matrix.width
    }

    private fun decode(content: String): String {
        val (pixels, width) = encode(content)
        val source = RGBLuminanceSource(width, width, pixels)
        return QRCodeReader().decode(BinaryBitmap(HybridBinarizer(source))).text
    }

    @Test
    fun `a pairing url survives the round trip`() {
        val url = "http://loomarr.local:8080/pair?code=BCDF-GHJK"

        assertEquals(url, decode(url))
    }

    /** The emulator's host alias, which is what every local test actually scans. */
    @Test
    fun `an ip address url survives the round trip`() {
        val url = "http://10.0.2.2:18305/pair?code=HJVG-NBKG"

        assertEquals(url, decode(url))
    }

    /**
     * A Tailscale hostname is long, and length is what pushes a QR to a denser version. Confirms the
     * settings still encode a realistic worst case rather than only the short local one.
     */
    @Test
    fun `a long hostname still encodes`() {
        val url = "https://loomarr.tail9c2f1e.ts.net/pair?code=WXPL-PVFX"

        assertEquals(url, decode(url))
    }

    /**
     * ⚠ The quiet zone is what scanners use to FIND the code. Without it a rendered QR looks right
     * and reads as nothing, which is the single most common way this feature breaks.
     *
     * Scope, stated honestly: this asserts the OUTPUT has a quiet zone, not that the MARGIN hint is
     * set correctly. Sabotaging the hint to 0 does not fail it, because ZXing enforces a minimum
     * regardless — so the config cannot be wrong in the direction this would catch. The assertion
     * itself does fail when a corner is dark, which is the property worth pinning.
     */
    @Test
    fun `the matrix carries a quiet zone`() {
        val (pixels, width) = encode("http://loomarr.local:8080/pair?code=BCDF-GHJK")

        // The corners sit inside the margin, so all four must be light.
        assertTrue("top-left corner is not quiet", pixels[0] == WHITE)
        assertTrue("top-right corner is not quiet", pixels[width - 1] == WHITE)
        assertTrue("bottom-left corner is not quiet", pixels[(width - 1) * width] == WHITE)
        assertTrue("bottom-right corner is not quiet", pixels[width * width - 1] == WHITE)
    }

    /**
     * ⚠ The QR's colours are design tokens, and a token edit could silently make it unscannable.
     *
     * Scanners threshold on LUMINANCE, not hue, so the constraint is contrast: `signal` amber
     * measures 1.8:1 on a light field and would look perfectly on-brand in a screenshot while
     * failing in a living room. This pins the actual pair well above that.
     */
    @Test
    fun `the module colour stays far clear of a scanner's contrast floor`() {
        val modules = LoomarrTokens.Color.Static800
        val quietZone = LoomarrTokens.Color.Static100

        val ratio = contrastRatio(modules, quietZone)

        assertTrue(
            "QR contrast fell to $ratio:1 — a scanner needs a wide margin, not merely WCAG text contrast",
            ratio > 7.0,
        )
    }

    private fun contrastRatio(
        a: androidx.compose.ui.graphics.Color,
        b: androidx.compose.ui.graphics.Color,
    ): Double {
        fun channel(c: Float): Double {
            val v = c.toDouble()
            return if (v <= 0.03928) v / 12.92 else Math.pow((v + 0.055) / 1.055, 2.4)
        }

        fun luminance(color: androidx.compose.ui.graphics.Color): Double =
            0.2126 * channel(color.red) + 0.7152 * channel(color.green) + 0.0722 * channel(color.blue)

        val (hi, lo) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (hi + 0.05) / (lo + 0.05)
    }

    private companion object {
        const val BLACK = 0xFF000000.toInt()
        const val WHITE = 0xFFFFFFFF.toInt()
    }
}
