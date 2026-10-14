package playoutbench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

// startSegment clocks the production packager publishing a complete first second, including its
// parsing, trimming, timestamping and disk write. Encoder cleanup is awaited after taking the clock.
func (r *run) startSegment(ctx context.Context, path string, seek time.Duration, pipe playout.Pipeline, fps int) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp(r.Dir, "start-segment-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	var opened bool
	var encoder *startEncoder
	schedule := func(ctx context.Context, _ time.Time) (packager.Item, error) {
		if opened {
			<-ctx.Done()
			return packager.Item{}, ctx.Err()
		}
		opened = true
		return packager.Item{Label: "bench-start", Duration: time.Second, Open: func(ctx context.Context, slot packager.Slot) (io.ReadCloser, error) {
			started, err := openStartEncoder(ctx, r.FFmpeg, pipe.FragmentArgs(path, seek, slot.Offset, slot.Frames, slot.AudioFrames, fps, 0))
			if err != nil {
				return nil, err
			}
			encoder = started
			return started, nil
		}}, nil
	}
	p, err := packager.New(packager.Config{FPS: fps, Dir: dir, FirstItemWait: 3 * time.Minute}, schedule,
		func(context.Context) (*packager.Slate, error) {
			return nil, errors.New("bench: first item did not produce media; slate is not a startup")
		})
	if err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- p.Run(ctx) }()
	segment, err := p.WaitSegment(ctx, 0)
	elapsed := time.Since(start)
	cancel()
	<-done // cancellation closes and reaps the child before scratch is removed
	if err != nil {
		if encoder != nil {
			return 0, fmt.Errorf("first segment: %w: %s", err, tail(encoder.stderr.String()))
		}
		return 0, fmt.Errorf("first segment: %w", err)
	}
	if err := completeStartSegment(segment, fps); err != nil {
		return 0, err
	}
	return elapsed, nil
}

// A short or video-only fragment cannot satisfy the one-second startup contract.
func completeStartSegment(segment []byte, fps int) error {
	var parts fmp4.Parts
	if err := parts.Unmarshal(segment); err != nil {
		return fmt.Errorf("first segment: %w", err)
	}
	var videoFrames, videoTicks, audioFrames int64
	for _, part := range parts {
		for _, track := range part.Tracks {
			for _, sample := range track.Samples {
				switch track.ID {
				case 1:
					videoFrames++
					videoTicks += int64(sample.Duration)
				case 2:
					audioFrames++
				}
			}
		}
	}
	if videoFrames != int64(fps) || videoTicks != 90000 || audioFrames < (48000+512)/1024 {
		return fmt.Errorf("first segment: incomplete second (%d video frames, %d ticks, %d audio frames)", videoFrames, videoTicks, audioFrames)
	}
	return nil
}

type startEncoder struct {
	io.ReadCloser
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stderr bytes.Buffer
	once   sync.Once
	err    error
}

func openStartEncoder(ctx context.Context, ffmpeg string, args []string) (*startEncoder, error) {
	ctx, cancel := context.WithCancel(ctx)
	output := &startEncoder{cancel: cancel}
	output.cmd = exec.CommandContext(ctx, ffmpeg, args...)
	output.cmd.WaitDelay = 2 * time.Second
	output.cmd.Stderr = &output.stderr
	stdout, err := output.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output.ReadCloser = stdout
	if err := output.cmd.Start(); err != nil {
		cancel()
		_ = stdout.Close()
		return nil, err
	}
	return output, nil
}

func (e *startEncoder) Close() error {
	e.once.Do(func() {
		e.cancel()
		_ = e.ReadCloser.Close()
		e.err = e.cmd.Wait()
	})
	return e.err
}
