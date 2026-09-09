package playoutcert

import "errors"

// ProfileEvidence binds a measured target to the existing production quality
// policy. Admission capacity is a probe result, not a concurrent-load verdict.
type ProfileEvidence struct {
	QualityTier      string            `json:"qualityTier"`
	Encoder          string            `json:"encoder"`
	MeasuredCapacity int               `json:"measuredCapacity"`
	Probe            ProfileDimensions `json:"probe"`
	Prepared         ProfileDimensions `json:"prepared"`
}

type ProfileDimensions struct {
	Width            int `json:"width"`
	Height           int `json:"height"`
	FrameRate        int `json:"frameRate"`
	VideoBitrateKbps int `json:"videoBitrateKbps"`
	AudioBitrateKbps int `json:"audioBitrateKbps"`
}

func (p *ProfileEvidence) validate() error {
	if p == nil {
		return nil
	}
	if !oneOf(p.QualityTier, "efficient", "balanced", "quality") ||
		!oneOf(p.Encoder, "libx264", "h264_nvenc", "h264_qsv", "h264_vaapi", "h264_amf", "h264_videotoolbox", "h264_rkmpp", "h264_v4l2m2m", "h264_vulkan") ||
		p.MeasuredCapacity < 1 || p.MeasuredCapacity > 64 {
		return errors.New("invalid declared target profile")
	}
	for _, d := range []ProfileDimensions{p.Probe, p.Prepared} {
		if d.Width < 2 || d.Width > 7680 || d.Width%2 != 0 || d.Height < 2 || d.Height > 4320 || d.Height%2 != 0 ||
			d.FrameRate < 1 || d.FrameRate > 120 || d.VideoBitrateKbps < 1 || d.VideoBitrateKbps > 100000 ||
			d.AudioBitrateKbps < 1 || d.AudioBitrateKbps > 1024 {
			return errors.New("invalid declared target dimensions")
		}
	}
	return nil
}
