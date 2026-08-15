package playout

import (
	"strings"

	"github.com/mantonx/loomarr/internal/prepared"
)

// PreparedVideoArgs adapts the live playout encoder policy to finite prepared packaging. It reuses
// the same device setup, hardware decode/upload, scale, preset, rate-control, and GOP builders, so
// preparation cannot become a second hardware-driver table.
func PreparedVideoArgs(encoder Encoder, r prepared.RenditionContract) (prepared.VideoPlan, error) {
	if !strings.EqualFold(r.VideoCodec, "h264") ||
		(r.HDR != "" && !strings.EqualFold(r.HDR, "sdr")) ||
		!strings.EqualFold(r.PixelFormat, "yuv420p") ||
		r.SegmentDurationMS != preparedSegmentDurationMS {
		return prepared.VideoPlan{}, prepared.ErrUnsupportedRendition
	}
	if encoder == "" || encoder == EncoderSoftware {
		return prepared.VideoPlan{}, prepared.ErrUnsupportedRendition
	}
	profile := Profile{
		Width: r.Width, Height: r.Height, Framerate: r.FrameRate,
		VideoBitrate: r.VideoBitrateKbps, AudioBitrate: r.AudioBitrateKbps,
		Encoder: encoder,
	}
	args := profile.scaleFilterArgs("")
	args = append(args, profile.videoEncodeArgs()...)
	if r.VideoProfile != "" {
		args = append(args, "-profile:v", r.VideoProfile)
	}
	if r.VideoLevel != "" {
		args = append(args, "-level:v", r.VideoLevel)
	}
	input := append(deviceInitArgs(encoder), hardwareDecodeArgs(encoder)...)
	return prepared.VideoPlan{InputArgs: input, OutputArgs: args}, nil
}
