package fillerstructureassessment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/bgexec"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstructuremedia"
	"github.com/loomarr/loomarr/internal/mediatools"
)

const structureAssessmentToolOutputMaximumBytes = 256 << 10

// FFmpegStructureAssessmentMediaPreparer owns the one expensive rewrite before independent
// assessors run. Its narrow result hides staging, reuse, tool identity, and lineage persistence.
type FFmpegStructureAssessmentMediaPreparer struct {
	sourceRoot string
	mediaRoot  string
	ffmpegPath string
	profile    fillerstructuremedia.Profile
	probe      filler.Prober
	identify   func(context.Context, string) (mediatools.MediaToolIdentity, error)
	run        func(context.Context, string, []string) error
	decode     func(context.Context, string, string) error
	snapshot   func(context.Context, string, string) error
}

// NewFFmpegStructureAssessmentMediaPreparer binds production media preparation to the configured
// ffmpeg toolchain, the retained-source root, a separately owned private derivative root, and the
// one media profile it cuts to.
func NewFFmpegStructureAssessmentMediaPreparer(sourceRoot, mediaRoot, ffmpegPath string, profile fillerstructuremedia.Profile) (*FFmpegStructureAssessmentMediaPreparer, error) {
	if sourceRoot == "" || !filepath.IsAbs(sourceRoot) || filepath.Clean(sourceRoot) != sourceRoot ||
		mediaRoot == "" || !filepath.IsAbs(mediaRoot) || filepath.Clean(mediaRoot) != mediaRoot {
		return nil, errors.New("structure assessment media preparer requires clean absolute source and media roots")
	}
	if !fillerstructuremedia.Supported(profile) {
		return nil, errors.New("structure assessment media preparer requires a supported media profile")
	}
	return &FFmpegStructureAssessmentMediaPreparer{
		sourceRoot: sourceRoot, mediaRoot: mediaRoot, ffmpegPath: ffmpegPath, profile: profile, probe: filler.FFprobeNextTo(ffmpegPath),
		identify: mediatools.IdentifyFFmpeg, run: runStructureAssessmentFFmpeg,
		decode:   runStructureAssessmentDecode,
		snapshot: filler.SnapshotOwnedFile,
	}, nil
}

func validateStructureAssessmentSourceSnapshot(path string, source filler.SplitSourceAsset) error {
	digest, size, err := filler.FileSHA256(path)
	if err != nil || digest != source.SHA256 || size != source.Bytes {
		return errors.New("structure assessment source bytes do not match authority")
	}
	sparse, err := filler.ClipID(path)
	if err != nil || sparse != source.ClipHash {
		return errors.New("structure assessment source sparse identity does not match authority")
	}
	return nil
}

type boundedStructureAssessmentOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	overflow bool
}

func (w *boundedStructureAssessmentOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := structureAssessmentToolOutputMaximumBytes - w.buffer.Len()
	if remaining > 0 {
		_, _ = w.buffer.Write(data[:min(len(data), remaining)])
	}
	if len(data) > remaining {
		w.overflow = true
	}
	return len(data), nil
}

func runStructureAssessmentFFmpeg(ctx context.Context, executable string, arguments []string) error {
	output := &boundedStructureAssessmentOutput{}
	command := bgexec.FFmpeg(ctx, executable, arguments...)
	command.Stdout, command.Stderr = output, output
	err := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if output.overflow {
		return errors.New("ffmpeg output exceeded 256 KiB")
	}
	if err != nil {
		output.mu.Lock()
		message := strings.TrimSpace(output.buffer.String())
		output.mu.Unlock()
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}

func runStructureAssessmentDecode(ctx context.Context, ffmpegPath, path string) error {
	return runStructureAssessmentFFmpeg(ctx, mediatools.FFmpegOr(ffmpegPath), []string{
		"-nostdin", "-hide_banner", "-v", "error", "-i", path,
		"-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-",
	})
}
