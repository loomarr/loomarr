package playoutcert

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// preparedHLSReader turns the public, signed prepared playlist into one ordered
// fMP4 byte stream.  It intentionally owns both playlist and asset fetching: a
// raw programme stream is not evidence for a transition advertised elsewhere.
type preparedHLSReader struct {
	ctx            context.Context
	cancel         context.CancelFunc
	e              *endpoint
	playlist       *url.URL
	mu             sync.Mutex
	closed         bool
	seen           map[string]preparedHLSSegment
	queue          []preparedHLSSegment
	current        io.ReadCloser
	currentInit    string
	initial        bool
	armed          bool
	boundary       string
	transition     chan struct{}
	transitionOnce sync.Once
}

type preparedHLSSegment struct{ uri, init, id, boundary string }

func newPreparedHLSReader(ctx context.Context, e *endpoint, signed *url.URL) *preparedHLSReader {
	p := *signed
	q := p.Query()
	q.Set("mode", "prepared")
	p.RawQuery = q.Encode()
	owned, cancel := context.WithCancel(ctx)
	return &preparedHLSReader{ctx: owned, cancel: cancel, e: e, playlist: &p, seen: map[string]preparedHLSSegment{}, transition: make(chan struct{})}
}

func (r *preparedHLSReader) Read(p []byte) (int, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return 0, io.ErrClosedPipe
		}
		current := r.current
		r.mu.Unlock()
		if current != nil {
			n, err := current.Read(p)
			if errors.Is(err, io.EOF) {
				_ = current.Close()
				r.mu.Lock()
				r.current = nil
				r.mu.Unlock()
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, err
		}
		if err := r.openNext(); err != nil {
			return 0, err
		}
	}
}

func (r *preparedHLSReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cancel()
	if r.current != nil {
		return r.current.Close()
	}
	return nil
}

func (r *preparedHLSReader) openNext() error {
	for len(r.queue) == 0 {
		if err := r.refresh(); err != nil {
			return err
		}
		if len(r.queue) == 0 {
			select {
			case <-r.ctx.Done():
				return r.ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	seg := r.queue[0]
	r.queue = r.queue[1:]
	if r.boundary == "" {
		r.boundary = seg.boundary
	}
	if seg.boundary != r.boundary && r.initial {
		// Do not let a boundary already buffered in the initial playlist race
		// ahead of validation and become retrospective proof.
		for !r.isArmed() {
			select {
			case <-r.ctx.Done():
				return r.ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
		r.boundary = seg.boundary
		// A discontinuity starts a new fMP4 decode epoch even where the
		// publication happens to reuse byte-identical initialization media.
		r.currentInit = ""
		r.transitionOnce.Do(func() { close(r.transition) })
	}
	if seg.init != r.currentInit {
		if err := r.openAsset(seg.init); err != nil {
			return err
		}
		r.currentInit = seg.init
		// Deliver an initialization map before its first media fragment.
		r.queue = append([]preparedHLSSegment{seg}, r.queue...)
		return nil
	}
	err := r.openAsset(seg.uri)
	if err == nil {
		r.initial = true
	}
	return err
}

// arm excludes a discontinuity already buffered in the first playlist.  Only
// a subsequently fetched boundary can be post-validation evidence.
func (r *preparedHLSReader) arm()          { r.mu.Lock(); r.armed = true; r.mu.Unlock() }
func (r *preparedHLSReader) isArmed() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.armed }

func (r *preparedHLSReader) openAsset(raw string) error {
	u, err := r.playlist.Parse(raw)
	if err != nil || !sameOrigin(r.e.base, u) {
		return errors.New("invalid_hls")
	}
	resp, err := r.e.requestFor(r.ctx, http.MethodGet, u.String(), nil, false, r.e.timeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("asset status %d", resp.StatusCode)
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = resp.Body.Close()
		return io.ErrClosedPipe
	}
	r.current = &limitedReadCloser{body: resp.Body, reader: io.LimitReader(resp.Body, 32<<20)}
	r.mu.Unlock()
	return nil
}

func (r *preparedHLSReader) refresh() error {
	resp, err := r.e.requestFor(r.ctx, http.MethodGet, r.playlist.String(), nil, false, r.e.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("playlist status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	segments, err := parsePreparedHLS(body)
	if err != nil {
		return err
	}
	for _, seg := range segments {
		if previous, ok := r.seen[seg.id]; ok {
			if previous.uri != seg.uri || previous.init != seg.init || previous.boundary != seg.boundary {
				return errors.New("inconsistent_hls_replay")
			}
			continue
		}
		if len(r.seen) >= 4096 || len(r.queue) >= 512 {
			return errors.New("hls_limit_exceeded")
		}
		r.seen[seg.id] = seg
		r.queue = append(r.queue, seg)
	}
	return nil
}

func parsePreparedHLS(body []byte) ([]preparedHLSSegment, error) {
	var out []preparedHLSSegment
	sequence := int64(0)
	index := int64(0)
	discontinuitySequence := int64(0)
	localDiscontinuities := int64(0)
	init := ""
	sawMedia := false
	pdt, discontinuity := false, false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64)
			if err != nil || v < 0 {
				return nil, errors.New("invalid_hls")
			}
			sequence = v
			index = 0
		case strings.HasPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:"):
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:"), 10, 64)
			if err != nil || v < 0 {
				return nil, errors.New("invalid_hls")
			}
			discontinuitySequence = v
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			v := mapURI(line)
			if v == "" {
				return nil, errors.New("invalid_hls")
			}
			init = v
		case line == "#EXT-X-DISCONTINUITY":
			discontinuity = true
			localDiscontinuities++
		case strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"):
			if _, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")); err != nil {
				return nil, errors.New("invalid_hls")
			}
			pdt = true
		case strings.HasPrefix(line, "#EXTINF:"):
			if init == "" {
				return nil, errors.New("invalid_hls")
			}
			first := !sawMedia
			sawMedia = true
			if !scanner.Scan() {
				return nil, errors.New("invalid_hls")
			}
			uri := strings.TrimSpace(scanner.Text())
			if uri == "" || strings.HasPrefix(uri, "#") {
				return nil, errors.New("invalid_hls")
			}
			if (discontinuity || first) && !pdt {
				return nil, errors.New("invalid_hls")
			}
			boundary := fmt.Sprintf("%d", discontinuitySequence+localDiscontinuities)
			out = append(out, preparedHLSSegment{uri: uri, init: init, id: fmt.Sprintf("%d", sequence+index), boundary: boundary})
			index++
			discontinuity, pdt = false, false
		}
	}
	if scanner.Err() != nil || !sawMedia {
		return nil, errors.New("invalid_hls")
	}
	return out, nil
}

func mapURI(line string) string {
	const key = `URI="`
	start := strings.Index(line, key)
	if start < 0 {
		return ""
	}
	rest := line[start+len(key):]
	end := strings.IndexByte(rest, '"')
	if end < 1 {
		return ""
	}
	return rest[:end]
}

type limitedReadCloser struct {
	body   io.ReadCloser
	reader io.Reader
}

func (r *limitedReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *limitedReadCloser) Close() error               { return r.body.Close() }
