package prepared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// verifyVideoReordering checks the encoded contract rather than assuming every encoder honors
// -bf. It inspects only the newly packaged local publication, never the original source.
func (p *FFmpegPackager) verifyVideoReordering(ctx context.Context, workspace string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	bin := "ffprobe"
	if p.path != "ffmpeg" {
		dir, base := filepath.Split(p.path)
		bin = filepath.Join(dir, strings.Replace(base, "ffmpeg", "ffprobe", 1))
	}
	args := []string{"-v", "error", "-select_streams", "v", "-show_entries",
		"stream=codec_type,has_b_frames", "-of", "json", filepath.Join(workspace, MediaManifestName)}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // local output and fixed probe arguments
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("prepared: verify video reordering: %w", err)
	}
	run := p.diagnostics.Begin(diagnostics.ProcessSpec{
		Purpose: "prepared_verify", Target: "video_reordering", Executable: bin, Args: args,
	})
	if err = cmd.Start(); err != nil {
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return fmt.Errorf("prepared: verify video reordering: %w", err)
	}
	const maxOutput = 64 << 10
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxOutput+1))
	if readErr != nil || len(output) > maxOutput {
		cancel()
	}
	err = cmd.Wait()
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil, TerminationReason: cancellationReason(ctx)})
	}
	if err != nil || readErr != nil {
		return fmt.Errorf("prepared: verify video reordering: %w", errors.Join(err, readErr))
	}
	if len(output) > maxOutput {
		return errors.New("prepared: video reordering probe exceeded output limit")
	}
	return validateVideoReordering(output)
}

func validateVideoReordering(output []byte) error {
	var result struct {
		Streams []struct {
			Kind       string `json:"codec_type"`
			Reordering *int   `json:"has_b_frames"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return errors.New("prepared: invalid video reordering observation")
	}
	if len(result.Streams) != 1 || result.Streams[0].Kind != "video" ||
		result.Streams[0].Reordering == nil || *result.Streams[0].Reordering != 0 {
		return errors.New("prepared: output does not establish zero video reordering")
	}
	return nil
}
