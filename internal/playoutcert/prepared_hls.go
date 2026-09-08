package playoutcert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// preparedHLSReader turns a public signed media playlist into an ordered fMP4
// or MPEG-TS byte stream.  It intentionally owns both playlist and asset fetching: a
// raw programme stream is not evidence for a transition advertised elsewhere.
type preparedHLSReader struct {
	ctx              context.Context
	cancel           context.CancelFunc
	e                *endpoint
	playlist         *url.URL
	mu               sync.Mutex
	closed           bool
	closeErr         error
	seen             map[string]preparedHLSSegment
	queue            []preparedHLSSegment
	current          io.ReadCloser
	currentInit      string
	initial          bool
	boundary         string
	epochPending     bool
	requireMap       bool
	transition       chan struct{}
	onSegment        func(preparedHLSSegment) (ProgrammeAssetEvidence, error)
	onAssetMismatch  func()
	pendingProof     *ProgrammeAssetEvidence
	currentInitProof []byte
	validateSource   func() error
}

type preparedHLSSegment struct {
	uri, init, id, boundary string
	startedAt               time.Time
	duration                time.Duration
}

var errPreparedHLSEpoch = errors.New("prepared HLS decode epoch ended")

func newPreparedHLSReader(ctx context.Context, e *endpoint, signed *url.URL) *preparedHLSReader {
	p := *signed
	q := p.Query()
	q.Set("mode", "prepared")
	p.RawQuery = q.Encode()
	owned, cancel := context.WithCancel(ctx)
	return &preparedHLSReader{ctx: owned, cancel: cancel, e: e, playlist: &p, seen: map[string]preparedHLSSegment{}, transition: make(chan struct{}, 1), requireMap: true}
}

// newLiveHLSReader follows ordinary signed HLS, including MPEG-TS playlists
// without an initialization map. It never requests prepared-only delivery.
func newLiveHLSReader(ctx context.Context, e *endpoint, signed *url.URL) *preparedHLSReader {
	r := newPreparedHLSReader(ctx, e, signed)
	q := r.playlist.Query()
	q.Del("mode")
	r.playlist.RawQuery = q.Encode()
	r.requireMap = false
	return r
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
				closeErr := current.Close()
				r.mu.Lock()
				r.closeErr = errors.Join(r.closeErr, closeErr)
				if r.current == current {
					r.current = nil
				}
				r.mu.Unlock()
				if closeErr != nil {
					return n, closeErr
				}
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
	if r.closed {
		err := r.closeErr
		r.mu.Unlock()
		return err
	}
	r.closed = true
	current := r.current
	r.current = nil
	r.mu.Unlock()
	r.cancel()
	var closeErr error
	if current != nil {
		closeErr = current.Close()
	}
	r.mu.Lock()
	r.closeErr = errors.Join(r.closeErr, closeErr)
	err := r.closeErr
	r.mu.Unlock()
	return err
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
	if r.boundary == "" {
		r.boundary = seg.boundary
	}
	if seg.boundary != r.boundary && r.initial {
		if !r.epochPending {
			r.epochPending = true
			r.transition <- struct{}{}
			return errPreparedHLSEpoch
		}
		r.epochPending = false
		r.boundary = seg.boundary
		// A discontinuity starts a new fMP4 decode epoch even where the
		// publication happens to reuse byte-identical initialization media.
		r.currentInit = ""
		r.currentInitProof = nil
	}
	r.queue = r.queue[1:]
	proof := r.pendingProof
	r.pendingProof = nil
	if proof == nil && r.onSegment != nil {
		resolved, err := r.onSegment(seg)
		if err != nil {
			return err
		}
		proof = &resolved
	}
	if proof != nil {
		r.validateSource = proof.Validate
	}
	if seg.init != r.currentInit {
		var expected []byte
		if proof != nil {
			expected = proof.Init
		}
		if err := r.openAsset(seg.init, expected, proof != nil); err != nil {
			return err
		}
		r.currentInit = seg.init
		r.currentInitProof = expected
		r.pendingProof = proof
		// Deliver an initialization map before its first media fragment.
		r.queue = append([]preparedHLSSegment{seg}, r.queue...)
		return nil
	}
	var expected []byte
	if proof != nil {
		if !bytes.Equal(proof.Init, r.currentInitProof) {
			r.onAssetMismatch()
			return errors.New("asset_clock_mismatch")
		}
		expected = proof.Media
	}
	err := r.openAsset(seg.uri, expected, proof != nil)
	if err == nil {
		r.initial = true
	}
	return err
}

type preparedHLSEpochReader struct{ reader *preparedHLSReader }

func (r preparedHLSEpochReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if errors.Is(err, errPreparedHLSEpoch) {
		err = io.EOF
	}
	return n, err
}

func (preparedHLSEpochReader) Close() error { return nil }

func (r *preparedHLSReader) epoch() io.ReadCloser { return preparedHLSEpochReader{reader: r} }

func (r *preparedHLSReader) openAsset(raw string, expected []byte, verify bool) error {
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
	var content io.Reader = newBoundedReader(resp.Body, 32<<20)
	if verify {
		content = &assetReferenceReader{source: content, expected: expected, validate: r.validateSource, mismatch: r.onAssetMismatch}
	}
	r.current = newAssetReadCloser(resp.Body, content)
	r.mu.Unlock()
	return nil
}

// assetReferenceReader admits only bytes matching the independent reference.
// It never reads ahead or manufactures transport progress from a private copy.
type assetReferenceReader struct {
	source   io.Reader
	expected []byte
	mismatch func()
	validate func() error
}

func (r *assetReferenceReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if (r.validate != nil && r.validate() != nil) || n > len(r.expected) || !bytes.Equal(p[:n], r.expected[:min(n, len(r.expected))]) ||
		(errors.Is(err, io.EOF) && n != len(r.expected)) {
		r.mismatch()
		return 0, errors.New("asset_clock_mismatch")
	}
	r.expected = r.expected[n:]
	return n, err
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
	body, err := readBoundedBody(resp.Body, 1<<20)
	if err != nil {
		return err
	}
	segments, err := parseHLSMedia(body, r.requireMap)
	if err != nil {
		return err
	}
	for _, seg := range segments {
		if previous, ok := r.seen[seg.id]; ok {
			if previous.uri != seg.uri || previous.init != seg.init || previous.boundary != seg.boundary || !previous.startedAt.Equal(seg.startedAt) || previous.duration != seg.duration {
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

// assetReadCloser is the sole owner of an admitted asset response. An EOF and
// a caller cancellation can overlap, but only one may close the transport.
// This keeps request-context cancellation paired with exactly one body close.
type assetReadCloser struct {
	body   io.ReadCloser
	reader io.Reader
	once   sync.Once
	err    error
}

func newAssetReadCloser(body io.ReadCloser, reader io.Reader) *assetReadCloser {
	return &assetReadCloser{body: body, reader: reader}
}

func (r *assetReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *assetReadCloser) Close() error {
	r.once.Do(func() { r.err = r.body.Close() })
	return r.err
}
