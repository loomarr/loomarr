package loomarr.media.guide

import android.os.SystemClock
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import kotlinx.coroutines.delay

/** Advances a server-authored instant with monotonic elapsed time, never the television RTC. */
@Composable
internal fun rememberServerNow(anchorMs: Long): Long {
    val startedAt = remember(anchorMs) { SystemClock.elapsedRealtime() }
    var nowMs by remember(anchorMs) { mutableLongStateOf(anchorMs) }
    LaunchedEffect(anchorMs) {
        while (true) {
            nowMs = anchorMs + (SystemClock.elapsedRealtime() - startedAt)
            delay(SERVER_CLOCK_TICK_MS)
        }
    }
    return nowMs
}

private const val SERVER_CLOCK_TICK_MS = 1_000L
