package playout

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ffmpeg process supervision for playout (§9.1).
//
// A playout encoder differs from a VOD one in a way that dominates the design: IT NEVER
// EXITS. A VOD transcode ends at EOF, so leaking one is self-correcting. A live encoder
// leaked is a core burned until the process dies — which is why viewra's "no
// client-disconnect detection, sweep on a timer" posture (prior-art, viewra §4) is not
// survivable here.
//
// So three things are non-negotiable:
//
//  1. Progress on a DEDICATED fd, structured. Not stderr scraping.
//  2. Kill the process GROUP, not the process. ffmpeg spawns children; viewra's watchdog
//     killed only the parent while start used Setpgid, so children orphaned.
//  3. The context is the lifetime. When it is cancelled the process dies — no timer, no
//     idle sweep, no last-accessed timestamp.

// progressFD is the fd ffmpeg writes `-progress` key=value lines to.
//
// 3 rather than stdout because STDOUT CARRIES THE MPEG-TS — mixing progress text into the
// transport stream would corrupt it. fd 3 is the first free descriptor after the standard
// three, and it exists only because Cmd.ExtraFiles wires it: `-progress pipe:3` with nothing
// on fd 3 fails at startup with "Error parsing global options: Bad file descriptor", a
// message that never mentions progress. (Found by executing it; the arg-shape test asserted
// "progress goes to a pipe" and passed happily on an fd that was not there.)
const progressFD = 3

// progressPipeArg is the `-progress` value matching the fd Start wires. Args and supervisor
// read the SAME constant on purpose: when they were independent literals they drifted, and
// the failure was ffmpeg refusing to start with a message that never mentions progress.
func progressPipeArg() string { return "pipe:" + strconv.Itoa(progressFD) }

// Progress is one sample of ffmpeg's structured progress output.
type Progress struct {
	// Frame is the encoded frame count; monotonic while healthy.
	Frame int64
	// Speed is the realtime multiple. Below ~1.0 sustained means the encoder cannot keep
	// up and the channel will stutter — the single most useful health signal there is.
	Speed float64
	// OutTimeMS is how much output has been produced, in milliseconds.
	OutTimeMS int64
}

// Process is a supervised ffmpeg. Its Stdout is the caller's to read — for playout that is
// the MPEG-TS the session fans out to viewers.
type Process struct {
	cmd    *exec.Cmd
	Stdout io.ReadCloser

	log *slog.Logger

	mu       sync.Mutex
	lastErr  string
	waitOnce sync.Once
	waitErr  error
}

// Start launches ffmpeg with the given args under ctx.
//
// The process is put in its own process GROUP so teardown can signal the whole tree. ffmpeg
// spawns helpers, and signalling only the parent leaves them running — the exact bug
// viewra has, where start uses Setpgid but the watchdog calls Process.Kill().
func Start(ctx context.Context, bin string, args []string, log *slog.Logger, onProgress func(Progress)) (*Process, error) {
	cmd := exec.Command(bin, args...) //nolint:gosec // args are built by this package, never user text
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Progress on fd 3. Created here because ffmpeg cannot open it itself — see progressFD.
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("progress pipe: %w", err)
	}
	cmd.ExtraFiles = []*os.File{pw} // becomes fd 3 in the child

	// stderr is kept for the LAST error only. ffmpeg is chatty and a live process runs for
	// days; retaining all of it is a slow memory leak, and the useful part when something
	// breaks is the final line.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	p := &Process{cmd: cmd, Stdout: stdout, log: log}

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	// The child holds its own copy of the write end; ours must go or the reader below
	// never sees EOF when ffmpeg exits.
	_ = pw.Close()

	go p.readProgress(pr, onProgress)
	go p.readStderr(stderr)

	// THE lifetime binding. Cancellation kills the group rather than waiting for a sweep:
	// a live encoder never exits on its own, so nothing else would ever stop it.
	go func() {
		<-ctx.Done()
		p.Stop()
	}()

	return p, nil
}

// readProgress parses ffmpeg's key=value progress stream.
//
// Structured, line-oriented, on its own fd. viewra scraped stderr in 4096-byte reads looking
// for "frame=" substrings, and a chunked read can split a token across the buffer boundary —
// a bufio.Scanner over a dedicated pipe cannot.
func (p *Process) readProgress(r io.ReadCloser, onProgress func(Progress)) {
	ReadProgress(r, onProgress)
}

// ReadProgress parses ffmpeg's `-progress` stream and calls onProgress once per block.
//
// ⚠ **Exported so the filler transcode stage shares this parser rather than growing a second
// copy** (§10 V51b). It is the same fd, the same key set and the same block-boundary rule, and the
// two would drift the moment ffmpeg changed a field name — `out_time_ms` already reports
// MICROSECONDS despite its name, which is exactly the sort of fact that gets fixed in one copy.
// The behaviour here is byte-for-byte what the method did before the extraction.
//
// It takes ownership of r and closes it. A nil onProgress still DRAINS the pipe: a full pipe
// blocks ffmpeg's writes and stalls the encode.
func ReadProgress(r io.ReadCloser, onProgress func(Progress)) {
	defer func() { _ = r.Close() }()
	if onProgress == nil {
		// Still drain it: a full pipe blocks ffmpeg's writes and stalls the encode.
		_, _ = io.Copy(io.Discard, r)
		return
	}
	var cur Progress
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "frame":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				cur.Frame = n
			}
		case "out_time_ms":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				cur.OutTimeMS = n / 1000 // ffmpeg reports microseconds despite the name
			}
		case "speed":
			if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "x"), 64); err == nil {
				cur.Speed = f
			}
		case "progress":
			// ffmpeg terminates each block with progress=continue|end. Emit on the
			// boundary so a consumer sees whole samples, never half-updated ones.
			onProgress(cur)
		}
	}
}

// readStderr keeps only the most recent non-empty line.
func (p *Process) readStderr(r io.ReadCloser) {
	defer func() { _ = r.Close() }()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p.mu.Lock()
		p.lastErr = line
		p.mu.Unlock()
		if p.log != nil {
			// Debug, not warn: ffmpeg writes routine notices to stderr, and logging them
			// as problems trains an operator to ignore the log. viewra needed an explicit
			// non-fatal allowlist for exactly this.
			p.log.Debug("ffmpeg", "line", line)
		}
	}
}

// LastError returns ffmpeg's most recent stderr line, for surfacing why a channel stopped.
func (p *Process) LastError() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

// stopGrace is how long ffmpeg gets to flush and exit after SIGTERM before SIGKILL. Short:
// there is nothing to flush that matters for live playout, and a channel that will not die
// blocks the slot its replacement needs.
const stopGrace = 2 * time.Second

// Stop terminates the process GROUP and waits for it.
//
// Group, not process: ffmpeg spawns children and signalling only the parent orphans them —
// on a 24/7 box that accumulates until something notices. The negative pid is the group.
//
// The wait is synchronous and guarded by sync.Once: the monitoring goroutine may already
// have reaped the child ("no child processes" otherwise), and callers need Stop to mean
// "it is gone" so a replacement can take the slot.
func (p *Process) Stop() {
	if p.cmd.Process == nil {
		return
	}
	pgid := p.cmd.Process.Pid

	// SIGTERM the group, then escalate. Errors are ignored throughout: every one of them
	// means "already gone", which is the desired state.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		p.wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(stopGrace):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
	}
}

// Wait blocks until the process exits, returning its error. Safe to call concurrently with
// Stop and more than once.
func (p *Process) Wait() error {
	p.wait()
	return p.waitErr
}

func (p *Process) wait() {
	p.waitOnce.Do(func() {
		err := p.cmd.Wait()
		// A process we killed reports a signal, which is not a failure — it is the
		// teardown we asked for.
		if err != nil && isSignalExit(err) {
			err = nil
		}
		p.waitErr = err
	})
}

func isSignalExit(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	st, ok := ee.Sys().(syscall.WaitStatus)
	return ok && st.Signaled()
}
