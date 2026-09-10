package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

type certificationSources struct {
	paths      map[string][2]string
	signatures map[string][2]playoutcert.ProgrammeSignature
	formats    map[string]playout.MediaFormat
	tracks     map[string]playout.MediaTracks
	private    []string
}

func defaultCertificationSignatures() [2]playoutcert.ProgrammeSignature {
	return [2]playoutcert.ProgrammeSignature{
		{Luma: playoutcert.SignalRange{Min: 0, Max: 25}, ZeroCrossingRate: playoutcert.SignalRange{Min: 0.012, Max: 0.026}, RMSDB: playoutcert.SignalRange{Min: -80, Max: -1}},
		{Luma: playoutcert.SignalRange{Min: 225, Max: 255}, ZeroCrossingRate: playoutcert.SignalRange{Min: 0.027, Max: 0.050}, RMSDB: playoutcert.SignalRange{Min: -80, Max: -1}},
	}
}

func prepareCertificationSources(ctx context.Context, config PlayoutCertificationConfig, root, ffmpeg string) (certificationSources, error) {
	result := certificationSources{paths: make(map[string][2]string), signatures: make(map[string][2]playoutcert.ProgrammeSignature), formats: make(map[string]playout.MediaFormat), tracks: make(map[string]playout.MediaTracks)}
	if config.Cohort == nil {
		return prepareGeneratedCertificationSources(ctx, config, root, ffmpeg, result)
	}
	if !config.Cohort.MatchesChannels(config.Channels) {
		return certificationSources{}, errors.New("operator cohort channel coverage changed")
	}
	staged, err := config.Cohort.Stage(ctx, root)
	if err != nil {
		return certificationSources{}, err
	}
	if len(staged.Channels) != len(config.Channels) {
		return certificationSources{}, errors.New("operator cohort channel coverage changed")
	}
	for _, channel := range config.Channels {
		if _, ok := staged.Channels[channel.ID]; !ok {
			return certificationSources{}, errors.New("operator cohort channel coverage changed")
		}
	}
	result.private = append(config.Cohort.PrivateValues(), staged.Directory)
	probe := playout.FFprobeSourceNextTo(ffmpeg)
	for _, channel := range config.Channels {
		var paths [2]string
		var signatures [2]playoutcert.ProgrammeSignature
		for variant, media := range staged.Channels[channel.ID] {
			paths[variant], signatures[variant] = media.Path, media.Signature
			if _, ok := result.formats[media.Path]; ok {
				continue
			}
			observed, err := probe(ctx, media.Path)
			if err != nil {
				return certificationSources{}, errors.New("operator source profile unavailable")
			}
			format, tracks, err := certificationSourceProfile(observed, config.ProgrammeDuration)
			if err != nil {
				return certificationSources{}, err
			}
			result.formats[media.Path], result.tracks[media.Path] = format, tracks
			result.private = append(result.private, media.Path)
			for _, stream := range observed.Streams {
				if stream.Title != "" {
					result.private = append(result.private, stream.Title)
				}
			}
		}
		result.paths[channel.ID], result.signatures[channel.ID] = paths, signatures
		if err := validateCertificationCodecRoles(channel, [2]playout.MediaFormat{result.formats[paths[0]], result.formats[paths[1]]}); err != nil {
			return certificationSources{}, err
		}
	}
	return result, nil
}

func prepareGeneratedCertificationSources(ctx context.Context, config PlayoutCertificationConfig, root, ffmpeg string, result certificationSources) (certificationSources, error) {
	var ordinary, copyPaths [2]string
	var ordinaryFormats [2]playout.MediaFormat
	probe := playout.FFprobeSourceNextTo(ffmpeg)
	var duration time.Duration
	if config.QualityTier != "" {
		duration = config.ProgrammeDuration
	}
	for variant := range ordinary {
		ordinary[variant] = filepath.Join(root, fmt.Sprintf("source-%d.mp4", variant))
		if err := generateSyntheticSource(ctx, ffmpeg, ordinary[variant], variant, syntheticSourceProfile{keyframeInterval: config.sourceProfile().Framerate, audioChannels: 1, video: config.sourceProfile(), duration: duration}); err != nil {
			return certificationSources{}, err
		}
		observed, err := probe(ctx, ordinary[variant])
		if err != nil {
			return certificationSources{}, errors.New("generated source profile unavailable")
		}
		format, _, err := certificationSourceProfile(observed, config.ProgrammeDuration)
		if err != nil {
			return certificationSources{}, err
		}
		// These measured facts validate declarations. They must not opt the
		// deliberate cold workload into the resolver's copy-planning map.
		ordinaryFormats[variant] = format
	}
	result.private = append(result.private, ordinary[:]...)
	for _, channel := range config.Channels {
		paths := ordinary
		formats := ordinaryFormats
		if slices.Contains(channel.Roles, "copy") {
			if copyPaths[0] == "" {
				for variant := range copyPaths {
					copyPaths[variant] = filepath.Join(root, fmt.Sprintf("copy-source-%d.mp4", variant))
					if err := generateSyntheticSource(ctx, ffmpeg, copyPaths[variant], variant, syntheticSourceProfile{keyframeInterval: 1, audioChannels: 2, video: config.sourceProfile(), duration: duration}); err != nil {
						return certificationSources{}, err
					}
					observed, err := probe(ctx, copyPaths[variant])
					if err != nil {
						return certificationSources{}, errors.New("generated copy source profile unavailable")
					}
					format, tracks, err := certificationSourceProfile(observed, config.ProgrammeDuration)
					if err != nil {
						return certificationSources{}, err
					}
					result.formats[copyPaths[variant]], result.tracks[copyPaths[variant]] = format, tracks
				}
				result.private = append(result.private, copyPaths[:]...)
			}
			paths = copyPaths
			formats = [2]playout.MediaFormat{result.formats[paths[0]], result.formats[paths[1]]}
		}
		if err := validateCertificationCodecRoles(channel, formats); err != nil {
			return certificationSources{}, err
		}
		result.paths[channel.ID] = paths
		result.signatures[channel.ID] = defaultCertificationSignatures()
	}
	return result, nil
}

func validateCertificationCodecRoles(channel playoutcert.Channel, formats [2]playout.MediaFormat) error {
	for _, role := range channel.Roles {
		var video, audio string
		switch role {
		case "transcode_h264":
			video = "h264"
		case "transcode_hevc":
			video = "hevc"
		case "audio_aac":
			audio = "aac"
		case "audio_ac3":
			audio = "ac3"
		case "audio_eac3":
			audio = "eac3"
		default:
			continue
		}
		if !slices.ContainsFunc(formats[:], func(format playout.MediaFormat) bool {
			return (video != "" && format.VideoCodec == video) || (audio != "" && format.AudioCodec == audio)
		}) {
			return errors.New("certification source codec does not match declared role")
		}
	}
	return nil
}

func certificationSourceProfile(observed playout.SourceObservation, programmeDuration time.Duration) (playout.MediaFormat, playout.MediaTracks, error) {
	format := playoutFormatOf(inventoryFactsOf(observed))
	var video, audio int
	var tracks playout.MediaTracks
	for _, stream := range observed.Streams {
		switch stream.Kind {
		case "video":
			video++
		case "audio":
			tracks.Audio = append(tracks.Audio, playout.Track{Index: audio, Language: stream.Language, Title: stream.Title})
			audio++
		case "subtitle":
			tracks.Subtitles = append(tracks.Subtitles, playout.Track{Index: len(tracks.Subtitles), Language: stream.Language, Title: stream.Title})
		}
	}
	if video != 1 || audio != 1 || format.VideoCodec == "" || format.AudioCodec == "" || format.Width <= 0 || format.Height <= 0 || format.FrameRate <= 0 || math.IsNaN(format.FrameRate) || math.IsInf(format.FrameRate, 0) || format.AudioChannels <= 0 || format.AudioSampleRate <= 0 || observed.DurationMillis < programmeDuration.Milliseconds() || observed.DurationMillis > 90_000 {
		return playout.MediaFormat{}, playout.MediaTracks{}, errors.New("operator source profile invalid")
	}
	return format, tracks, nil
}
