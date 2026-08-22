package tv.loomarr.tv.guide

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData

// The pieces every timeline in the guide is drawn from: a ruler, hour rules, the now-line, and a
// block.
//
// Shared rather than owned by one screen because GuideGrid draws one timeline PER CHANNEL against a
// single window, and these must agree to the pixel — a ruler that measured differently from the
// gridlines beneath it, or a now-line that landed at a different offset than the blocks it crosses,
// would make the grid lie about time. They take their width and height as parameters for that
// reason: the geometry is shared, the size is the caller's.

/**
 * The hour labels above a timeline.
 *
 * ⚠ Each label is offset to its hour and then nudged right by [RulerLabelInset] — and that nudge is
 * only honest because [HourGridlines] draws a rule at the unnudged position. A label is a claim
 * about WHERE an hour falls; padding one sideways with nothing to measure it against moves the claim
 * without moving the hour. The first build did exactly that and the ruler was wrong by the inset.
 */
@Composable
internal fun TimeRuler(
    window: GuideWindow,
    timelineWidth: Dp,
    modifier: Modifier = Modifier,
) {
    Box(modifier = modifier.fillMaxWidth().height(LoomarrTokens.Space.S6)) {
        hourBoundaries(window).forEach { boundaryMs ->
            MonoData(
                text = clockLabel(boundaryMs),
                color = LoomarrTokens.Color.Static500,
                fontSize = LoomarrTokens.Type.Xs,
                // ⚠ Capped, per the MonoData comment: an uncapped ruler label wrapped into a
                // vertical column of single characters when its box was narrower than the text.
                maxLines = 1,
                modifier =
                    Modifier.offset(
                        x = window.offsetOf(boundaryMs, timelineWidth) + RulerLabelInset,
                    ),
            )
        }
    }
}

/** Clears the gridline so the label reads beside the rule rather than on top of it. */
private val RulerLabelInset = 6.dp

/**
 * A hairline at every hour, behind the blocks.
 *
 * Without these the ruler is decoration: the labels float over the timeline with nothing tying a
 * time to a position, so a viewer cannot tell whether a block starts at half past or quarter to.
 * The gridline is what turns the row of numbers into a scale.
 */
@Composable
internal fun HourGridlines(
    window: GuideWindow,
    timelineWidth: Dp,
    height: Dp,
) {
    hourBoundaries(window).forEach { boundaryMs ->
        Box(
            modifier =
                Modifier
                    .offset(x = window.offsetOf(boundaryMs, timelineWidth))
                    .width(1.dp)
                    .height(height)
                    .background(LoomarrTokens.Color.Static700),
        )
    }
}

/**
 * Where "now" falls in the window.
 *
 * ⚠ [nowMs] is the SERVER's now, threaded down from `GuideWindow.fromMs`, not
 * `System.currentTimeMillis()`. A television is exactly the hardware whose clock cannot be trusted —
 * some boxes have no RTC and NTP may not have synced — and reading device time here would slide this
 * line to an arbitrary place, or off the grid entirely.
 *
 * ⚠ `onair` red, not `signal` amber. Amber is the brand and marks what is AIRING, so an amber line
 * through amber blocks disappears into them — the web grid carries the same note.
 */
@Composable
internal fun NowLine(
    nowMs: Long,
    window: GuideWindow,
    timelineWidth: Dp,
    modifier: Modifier = Modifier,
) {
    if (nowMs < window.fromMs || nowMs > window.toMs) return

    Box(
        modifier =
            modifier
                .offset(x = window.offsetOf(nowMs, timelineWidth))
                .width(NowLineWidth)
                // Spans whatever it is drawn into — in the grid that is every row at once, which is
                // what makes it a single mark across the whole schedule rather than a per-row tick.
                .fillMaxHeight()
                .background(LoomarrTokens.Color.Onair),
    )
}

/** Thin enough to read as a marker rather than a block, thick enough to survive TV upscaling. */
private val NowLineWidth = 3.dp

/** One block, rendered according to what its width can actually hold. */
@Composable
internal fun TimelineBlock(
    airing: Airing,
    width: Dp,
    onAir: Boolean,
    height: Dp,
    modifier: Modifier = Modifier,
    focused: Boolean = false,
) {
    val treatment = blockTreatment(width.value, airing.kind, onAir)

    Box(
        modifier =
            modifier
                .width(width)
                .height(height)
                // A hairline gap so adjacent blocks read as two things, not one wide one.
                .padding(end = 2.dp, bottom = 2.dp)
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                .background(treatment.fill)
                .border(
                    width = if (focused) 3.dp else 1.dp,
                    color = if (focused) LoomarrTokens.Color.Signal else treatment.stroke,
                    shape = RoundedCornerShape(LoomarrTokens.Radius.Sm),
                ),
    ) {
        if (treatment.showsText) {
            Column(modifier = Modifier.padding(LoomarrTokens.Space.S3)) {
                Body(
                    text = airing.heading,
                    color = LoomarrTokens.Color.Static0,
                    // ⚠ `Xs`, a step below the rest of the screen. Body's own contract allows this
                    // for dense surfaces, and a guide block is the densest in the app: its width is
                    // set by a programme's DURATION, not by what the text needs, so type that suits
                    // an otherwise empty screen prices short blocks out of saying anything. Still
                    // 18sp — comfortably above the ~13px this reads at on web.
                    fontSize = LoomarrTokens.Type.Xs,
                    maxLines = 1,
                )
                if (treatment.showsTime) {
                    Row(modifier = Modifier.padding(top = LoomarrTokens.Space.S1)) {
                        MonoData(
                            // ⚠ A range, not a start time. The web guide shows "5:38 AM–8:00 AM" on
                            // every block, and the second half is what tells a viewer whether they
                            // have time to watch it before something else they wanted.
                            text = "${clockLabel(airing.startMs)}–${clockLabel(airing.stopMs)}",
                            color = LoomarrTokens.Color.Static400,
                            fontSize = LoomarrTokens.Type.Xs2,
                            maxLines = 1,
                        )
                        if (airing.nominal) {
                            // ⚠ These times are a DISPLAY ESTIMATE, not scheduled airtime — a
                            // pending acquisition has no known duration and is drawn at a nominal
                            // width to hold its place. Presenting it unmarked would turn a
                            // placeholder into a promise that something airs then.
                            MonoData(
                                text = "≈",
                                color = LoomarrTokens.Color.Caution,
                                fontSize = LoomarrTokens.Type.Xs2,
                                maxLines = 1,
                                modifier = Modifier.padding(start = LoomarrTokens.Space.S2),
                            )
                        }
                    }
                }
            }
        }
    }
}

/** How a block should be drawn at a given width. */
data class BlockTreatment(
    val fill: Color,
    val stroke: Color,
    val showsText: Boolean,
    /** Whether the time range fits beneath the heading. */
    val showsTime: Boolean,
)

/**
 * How a block renders at [widthDp] — the same block is a titled card at 300dp and a sliver at 8dp.
 *
 * [kind] is `program`, `filler`, `pending`, or `flex`. [airing] is whether a programme is on air
 * NOW, which is what earns it the amber treatment.
 *
 * ⚠ Amber marks what is AIRING, not what is filler. That is the web guide's rule and it is easy to
 * get backwards from a screenshot, where the only amber visible happens to be commercial breaks:
 * they look solid at seven pixels, but they are a 30% wash, and an on-air programme carries the
 * same hue at 12%. Inverting this would make every commercial break look like the thing on air.
 */
internal fun blockTreatment(
    widthDp: Float,
    kind: String,
    airing: Boolean,
): BlockTreatment =
    when (kind) {
        // Never labelled, at any width. The web grid renders filler as a bare wash and lets the
        // WIDTH carry the meaning — a four-minute break is visibly a quarter of a sixteen-minute
        // one. Naming it "Commercial break" would give a break the same visual weight as a film,
        // so a channel with five breaks an hour reads as five programmes.
        "filler" ->
            BlockTreatment(
                fill = LoomarrTokens.Color.Signal.copy(alpha = 0.30f),
                stroke = LoomarrTokens.Color.Signal.copy(alpha = 0.40f),
                showsText = false,
                showsTime = false,
            )

        // `tune` cyan, and deliberately the faintest fill in the set: a pending block is a
        // placeholder for something being acquired, so it should read as an absence with a shape
        // rather than as programming.
        "pending" ->
            BlockTreatment(
                fill = LoomarrTokens.Color.Tune.copy(alpha = 0.08f),
                stroke = LoomarrTokens.Color.Tune.copy(alpha = 0.40f),
                showsText = widthDp >= LABEL_MIN_DP,
                showsTime = false,
            )

        // Off air: no stroke accent, nothing to tune to.
        "flex" ->
            BlockTreatment(
                fill = LoomarrTokens.Color.Static800,
                stroke = LoomarrTokens.Color.Static700,
                showsText = widthDp >= LABEL_MIN_DP,
                showsTime = widthDp >= META_MIN_DP,
            )

        else ->
            BlockTreatment(
                fill =
                    if (airing) {
                        LoomarrTokens.Color.Signal.copy(alpha = 0.12f)
                    } else {
                        LoomarrTokens.Color.Static800
                    },
                stroke =
                    if (airing) {
                        LoomarrTokens.Color.Signal.copy(alpha = 0.40f)
                    } else {
                        LoomarrTokens.Color.Static700
                    },
                showsText = widthDp >= LABEL_MIN_DP,
                showsTime = widthDp >= META_MIN_DP,
            )
    }

/**
 * Below this, a heading degrades to a couple of glyphs and an ellipsis — which reads as a rendering
 * fault rather than as a name.
 *
 * ⚠ Derived from web's MEASURED 74px, not guessed: below it "The Simpsons · Bart…" stops being a
 * name. Scaled by this block's type rather than by the global TV factor — the heading is `Xs` (18sp)
 * against web's ~13px, so ×1.38, not the ×1.5 the rest of the screen uses.
 */
private const val LABEL_MIN_DP = 95f

/**
 * Web's measured 132px for the time range, scaled the same way.
 *
 * ⚠ This number is why the channel list narrowed and this block's type dropped a step. At the
 * original 420dp list and `Sm` type it worked out to 198dp, while a 108-minute film across a
 * four-hour window measured only 185 — so almost nothing showed its times. The honest fix was to
 * widen the strip and tighten the type, not to halve the window: the span is what the guide MEANS,
 * and a two-hour guide shows the viewer half as much of what is coming.
 */
private const val META_MIN_DP = 170f

// ── geometry ────────────────────────────────────────────────────────────────────────────────────

/**
 * Where [atMs] sits across [paneWidth].
 *
 * ⚠ Measured against the window the server actually SERVED. [GuideWindow.fromMs]/[GuideWindow.toMs]
 * are the clamped window, not what was requested, and laying out against a request the server
 * narrowed draws every block at the wrong offset.
 */
internal fun GuideWindow.offsetOf(
    atMs: Long,
    paneWidth: Dp,
): Dp {
    if (durationMs <= 0) return 0.dp
    val fraction = (atMs - fromMs).toDouble() / durationMs.toDouble()
    return paneWidth * fraction.coerceIn(0.0, 1.0).toFloat()
}

/** Where this block starts, clamped to the window's left edge. */
internal fun Airing.offsetIn(
    window: GuideWindow,
    paneWidth: Dp,
): Dp = window.offsetOf(startMs, paneWidth)

/**
 * How wide this block is, clipped to the window on both sides.
 *
 * A block that began before the window opened is drawn from the left edge, so its width is the
 * VISIBLE portion rather than its full duration — otherwise the first block of every channel
 * overhangs the strip by however long it had already been running.
 */
internal fun Airing.widthIn(
    window: GuideWindow,
    paneWidth: Dp,
): Dp {
    val visibleStart = startMs.coerceAtLeast(window.fromMs)
    val visibleStop = stopMs.coerceAtMost(window.toMs)
    if (visibleStop <= visibleStart) return 0.dp
    return window.offsetOf(visibleStop, paneWidth) - window.offsetOf(visibleStart, paneWidth)
}

/** Every whole hour inside the window, for the ruler. */
private fun hourBoundaries(window: GuideWindow): List<Long> {
    val hour = 3_600_000L
    // Round the window's start UP to the next whole hour: the window opens at "now", which is
    // almost never on the hour, and a label at the very left edge would sit under the first block.
    var t = ((window.fromMs + hour - 1) / hour) * hour
    val out = mutableListOf<Long>()
    while (t < window.toMs) {
        out += t
        t += hour
    }
    return out
}

/**
 * "8:30" in the device's own timezone.
 *
 * ⚠ Converted through `ZoneId.systemDefault()`, not by arithmetic on the epoch. Dividing epoch ms
 * into hours and minutes yields UTC, which is right only in Britain in winter.
 *
 * The server sends absolute epoch ms deliberately: "a timezone is a formatting choice, and putting
 * instants on the wire in local time would invite a client to reinterpret rather than merely format
 * them." This is that formatting choice.
 */
internal fun clockLabel(epochMs: Long): String {
    val local =
        java.time.Instant
            .ofEpochMilli(epochMs)
            .atZone(java.time.ZoneId.systemDefault())
    return "%d:%02d".format(local.hour, local.minute)
}
