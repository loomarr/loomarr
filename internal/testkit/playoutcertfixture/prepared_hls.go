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
)

// PreparedHLSMode selects transport metadata for reader and size-bound tests.
// Programme qualification uses independently declared signal fixtures instead.
type PreparedHLSMode uint8

const (
	PreparedHLSReplacement PreparedHLSMode = iota
	PreparedHLSChangedTime
	PreparedHLSChangedDuration
)

type PreparedHLS struct {
	Server      *httptest.Server
	Admin       string
	AssetBodies map[string][]byte
	mode        PreparedHLSMode
	playlists   atomic.Int32
}

func NewPreparedHLS(t testing.TB, mode PreparedHLSMode) *PreparedHLS {
	t.Helper()
	f := &PreparedHLS{Admin: "admin", mode: mode}
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
		body := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init-a\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:00Z\n#EXTINF:1,\na\n"
		if f.playlists.Add(1) > 1 {
			switch f.mode {
			case PreparedHLSChangedTime:
				body = strings.ReplaceAll(body, "12:00:00Z", "12:00:01Z")
			case PreparedHLSChangedDuration:
				body = strings.ReplaceAll(body, "#EXTINF:1,", "#EXTINF:2,")
			case PreparedHLSReplacement:
				body += "#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-b\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:01Z\n#EXTINF:1,\nb\n"
			}
		}
		_, _ = io.WriteString(w, body)
	case "/init-a", "/a", "/init-b", "/b":
		body, ok := f.AssetBodies[r.URL.Path]
		if !ok {
			body = make([]byte, 188)
		}
		_, _ = w.Write(body)
	default:
		http.NotFound(w, r)
	}
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
