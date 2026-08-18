package tv.loomarr.tv.design

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
 * A QR code, drawn rather than bitmapped.
 *
 * The matrix is encoded with ZXing and painted with Compose, so the code takes our own tokens
 * instead of a stock black-on-white image. That matters on a dark TV: an inverted photo-negative
 * QR reads badly to some scanners, so this keeps dark modules on a light quiet-zone the way a
 * printed code does, but frames it in the product's own surface.
 *
 * ⚠ The quiet zone is not optional. A QR needs a light margin of at least four modules or scanners
 * fail to find it, so the light background extends past the matrix by design — dropping it to save
 * space is the single most common way a rendered QR becomes unscannable.
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
                .background(Color.White)
                .padding(4.dp),
    ) {
        val moduleSize = this.size.width / matrix.width
        for (x in 0 until matrix.width) {
            for (y in 0 until matrix.height) {
                if (!matrix.get(x, y)) continue
                drawRect(
                    color = Color.Black,
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
