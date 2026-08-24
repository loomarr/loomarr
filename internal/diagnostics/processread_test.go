package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type processReadMemory struct {
	runs   []ProcessRun
	events []Record
}

func (m *processReadMemory) QueryDiagnosticProcessRuns(_ context.Context, query ProcessStoreQuery) ([]ProcessRun, error) {
	matches := make([]ProcessRun, 0, len(m.runs))
	for _, run := range m.runs {
		if run.StartedAt < query.From || run.StartedAt > query.To || query.Status != "" && run.Status != query.Status ||
			query.Purpose != "" && run.Purpose != query.Purpose || query.ChannelID != "" && run.ChannelID != query.ChannelID ||
			query.JobID != "" && run.JobID != query.JobID {
			continue
		}
		matches = append(matches, run)
	}
	if len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches, nil
}

func (m *processReadMemory) FindDiagnosticProcessRun(_ context.Context, id string) (ProcessRun, bool, error) {
	for _, run := range m.runs {
		if run.ID == id {
			return run, true, nil
		}
	}
	return ProcessRun{}, false, nil
}

func (m *processReadMemory) QueryDiagnosticEvents(_ context.Context, query EventStoreQuery) ([]Record, error) {
	var matches []Record
	for _, event := range m.events {
		if event.ProcessRunID == query.ProcessRunID && event.Event == query.Event {
			matches = append(matches, event)
		}
	}
	return matches, nil
}

func TestProcessLogOwnsBoundsProjectionDetailAndOutput(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run-new.log"), []byte("ffmpeg start\nframe=10\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := &processReadMemory{
		runs: []ProcessRun{
			{ID: "run-new", Purpose: "playout_program", ChannelID: "channel-1", StartedAt: now.Add(-time.Minute).UnixMilli(), Status: ProcessRunning, OutputRef: "run-new.log", OutputBytes: 22, DiscardedLines: 3},
			{ID: "run-old", Purpose: "probe", StartedAt: now.Add(-30 * time.Minute).UnixMilli(), Status: ProcessRunning},
		},
		events: []Record{{ID: "progress-1", Event: "process.progress", ProcessRunID: "run-new", OccurredAt: now.Add(-30 * time.Second).UnixMilli(), AttributesJSON: `{"frame":10,"speed":1.25,"out_time_ms":9000}`}},
	}
	log := NewProcessLog(store, ProcessReadOptions{OutputDir: dir, Now: func() time.Time { return now }})

	page, err := log.Query(t.Context(), ProcessQuery{Limit: 1, Status: ProcessRunning})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "run-new" || page.NextCursor == "" {
		t.Fatalf("page = %#v", page)
	}
	detail, err := log.Get(t.Context(), "run-new")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Progress) != 1 || detail.Progress[0].Frame != 10 || detail.Progress[0].Speed != 1.25 {
		t.Fatalf("detail = %#v", detail)
	}

	output, err := log.Output(t.Context(), "run-new")
	if err != nil {
		t.Fatal(err)
	}
	if string(output.Content) != "ffmpeg start\nframe=10\n" || output.DiscardedLines != 3 {
		t.Fatalf("output = %#v", output)
	}
}

func TestProcessLogRejectsInvalidQueriesAndUnsafeOutputRefs(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := &processReadMemory{runs: []ProcessRun{{ID: "unsafe", Purpose: "probe", StartedAt: now.UnixMilli(), Status: ProcessFailed, OutputRef: "../secret"}}}
	log := NewProcessLog(store, ProcessReadOptions{OutputDir: t.TempDir(), Now: func() time.Time { return now }})

	for _, query := range []ProcessQuery{
		{From: 2, To: 1},
		{From: now.Add(-25 * time.Hour).UnixMilli(), To: now.UnixMilli()},
		{Limit: 201},
		{Status: "mystery"},
		{Purpose: string(make([]byte, 129))},
		{Cursor: "not-base64"},
	} {
		if _, err := log.Query(t.Context(), query); !errors.Is(err, ErrInvalidProcessQuery) {
			t.Fatalf("Query(%#v) error = %v", query, err)
		}
	}
	if _, err := log.Get(t.Context(), "missing"); !errors.Is(err, ErrProcessNotFound) {
		t.Fatalf("Get missing error = %v", err)
	}
	if _, err := log.Output(t.Context(), "unsafe"); !errors.Is(err, ErrProcessOutputUnavailable) {
		t.Fatalf("Output unsafe error = %v", err)
	}
}
