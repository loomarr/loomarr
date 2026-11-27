package playout

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
)

const audioRateHz = 48000

// startFragmentEncoder runs one item's ffmpeg and returns its stdout. Close (or cancelling ctx)
// stops it; a failure that was not a stop is logged with ffmpeg's last words. failed, when set, hears an encoder that exited
// on its own with an error before any output: a fault of this source on this host, not a slow
// start (a late item's context is cancelled first) and not an item the packager finished.
//
// watch, when set, is fed the encoder's -progress, one SpeedSample per block with the process's
// CPU time (RungMonitor, #1517). When it returns true the stream ends cleanly at the next top-level
// box, so the packager takes the fragments produced so far and none of what the killed encoder had
// in flight (#1533's contract: no flush reaches the channel); the schedule then resumes the item on
// the new rung.
func startFragmentEncoder(ctx context.Context, ffmpeg string, args []string, log *slog.Logger,
	failed func(decodeFault bool), done func(cpu time.Duration), watch func(SpeedSample) bool,
) (io.ReadCloser, error) {
	var progress, progressW *os.File
	if watch != nil {
		var err error
		if progress, progressW, err = os.Pipe(); err != nil {
			return nil, err
		}
		args = append([]string{"-progress", "pipe:3"}, args...)
	}
	closeProgress := func() {
		if progress != nil {
			_ = progress.Close()
			_ = progressW.Close()
		}
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	faults := &limitedWriter{w: &stderr, n: 4096}
	cmd.Stderr = faults
	if progressW != nil {
		cmd.ExtraFiles = []*os.File{progressW}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		closeProgress()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		closeProgress()
		return nil, err
	}
	cut := &boxCut{r: stdout}
	if progress != nil {
		_ = progressW.Close() // the child holds its own copy
		pid := cmd.Process.Pid
		go encode.ReadProgress(progress, func(p encode.Progress) {
			if cut.stop.Load() {
				return
			}
			cpu, _ := processCPUTime(pid)
			if watch(SpeedSample{At: time.Now(), OutTime: time.Duration(p.OutTimeMS) * time.Millisecond, CPU: cpu, Speed: p.Speed}) {
				cut.stop.Store(true)
			}
		})
	}
	return &encoderOutput{ReadCloser: stdout, cut: cut, ctx: ctx, cmd: cmd, stderr: &stderr, watch: faults, failed: failed, done: done, log: log}, nil
}

// boxCut passes an fMP4 stream through whole top-level boxes and, once stopped, ends it with a
// clean EOF at the next box boundary: the reader never sees a partial box, so the packager's
// stream ends as if the encoder had finished there.
type boxCut struct {
	r       io.Reader
	stop    atomic.Bool
	left    int64 // bytes left in the current box; 0 at a boundary, -1 for a box that runs to EOF
	pending []byte
}

func (c *boxCut) Read(p []byte) (int, error) {
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	if c.left == 0 {
		if c.stop.Load() {
			return 0, io.EOF
		}
		var h [16]byte
		if _, err := io.ReadFull(c.r, h[:8]); err != nil {
			return 0, err // io.EOF here is the encoder finishing at a boundary
		}
		size, hl := int64(binary.BigEndian.Uint32(h[:4])), 8
		if size == 1 {
			if _, err := io.ReadFull(c.r, h[8:16]); err != nil {
				return 0, io.ErrUnexpectedEOF
			}
			size, hl = int64(binary.BigEndian.Uint64(h[8:16])), 16
		}
		switch {
		case size == 0:
			c.left = -1
		case size < int64(hl):
			return 0, fmt.Errorf("packager hls: box size %d", size)
		default:
			c.left = size - int64(hl)
		}
		c.pending = append([]byte(nil), h[:hl]...)
		return c.Read(p)
	}
	if c.left > 0 && int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	if c.left > 0 {
		c.left -= int64(n)
		if err == io.EOF && c.left > 0 {
			err = io.ErrUnexpectedEOF
		}
	}
	return n, err
}

type encoderOutput struct {
	io.ReadCloser // the encoder's stdout; reads go through cut
	cut           *boxCut
	ctx           context.Context
	cmd           *exec.Cmd
	stderr        *bytes.Buffer
	watch         *limitedWriter
	failed        func(decodeFault bool)
	// done receives the CPU time of an encoder that produced, for the ledger's measured class cost
	// (Lease.ObserveCPU, #1520); the schedule divides it by the frames the packager took.
	done     func(cpu time.Duration)
	produced bool // read on the packager's reader goroutine, then by Close after it
	// eof: the encoder's own output ended, not a ladder step's cut (boxCut.stop). Read on the
	// packager's reader goroutine, then by SourceEnded after it.
	eof     bool
	log     *slog.Logger
	once    sync.Once
	reaping sync.Once
	reaped  chan struct{} // closed once cmd.Wait has returned waitErr
	waitErr error
}

func (e *encoderOutput) Read(p []byte) (int, error) {
	n, err := e.cut.Read(p)
	if n > 0 {
		e.produced = true
	}
	if errors.Is(err, io.EOF) && !e.cut.stop.Load() {
		e.eof = true
	}
	return n, err
}

// sourceEndWait bounds how long SourceEnded waits for an encoder whose output ended to exit. An
// ffmpeg that has written its last fragment exits at once; one that does not is not trusted, and
// its airing rejoins through the schedule. Short, because the channel's timeline waits on it.
const sourceEndWait = 500 * time.Millisecond

// SourceEnded implements packager.SourceEnd: the encoder's output ended on its own (not cut for a
// ladder step) and the process then exited 0, so it read its input to the end. A commercial whose
// audio runs past its picture ends this way before its slot fills; reopening it there would start
// an encoder at the last frame, which produces nothing and fails (on VAAPI: "A hardware frames
// reference is required", the misleading error of 2026-10-24).
func (e *encoderOutput) SourceEnded() bool {
	if !e.eof {
		return false
	}
	select {
	case <-e.reap():
		return e.waitErr == nil
	case <-time.After(sourceEndWait):
		if e.log != nil {
			e.log.Warn("packager hls: encoder still running after its output ended; rejoining", "wait_ms", sourceEndWait.Milliseconds())
		}
		return false
	}
}

// reap waits for the encoder once, for SourceEnded and Close alike.
func (e *encoderOutput) reap() <-chan struct{} {
	e.reaping.Do(func() {
		e.reaped = make(chan struct{})
		go func() {
			e.waitErr = e.cmd.Wait()
			close(e.reaped)
		}()
	})
	return e.reaped
}

func (e *encoderOutput) Close() error {
	e.once.Do(func() {
		_ = e.ReadCloser.Close()
		select {
		case <-e.reap():
		default:
			if e.cmd.Process != nil {
				_ = e.cmd.Process.Kill() // the packager is done with this item, finished or not
			}
			<-e.reaped
		}
		err := e.waitErr
		// Every encoder that produced reports its CPU, however it ended (the packager cancels the
		// item's context before closing it): the schedule counts it only over the frames the
		// packager reported delivered.
		if e.produced && e.done != nil && e.cmd.ProcessState != nil {
			e.done(e.cmd.ProcessState.UserTime() + e.cmd.ProcessState.SystemTime())
		}
		if e.ctx.Err() != nil || err == nil {
			return // ended by the packager or its channel: not a failure
		}
		if e.stderr.Len() > 0 {
			e.log.Warn("packager hls: item encoder failed", "err", err, "stderr", strings.TrimSpace(e.stderr.String()))
		}
		if !e.produced && e.failed != nil {
			e.failed(e.watch.decodeFault)
		}
	})
	return nil
}

// limitedWriter keeps the first n bytes of an encoder's stderr for its failure log, and notes a GPU
// decode fault anywhere in it. ffmpeg writes one log line per write, so a line is never split.
// The fault is read only after cmd.Wait, which waits for the copy into this writer.
type limitedWriter struct {
	w           io.Writer
	n           int
	decodeFault bool
}

func (l *limitedWriter) Write(b []byte) (int, error) {
	if !l.decodeFault && encode.IsHardwareDecodeFault(string(b)) {
		l.decodeFault = true
	}
	if l.n > 0 {
		k := min(len(b), l.n)
		_, _ = l.w.Write(b[:k])
		l.n -= k
	}
	return len(b), nil
}
