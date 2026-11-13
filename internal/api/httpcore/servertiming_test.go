package httpcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestServerTimingNamesPhasesAndTotal(t *testing.T) {
	h := WithServerTiming(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		end := Phase(r.Context(), "store")
		end()
		again := Phase(r.Context(), "store") // a repeated phase adds up, it doesn't duplicate
		again()
		Phase(r.Context(), "compose")()
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get("Server-Timing")
	want := regexp.MustCompile(`^store;dur=[0-9.]+, compose;dur=[0-9.]+, total;dur=[0-9.]+$`)
	if !want.MatchString(got) {
		t.Fatalf("Server-Timing = %q", got)
	}
}

func TestServerTimingStampsOnImplicitHeaderAndKeepsFlush(t *testing.T) {
	h := WithServerTiming(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("wrapped writer lost http.Flusher; the SSE stream needs it")
		}
		_, _ = w.Write([]byte("ok")) // no explicit WriteHeader
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Server-Timing") == "" {
		t.Fatal("no Server-Timing on an implicit 200")
	}
}

func TestPhaseWithoutMiddlewareIsANoop(t *testing.T) {
	Phase(context.Background(), "store")()
}
