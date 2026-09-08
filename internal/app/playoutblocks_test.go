package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

func TestPlayoutBlockSourcePinsTheFirstBroadcastFormat(t *testing.T) {
	t.Helper()
	const format = "h264-1280x720-25-2500-128"
	origin := time.Unix(10, 123).UTC()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get(api.PlayoutTimelineOriginHeader); got != origin.Format(time.RFC3339Nano) {
			t.Errorf("timeline origin = %q", got)
		}
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
		w.Header().Set(api.PlayoutScheduleBlockHeader, "block_episode")
		_, _ = io.WriteString(w, "block")
	}))
	t.Cleanup(srv.Close)

	source := playoutBlockSource(srv.URL, func() string { return "secret" }, srv.Client(), nil)
	if _, err := source(t.Context(), playout.BlockRequest{ChannelID: "channel/one", Plan: playout.PlanFull, AiringAt: origin.Add(time.Second)}); !errors.Is(err, playout.ErrPreparedUnavailable) || requests != 0 {
		t.Fatalf("prospective lookup without prepared source: err=%v HTTP=%d", err, requests)
	}
	for i := 0; i < 2; i++ {
		block, err := source(context.Background(), playout.BlockRequest{ChannelID: "channel/one", Plan: playout.PlanFull, TimelineOrigin: origin})
		if err != nil {
			t.Fatalf("block %d: %v", i+1, err)
		}
		if block.Identity.Kind != schedule.SlotProgram || block.Identity.ContentID != "episode" ||
			block.Identity.ScheduleBlockID != "block_episode" {
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
		w.Header().Set(api.PlayoutScheduleBlockHeader, "block_episode")
		formats = formats[1:]
		_, _ = io.WriteString(w, "block")
	}))
	t.Cleanup(srv.Close)

	source := playoutBlockSource(srv.URL, func() string { return "secret" }, srv.Client(), nil)
	first, err := source(context.Background(), playout.BlockRequest{ChannelID: "channel", Plan: playout.PlanBaseline})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Content.Close()
	if _, err := source(context.Background(), playout.BlockRequest{ChannelID: "channel", Plan: playout.PlanBaseline}); err == nil {
		t.Fatal("second block accepted a changed broadcast format")
	}
}

func TestPlayoutBlockSourceUsesPreparedThenPinsLiveFallback(t *testing.T) {
	const formatToken = "h264-1920x1080-25-5000-160"
	format, ok := playout.ParseBroadcastFormat(formatToken)
	if !ok {
		t.Fatal("test broadcast format is invalid")
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get(api.PlayoutBroadcastFormatQuery); got != formatToken {
			t.Errorf("live fallback broadcast = %q, want prepared format %q", got, formatToken)
		}
		w.Header().Set(api.PlayoutBroadcastFormatHeader, formatToken)
		w.Header().Set(api.PlayoutAiringStartedAtHeader, time.Unix(2, 0).UTC().Format(time.RFC3339Nano))
		w.Header().Set(api.PlayoutAiringKindHeader, string(schedule.SlotProgram))
		w.Header().Set(api.PlayoutAiringContentHeader, "episode-two")
		w.Header().Set(api.PlayoutScheduleBlockHeader, "block-two")
		_, _ = io.WriteString(w, "live")
	}))
	t.Cleanup(srv.Close)

	preparedCalls := 0
	var preparedRequests []playout.BlockRequest
	prepared := playout.BlockSource(func(_ context.Context, blockRequest playout.BlockRequest) (playout.Block, error) {
		preparedCalls++
		preparedRequests = append(preparedRequests, blockRequest)
		if preparedCalls > 1 {
			return playout.Block{}, playout.ErrPreparedUnavailable
		}
		return playout.Block{
			Content: io.NopCloser(strings.NewReader("prepared")), Format: format,
			Identity: playout.AiringIdentity{
				StartedAt: time.Unix(1, 0), EndsAt: time.Unix(2, 0), Kind: schedule.SlotProgram,
				ContentID: "episode-one", ScheduleBlockID: "block-one",
			},
		}, nil
	})
	source := playoutBlockSource(srv.URL, func() string { return "secret" }, srv.Client(), prepared)

	first, err := source(t.Context(), playout.BlockRequest{ChannelID: "channel", Plan: playout.PlanFull})
	if err != nil {
		t.Fatal(err)
	}
	firstBody, _ := io.ReadAll(first.Content)
	_ = first.Content.Close()
	if string(firstBody) != "prepared" || first.Format != format || requests != 0 {
		t.Fatalf("first block = body %q format %+v HTTP requests %d, want prepared hit", firstBody, first.Format, requests)
	}
	prospective := playout.BlockRequest{ChannelID: "channel", Plan: playout.PlanFull, TimelineOrigin: time.Unix(1, 0), AiringAt: time.Unix(2, 0)}
	if _, err := source(t.Context(), prospective); !errors.Is(err, playout.ErrPreparedUnavailable) || requests != 0 {
		t.Fatalf("prospective prepared miss started live effects: err=%v HTTP=%d", err, requests)
	}
	if preparedRequests[1] != prospective {
		t.Fatalf("prepared request lost its timing: %+v", preparedRequests[1])
	}
	second, err := source(t.Context(), playout.BlockRequest{ChannelID: "channel", Plan: playout.PlanFull})
	if err != nil {
		t.Fatal(err)
	}
	secondBody, _ := io.ReadAll(second.Content)
	_ = second.Content.Close()
	if string(secondBody) != "live" || second.Format != format || requests != 1 || preparedCalls != 3 {
		t.Fatalf("second block = body %q format %+v HTTP %d prepared %d, want pinned live fallback",
			secondBody, second.Format, requests, preparedCalls)
	}
}
