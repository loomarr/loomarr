package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

func inventorySnapshot(authority, externalItemID, externalSourceID, revision, name string, at time.Time) inventory.Snapshot {
	return inventory.Snapshot{
		Origin: inventory.OriginKey{Authority: inventory.AuthorityID(authority), ExternalItemID: externalItemID},
		Kind:   "episode",
		Observation: inventory.Observation[inventory.ItemFacts]{
			SchemaVersion: 1, ObservedAt: at,
			Coverage:  map[string]inventory.Coverage{"genres": inventory.CoverageEmpty, "sources": inventory.CoveragePresent},
			Facts:     inventory.ItemFacts{Name: name, Overview: "A broad retained synopsis", Genres: []string{}},
			Extension: json.RawMessage(`{"FutureField":{"Value":17},"ApiKey":"must-not-persist"}`),
		},
		ExternalIDs: []inventory.ExternalID{{Namespace: "tmdb", Value: externalItemID}},
		Sources: []inventory.SourceSnapshot{{
			ExternalSourceID: externalSourceID,
			Kind:             inventory.SourceLibraryOriginal,
			Revision:         revision,
			Locator: inventory.Locator{
				Authority: inventory.AuthorityID(authority), ExternalItemID: externalItemID,
				ExternalSourceID: externalSourceID,
			},
			Observation: inventory.Observation[inventory.SourceFacts]{
				SchemaVersion: 1, ObservedAt: at,
				Coverage: map[string]inventory.Coverage{"streams": inventory.CoveragePresent},
				Facts: inventory.SourceFacts{Container: "mkv", Streams: []inventory.Stream{
					{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: 1920, Height: 1080},
					{Index: 2, Kind: inventory.StreamAudio, Codec: "aac", Language: "eng", Channels: 2},
				}},
			},
		}},
	}
}

func testInventoryRoundTrip(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	snapshot := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Pilot", at)
	firstID, err := st.ApplyInventorySnapshot(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := st.ApplyInventorySnapshot(ctx, snapshot)
	if err != nil || secondID != firstID {
		t.Fatalf("idempotent apply = (%q, %v), want %q", secondID, err, firstID)
	}
	item, ok, err := st.InventoryItem(ctx, inventory.ItemRef{Origin: &snapshot.Origin})
	if err != nil || !ok {
		t.Fatalf("InventoryItem = (%+v, %v, %v), want hit", item, ok, err)
	}
	if item.ID != firstID || len(item.Origins) != 1 || len(item.Sources) != 1 ||
		item.Origins[0].Observation.Facts.Name != "Pilot" ||
		item.Origins[0].Observation.Coverage["genres"] != inventory.CoverageEmpty ||
		len(item.Sources[0].Origins[0].Observation.Facts.Streams) != 2 {
		t.Fatalf("inventory round trip = %+v", item)
	}
	var extension map[string]any
	if err := json.Unmarshal(item.Origins[0].Observation.Extension, &extension); err != nil {
		t.Fatal(err)
	}
	if extension["FutureField"] == nil {
		t.Fatalf("unknown safe field lost: %s", item.Origins[0].Observation.Extension)
	}
	if _, exists := extension["ApiKey"]; exists {
		t.Fatalf("credential survived store boundary: %s", item.Origins[0].Observation.Extension)
	}
}

func testInventoryGroundedIdentity(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	first := inventorySnapshot("library-a", "shared", "source-a", "rev-a", "Same Name", at)
	first.ExternalIDs = []inventory.ExternalID{{Namespace: "tmdb", Value: "700"}}
	firstID, err := st.ApplyInventorySnapshot(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	grounded := inventorySnapshot("library-b", "other-id", "source-b", "rev-b", "Completely Different Name", at)
	grounded.ExternalIDs = []inventory.ExternalID{{Namespace: "tmdb", Value: "700"}}
	groundedID, err := st.ApplyInventorySnapshot(ctx, grounded)
	if err != nil || groundedID != firstID {
		t.Fatalf("grounded merge = (%q, %v), want %q", groundedID, err, firstID)
	}
	item, ok, err := st.InventoryItem(ctx, inventory.ItemRef{ID: firstID})
	if err != nil || !ok || len(item.Origins) != 2 {
		t.Fatalf("grounded item = (%+v, %v, %v), want two origins", item, ok, err)
	}

	unlinked := inventorySnapshot("library-c", "unlinked", "source-c", "rev-c", "Same Name", at)
	unlinked.ExternalIDs = nil
	unlinkedID, err := st.ApplyInventorySnapshot(ctx, unlinked)
	if err != nil {
		t.Fatal(err)
	}
	if unlinkedID == firstID {
		t.Fatal("matching name merged identity without a grounded id")
	}
}

func testInventoryMeasurementRevision(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	snapshot := inventorySnapshot("library-a", "item-1", "source-1", "rev-1", "Pilot", at)
	itemID, err := st.ApplyInventorySnapshot(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := st.InventoryItem(ctx, inventory.ItemRef{ID: itemID})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := item.Sources[0].ID
	measurement := inventory.Measurement{
		SourceID: sourceID, Revision: "rev-1",
		Observation: inventory.Observation[inventory.SourceFacts]{SchemaVersion: 1, ObservedAt: at.Add(time.Minute),
			Coverage: map[string]inventory.Coverage{"streams": inventory.CoveragePresent},
			Facts:    inventory.SourceFacts{Streams: []inventory.Stream{{Index: 0, Kind: inventory.StreamAudio, Language: "jpn"}}}},
	}
	if err := st.RecordInventoryMeasurement(ctx, measurement); err != nil {
		t.Fatal(err)
	}
	item, _, err = st.InventoryItem(ctx, inventory.ItemRef{ID: itemID})
	if err != nil || item.Sources[0].Measurement == nil ||
		item.Sources[0].Measurement.Observation.Facts.Streams[0].Language != "jpn" {
		t.Fatalf("measured item = %+v, err %v", item, err)
	}

	snapshot.Sources[0].Revision = "rev-2"
	snapshot.Observation.ObservedAt = at.Add(2 * time.Minute)
	snapshot.Sources[0].Observation.ObservedAt = at.Add(2 * time.Minute)
	if _, err := st.ApplyInventorySnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	item, _, err = st.InventoryItem(ctx, inventory.ItemRef{ID: itemID})
	if err != nil || item.Sources[0].Measurement != nil {
		t.Fatalf("changed source kept stale measurement: %+v, err %v", item.Sources[0].Measurement, err)
	}
	if err := st.RecordInventoryMeasurement(ctx, measurement); !errors.Is(err, inventory.ErrSourceRevisionGone) {
		t.Fatalf("old-revision measurement error = %v, want ErrSourceRevisionGone", err)
	}
}

func testInventoryExplicitMissing(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	seen := inventorySnapshot("library-a", "seen", "source-seen", "rev-1", "Seen", at)
	missing := inventorySnapshot("library-a", "missing", "source-missing", "rev-1", "Missing", at)
	if _, err := st.ApplyInventorySnapshot(ctx, seen); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyInventorySnapshot(ctx, missing); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkInventoryUnseen(ctx, "library-a", at.Add(time.Hour), []inventory.OriginKey{seen.Origin}); err != nil {
		t.Fatal(err)
	}
	item, ok, err := st.InventoryItem(ctx, inventory.ItemRef{Origin: &missing.Origin})
	if err != nil || !ok || item.Origins[0].MissingAt.IsZero() || item.Sources[0].Origins[0].MissingAt.IsZero() {
		t.Fatalf("missing item retained = (%+v, %v, %v), want explicit missing timestamps", item, ok, err)
	}
	item, ok, err = st.InventoryItem(ctx, inventory.ItemRef{Origin: &seen.Origin})
	if err != nil || !ok || !item.Origins[0].MissingAt.IsZero() {
		t.Fatalf("seen item = (%+v, %v, %v), want present", item, ok, err)
	}
}

func testInventoryMalformedRejected(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	snapshot := inventorySnapshot("library-a", "item-1", "source-1", "rev-1", "Pilot", time.Now())
	snapshot.Sources[0].Locator.Path = "https://library.test/video?api_key=secret"
	if _, err := st.ApplyInventorySnapshot(context.Background(), snapshot); !errors.Is(err, inventory.ErrInvalid) {
		t.Fatalf("malformed inventory error = %v, want ErrInvalid", err)
	}
}

func testInventoryBoundsRejected(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	for _, tc := range []struct {
		name   string
		mutate func(*inventory.Snapshot)
	}{
		{"sources", func(snapshot *inventory.Snapshot) {
			snapshot.Sources = make([]inventory.SourceSnapshot, 129)
			for i := range snapshot.Sources {
				externalID := fmt.Sprintf("source-%d", i)
				snapshot.Sources[i] = inventory.SourceSnapshot{
					ExternalSourceID: externalID, Kind: inventory.SourceLibraryOriginal, Revision: "rev-1",
					Locator: inventory.Locator{
						Authority: snapshot.Origin.Authority, ExternalItemID: snapshot.Origin.ExternalItemID,
						ExternalSourceID: externalID,
					},
					Observation: inventory.Observation[inventory.SourceFacts]{
						SchemaVersion: 1, ObservedAt: snapshot.Observation.ObservedAt,
					},
				}
			}
		}},
		{"streams", func(snapshot *inventory.Snapshot) {
			streams := make([]inventory.Stream, 513)
			for i := range streams {
				streams[i] = inventory.Stream{Index: i, Kind: inventory.StreamAudio}
			}
			snapshot.Sources[0].Observation.Facts.Streams = streams
		}},
		{"extension bytes", func(snapshot *inventory.Snapshot) {
			snapshot.Observation.Extension = json.RawMessage(`{"payload":"` + strings.Repeat("x", 1<<20) + `"}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStore(t)
			snapshot := inventorySnapshot("library-a", "item-1", "source-1", "rev-1", "Pilot", time.Now())
			tc.mutate(&snapshot)
			if _, err := st.ApplyInventorySnapshot(context.Background(), snapshot); !errors.Is(err, inventory.ErrInvalid) {
				t.Fatalf("over-bound inventory error = %v, want ErrInvalid", err)
			}
		})
	}
}

func testInventoryAnalysisRevision(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	snapshot := inventorySnapshot("library-a", "item-1", "source-1", "rev-1", "Pilot", at)
	itemID, err := st.ApplyInventorySnapshot(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := st.InventoryItem(ctx, inventory.ItemRef{ID: itemID})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := item.Sources[0].ID
	if _, ok, err := st.InventoryAnalysis(ctx, sourceID); err != nil || ok {
		t.Fatalf("unmeasured source analysis = ok %v err %v, want absent", ok, err)
	}
	lufs, peak := -23.4, -1.2
	analysis := inventory.Analysis{
		SourceID: sourceID, Revision: "rev-1", AnalyzedAt: at.Add(time.Minute),
		Keyframes:      []inventory.Keyframe{{PTSMs: 0, Offset: 48}, {PTSMs: 2000, Offset: 90_000}},
		IntegratedLUFS: &lufs, TruePeakDBTP: &peak,
		Breaks: []inventory.Break{{AtMs: 3500, KeyframeMs: 4000, OverlapMs: 900, Confidence: 0.9}},
	}
	if err := st.RecordInventoryAnalysis(ctx, analysis); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.InventoryAnalysis(ctx, sourceID)
	if err != nil || !ok || len(got.Keyframes) != 2 || got.Keyframes[1].Offset != 90_000 ||
		got.IntegratedLUFS == nil || *got.IntegratedLUFS != lufs || len(got.Breaks) != 1 || got.Revision != "rev-1" {
		t.Fatalf("analysis = %+v ok %v err %v", got, ok, err)
	}

	// An analysis written by an older schema (version 1 trusted every chapter mark) reads as absent,
	// so the measurement job measures the source again.
	if sql, ok := st.(*sqlStore); ok {
		if _, err := sql.db.ExecContext(ctx, sql.ph(`UPDATE inventory_source_analysis SET schema_version = ? WHERE source_id = ?`),
			inventory.AnalysisSchemaVersion-1, string(sourceID)); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := st.InventoryAnalysis(ctx, sourceID); err != nil || ok {
			t.Fatalf("older-schema analysis = ok %v err %v, want absent (re-measure)", ok, err)
		}
		if err := st.RecordInventoryAnalysis(ctx, analysis); err != nil {
			t.Fatal(err)
		}
	}

	snapshot.Sources[0].Revision = "rev-2"
	snapshot.Observation.ObservedAt = at.Add(2 * time.Minute)
	snapshot.Sources[0].Observation.ObservedAt = at.Add(2 * time.Minute)
	if _, err := st.ApplyInventorySnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.InventoryAnalysis(ctx, sourceID); err != nil || ok {
		t.Fatalf("changed source kept a stale analysis: ok %v err %v", ok, err)
	}
	if err := st.RecordInventoryAnalysis(ctx, analysis); !errors.Is(err, inventory.ErrSourceRevisionGone) {
		t.Fatalf("old-revision analysis error = %v, want ErrSourceRevisionGone", err)
	}
}
