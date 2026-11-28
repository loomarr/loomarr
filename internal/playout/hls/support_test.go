package hls

import "github.com/loomarr/loomarr/internal/playout/media"

// testSources are content classes as Loomarr's inventory measures them.
func testSources() map[string]media.MediaFormat {
	return map[string]media.MediaFormat{
		"h264-1080p-sdr-25": {VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: 25, PixelFormat: "yuv420p",
			AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm"},
		"hevc-4k-hdr-dv": {VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24000.0 / 1001, PixelFormat: "yuv420p10le",
			ColorTransfer: "smpte2084", AudioCodec: "truehd", AudioChannels: 8, AudioSampleRate: 48000, Container: "matroska,webm"},
	}
}
