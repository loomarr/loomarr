package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type memorySink struct {
	mu      sync.Mutex
	records []Record
	err     error
	block   <-chan struct{}
	entered chan<- struct{}
}

func (s *memorySink) AppendDiagnosticEvents(_ context.Context, records []Record) error {
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		<-s.block
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	return s.err
}

func (s *memorySink) snapshot() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Record(nil), s.records...)
}

func TestRecorderNormalizesRedactsAndFlushes(t *testing.T) {
	now := time.Date(2026, 8, 23, 14, 5, 6, 789_000_000, time.UTC)
	sink := &memorySink{}
	recorder := New(sink, Options{Now: func() time.Time { return now }, BatchSize: 8, FlushInterval: time.Hour})
	recorder.Record(context.Background(), Event{
		Level: Level("loud"), Source: Source("browser"), Subsystem: " api ", Name: "request.failed",
		Message: " request failed ", RequestID: " req-1 ",
		Attributes: map[string]any{
			"authorization": "Bearer should-not-survive",
			"token":         "should-not-survive-either",
			"media_url":     "https://media.invalid/video?id=7&access_token=should-not-survive&signature=also-secret",
			"source_path":   "/private/library/film.mkv",
			"token_count":   42,
		},
	})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	records := sink.snapshot()
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	got := records[0]
	if got.Level != LevelInfo || got.Source != SourceServer {
		t.Fatalf("normalized level/source = %q/%q, want info/server", got.Level, got.Source)
	}
	if got.OccurredAt != now.UnixMilli() || got.ReceivedAt != now.UnixMilli() {
		t.Fatalf("times = %d/%d, want %d", got.OccurredAt, got.ReceivedAt, now.UnixMilli())
	}
	if got.Subsystem != "api" || got.RequestID != "req-1" {
		t.Fatalf("trimmed fields = subsystem %q request %q", got.Subsystem, got.RequestID)
	}
	if got.SizeBytes <= 0 {
		t.Fatal("retained size was not accounted")
	}

	var attributes map[string]any
	if err := json.Unmarshal([]byte(got.AttributesJSON), &attributes); err != nil {
		t.Fatal(err)
	}
	if attributes["authorization"] != redacted {
		t.Fatalf("authorization = %#v, want redacted", attributes["authorization"])
	}
	if attributes["token"] != redacted {
		t.Fatalf("token = %#v, want redacted", attributes["token"])
	}
	if attributes["source_path"] != "[path]" {
		t.Fatalf("source_path = %#v, want path marker", attributes["source_path"])
	}
	if attributes["token_count"] != float64(42) {
		t.Fatalf("token_count = %#v, want preserved non-secret metric", attributes["token_count"])
	}
	mediaURL, _ := attributes["media_url"].(string)
	if strings.Contains(mediaURL, "should-not-survive") || strings.Contains(mediaURL, "also-secret") ||
		!strings.Contains(mediaURL, "%5Bredacted%5D") {
		t.Fatalf("media_url was not safely query-redacted: %q", mediaURL)
	}
}

func TestRecorderTruncationKeepsValidUTF8(t *testing.T) {
	sink := &memorySink{}
	recorder := New(sink, Options{FlushInterval: time.Hour})
	recorder.Record(context.Background(), Event{Name: "unicode", Message: strings.Repeat("界", 2000)})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	message := sink.snapshot()[0].Message
	if !strings.HasSuffix(message, "…[truncated]") || !json.Valid([]byte(`{"message":`+strconv.Quote(message)+`}`)) {
		t.Fatalf("truncated message is not valid UTF-8 JSON: %q", message)
	}
}

func TestRecorderReservesCapacityForWarnAndError(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	sink := &memorySink{block: release, entered: entered}
	recorder := New(sink, Options{Capacity: 4, BatchSize: 1, FlushInterval: time.Hour, WriteTimeout: time.Hour})

	recorder.Record(context.Background(), Event{Level: LevelInfo, Name: "normal.first"})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not begin the blocking append")
	}
	for i := 0; i < 4; i++ {
		recorder.Record(context.Background(), Event{Level: LevelInfo, Name: "normal.queued"})
	}
	recorder.Record(context.Background(), Event{Level: LevelError, Name: "priority.accepted"})
	if recorder.Dropped() == 0 {
		t.Fatal("normal saturation did not report a drop")
	}
	close(release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	foundPriority := false
	for _, record := range sink.snapshot() {
		foundPriority = foundPriority || record.Event == "priority.accepted"
	}
	if !foundPriority {
		t.Fatal("reserved priority event was lost behind saturated info traffic")
	}
}

func TestRecorderReportsPersistenceFailureWithoutReturningItToCaller(t *testing.T) {
	want := errors.New("store unavailable")
	sink := &memorySink{err: want}
	failures := make(chan error, 1)
	recorder := New(sink, Options{
		BatchSize: 1, FlushInterval: time.Hour,
		OnFailure: func(err error, count int) {
			if count != 1 {
				t.Errorf("failed batch count = %d, want 1", count)
			}
			failures <- err
		},
	})
	recorder.Record(context.Background(), Event{Name: "store.failure"})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-failures:
		if !errors.Is(got, want) {
			t.Fatalf("failure = %v, want %v", got, want)
		}
	default:
		t.Fatal("persistence failure was not reported to the stdout-only hook")
	}
}

func TestRecorderCloseRejectsLateRecords(t *testing.T) {
	sink := &memorySink{}
	recorder := New(sink, Options{FlushInterval: time.Hour})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	recorder.Record(context.Background(), Event{Name: "too.late"})
	if err := recorder.RecordDurable(context.Background(), Event{Name: "durable.too_late"}); err == nil {
		t.Fatal("durable record accepted after Close")
	}
	if recorder.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", recorder.Dropped())
	}
	if len(sink.snapshot()) != 0 {
		t.Fatal("record accepted after Close")
	}
}

func TestRecorderDefaultThresholdExcludesDebugWithoutCountingADrop(t *testing.T) {
	sink := &memorySink{}
	recorder := New(sink, Options{FlushInterval: time.Hour})
	recorder.Record(context.Background(), Event{Level: LevelDebug, Name: "debug.excluded"})
	recorder.Record(context.Background(), Event{Level: LevelInfo, Name: "info.retained"})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.Dropped() != 0 {
		t.Fatalf("dropped = %d, want 0 for intentionally filtered debug", recorder.Dropped())
	}
	records := sink.snapshot()
	if len(records) != 1 || records[0].Event != "info.retained" {
		t.Fatalf("records = %+v, want only info", records)
	}
}

func TestRecorderCanBoundedlyEnableDebug(t *testing.T) {
	sink := &memorySink{}
	recorder := New(sink, Options{MinLevel: LevelDebug, FlushInterval: time.Hour})
	recorder.Record(context.Background(), Event{Level: LevelDebug, Name: "debug.retained"})
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if records := sink.snapshot(); len(records) != 1 || records[0].Level != LevelDebug {
		t.Fatalf("records = %+v, want retained debug", records)
	}
}
