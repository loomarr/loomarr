package loomarr.media.design

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel

/**
 * A QR code, drawn rather than bitmapped, in Loomarr's own colours.
 *
 * ⚠ Scanning depends on LUMINANCE contrast, not hue — a scanner thresholds the image to light and
 * dark — so the palette choice here is constrained by physics rather than taste. Measured against
 * the light quiet zone:
 *
 * ```
 *   static-800  #1B1E24   16.7:1   used here
 *   static-950  #0B0C0E   19.6:1   near-black
 *   signal      #FFB020    1.8:1   unscannable
 *   tune        #4CC9E8    1.9:1   unscannable
 * ```
 *
 * So the modules take `static-800` — the design's nested-surface token, which reads as the
 * product's near-black rather than a stock `#000000`, while keeping contrast far above what any
 * scanner needs. An accent-coloured QR would look on-brand in a screenshot and fail in a living
 * room.
 *
 * ⚠ NOT inverted. Light modules on a dark field is the obvious "dark theme" move and it breaks
 * scanners: the finder patterns are defined dark-on-light, and while some readers cope, many do
 * not. The quiet zone therefore stays light and the code stays dark-on-light, framed by the
 * product's surface rather than floating on white.
 *
 * ⚠ The quiet zone is not optional. A QR needs a light margin of at least four modules or scanners
 * fail to find it, so the light background extends past the matrix by design — dropping it to save
 * space is the most common way a rendered QR becomes unscannable.
 */
@Composable
fun QrCode(
    content: String,
    modifier: Modifier = Modifier,
    size: Dp = 220.dp,
) {
    // Cheap, but not free, and it never changes for a given string — so it is remembered rather
    // than recomputed on every recomposition of a screen that also polls every few seconds.
    val matrix =
        remember(content) {
            runCatching {
                QRCodeWriter().encode(
                    content,
                    BarcodeFormat.QR_CODE,
                    QR_MODULES,
                    QR_MODULES,
                    mapOf(
                        // A pairing URL is short, so the highest correction level costs little and
                        // buys tolerance for a camera at an angle across a room.
                        EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.H,
                        // ZXing's own margin, in modules. Four is the spec minimum.
                        EncodeHintType.MARGIN to 4,
                    ),
                )
            }.getOrNull()
        } ?: return

    Canvas(
        modifier =
            modifier
                .size(size)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                // `static-100` rather than pure white: the design's own light tone. Measured at
                // 13.9:1 against the modules — down from 16.7:1 on pure white, and still far above
                // anything a scanner needs — so the whole mark sits inside the palette at no real
                // cost.
                .background(LoomarrTokens.Color.Static100)
                .padding(4.dp),
    ) {
        val moduleSize = this.size.width / matrix.width
        for (x in 0 until matrix.width) {
            for (y in 0 until matrix.height) {
                if (!matrix.get(x, y)) continue
                drawRect(
                    color = LoomarrTokens.Color.Static800,
                    topLeft = Offset(x * moduleSize, y * moduleSize),
                    size = Size(moduleSize, moduleSize),
                )
            }
        }
    }
}

/**
 * The matrix ZXing is asked for.
 *
 * Not the module count of the finished code — ZXing picks the QR version from the payload and
 * scales it to fit this box. It is a rendering resolution, kept generous so the drawn squares land
 * on clean boundaries.
 */
private const val QR_MODULES = 512
