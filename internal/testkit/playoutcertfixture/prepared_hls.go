package playoutcertfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// PreparedHLSMode selects the bounded public HLS behaviour used by prepared
// programme-boundary regressions.
type PreparedHLSMode uint8

const (
	PreparedHLSReplacement PreparedHLSMode = iota
	PreparedHLSBlockedRefresh
	PreparedHLSQueuedTransition
	PreparedHLSBlockedAfterQualified
	PreparedHLSChangedTime
	PreparedHLSChangedDuration
)

// PreparedHLS is a public mint/HLS fixture. Its second playlist either
// advertises a replacement epoch or remains live until the client cancels it.
type PreparedHLS struct {
	Server   *httptest.Server
	Admin    string
	Blocked  <-chan struct{}
	Canceled <-chan struct{}
	// ReleaseNext lets the qualified live epoch finish after the observation
	// window. TransitionQueued reports that the reader's subsequent playlist
	// response contained its next genuine discontinuity.
	ReleaseNext      chan<- struct{}
	TransitionQueued <-chan struct{}
	// AssetBodies replaces individual valid prepared-HLS assets by path. It
	// lets callers exercise the real HTTP asset reader at size boundaries.
	AssetBodies map[string][]byte

	mode             PreparedHLSMode
	playlists        atomic.Int32
	blocked          chan struct{}
	canceled         chan struct{}
	releaseNext      chan struct{}
	transitionQueued chan struct{}
}

func NewPreparedHLS(t testing.TB, mode PreparedHLSMode) *PreparedHLS {
	t.Helper()
	f := &PreparedHLS{Admin: "admin", mode: mode, blocked: make(chan struct{}), canceled: make(chan struct{}), releaseNext: make(chan struct{}), transitionQueued: make(chan struct{})}
	f.Blocked = f.blocked
	f.Canceled = f.canceled
	f.ReleaseNext = f.releaseNext
	f.TransitionQueued = f.transitionQueued
	f.Server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *PreparedHLS) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/channels/prepared/play-url":
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+f.Admin {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"relativeUrl": "/hls?signature=test"})
	case "/hls":
		if r.Method != http.MethodGet || r.URL.Query().Get("signature") != "test" || r.URL.Query().Get("mode") != "prepared" {
			http.NotFound(w, r)
			return
		}
		playlist := f.playlists.Add(1)
		if playlist == 1 {
			_, _ = io.WriteString(w, preparedHLSManifest("init-a", "a", ""))
			return
		}
		if f.mode == PreparedHLSChangedTime || f.mode == PreparedHLSChangedDuration {
			body := preparedHLSManifest("init-a", "a", "")
			if f.mode == PreparedHLSChangedTime {
				body = strings.ReplaceAll(body, "12:00:00Z", "12:00:01Z")
			} else {
				body = strings.ReplaceAll(body, "#EXTINF:1,", "#EXTINF:2,")
			}
			_, _ = io.WriteString(w, body)
			return
		}
		if f.mode == PreparedHLSBlockedRefresh {
			select {
			case <-f.blocked:
			default:
				close(f.blocked)
			}
			<-r.Context().Done()
			select {
			case <-f.canceled:
			default:
				close(f.canceled)
			}
			return
		}
		if playlist == 3 {
			switch f.mode {
			case PreparedHLSQueuedTransition:
				f.signalTransitionQueued()
				_, _ = io.WriteString(w, preparedHLSQueuedTransitionManifest())
				return
			case PreparedHLSBlockedAfterQualified:
				f.signalBlocked()
				<-r.Context().Done()
				f.signalCanceled()
				return
			}
		}
		_, _ = io.WriteString(w, preparedHLSManifest("init-a", "a", "#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-b\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:01Z\n#EXTINF:1,\nb\n"))
		return
	case "/init-a", "/a", "/init-b", "/init-c", "/c", "/init-d":
		if body, ok := f.AssetBodies[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write(make([]byte, 188))
	case "/b":
		if f.mode == PreparedHLSQueuedTransition {
			f.serveQueuedEpochAsset(w, r)
			return
		}
		_, _ = w.Write(make([]byte, 188))
	case "/d":
		if f.mode == PreparedHLSQueuedTransition {
			f.serveFinalEpochAsset(w, r)
			return
		}
		_, _ = w.Write(make([]byte, 188))
	default:
		http.NotFound(w, r)
	}
}

func (f *PreparedHLS) signalBlocked() {
	select {
	case <-f.blocked:
	default:
		close(f.blocked)
	}
}

func (f *PreparedHLS) signalCanceled() {
	select {
	case <-f.canceled:
	default:
		close(f.canceled)
	}
}

func (f *PreparedHLS) signalTransitionQueued() {
	select {
	case <-f.transitionQueued:
	default:
		close(f.transitionQueued)
	}
}

func (f *PreparedHLS) serveQueuedEpochAsset(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(make([]byte, 188))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	select {
	case <-f.releaseNext:
		_, _ = w.Write(make([]byte, 188))
	case <-r.Context().Done():
		f.signalCanceled()
	}
}

func (f *PreparedHLS) serveFinalEpochAsset(w http.ResponseWriter, r *http.Request) {
	flusher, _ := w.(http.Flusher)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, _ = w.Write(make([]byte, 188))
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func preparedHLSManifest(init, media, suffix string) string {
	return preparedHLSManifestAt(0, init, media, suffix)
}

func preparedHLSManifestAt(sequence int, init, media, suffix string) string {
	return fmt.Sprintf("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-MAP:URI=\"%s\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:00Z\n#EXTINF:1,\n%s\n%s", sequence, init, media, suffix)
}

func preparedHLSQueuedTransitionManifest() string {
	return "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:2\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n#EXT-X-MAP:URI=\"init-c\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:02Z\n#EXTINF:1,\nc\n#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-d\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:03Z\n#EXTINF:1,\nd\n"
}

// ShapeValidator is a generic validator double. Callers provide their local
// media-shape type so this neutral fixture package does not import application
// packages. It returns plans in order and repeats the final shape thereafter.
type ShapeValidator[T any] struct {
	Shapes      []T
	WaitFor     <-chan struct{}
	WaitForCall int
	CallStarted chan<- int
	calls       atomic.Int32
}

func (v *ShapeValidator[T]) Validate(ctx context.Context, _ []byte) (T, error) {
	call := int(v.calls.Add(1))
	if v.CallStarted != nil {
		select {
		case v.CallStarted <- call:
		default:
		}
	}
	if v.WaitFor != nil && (v.WaitForCall == 0 || v.WaitForCall == call) {
		select {
		case <-v.WaitFor:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	if len(v.Shapes) == 0 {
		var zero T
		return zero, fmt.Errorf("shape validator has no shapes")
	}
	if call > len(v.Shapes) {
		call = len(v.Shapes)
	}
	return v.Shapes[call-1], nil
}

func (v *ShapeValidator[T]) Calls() int { return int(v.calls.Load()) }
