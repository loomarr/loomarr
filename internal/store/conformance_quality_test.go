package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/quality"
)

func testDiscoveryQualityLedger(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	snapshot := quality.RunSnapshot{
		ID:                  "run-v1",
		SchemaVersion:       1,
		CorpusVersion:       "corpus-v1",
		RequestedModel:      "planner-requested",
		ResolvedModel:       "planner-resolved",
		Provider:            quality.ProviderOpenRouter,
		BudgetProfile:       "bounded-v1",
		ApplicationVersion:  "0.1.0-test",
		AccountingAvailable: true,
		CreatedAt:           createdAt,
	}
	if err := s.PutQualityRunSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := s.PutQualityRunSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent snapshot put: %v", err)
	}
	conflict := snapshot
	conflict.ResolvedModel = "different-model"
	if err := s.PutQualityRunSnapshot(ctx, conflict); !errors.Is(err, ErrQualitySnapshotConflict) {
		t.Fatalf("snapshot conflict = %v, want ErrQualitySnapshotConflict", err)
	}

	at := time.Date(2026, time.February, 1, 10, 0, 0, 0, time.UTC)
	observation := quality.Observation{
		IdempotencyKey: "private-title-must-not-export:proposal-123",
		At:             at,
		Stage:          quality.StageRetrieval,
		Outcome:        quality.OutcomeSucceeded,
		Duration:       1500 * time.Millisecond,
		ToolCalls:      2,
		CandidateCount: 12,
		CostNanos:      40,
		RunSnapshotID:  snapshot.ID,
	}
	if err := s.RecordQualityObservation(ctx, observation); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordQualityObservation(ctx, observation); err != nil {
		t.Fatalf("idempotent observation record: %v", err)
	}
	second := observation
	second.IdempotencyKey = "second-private-key"
	second.Duration = 500 * time.Millisecond
	second.ToolCalls = 1
	second.CandidateCount = 4
	second.CostNanos = 10
	if err := s.RecordQualityObservation(ctx, second); err != nil {
		t.Fatal(err)
	}
	missing := observation
	missing.IdempotencyKey = "missing-snapshot"
	missing.RunSnapshotID = "not-there"
	if err := s.RecordQualityObservation(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing snapshot = %v, want ErrNotFound", err)
	}

	now := at.Add(31 * 24 * time.Hour)
	if err := s.MaintainQualityLedger(ctx, now); err != nil {
		t.Fatal(err)
	}
	exported, err := s.ExportQualityLedger(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if exported.SchemaVersion != 1 || len(exported.Aggregates) != 1 || len(exported.RunSnapshots) != 1 {
		t.Fatalf("export shape = %+v", exported)
	}
	aggregate := exported.Aggregates[0]
	if aggregate.Day != "2026-02-01" || aggregate.Count != 2 || aggregate.DurationMillis != 2000 ||
		aggregate.ToolCalls != 3 || aggregate.CandidateCount != 16 || aggregate.CostNanos != 50 {
		t.Fatalf("aggregate = %+v", aggregate)
	}
	encoded, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-title", "proposal-123", "idempotency"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("sanitized export contains %q: %s", forbidden, encoded)
		}
	}

	if err := s.MaintainQualityLedger(ctx, now); err != nil {
		t.Fatal(err)
	}
	exported, err = s.ExportQualityLedger(ctx, now)
	if err != nil || exported.Aggregates[0].Count != 2 {
		t.Fatalf("repeat maintenance changed aggregate: export=%+v err=%v", exported, err)
	}

	if err := s.MaintainQualityLedger(ctx, now.AddDate(3, 0, 0)); err != nil {
		t.Fatal(err)
	}
	exported, err = s.ExportQualityLedger(ctx, now.AddDate(3, 0, 0))
	if err != nil || len(exported.Aggregates) != 0 || len(exported.RunSnapshots) != 0 {
		t.Fatalf("expired export = %+v err=%v", exported, err)
	}
}
