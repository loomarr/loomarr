package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

func TestPlayoutBlockSourcePinsTheFirstBroadcastFormat(t *testing.T) {
	t.Helper()
	const format = "h264-1280x720-25-2500-128"
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("plan"); got != "full" {
			t.Errorf("request %d plan = %q, want full", requests, got)
		}
		if got := r.URL.Query().Get(api.PlayoutBroadcastFormatQuery); requests == 1 && got != "" {
			t.Errorf("first request broadcast = %q, want empty", got)
		} else if requests == 2 && got != format {
			t.Errorf("second request broadcast = %q, want %q", got, format)
		}
		w.Header().Set(api.PlayoutBroadcastFormatHeader, format)
		w.Header().Set(api.PlayoutAiringStartedAtHeader, time.Unix(int64(requests), 0).UTC().Format(time.RFC3339Nano))
		w.Header().Set(api.PlayoutAiringKindHeader, string(schedule.SlotProgram))
		w.Header().Set(api.PlayoutAiringContentHeader, "episode")
		_, _ = io.WriteString(w, "block")
	}))
	t.Cleanup(srv.Close)

	source := playoutBlockSource(srv.URL, func() string { return "secret" }, srv.Client())
	for i := 0; i < 2; i++ {
		block, err := source(context.Background(), "channel/one", playout.PlanFull)
		if err != nil {
			t.Fatalf("block %d: %v", i+1, err)
		}
		if block.Identity.Kind != schedule.SlotProgram || block.Identity.ContentID != "episode" {
			t.Fatalf("block %d identity = %+v", i+1, block.Identity)
		}
		_, _ = io.Copy(io.Discard, block.Content)
		_ = block.Content.Close()
	}
}

func TestPlayoutBlockSourceRejectsAFormatChange(t *testing.T) {
	formats := []string{"h264-1280x720-25-2500-128", "h264-1920x1080-25-5000-160"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(api.PlayoutBroadcastFormatHeader, formats[0])
		w.Header().Set(api.PlayoutAiringStartedAtHeader, time.Unix(1, 0).UTC().Format(time.RFC3339Nano))
		w.Header().Set(api.PlayoutAiringKindHeader, string(schedule.SlotProgram))
		w.Header().Set(api.PlayoutAiringContentHeader, "episode")
		formats = formats[1:]
		_, _ = io.WriteString(w, "block")
	}))
	t.Cleanup(srv.Close)

	source := playoutBlockSource(srv.URL, func() string { return "secret" }, srv.Client())
	first, err := source(context.Background(), "channel", playout.PlanBaseline)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Content.Close()
	if _, err := source(context.Background(), "channel", playout.PlanBaseline); err == nil {
		t.Fatal("second block accepted a changed broadcast format")
	}
}
