// Package bgexec is the ONE way filler-owned code runs an external media tool (#1512 G5).
//
// Filler work — transcodes, quality inspection, decoding for review, artwork, transcription — is
// batch work in the same container as the app and the live streams. Production measured an
// uncapped filler ffmpeg at about 350% CPU in a 4-CPU container, slowing every page. Every tool
// started through this package therefore runs at background scheduling priority (see
// proctree.LowPriority) and every ffmpeg additionally gets a worker-thread cap.
//
// A guard test (guard_test.go) fails when a filler-owned package starts a process any other way,
// so a new call site cannot quietly reintroduce the problem.
package bgexec

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/proctree"
)

// Threads is the worker-thread cap for one filler ffmpeg stage (decoder, filter graph, encoder).
//
// Measured on a 4-CPU pin with a real libx264 transcode: uncapped ~393% peak CPU; two threads
// ~196–200% (no margin, and 1080p exceeds it); one thread 134–138% at 720p and 154–162% at 1080p.
const Threads = 1

// Cmd is an exec.Cmd that starts in its own supervised, low-priority process tree. It keeps the
// exec.Cmd surface (pipes, Stdin/Stdout/Stderr, Process) so a call site changes only its
// constructor.
type Cmd struct {
	*exec.Cmd
	ctx context.Context
	sup *proctree.Supervisor
}

// Tool prepares a low-priority command that is stopped, with its whole process tree, when ctx ends.
// Use it for ffprobe, whisper and other non-ffmpeg helpers.
func Tool(ctx context.Context, path string, args ...string) *Cmd {
	return &Cmd{Cmd: exec.Command(path, args...), ctx: ctx} //nolint:gosec // callers pass operator-resolved media tools
}

// FFmpeg is Tool plus a worker-thread cap on the decoder, filter graph and (last) output. An
// argument list that already sets `-threads` (or `-threads:v`) is left alone: the caller chose a
// number, often for a determinism reason.
func FFmpeg(ctx context.Context, path string, args ...string) *Cmd {
	return Tool(ctx, path, CapThreads(args)...)
}

// CapThreads returns ffmpeg arguments with worker threads capped at Threads. `-threads` before the
// first input bounds the decoder, and before the final output the encoder; `-filter_threads` and
// `-filter_complex_threads` bound the filter graph.
func CapThreads(args []string) []string {
	if len(args) == 0 || slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-threads") }) {
		return args
	}
	n := strconv.Itoa(Threads)
	out := make([]string, 0, len(args)+8)
	out = append(out, "-threads", n, "-filter_threads", n, "-filter_complex_threads", n)
	out = append(out, args[:len(args)-1]...)
	return append(out, "-threads", n, args[len(args)-1])
}

// Start starts the command at background priority.
func (c *Cmd) Start() error {
	sup, err := proctree.Start(c.ctx, c.Cmd, proctree.LowPriority())
	if err != nil {
		return err
	}
	c.sup = sup
	return nil
}

// Wait reaps the command. proctree reports a stop it caused (context end) as success, which is
// right for supervisors and wrong here: a killed tool's partial output is not a result, so a
// finished context is surfaced as an error, exactly as exec.CommandContext does.
func (c *Cmd) Wait() error {
	if c.sup == nil {
		return errors.New("bgexec: Wait before Start")
	}
	err := c.sup.Wait()
	if err == nil && c.ctx.Err() != nil && c.sup.Stopped() {
		return c.ctx.Err()
	}
	return err
}

// Run starts the command and waits for it.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Output runs the command and returns its standard output, filling ExitError.Stderr like
// exec.Cmd.Output when Stderr was not set.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("bgexec: Stdout already set")
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	captured := c.Stderr == nil
	if captured {
		c.Stderr = &stderr
	}
	err := c.Run()
	var exit *exec.ExitError
	if captured && errors.As(err, &exit) {
		exit.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// CombinedOutput runs the command and returns stdout and stderr interleaved.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil || c.Stderr != nil {
		return nil, errors.New("bgexec: Stdout or Stderr already set")
	}
	var buf bytes.Buffer
	c.Stdout, c.Stderr = &buf, &buf
	err := c.Run()
	return buf.Bytes(), err
}

// Stopped reports whether the process tree was stopped through context cancellation.
func (c *Cmd) Stopped() bool { return c.sup != nil && c.sup.Stopped() }

// WhisperThreads caps whisper.cpp's `-t`. Transcription of a whole clip on a single thread would
// take minutes, so it gets one more thread than ffmpeg; it still runs niced.
const WhisperThreads = 2

// Whisper is Tool plus a `-t` thread cap, unless the caller already set one.
func Whisper(ctx context.Context, path string, args ...string) *Cmd {
	if !slices.Contains(args, "-t") {
		args = append(slices.Clone(args), "-t", strconv.Itoa(WhisperThreads))
	}
	return Tool(ctx, path, args...)
}
