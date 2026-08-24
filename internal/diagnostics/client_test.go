package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func clientTestIngestor(t *testing.T, now time.Time) (*ClientIngestor, *memorySink, *Recorder) {
	t.Helper()
	sink := &memorySink{}
	recorder := New(sink, Options{Now: func() time.Time { return now }, FlushInterval: time.Hour})
	return NewClientIngestor(recorder, func() time.Time { return now }), sink, recorder
}

func TestClientIngestorProjectsOnlyClosedEventFields(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	ingestor, sink, recorder := clientTestIngestor(t, now)
	status := 502
	accepted, err := ingestor.Ingest(t.Context(), "user_1", ClientBatch{
		Source: SourceWeb, ClientVersion: "v0.1.0-beta.4", Platform: "firefox",
		Events: []ClientObservation{{
			Event: ClientAPIFailed, OccurredAt: now.Add(-time.Second).UnixMilli(),
			RequestID: "req_1", HTTPStatus: &status,
		}},
	})
	if err != nil || accepted != 1 {
		t.Fatalf("Ingest = (%d, %v), want (1, nil)", accepted, err)
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	records := sink.snapshot()
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Source != SourceWeb || record.Level != LevelWarn || record.Subsystem != "api" ||
		record.ActorID != "user_1" || record.RequestID != "req_1" || record.Event != ClientAPIFailed {
		t.Fatalf("record projection = %+v", record)
	}
	var attributes map[string]any
	if err := json.Unmarshal([]byte(record.AttributesJSON), &attributes); err != nil {
		t.Fatal(err)
	}
	if len(attributes) != 3 || attributes["http_status"] != float64(502) ||
		attributes["client_version"] != "v0.1.0-beta.4" || attributes["platform"] != "firefox" {
		t.Fatalf("attributes = %#v", attributes)
	}
}

func TestClientIngestorRejectsWholeInvalidBatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	ingestor, sink, recorder := clientTestIngestor(t, now)
	viewer, server, wrongDrift := now.UnixMilli()-2_000, now.UnixMilli(), int64(-1_000)
	_, err := ingestor.Ingest(t.Context(), "user_1", ClientBatch{
		Source: SourceAndroidTV, ClientVersion: "0.1.0", Platform: "android_tv",
		Events: []ClientObservation{
			{Event: PlayerAttached, OccurredAt: now.UnixMilli(), PlaybackSessionID: "play_1", ChannelID: "ch_1", Transport: "media3"},
			{Event: PlayerPlayheadDrift, OccurredAt: now.UnixMilli(), PlaybackSessionID: "play_1", ChannelID: "ch_1", ViewerTimeMs: &viewer, ServerTimeMs: &server, DriftMs: &wrongDrift},
		},
	})
	if !errors.Is(err, ErrInvalidClientBatch) {
		t.Fatalf("Ingest error = %v, want ErrInvalidClientBatch", err)
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if records := sink.snapshot(); len(records) != 0 {
		t.Fatalf("invalid batch persisted %d records", len(records))
	}
}

func TestClientIngestorRejectsFieldsOutsideAnEventsContract(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	ingestor, _, recorder := clientTestIngestor(t, now)
	defer func() { _ = recorder.Close(context.Background()) }()
	_, err := ingestor.Ingest(t.Context(), "user_1", ClientBatch{
		Source: SourceWeb, ClientVersion: "dev", Platform: "chromium",
		Events: []ClientObservation{{
			Event: ClientUnhandledError, OccurredAt: now.UnixMilli(), Surface: "root",
			ErrorClass: "type_error", ErrorCode: "secret-as-extra-field",
		}},
	})
	if !errors.Is(err, ErrInvalidClientBatch) || !strings.Contains(err.Error(), "errorCode is not allowed") {
		t.Fatalf("Ingest error = %v", err)
	}
}

func TestClientDiagnosticJSONRejectsUnknownFields(t *testing.T) {
	var batch ClientBatch
	err := json.Unmarshal([]byte(`{
		"source":"web","clientVersion":"dev","platform":"chromium",
		"events":[{"event":"client.unhandled_error","occurredAt":1800000000000,
		"surface":"root","errorClass":"type_error","authorization":"Bearer secret"}]
	}`), &batch)
	if err == nil || !strings.Contains(err.Error(), "unknown field \"authorization\"") {
		t.Fatalf("Unmarshal error = %v, want unknown-field refusal", err)
	}
}

func TestClientIngestorRateLimitsPerDerivedActor(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	ingestor, _, recorder := clientTestIngestor(t, now)
	defer func() { _ = recorder.Close(context.Background()) }()
	batch := ClientBatch{Source: SourceWeb, ClientVersion: "dev", Platform: "webkit"}
	for range 20 {
		batch.Events = append(batch.Events, ClientObservation{
			Event: PlayerAttached, OccurredAt: now.UnixMilli(), PlaybackSessionID: "play_1",
			ChannelID: "ch_1", Transport: "native_hls",
		})
	}
	if accepted, err := ingestor.Ingest(t.Context(), "user_1", batch); err != nil || accepted != 20 {
		t.Fatalf("first actor batch = (%d, %v)", accepted, err)
	}
	batch.Events = batch.Events[:11]
	if _, err := ingestor.Ingest(t.Context(), "user_1", batch); !errors.Is(err, ErrClientRateLimited) {
		t.Fatalf("second actor batch error = %v, want rate limit", err)
	}
	if accepted, err := ingestor.Ingest(t.Context(), "user_2", batch); err != nil || accepted != 11 {
		t.Fatalf("independent actor batch = (%d, %v)", accepted, err)
	}
}

func TestClientIngestorDerivesMediaErrorSeverity(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	ingestor, sink, recorder := clientTestIngestor(t, now)
	fatal := true
	_, err := ingestor.Ingest(t.Context(), "user_1", ClientBatch{
		Source: SourceAndroidTV, ClientVersion: "0.1.0", Platform: "shield_tv",
		Events: []ClientObservation{{
			Event: PlayerMediaError, OccurredAt: now.UnixMilli(), PlaybackSessionID: "play_1",
			ChannelID: "ch_1", Transport: "media3", ErrorCode: "ERROR_CODE_DECODING_FAILED", Fatal: &fatal,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := sink.snapshot(); len(got) != 1 || got[0].Level != LevelError {
		t.Fatalf("records = %+v", got)
	}
}
