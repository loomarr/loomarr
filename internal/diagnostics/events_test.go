package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type eventReaderFunc func(context.Context, EventStoreQuery) ([]Record, error)

func (f eventReaderFunc) QueryDiagnosticEvents(ctx context.Context, query EventStoreQuery) ([]Record, error) {
	return f(ctx, query)
}

func TestEventLogOwnsBoundsProjectionAndOpaqueCursor(t *testing.T) {
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	var captured EventStoreQuery
	reader := eventReaderFunc(func(_ context.Context, query EventStoreQuery) ([]Record, error) {
		captured = query
		return []Record{
			{ID: "diag-b", OccurredAt: now.Add(-time.Minute).UnixMilli(), ReceivedAt: now.UnixMilli(), Level: LevelError, Source: SourceServer, Event: "api.request_failed", RequestID: "req-7", AttributesJSON: `{"status":503}`},
			{ID: "diag-a", OccurredAt: now.Add(-2 * time.Minute).UnixMilli(), ReceivedAt: now.UnixMilli(), Level: LevelInfo, Source: SourceServer, Event: "api.request_rejected", AttributesJSON: `{}`},
		}, nil
	})
	log := NewEventLog(reader, func() time.Time { return now })
	page, err := log.Query(context.Background(), EventQuery{
		Limit: 1, Order: EventOrderOldest, Level: LevelError, Source: SourceServer, Subsystem: "api", RequestID: "req-7",
		ChannelID: "channel-1", Text: "failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.From != now.Add(-time.Hour).UnixMilli() || captured.To != now.UnixMilli() || captured.Limit != 2 {
		t.Fatalf("resolved bounds = %+v", captured)
	}
	if captured.Order != EventOrderOldest || captured.Level != LevelError || captured.Source != SourceServer || captured.Subsystem != "api" ||
		captured.RequestID != "req-7" || captured.ChannelID != "channel-1" || captured.Text != "failed" {
		t.Fatalf("filters not preserved: %+v", captured)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "diag-b" || page.Items[0].Attributes["status"] != float64(503) {
		t.Fatalf("page = %+v", page)
	}
	if page.NextCursor == "" {
		t.Fatal("sentinel row did not produce a cursor")
	}

	_, err = log.Query(context.Background(), EventQuery{
		From: now.Add(-time.Hour).UnixMilli(), To: now.UnixMilli(), Limit: 1, Order: EventOrderOldest, Cursor: page.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.CursorOccurredAt != page.Items[0].OccurredAt || captured.CursorID != page.Items[0].ID {
		t.Fatalf("decoded cursor = %d/%q", captured.CursorOccurredAt, captured.CursorID)
	}
	if _, err := log.Query(context.Background(), EventQuery{
		From: now.Add(-time.Hour).UnixMilli(), To: now.UnixMilli(), Limit: 1,
		Order: EventOrderNewest, Cursor: page.NextCursor,
	}); !errors.Is(err, ErrInvalidEventQuery) {
		t.Fatalf("cursor reused with another order error = %v, want ErrInvalidEventQuery", err)
	}
}

func TestEventLogRejectsUnsafeQueriesBeforeStore(t *testing.T) {
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	calls := 0
	log := NewEventLog(eventReaderFunc(func(context.Context, EventStoreQuery) ([]Record, error) {
		calls++
		return nil, nil
	}), func() time.Time { return now })
	tests := []EventQuery{
		{From: now.Add(-25 * time.Hour).UnixMilli(), To: now.UnixMilli()},
		{From: now.UnixMilli(), To: now.Add(-time.Minute).UnixMilli()},
		{Limit: 201},
		{Level: Level("fatal")},
		{Source: Source("browser")},
		{Order: EventOrder("sideways")},
		{Text: strings.Repeat("x", 257)},
		{RequestID: strings.Repeat("x", 129)},
		{Cursor: "not-base64"},
	}
	for _, query := range tests {
		if _, err := log.Query(context.Background(), query); !errors.Is(err, ErrInvalidEventQuery) {
			t.Errorf("Query(%+v) error = %v, want ErrInvalidEventQuery", query, err)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid queries reached store %d times", calls)
	}
}

func TestNDJSONIsTheSameFilteredPageAsJSON(t *testing.T) {
	page := EventPage{Items: []EventView{
		{ID: "diag-1", Level: LevelInfo, Source: SourceServer, Event: "one", Attributes: map[string]any{}},
		{ID: "diag-2", Level: LevelWarn, Source: SourceWeb, Event: "two", Attributes: map[string]any{"code": "E2"}},
	}, NextCursor: "opaque"}
	var output bytes.Buffer
	if err := WriteNDJSON(&output, page); err != nil {
		t.Fatal(err)
	}
	var decoded []EventView
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var event EventView
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, event)
	}
	if !reflect.DeepEqual(decoded, page.Items) {
		t.Fatalf("NDJSON = %+v, JSON items = %+v", decoded, page.Items)
	}
}

func TestEventLogDoesNotHideStoreOrAttributeCorruption(t *testing.T) {
	want := errors.New("store offline")
	log := NewEventLog(eventReaderFunc(func(context.Context, EventStoreQuery) ([]Record, error) {
		return nil, want
	}), time.Now)
	if _, err := log.Query(context.Background(), EventQuery{}); !errors.Is(err, want) {
		t.Fatalf("store error = %v, want wrapped %v", err, want)
	}

	log = NewEventLog(eventReaderFunc(func(context.Context, EventStoreQuery) ([]Record, error) {
		return []Record{{ID: "broken", Event: "bad", AttributesJSON: `{`}}, nil
	}), time.Now)
	if _, err := log.Query(context.Background(), EventQuery{}); err == nil {
		t.Fatal("invalid retained attributes were silently returned")
	}
}
