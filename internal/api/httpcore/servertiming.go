package httpcore

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server-Timing (#3399). Every response carries `total;dur=<ms>`, and a handler can name its hot
// phases with Phase, so a slow page shows where its time went in the browser's network panel
// with no server log access. Values are durations and fixed phase names only: no ids, no counts,
// nothing a viewer could learn about the install beyond how long it took.
const serverTimingHeader = "Server-Timing"

type ctxKeyServerTiming struct{}

type serverTiming struct {
	start  time.Time
	mu     sync.Mutex
	phases []timedPhase
}

type timedPhase struct {
	name string
	dur  time.Duration
}

// Phase starts timing a named part of the request and returns the func that ends it. Phases may
// run concurrently and repeat; a repeated name adds up. On a context that never passed through
// WithServerTiming (a unit test calling a handler directly) it is a no-op.
func Phase(ctx context.Context, name string) func() {
	t, _ := ctx.Value(ctxKeyServerTiming{}).(*serverTiming)
	if t == nil {
		return func() {}
	}
	began := time.Now()
	return func() {
		elapsed := time.Since(began)
		t.mu.Lock()
		defer t.mu.Unlock()
		for i := range t.phases {
			if t.phases[i].name == name {
				t.phases[i].dur += elapsed
				return
			}
		}
		t.phases = append(t.phases, timedPhase{name: name, dur: elapsed})
	}
}

func (t *serverTiming) header() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var b strings.Builder
	for _, p := range t.phases {
		fmt.Fprintf(&b, "%s;dur=%.1f, ", p.name, float64(p.dur.Microseconds())/1000)
	}
	fmt.Fprintf(&b, "total;dur=%.1f", float64(time.Since(t.start).Microseconds())/1000)
	return b.String()
}

// WithServerTiming stamps the Server-Timing header as the response's headers are sent, so it
// covers everything the handler did before its first byte.
func WithServerTiming(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := &serverTiming{start: time.Now()}
		tw := &timingWriter{ResponseWriter: w, timing: t}
		next.ServeHTTP(tw, r.WithContext(context.WithValue(r.Context(), ctxKeyServerTiming{}, t)))
	})
}

// timingWriter sets the header once, just before the status line. It keeps Flush (the SSE stream
// needs it) and Unwrap (http.ResponseController reaches the real writer through it).
type timingWriter struct {
	http.ResponseWriter
	timing *serverTiming
	sent   bool
}

func (w *timingWriter) stamp() {
	if w.sent {
		return
	}
	w.sent = true
	w.Header().Set(serverTimingHeader, w.timing.header())
}

func (w *timingWriter) WriteHeader(code int) {
	w.stamp()
	w.ResponseWriter.WriteHeader(code)
}

func (w *timingWriter) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

func (w *timingWriter) Flush() {
	w.stamp()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *timingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
