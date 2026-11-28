package playout

import (
	"io"

	"github.com/loomarr/loomarr/internal/media/encode"
)

// LimitedWriter keeps the first n bytes of an encoder's stderr for its failure log, and notes a GPU
// decode fault anywhere in it. ffmpeg writes one log line per write, so a line is never split.
type LimitedWriter struct {
	w           io.Writer
	n           int
	decodeFault bool
}

// CappedWriter passes the first n bytes through to w and discards the rest, so a failing encoder
// cannot grow its captured output without bound.
func CappedWriter(w io.Writer, n int) *LimitedWriter { return &LimitedWriter{w: w, n: n} }

func (l *LimitedWriter) Write(b []byte) (int, error) {
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

// DecodeFault reports a GPU decode fault in what was written. Read it only after cmd.Wait, which
// waits for the copy into this writer.
func (l *LimitedWriter) DecodeFault() bool { return l.decodeFault }
