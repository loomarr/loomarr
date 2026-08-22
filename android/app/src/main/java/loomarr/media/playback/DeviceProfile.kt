package loomarr.media.playback

import android.media.MediaCodecInfo
import android.media.MediaCodecList
import org.json.JSONArray
import org.json.JSONObject

/**
 * What this device can actually decode, probed at runtime.
 *
 * ⚠ PROBED, never hardcoded. The same APK runs on a Shield Pro that decodes HEVC 10-bit and on a
 * cheap stick that manages h264 only, and the server bases its copy-vs-transcode decision on what
 * we claim here. A false positive is a black screen; a false negative is only a needless transcode,
 * so every check below fails toward the safe answer.
 *
 * This mirrors how Jellyfin's Android TV client builds its profile — from `MediaCodecList` at
 * runtime, posted per playback — rather than shipping a device table.
 */
data class DeviceProfile(
    val video: List<String>,
    val audio: List<String>,
    val video10Bit: Boolean,
    val hdr: Boolean,
    val maxResolution: Int,
) {
    fun toJson(): JSONObject =
        JSONObject()
            .put("video", JSONArray(video))
            .put("audio", JSONArray(audio))
            .put("video10bit", video10Bit)
            .put("hdr", hdr)
            .put("maxResolution", maxResolution)

    companion object {
        /**
         * Probe the device.
         *
         * ⚠ Software decoders are EXCLUDED. A software HEVC decoder exists on most devices and will
         * technically decode, but at 4K it drops frames badly — advertising it turns a "we can play
         * this" promise into a stuttering picture. Jellyfin filters these out for the same reason;
         * the codec being *listed* is not the same as it being *usable*.
         */
        fun probe(): DeviceProfile {
            val codecs = MediaCodecList(MediaCodecList.REGULAR_CODECS)
            val video = mutableSetOf<String>()
            val audio = mutableSetOf<String>()
            var tenBit = false
            var hdr = false

            // ⚠ Starts UNKNOWN, not at 1080.
            //
            // Seeded to 1080 and combined with maxOf, this could never report anything BELOW 1080:
            // a device whose HEVC decoder tops out at 720 still claimed 1080, because the seed
            // won. A floor dressed as a measurement — the probe exists precisely so capability is
            // read from the device rather than assumed, and this assumed the common answer.
            //
            // Harmless today only because the server does not yet read `maxResolution` (it is
            // plumbed to playout.CopyPlan and never consumed). The day it gates a copy plan, this
            // would hand low-end hardware a stream it cannot decode, and the symptom — a black
            // screen — would surface nowhere near this line.
            var maxHeight: Int? = null

            for (info in codecs.codecInfos) {
                if (info.isEncoder) continue
                if (isSoftwareOnly(info)) continue

                for (type in info.supportedTypes) {
                    when (type.lowercase()) {
                        "video/avc" -> video += "h264"
                        "video/hevc" -> {
                            video += "hevc"
                            val caps = runCatching { info.getCapabilitiesForType(type) }.getOrNull()
                            if (caps != null) {
                                if (supportsHevc10Bit(caps)) tenBit = true
                                if (supportsHdr(caps)) hdr = true
                                maxHeight = tallerOf(maxHeight, maxHeightFor(caps))
                            }
                        }
                        // AV1 is probed like anything else rather than special-cased. The Shield has
                        // no AV1 decoder and never will; a newer box does, and the probe is what
                        // tells them apart without a device table.
                        "video/av01" -> video += "av1"
                        "audio/mp4a-latm" -> audio += "aac"
                        "audio/ac3" -> audio += "ac3"
                        "audio/eac3" -> audio += "eac3"
                    }
                }
            }

            // h264/aac are the implied floor the server assumes anyway; including them keeps the
            // posted profile self-describing rather than relying on that assumption.
            video += "h264"
            audio += "aac"

            return DeviceProfile(
                video = video.toList(),
                audio = audio.toList(),
                video10Bit = tenBit,
                hdr = hdr,
                // ⚠ 0 means "no cap", which is the wire contract's own word for unknown — the
                // server documents `0 = no cap` and omitempty drops the field entirely. So a device
                // that would not report a height says nothing rather than guessing, and the server
                // falls back to whatever it does for a client that never sent a profile.
                //
                // Deliberately NOT a conservative low guess either: inventing 720 would cap a
                // device that never claimed to be limited, transcoding streams it could have
                // direct-played. Silence is the honest answer to a question the device declined.
                maxResolution = maxHeight ?: 0,
            )
        }

        /**
         * A codec is software-only when the platform says so, or when its name carries one of the
         * conventional software prefixes.
         *
         * ⚠ `isSoftwareOnly` is API 29+, and `minSdk` is 23, so the version guard is load-bearing
         * rather than defensive — calling it unguarded would crash on an older device. The name
         * prefixes are the fallback below 29 and still catch oddly-named vendor codecs above it.
         */
        private fun isSoftwareOnly(info: MediaCodecInfo): Boolean {
            if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.Q && info.isSoftwareOnly) {
                return true
            }
            val name = info.name.lowercase()
            return name.startsWith("omx.google.") || name.startsWith("c2.android.")
        }

        // ⚠ The HDR profile CONSTANTS are themselves API-gated — Main10HDR10 is API 24, HDR10Plus is
        // API 29 — so they are compared as literals rather than referenced by name. Referencing the
        // constant on an older device throws NoSuchFieldError at class-load time, which is a crash
        // rather than a missing capability. Their integer values are frozen platform API and cannot
        // change; a device too old to define them simply never reports them.
        // Values read from android.jar (API 36) with `javap -constants`, NOT from memory — a wrong
        // literal here silently reports the wrong capability, which is a black screen rather than a
        // compile error. HevcProfileConstantsTest pins them.
        private const val HEVC_PROFILE_MAIN10 = 2
        private const val HEVC_PROFILE_MAIN10_HDR10 = 4096
        private const val HEVC_PROFILE_MAIN10_HDR10_PLUS = 8192

        private fun supportsHevc10Bit(caps: MediaCodecInfo.CodecCapabilities): Boolean =
            caps.profileLevels.any {
                it.profile == HEVC_PROFILE_MAIN10 ||
                    it.profile == HEVC_PROFILE_MAIN10_HDR10 ||
                    it.profile == HEVC_PROFILE_MAIN10_HDR10_PLUS
            }

        private fun supportsHdr(caps: MediaCodecInfo.CodecCapabilities): Boolean =
            caps.profileLevels.any {
                it.profile == HEVC_PROFILE_MAIN10_HDR10 || it.profile == HEVC_PROFILE_MAIN10_HDR10_PLUS
            }

        /**
         * The taller of two reported heights, where null means "not reported".
         *
         * A device may expose several hardware HEVC decoders, and what it can play is the best of
         * them — but a decoder that declines to answer must not drag the answer down, and two
         * silences must stay silent rather than becoming a number.
         *
         * ⚠ Extracted so it can be tested. The bug it replaces — a `maxHeight` seeded to 1080 and
         * combined with `maxOf`, which could never report anything lower — survived precisely
         * because it lived inside a loop over `MediaCodecList`, a static platform call no unit test
         * reaches. The arithmetic is the part worth pinning; the enumeration is not.
         */
        internal fun tallerOf(
            a: Int?,
            b: Int?,
        ): Int? =
            when {
                a == null -> b
                b == null -> a
                else -> maxOf(a, b)
            }

        /**
         * The tallest picture this decoder reports, or null if it will not say.
         *
         * ⚠ Nullable rather than defaulted, because "the decoder says 1080" and "the decoder would
         * not answer" are different facts and the caller resolves them differently. Collapsing them
         * to a bare 1080 was how a failed query became an affirmative capability claim.
         *
         * `videoCapabilities` is @Nullable in the platform — it is null for an AUDIO codec, which
         * has no video capabilities. Unreachable from the one call site (guarded by `video/hevc`),
         * but written safely rather than relying on a caller staying correct: this returns null for
         * an audio codec instead of throwing inside a `runCatching` that would silently answer 1080.
         */
        private fun maxHeightFor(caps: MediaCodecInfo.CodecCapabilities): Int? =
            runCatching { caps.videoCapabilities?.supportedHeights?.upper }.getOrNull()
    }
}
