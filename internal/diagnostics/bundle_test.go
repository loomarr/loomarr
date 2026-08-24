package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type bundleMemory struct {
	events []Record
	runs   []ProcessRun
}

func (m *bundleMemory) QueryDiagnosticEvents(ctx context.Context, query EventStoreQuery) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var matches []Record
	for _, event := range m.events {
		if event.OccurredAt < query.From || event.OccurredAt > query.To || query.ProcessRunID != "" && event.ProcessRunID != query.ProcessRunID {
			continue
		}
		matches = append(matches, event)
	}
	if len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches, nil
}

func (m *bundleMemory) QueryDiagnosticProcessRuns(ctx context.Context, query ProcessStoreQuery) ([]ProcessRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var matches []ProcessRun
	for _, run := range m.runs {
		if run.StartedAt < query.From || run.StartedAt > query.To || query.ChannelID != "" && run.ChannelID != query.ChannelID || query.JobID != "" && run.JobID != query.JobID {
			continue
		}
		matches = append(matches, run)
	}
	if len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches, nil
}

func (m *bundleMemory) FindDiagnosticProcessRun(ctx context.Context, id string) (ProcessRun, bool, error) {
	if err := ctx.Err(); err != nil {
		return ProcessRun{}, false, err
	}
	for _, run := range m.runs {
		if run.ID == id {
			return run, true, nil
		}
	}
	return ProcessRun{}, false, nil
}

func TestSupportBundlePreviewAndDownloadShareBoundedRedactedSelection(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	secret := "super-secret-token"
	if err := os.WriteFile(filepath.Join(dir, "opaque.log"), []byte("token="+secret+" input=/srv/private/movie.mkv url=https://user:pass@example.test/live?token=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &bundleMemory{
		events: []Record{{ID: "event-1", OccurredAt: now.Add(-time.Minute).UnixMilli(), ReceivedAt: now.UnixMilli(), Level: LevelError, Source: SourceWeb,
			Event: "client.playback_error", Message: "failed at /home/person/library/file.mkv token=" + secret,
			AttributesJSON: `{"client_version":"v0.8.0","api_key":"legacy-secret","media_url":"https://u:p@example.test/a?token=raw"}`}},
		runs: []ProcessRun{{ID: "../../caller-controlled", Purpose: "playout_program", StartedAt: now.Add(-2 * time.Minute).UnixMilli(), Status: ProcessFailed,
			OutputRef: "opaque.log", Target: "/srv/private/movie.mkv", FirstError: "authorization: Bearer " + secret, DiscardedLines: 4}},
	}
	events := NewEventLog(store, func() time.Time { return now }).WithDropped(func() uint64 { return 7 })
	processes := NewProcessLog(store, ProcessReadOptions{OutputDir: dir, Now: func() time.Time { return now }})
	service := NewBundleService(BundleOptions{Events: events, Processes: processes, Now: func() time.Time { return now },
		Build: func() BundleBuild { return BundleBuild{Version: "v0.9.0", Commit: "abc123"} },
		Health: func() HealthReport {
			return HealthReport{GenerationID: "generation-1", Version: "v0.9.0", State: HealthHealthy, Checks: []HealthCheck{{Key: "media", Label: "Media", Status: HealthWarning, Detail: "token=" + secret + " at /var/lib/private"}}}
		},
	})
	selection := BundleSelection{From: now.Add(-time.Hour).UnixMilli(), To: now.UnixMilli(), Events: true, Processes: true, ProcessOutput: true, RequestID: "https://user:pass@example.test/request?token=raw"}
	preview, err := service.Preview(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Build(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Manifest.Counts.Events != result.Manifest.Counts.Events || preview.Manifest.Counts.Processes != result.Manifest.Counts.Processes || preview.Manifest.Counts.Redactions == 0 {
		t.Fatalf("preview and build diverged: %#v / %#v", preview, result.Manifest)
	}
	if result.Manifest.FinalArchiveBytes != int64(len(result.Content)) {
		t.Fatalf("final bytes = %d, content = %d", result.Manifest.FinalArchiveBytes, len(result.Content))
	}
	repeated, err := service.Build(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Content, repeated.Content) {
		t.Fatal("identical selected evidence did not produce a deterministic archive")
	}

	zr, err := zip.NewReader(bytes.NewReader(result.Content), int64(len(result.Content)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	combined := bytes.Buffer{}
	for _, file := range zr.File {
		names = append(names, file.Name)
		if strings.Contains(file.Name, "..") || strings.HasPrefix(file.Name, "/") {
			t.Fatalf("unsafe archive name %q", file.Name)
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		_, _ = io.Copy(&combined, reader)
		_ = reader.Close()
	}
	wantNames := []string{"manifest.json", "system.json", "events.ndjson", "processes/index.json", "processes/0001.json", "processes/0001.log"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("entries = %#v, want %#v", names, wantNames)
	}
	text := combined.String()
	for _, forbidden := range []string{secret, "legacy-secret", "/home/person", "/srv/private", "/var/lib/private", "user:pass"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("bundle leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "[redacted]") || !strings.Contains(text, "[path]") {
		t.Fatalf("redaction markers absent: %s", text)
	}
}

func TestSupportBundleDeclaresEventAndProcessOutputCaps(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	store := &bundleMemory{}
	for index := range maxEventLimit + 1 {
		store.events = append(store.events, Record{ID: fmt.Sprintf("event-%03d", index), OccurredAt: now.Add(-time.Minute).UnixMilli(), ReceivedAt: now.UnixMilli(), Level: LevelInfo, Source: SourceServer, Event: "test.event", AttributesJSON: `{}`})
	}
	for index := range 10 {
		name := fmt.Sprintf("run-%02d.log", index)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("bounded output\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		store.runs = append(store.runs, ProcessRun{ID: fmt.Sprintf("run-%02d", index), Purpose: "playout_program", StartedAt: now.Add(-time.Minute).UnixMilli(), Status: ProcessSucceeded, OutputRef: name})
	}
	service := NewBundleService(BundleOptions{Events: NewEventLog(store, func() time.Time { return now }), Processes: NewProcessLog(store, ProcessReadOptions{OutputDir: dir, Now: func() time.Time { return now }}), Now: func() time.Time { return now }})
	preview, err := service.Preview(t.Context(), BundleSelection{From: now.Add(-time.Hour).UnixMilli(), To: now.UnixMilli(), Events: true, Processes: true, ProcessOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Manifest.Counts.Events != maxBundleEvents || preview.Manifest.Counts.EventsOmittedAtLeast != 1 {
		t.Fatalf("event cap = %#v", preview.Manifest.Counts)
	}
	if preview.Manifest.Counts.ProcessOutputs != maxBundleProcessOutputs || preview.Manifest.Counts.ProcessOutputsOmitted != 2 {
		t.Fatalf("output cap = %#v", preview.Manifest.Counts)
	}
}

func TestSupportBundleRejectsUnsafeSelectionsAndHonorsCancellation(t *testing.T) {
	now := time.Now().UTC()
	store := &bundleMemory{}
	service := NewBundleService(BundleOptions{Events: NewEventLog(store, func() time.Time { return now }), Processes: NewProcessLog(store, ProcessReadOptions{Now: func() time.Time { return now }})})
	for _, selection := range []BundleSelection{{From: 2, To: 1, Events: true}, {From: now.Add(-25 * time.Hour).UnixMilli(), To: now.UnixMilli(), Events: true}, {From: 1, To: 2}, {From: 1, To: 2, ProcessOutput: true}, {From: 1, To: 2, Events: true, RequestID: strings.Repeat("x", 129)}} {
		if _, err := service.Preview(t.Context(), selection); !errors.Is(err, ErrInvalidBundleSelection) {
			t.Fatalf("Preview(%#v) error = %v", selection, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Build(ctx, BundleSelection{From: 1, To: 2, Events: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Build error = %v", err)
	}
}

func TestSupportBundleManifestInsideArchiveDeclaresFinalSize(t *testing.T) {
	store := &bundleMemory{}
	service := NewBundleService(BundleOptions{Events: NewEventLog(store, time.Now), Processes: NewProcessLog(store, ProcessReadOptions{}), Now: func() time.Time { return time.UnixMilli(2) }})
	result, err := service.Build(t.Context(), BundleSelection{From: 1, To: 2, Events: true})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(result.Content), int64(len(result.Content)))
	if err != nil {
		t.Fatal(err)
	}
	r, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var manifest BundleManifest
	if err := json.NewDecoder(r).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.FinalArchiveBytes != int64(len(result.Content)) {
		t.Fatalf("embedded final bytes = %d, content = %d", manifest.FinalArchiveBytes, len(result.Content))
	}
}
