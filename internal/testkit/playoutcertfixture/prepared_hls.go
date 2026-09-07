package playoutcertfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// PreparedHLSMode selects the bounded public HLS behaviour used by prepared
// programme-boundary regressions.
type PreparedHLSMode uint8

const (
	PreparedHLSReplacement PreparedHLSMode = iota
	PreparedHLSBlockedRefresh
)

// PreparedHLS is a public mint/HLS fixture. Its second playlist either
// advertises a replacement epoch or remains live until the client cancels it.
type PreparedHLS struct {
	Server   *httptest.Server
	Admin    string
	Blocked  <-chan struct{}
	Canceled <-chan struct{}

	mode      PreparedHLSMode
	playlists atomic.Int32
	blocked   chan struct{}
	canceled  chan struct{}
}

func NewPreparedHLS(t testing.TB, mode PreparedHLSMode) *PreparedHLS {
	t.Helper()
	f := &PreparedHLS{Admin: "admin", mode: mode, blocked: make(chan struct{}), canceled: make(chan struct{})}
	f.Blocked = f.blocked
	f.Canceled = f.canceled
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
		if f.playlists.Add(1) == 1 {
			_, _ = io.WriteString(w, preparedHLSManifest("init-a", "a", ""))
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
		_, _ = io.WriteString(w, preparedHLSManifest("init-a", "a", "#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-b\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:01Z\n#EXTINF:1,\nb\n"))
	case "/init-a", "/a", "/init-b", "/b":
		_, _ = w.Write(make([]byte, 188))
	default:
		http.NotFound(w, r)
	}
}

func preparedHLSManifest(init, media, suffix string) string {
	return "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"" + init + "\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:00Z\n#EXTINF:1,\n" + media + "\n" + suffix
}

// ShapeValidator is a generic validator double. Callers provide their local
// media-shape type so this neutral fixture package does not import application
// packages. It returns plans in order and repeats the final shape thereafter.
type ShapeValidator[T any] struct {
	Shapes  []T
	WaitFor <-chan struct{}
	calls   atomic.Int32
}

func (v *ShapeValidator[T]) Validate(ctx context.Context, _ []byte) (T, error) {
	if v.WaitFor != nil {
		select {
		case <-v.WaitFor:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	call := int(v.calls.Add(1))
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
