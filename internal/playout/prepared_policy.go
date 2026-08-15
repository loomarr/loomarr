package playout

import "github.com/mantonx/loomarr/internal/prepared"

const preparedSegmentDurationMS = 2_000

// CanonicalPreparedRendition is the one reusable rendition Loomarr prepares today. It deliberately
// resolves from the existing operator quality tier instead of inventing a second quality ladder,
// while fixing the codec contract to the baseline every supported client can decode. A future
// richer rendition is another contract selected by capability, never a platform-named cache.
func CanonicalPreparedRendition(tier Tier) prepared.RenditionContract {
	// Capacity 2 with no active work selects the tier's top rung. Encoder is irrelevant to the
	// returned media identity: software and hardware implementations must produce the same contract.
	profile := Resolve(tier, EncoderSoftware, 2, 0)
	return prepared.RenditionContract{
		VideoCodec:        "h264",
		VideoProfile:      "high",
		VideoLevel:        "4.1",
		PixelFormat:       "yuv420p",
		HDR:               "sdr",
		AudioCodec:        "aac",
		AudioLayout:       "stereo",
		Width:             profile.Width,
		Height:            profile.Height,
		FrameRate:         profile.Framerate,
		VideoBitrateKbps:  profile.VideoBitrate,
		AudioBitrateKbps:  profile.AudioBitrate,
		SegmentDurationMS: preparedSegmentDurationMS,
		PackagingVersion:  prepared.CurrentPackagingVersion,
	}
}
