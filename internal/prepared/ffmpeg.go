package prepared

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// MediaManifestName is the validated VOD media playlist every prepared packager publishes.
	MediaManifestName = "media.m3u8"
	// CurrentPackagingVersion changes whenever the prepared byte layout or manifest contract makes
	// older publications incompatible. It participates in immutable publication identity.
	CurrentPackagingVersion = 1
)

var ErrUnsupportedRendition = errors.New("prepared: unsupported rendition")

// FFmpegPackager is the real prepared-media driver. It produces finite fMP4 HLS as fast as the
// machine allows; it is control-plane work and deliberately carries no realtime pacing flags.
type FFmpegPackager struct {
	path      string
	videoArgs VideoArgs
}

// VideoPlan separates arguments that ffmpeg requires before its input from filters and encoder
// arguments that belong after it. That positional split is required by hardware device setup.
type VideoPlan struct {
	InputArgs  []string
	OutputArgs []string
}

// VideoArgs returns the ffmpeg plan that implements a rendition. It lets the playout policy supply
// the already-detected host encoder without prepared importing the live playout package (which
// would create an import cycle).
type VideoArgs func(RenditionContract) (VideoPlan, error)

func NewFFmpegPackager(path string, videoArgs ...VideoArgs) *FFmpegPackager {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "ffmpeg"
	}
	args := softwareVideoArgs
	if len(videoArgs) > 0 && videoArgs[0] != nil {
		args = videoArgs[0]
	}
	return &FFmpegPackager{path: path, videoArgs: args}
}

func (p *FFmpegPackager) Package(ctx context.Context, workspace string, source Source, rendition RenditionContract) (Output, error) {
	if p == nil {
		return Output{}, ErrPackagerUnavailable
	}
	args, err := ffmpegPackageArgsWith(workspace, source, rendition, p.videoArgs)
	if err != nil {
		return Output{}, err
	}
	output, err := exec.CommandContext(ctx, p.path, args...).CombinedOutput()
	if err != nil {
		return Output{}, fmt.Errorf("prepared: ffmpeg package: %w: %s", err, commandDiagnostic(output))
	}
	return collectPackagedOutput(workspace)
}

func ffmpegPackageArgs(workspace string, source Source, r RenditionContract) ([]string, error) {
	return ffmpegPackageArgsWith(workspace, source, r, softwareVideoArgs)
}

func ffmpegPackageArgsWith(
	workspace string, source Source, r RenditionContract, videoArgs VideoArgs,
) ([]string, error) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(source.Path) == "" || source.AudioTrack < 0 ||
		r.Width <= 0 || r.Height <= 0 || r.FrameRate <= 0 || r.VideoBitrateKbps <= 0 ||
		r.AudioBitrateKbps <= 0 || r.SegmentDurationMS <= 0 || r.PackagingVersion != CurrentPackagingVersion ||
		videoArgs == nil {
		return nil, ErrUnsupportedRendition
	}
	video, err := videoArgs(r)
	if err != nil || len(video.OutputArgs) == 0 {
		if err == nil {
			err = ErrUnsupportedRendition
		}
		return nil, err
	}
	audioChannels := 2
	switch strings.ToLower(r.AudioLayout) {
	case "", "stereo":
	case "5.1":
		audioChannels = 6
	default:
		return nil, ErrUnsupportedRendition
	}
	if !strings.EqualFold(r.AudioCodec, "aac") {
		return nil, ErrUnsupportedRendition
	}
	segmentSeconds := float64(r.SegmentDurationMS) / 1000
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, video.InputArgs...)
	args = append(args, "-i", source.Path, "-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", source.AudioTrack))
	args = append(args, video.OutputArgs...)
	args = append(args,
		"-c:a", "aac", "-b:a", fmt.Sprintf("%dk", r.AudioBitrateKbps), "-ac", strconv.Itoa(audioChannels),
		"-f", "hls", "-hls_time", fmt.Sprintf("%.3f", segmentSeconds),
		"-hls_playlist_type", "vod", "-hls_flags", "independent_segments",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(workspace, "segment-%06d.m4s"),
		filepath.Join(workspace, MediaManifestName),
	)
	return args, nil
}

func softwareVideoArgs(r RenditionContract) (VideoPlan, error) {
	videoEncoder := ""
	switch strings.ToLower(r.VideoCodec) {
	case "h264":
		videoEncoder = "libx264"
	case "hevc", "h265":
		videoEncoder = "libx265"
	default:
		return VideoPlan{}, ErrUnsupportedRendition
	}
	pixelFormat := strings.ToLower(r.PixelFormat)
	if pixelFormat == "" {
		pixelFormat = "yuv420p"
	}
	if pixelFormat != "yuv420p" && pixelFormat != "yuv420p10le" {
		return VideoPlan{}, ErrUnsupportedRendition
	}
	if r.HDR != "" && !strings.EqualFold(r.HDR, "sdr") {
		return VideoPlan{}, ErrUnsupportedRendition
	}

	segmentSeconds := float64(r.SegmentDurationMS) / 1000
	gop := max(1, r.FrameRate*r.SegmentDurationMS/1000)
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1",
		r.Width, r.Height, r.Width, r.Height,
	)
	args := []string{"-vf", filter, "-c:v", videoEncoder}
	if r.VideoProfile != "" {
		args = append(args, "-profile:v", r.VideoProfile)
	}
	if r.VideoLevel != "" {
		args = append(args, "-level:v", r.VideoLevel)
	}
	args = append(args,
		"-pix_fmt", pixelFormat,
		"-r", strconv.Itoa(r.FrameRate),
		"-b:v", fmt.Sprintf("%dk", r.VideoBitrateKbps),
		"-maxrate", fmt.Sprintf("%dk", r.VideoBitrateKbps*2),
		"-bufsize", fmt.Sprintf("%dk", r.VideoBitrateKbps*2),
		"-g", strconv.Itoa(gop), "-keyint_min", strconv.Itoa(gop),
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%.3f)", segmentSeconds),
	)
	if videoEncoder == "libx265" {
		args = append(args, "-x265-params", fmt.Sprintf("keyint=%d:min-keyint=%d:scenecut=0", gop, gop), "-tag:v", "hvc1")
	} else {
		args = append(args, "-sc_threshold", "0")
	}
	return VideoPlan{OutputArgs: args}, nil
}

func collectPackagedOutput(workspace string) (Output, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return Output{}, fmt.Errorf("prepared: read packaged output: %w", err)
	}
	files := make([]string, 0, len(entries))
	hasManifest, hasInit, segments := false, false, 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		switch {
		case name == MediaManifestName:
			hasManifest = true
		case name == "init.mp4":
			hasInit = true
		case strings.HasPrefix(name, "segment-") && strings.HasSuffix(name, ".m4s"):
			segments++
		default:
			continue
		}
		files = append(files, name)
	}
	if !hasManifest || !hasInit || segments == 0 {
		return Output{}, ErrIncomplete
	}
	slices.Sort(files)
	return Output{Files: files}, nil
}

func commandDiagnostic(output []byte) string {
	const limit = 4096
	if len(output) > limit {
		output = output[len(output)-limit:]
	}
	return strings.TrimSpace(string(output))
}
