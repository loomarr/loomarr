package demolibrary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

// downloader fetches the open pack's pinned files. A file is in place only once its byte count
// and SHA-256 match the manifest; anything else is fetched again, and a file that still does not
// match after every attempt fails the whole asset step rather than serving the wrong bytes.
type downloader struct {
	client   *http.Client
	attempts int
	backoff  time.Duration // doubled after each failed attempt
	log      *slog.Logger
}

func newDownloader(log *slog.Logger) downloader {
	// No overall timeout: a 300 MB file on a slow link is legitimate. A stalled transfer still
	// ends, because the response header and each read are bounded by the transport.
	return downloader{client: &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: time.Minute, IdleConnTimeout: time.Minute,
	}}, attempts: 5, backoff: 2 * time.Second, log: log}
}

// errHashMismatch is a download whose bytes are not the pinned file.
var errHashMismatch = errors.New("downloaded bytes do not match the pinned sha256")

// fetch makes dest hold m's pinned bytes. It is idempotent (a verified file is left alone) and
// resumable (an interrupted transfer continues from dest.partial with a Range request).
func (d downloader) fetch(ctx context.Context, m PackMedia, dest string) error {
	if err := verifyFile(dest, m); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		d.log.Warn("demo pack file does not match its pin; fetching it again", "file", dest, "err", err)
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	partial := dest + ".partial"
	d.log.Info("demo pack file downloading", "url", m.URL, "bytes", m.Size)
	var last error
	wait := d.backoff
	for attempt := 1; attempt <= d.attempts; attempt++ {
		last = d.transfer(ctx, m, partial)
		if last == nil {
			last = verifyFile(partial, m)
			if last == nil {
				return os.Rename(partial, dest)
			}
			_ = os.Remove(partial) // wrong bytes: the next attempt starts from zero
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		d.log.Warn("demo pack download failed", "url", m.URL, "attempt", attempt, "of", d.attempts, "err", last)
		if attempt < d.attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
		}
	}
	return fmt.Errorf("download %s: %w", m.URL, last)
}

// transfer appends the rest of the file to partial, resuming from its current length.
func (d downloader) transfer(ctx context.Context, m PackMedia, partial string) error {
	var have int64
	if info, err := os.Stat(partial); err == nil {
		have = info.Size()
	}
	if have > m.Size {
		if err := os.Remove(partial); err != nil {
			return err
		}
		have = 0
	}
	if have == m.Size {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return err
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC // the server ignored the range: start over
		have = 0
	default:
		return fmt.Errorf("GET %s: %s", m.URL, resp.Status)
	}
	f, err := os.OpenFile(partial, flags, 0o644)
	if err != nil {
		return err
	}
	// Read one byte past the pin, so an oversized body fails verification instead of being cut
	// to a size that happens to match.
	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, m.Size-have+1))
	if err := f.Close(); err != nil && copyErr == nil {
		copyErr = err
	}
	return copyErr
}

// verifyFile checks a file's byte count, then its SHA-256, against the pin.
func verifyFile(p string, m PackMedia) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() != m.Size {
		return fmt.Errorf("%s is %d bytes, want %d", p, info.Size(), m.Size)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != m.SHA256 {
		return fmt.Errorf("%w: %s has sha256 %s, want %s", errHashMismatch, p, got, m.SHA256)
	}
	return nil
}
