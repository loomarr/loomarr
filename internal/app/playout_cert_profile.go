package app

import (
	"context"
	"encoding/hex"
	"errors"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/prepared"
)

func certificationBuildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key != "vcs.revision" {
			continue
		}
		decoded, err := hex.DecodeString(setting.Value)
		if err == nil && len(decoded) == 20 && setting.Value == strings.ToLower(setting.Value) {
			return setting.Value
		}
	}
	return ""
}

// A tier is an explicit request for measured, production-profile qualification.
// The zero value retains the small deterministic software workload used by tests.
func (c PlayoutCertificationConfig) validateTier() error {
	if c.QualityTier != "" && !slices.Contains([]playout.Tier{
		playout.TierEfficient, playout.TierBalanced, playout.TierQuality,
	}, c.QualityTier) {
		return errors.New("invalid certification quality tier")
	}
	return nil
}

func certificationFixtureProfile() playout.Profile {
	return playout.Profile{Width: 320, Height: 180, Framerate: 25,
		VideoBitrate: 600, AudioBitrate: 96, Encoder: playout.EncoderSoftware}
}

func (c PlayoutCertificationConfig) rendition() prepared.RenditionContract {
	if c.QualityTier != "" {
		return playout.CanonicalPreparedRendition(c.QualityTier)
	}
	return prepared.RenditionContract{
		VideoCodec: "h264", VideoProfile: "high", VideoLevel: "4.1", PixelFormat: "yuv420p", HDR: "sdr",
		AudioCodec: "aac", AudioLayout: "stereo", Width: 320, Height: 180, FrameRate: 25,
		VideoBitrateKbps: 600, AudioBitrateKbps: 96, SegmentDurationMS: 1000, PackagingVersion: prepared.CurrentPackagingVersion,
	}
}

func (c PlayoutCertificationConfig) sourceProfile() playout.Profile {
	if c.QualityTier == "" {
		return certificationFixtureProfile()
	}
	return playout.Resolve(c.QualityTier, playout.EncoderSoftware, 2, 0)
}

func (t *PlayoutCertificationTarget) measureProfile(ctx context.Context, config *PlayoutCertificationConfig, ffmpeg string) error {
	if config.QualityTier == "" {
		return nil
	}
	probe := config.sourceProfile()
	capacity := playout.DetectObserved(ctx, ffmpeg, probe, "", t.diagnostics)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if capacity.MaxChannels < 1 || capacity.MaxChannels > 64 || !slices.ContainsFunc(capacity.All, func(c playout.Capability) bool {
		return c.Encoder == capacity.Chosen && c.Works && c.Speed > 0
	}) {
		return errors.New("declared profile has no measured encoder capacity")
	}
	config.Capacity = capacity.MaxChannels
	t.encoder = capacity.Chosen
	t.profileEvidence = certificationProfileEvidence(*config, probe, capacity)
	return nil
}

func certificationProfileEvidence(config PlayoutCertificationConfig, probe playout.Profile, capacity playout.Capacity) *playoutcert.ProfileEvidence {
	rendition := config.rendition()
	return &playoutcert.ProfileEvidence{
		QualityTier: string(config.QualityTier), Encoder: string(capacity.Chosen), MeasuredCapacity: capacity.MaxChannels,
		Probe: profileDimensions(probe),
		Prepared: playoutcert.ProfileDimensions{Width: rendition.Width, Height: rendition.Height, FrameRate: rendition.FrameRate,
			VideoBitrateKbps: rendition.VideoBitrateKbps, AudioBitrateKbps: rendition.AudioBitrateKbps},
	}
}

func profileDimensions(p playout.Profile) playoutcert.ProfileDimensions {
	return playoutcert.ProfileDimensions{Width: p.Width, Height: p.Height, FrameRate: p.Framerate,
		VideoBitrateKbps: p.VideoBitrate, AudioBitrateKbps: p.AudioBitrate}
}

// ProfileEvidence returns a value copy; the report cannot alias mutable target state.
func (t *PlayoutCertificationTarget) ProfileEvidence() *playoutcert.ProfileEvidence {
	if t.profileEvidence == nil {
		return nil
	}
	evidence := *t.profileEvidence
	return &evidence
}
