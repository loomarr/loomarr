package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSlogHandlerKeepsStdoutAndPromotesDiagnosticFields(t *testing.T) {
	var stdout bytes.Buffer
	sink := &memorySink{}
	recorder := New(sink, Options{FlushInterval: time.Hour})
	handler := NewSlogHandler(slog.NewJSONHandler(&stdout, &slog.HandlerOptions{Level: slog.LevelDebug}), recorder)
	log := slog.New(handler).With(
		"subsystem", "api", "requestId", "req-7", "event", "api.request_failed",
	).WithGroup("http")
	log.Error("request failed", "status", 503, "err", errors.New("offline"))
	log.Debug("debug only on stdout", "status", 200)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %d, want 2: %s", len(lines), stdout.String())
	}
	records := sink.snapshot()
	if len(records) != 1 {
		t.Fatalf("durable records = %d, want 1", len(records))
	}
	got := records[0]
	if got.Level != LevelError || got.Source != SourceServer || got.Event != "api.request_failed" ||
		got.Subsystem != "api" || got.RequestID != "req-7" || got.Message != "request failed" {
		t.Fatalf("promoted record = %+v", got)
	}
	var attrs map[string]any
	if err := json.Unmarshal([]byte(got.AttributesJSON), &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["http.status"] != float64(503) || attrs["http.err"] != "offline" {
		t.Fatalf("grouped attributes = %#v", attrs)
	}
}

func TestSlogHandlerPersistsInfoBelowStdoutThreshold(t *testing.T) {
	var stdout bytes.Buffer
	sink := &memorySink{}
	recorder := New(sink, Options{FlushInterval: time.Hour})
	log := slog.New(NewSlogHandler(
		slog.NewJSONHandler(&stdout, &slog.HandlerOptions{Level: slog.LevelWarn}), recorder,
	))
	log.Info("durable info")
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("info unexpectedly reached warn stdout: %s", stdout.String())
	}
	if records := sink.snapshot(); len(records) != 1 || records[0].Message != "durable info" {
		t.Fatalf("durable records = %+v", records)
	}
}

func TestSlogHandlerStoreFailureDoesNotRemoveStdout(t *testing.T) {
	var stdout bytes.Buffer
	sink := &memorySink{err: errors.New("store unavailable")}
	recorder := New(sink, Options{BatchSize: 1, FlushInterval: time.Hour})
	log := slog.New(NewSlogHandler(slog.NewJSONHandler(&stdout, nil), recorder))
	log.Warn("still visible", "event", "test.visible")
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"msg":"still visible"`) {
		t.Fatalf("stdout lost when store failed: %s", stdout.String())
	}
}
