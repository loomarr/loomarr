package prepared

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// verifyRandomAccess streams packet metadata from the finished local publication. It neither
// decodes the video nor retains a per-frame index, and cannot publish an unobserved encoder plan.
func (p *FFmpegPackager) verifyRandomAccess(ctx context.Context, workspace string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"-v", "error", "-select_streams", "v:0", "-show_packets", "-show_entries",
		"packet=pts_time,duration_time,flags", "-of", "compact=p=0", filepath.Join(workspace, MediaManifestName)}
	bin := p.probePath()
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // fixed arguments and local publication
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("prepared: verify random access: %w", err)
	}
	run := p.diagnostics.Begin(diagnostics.ProcessSpec{
		Purpose: "prepared_verify", Target: "video_random_access", Executable: bin, Args: args,
	})
	if err = cmd.Start(); err != nil {
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return fmt.Errorf("prepared: verify random access: %w", err)
	}
	observationErr := validateRandomAccess(stdout)
	if observationErr != nil {
		cancel()
	}
	err = cmd.Wait()
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: errors.Join(err, observationErr), Cancelled: ctx.Err() != nil,
			TerminationReason: cancellationReason(ctx)})
	}
	if err != nil || observationErr != nil {
		return fmt.Errorf("prepared: verify random access: %w", errors.Join(err, observationErr))
	}
	return nil
}

func (p *FFmpegPackager) probePath() string {
	if p.path == "ffmpeg" {
		return "ffprobe"
	}
	dir, base := filepath.Split(p.path)
	return filepath.Join(dir, strings.Replace(base, "ffmpeg", "ffprobe", 1))
}

func validateRandomAccess(input io.Reader) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 4096)
	var previous, access time.Duration
	packets := 0
	// ffprobe prints decimal seconds to microsecond precision; permit only that rounding error.
	const bound = MaxRandomAccessInterval + time.Microsecond
	for scanner.Scan() {
		packets++
		if packets > 10_000_000 {
			return errors.New("video random-access observation exceeds packet limit")
		}
		pts, duration, key, err := randomAccessPacket(scanner.Text())
		if err != nil {
			return err
		}
		if (packets == 1 && !key) || (packets > 1 && pts <= previous) {
			return errors.New("video random-access observation lacks initial access or increasing timestamps")
		}
		if packets > 1 && pts-access > bound {
			return errors.New("video random-access interval exceeds 200ms")
		}
		if key {
			access = pts
		}
		// This also covers the last partial GOP: EOF cannot leave an unbounded video tail.
		if duration > bound || pts-access > bound-duration {
			return errors.New("video random-access tail exceeds 200ms")
		}
		previous = pts
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("video random-access observation: %w", err)
	}
	if packets == 0 {
		return errors.New("missing video random-access observation")
	}
	return nil
}

func randomAccessPacket(line string) (time.Duration, time.Duration, bool, error) {
	var pts, duration time.Duration
	var havePTS, haveDuration, haveFlags, key bool
	for _, field := range strings.Split(line, "|") {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			return 0, 0, false, errors.New("invalid video random-access packet")
		}
		switch name {
		case "pts_time", "duration_time":
			parsed, err := time.ParseDuration(value + "s")
			if err != nil || parsed < 0 {
				return 0, 0, false, errors.New("invalid video random-access timestamp")
			}
			if name == "pts_time" && !havePTS {
				pts, havePTS = parsed, true
			} else if name == "duration_time" && !haveDuration {
				duration, haveDuration = parsed, true
			} else {
				return 0, 0, false, errors.New("duplicate video random-access timestamp")
			}
		case "flags":
			if haveFlags || value == "" {
				return 0, 0, false, errors.New("invalid video random-access flags")
			}
			haveFlags, key = true, strings.Contains(value, "K")
		default:
			return 0, 0, false, errors.New("unexpected video random-access packet field")
		}
	}
	if !havePTS || !haveDuration || !haveFlags || duration == 0 {
		return 0, 0, false, errors.New("incomplete video random-access packet")
	}
	return pts, duration, key, nil
}
