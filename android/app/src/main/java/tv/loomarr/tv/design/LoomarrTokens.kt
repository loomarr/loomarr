// GENERATED FILE — DO NOT EDIT.
//
// Produced by scripts/gen-android-tokens.mjs from web/packages/tokens/generated/tokens.json, which
// is the one source of truth the web app also renders from. Edit the tokens, then run
// `make android-tokens`.
//
// A colour that is not in the design system cannot be referenced from here, which is the point: the
// first hand-written TV screen used a Tailwind amber (#F59E0B) in place of Loomarr's signal colour
// (#FFB020), and nothing could catch it.

package tv.loomarr.tv.design

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** Loomarr's design tokens, shared with the web client. */
object LoomarrTokens {
    /** Palette. Identical values to the web client — same hex, same names. */
    object Color {
        /** `static-950` — #0B0C0E */
        val Static950 = Color(0xFF0B0C0E)

        /** `static-900` — #131519 */
        val Static900 = Color(0xFF131519)

        /** `static-800` — #1B1E24 */
        val Static800 = Color(0xFF1B1E24)

        /** `static-700` — #2A2E37 */
        val Static700 = Color(0xFF2A2E37)

        /** `static-500` — #5A6170 */
        val Static500 = Color(0xFF5A6170)

        /** `static-400` — #8B93A3 */
        val Static400 = Color(0xFF8B93A3)

        /** `static-100` — #E7EAF0 */
        val Static100 = Color(0xFFE7EAF0)

        /** `static-0` — #FFFFFF */
        val Static0 = Color(0xFFFFFFFF)

        /** `signal-400` — #FFC14D */
        val Signal400 = Color(0xFFFFC14D)

        /** `border-control` — #61646B */
        val BorderControl = Color(0xFF61646B)

        /** `signal` — #FFB020 */
        val Signal = Color(0xFFFFB020)

        /** `onair` — #E5484D */
        val Onair = Color(0xFFE5484D)

        /** `onair-300` — #E85A5F */
        val Onair300 = Color(0xFFE85A5F)

        /** `suggest` — #D6409F */
        val Suggest = Color(0xFFD6409F)

        /** `suggest-300` — #DC5BAC */
        val Suggest300 = Color(0xFFDC5BAC)

        /** `tune` — #4CC9E8 */
        val Tune = Color(0xFF4CC9E8)

        /** `lock` — #3DD68C */
        val Lock = Color(0xFF3DD68C)

        /** `caution` — #F5D90A */
        val Caution = Color(0xFFF5D90A)

        /** `primary` — #FFB020 */
        val Primary = Color(0xFFFFB020)

        /** `destructive` — #E5484D */
        val Destructive = Color(0xFFE5484D)

        /** `success` — #3DD68C */
        val Success = Color(0xFF3DD68C)

        /** `warning` — #F5D90A */
        val Warning = Color(0xFFF5D90A)

        /** `info` — #4CC9E8 */
        val Info = Color(0xFF4CC9E8)
    }

    /** Spacing scale, in dp. Same steps as the web client. */
    object Space {
        /** space.1 */
        val S1 = 4.dp

        /** space.2 */
        val S2 = 8.dp

        /** space.3 */
        val S3 = 12.dp

        /** space.4 */
        val S4 = 16.dp

        /** space.5 */
        val S5 = 20.dp

        /** space.6 */
        val S6 = 24.dp

        /** space.7 */
        val S7 = 28.dp

        /** space.8 */
        val S8 = 32.dp
    }

    /** Corner radii, in dp. */
    object Radius {
        /** radius.sm */
        val Sm = 4.dp

        /** radius.md */
        val Md = 8.dp

        /** radius.lg */
        val Lg = 12.dp
    }

    /**
     * Type scale, in sp.
     *
     * ⚠ Scaled ×1.5 from the web values. A television is viewed from roughly three metres
     * rather than an arm's length, so web's 14sp body would be unreadable. The RATIOS between steps
     * are preserved — this moves the whole scale, it does not redesign it.
     */
    object Type {
        /** typography.size.2xs (11sp on web, ×1.5 for 10-foot viewing) */
        val Xs2 = 17.sp

        /** typography.size.xs (12sp on web, ×1.5 for 10-foot viewing) */
        val Xs = 18.sp

        /** typography.size.sm (13sp on web, ×1.5 for 10-foot viewing) */
        val Sm = 20.sp

        /** typography.size.base (14sp on web, ×1.5 for 10-foot viewing) */
        val Base = 21.sp

        /** typography.size.md (16sp on web, ×1.5 for 10-foot viewing) */
        val Md = 24.sp

        /** typography.size.lg (20sp on web, ×1.5 for 10-foot viewing) */
        val Lg = 30.sp

        /** typography.size.xl (24sp on web, ×1.5 for 10-foot viewing) */
        val Xl = 36.sp

        /** typography.size.2xl (32sp on web, ×1.5 for 10-foot viewing) */
        val Xl2 = 48.sp

        /**
         * A pairing code, read across a room and typed into a phone.
         *
         * ⚠ NOT derived from the web scale, because the web has nothing like it — this is a value
         * a viewer must transcribe from several metres away, and the largest web step (32sp)
         * is still a heading. It is stated here rather than inline in a screen so it stays one
         * decision instead of a literal someone later "tidies".
         *
         * 52, down from 88 and then 64. Each cut was measured, not guessed: 88 left no room for the
         * countdown and control beneath it, and 64 rendered a nine-character code at ~346dp inside a
         * 356dp column — 97% of the width, so it crowded the divider on one side and the panel edge
         * on the other while the QR beside it sat with space to spare. At 52 the code is still the
         * largest thing on screen and easily read from a sofa, and both halves get real margins.
         */
        val Code = 52.sp
    }
}
