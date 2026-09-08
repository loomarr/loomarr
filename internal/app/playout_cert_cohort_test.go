package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestCertificationCodecRolesRequireOwnMeasuredSources(t *testing.T) {
	avcAAC := playout.MediaFormat{VideoCodec: "h264", AudioCodec: "aac"}
	hevcAC3 := playout.MediaFormat{VideoCodec: "hevc", AudioCodec: "ac3"}
	avcEAC3 := playout.MediaFormat{VideoCodec: "h264", AudioCodec: "eac3"}
	for _, tc := range []struct {
		name    string
		roles   []string
		formats [2]playout.MediaFormat
		valid   bool
	}{
		{name: "h264 aac", roles: []string{"transcode_h264", "audio_aac"}, formats: [2]playout.MediaFormat{avcAAC, avcAAC}, valid: true},
		{name: "hevc ac3", roles: []string{"transcode_hevc", "audio_ac3"}, formats: [2]playout.MediaFormat{hevcAC3, hevcAC3}, valid: true},
		{name: "copy eac3", roles: []string{"copy", "audio_eac3"}, formats: [2]playout.MediaFormat{avcEAC3, avcEAC3}, valid: true},
		{name: "mixed source pair", roles: []string{"transcode_h264", "transcode_hevc", "audio_aac", "audio_ac3"}, formats: [2]playout.MediaFormat{avcAAC, hevcAC3}, valid: true},
		{name: "second source supplies eac3", roles: []string{"audio_eac3"}, formats: [2]playout.MediaFormat{avcAAC, avcEAC3}, valid: true},
		{name: "h264 mislabeled hevc", roles: []string{"transcode_hevc"}, formats: [2]playout.MediaFormat{avcAAC, avcEAC3}},
		{name: "hevc mislabeled h264", roles: []string{"transcode_h264"}, formats: [2]playout.MediaFormat{hevcAC3, hevcAC3}},
		{name: "ac3 mislabeled eac3", roles: []string{"audio_eac3"}, formats: [2]playout.MediaFormat{hevcAC3, hevcAC3}},
		{name: "aac mislabeled ac3", roles: []string{"audio_ac3"}, formats: [2]playout.MediaFormat{avcAAC, avcAAC}},
		{name: "unknown is not measured aac", roles: []string{"audio_aac"}},
		{name: "all declared roles required", roles: []string{"audio_aac", "audio_ac3", "audio_eac3"}, formats: [2]playout.MediaFormat{avcAAC, hevcAC3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCertificationCodecRoles(playoutcert.Channel{ID: "private-channel", Roles: tc.roles}, tc.formats)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t error=%v", tc.valid, err)
			}
			if err != nil && err.Error() != "certification source codec does not match declared role" {
				t.Fatalf("unbounded codec error: %v", err)
			}
		})
	}
}

func TestCertificationSourceProfileUsesMeasuredStreams(t *testing.T) {
	observed := playout.SourceObservation{DurationMillis: 12000, Container: "matroska", Streams: []playout.ObservedStream{
		{Kind: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: "25/1", PixelFormat: "yuv420p10le"},
		{Kind: "audio", Codec: "ac3", Channels: 6, SampleRate: 48000, Language: "fr", Title: "private audio title"},
	}}
	format, tracks, err := certificationSourceProfile(observed, 6*time.Second)
	if err != nil || format.VideoCodec != "hevc" || format.AudioCodec != "ac3" || format.AudioChannels != 6 || len(tracks.Audio) != 1 || tracks.Audio[0].Title != "private audio title" {
		t.Fatalf("source profile not retained: %+v %+v %v", format, tracks, err)
	}
	for _, mode := range []string{"missing audio", "duplicate audio", "short", "long", "unknown fps", "nan fps", "missing codec"} {
		t.Run(mode, func(t *testing.T) {
			invalid := observed
			invalid.Streams = append([]playout.ObservedStream(nil), observed.Streams...)
			switch mode {
			case "missing audio":
				invalid.Streams = invalid.Streams[:1]
			case "duplicate audio":
				invalid.Streams = append(invalid.Streams, invalid.Streams[1])
			case "short":
				invalid.DurationMillis = 5999
			case "long":
				invalid.DurationMillis = 90001
			case "unknown fps":
				invalid.Streams[0].FrameRate = "0/0"
			case "nan fps":
				invalid.Streams[0].FrameRate = "NaN"
			case "missing codec":
				invalid.Streams[1].Codec = ""
			}
			if _, _, err := certificationSourceProfile(invalid, 6*time.Second); err == nil {
				t.Fatal("invalid source profile admitted")
			}
		})
	}
}

func TestCertificationResolverKeepsChannelSourcesAndTruthSeparate(t *testing.T) {
	epoch := time.Now().UTC()
	schedule := syntheticProgrammeSchedule{epoch: epoch, duration: 6 * time.Second}
	matching := playout.MediaFormat{VideoCodec: "h264", Width: 320, Height: 180, FrameRate: 25, PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000}
	resolver := syntheticLiveResolver{sources: map[string][2]string{"a": {"black", "white"}, "b": {"white", "black"}}, formats: map[string]playout.MediaFormat{"black": matching, "white": {VideoCodec: "hevc", AudioCodec: "ac3"}}, schedule: schedule, now: func() time.Time { return epoch.Add(time.Second) }}
	for id, want := range map[string]string{"a": "black", "b": "white"} {
		_, source, err := resolver.AiringNow(context.Background(), id)
		if err != nil || source != want {
			t.Fatalf("channel %s selected %q", id, source)
		}
	}
	if _, _, err := resolver.AiringNow(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown channel inherited source")
	}
	plan, format := resolver.PlanFor(context.Background(), "black", playout.PlanBaseline)
	if !plan.CopyVideo || !plan.CopyAudio || format != matching {
		t.Fatal("measured compatible input not offered for normal copy conformance")
	}
	plan, format = resolver.PlanFor(context.Background(), "white", playout.PlanBaseline)
	if plan.CopyVideo || format.VideoCodec != "hevc" || format.AudioCodec != "ac3" {
		t.Fatal("incompatible measured input not retained for transcode")
	}
	signatures := defaultCertificationSignatures()
	source := syntheticProgrammeEvidence{schedule: schedule, prepared: map[string]bool{"a": false, "b": false}, signatures: map[string][2]playoutcert.ProgrammeSignature{"a": signatures, "b": {signatures[1], signatures[0]}}}
	for id, want := range map[string]float64{"a": 25, "b": 255} {
		evidence, err := source.Freeze(id, epoch, epoch.Add(8*time.Second))
		if err != nil || evidence.Programmes[0].Luma.Max != want {
			t.Fatal("channel inherited another programme's expected signals")
		}
	}
	delete(source.signatures, "b")
	if _, err := source.Freeze("b", epoch, epoch.Add(8*time.Second)); err == nil {
		t.Fatal("missing operator truth inherited generated expectations")
	}
}
