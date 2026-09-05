package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type memoryRepository struct {
	snapshot    Snapshot
	item        Item
	measurement Measurement
}

func (m *memoryRepository) ApplyInventorySnapshot(_ context.Context, snapshot Snapshot) (ItemID, error) {
	m.snapshot = snapshot
	return "item-1", nil
}
func (m *memoryRepository) InventoryItem(context.Context, ItemRef) (Item, bool, error) {
	return m.item, m.item.ID != "", nil
}
func (m *memoryRepository) RecordInventoryMeasurement(_ context.Context, measurement Measurement) error {
	m.measurement = measurement
	return nil
}
func (m *memoryRepository) MarkInventoryUnseen(context.Context, AuthorityID, time.Time, []OriginKey) error {
	return nil
}

func validSnapshot(now time.Time) Snapshot {
	return Snapshot{
		Origin: OriginKey{Authority: "library-main", ExternalItemID: "item-7"}, Kind: "episode",
		Observation: Observation[ItemFacts]{SchemaVersion: 1, ObservedAt: now, Coverage: map[string]Coverage{
			"genres": CoverageEmpty,
		}, Facts: ItemFacts{Name: "Pilot"}, Extension: json.RawMessage(`{
			"FutureProviderFact":{"Nested":true},
			"ApiKey":"secret",
			"PlaybackSessionId":"session",
			"SafeURL":"https://example.test/art/7",
			"UnsafeURL":"https://example.test/video?api_key=secret"
		}`)},
		ExternalIDs: []ExternalID{{Namespace: "tmdb", Value: "99"}},
		Sources: []SourceSnapshot{{
			ExternalSourceID: "source-1", Kind: SourceLibraryOriginal, Revision: "rev-1",
			Locator: Locator{Authority: "library-main", ExternalItemID: "item-7", ExternalSourceID: "source-1"},
			Observation: Observation[SourceFacts]{SchemaVersion: 1, ObservedAt: now,
				Coverage: map[string]Coverage{"streams": CoveragePresent},
				Facts: SourceFacts{Container: "mkv", Streams: []Stream{
					{Index: 2, Kind: StreamAudio, Language: "eng"},
					{Index: 0, Kind: StreamVideo, Codec: "h264"},
				}}},
		}},
	}
}

func TestApplySnapshotPreservesBroadFactsAndStripsOperationalSecrets(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.FixedZone("test", 3600))
	repository := &memoryRepository{}
	service := New(repository)
	if _, err := service.ApplySnapshot(context.Background(), validSnapshot(now)); err != nil {
		t.Fatal(err)
	}
	got := repository.snapshot
	if got.Observation.ObservedAt.Location() != time.UTC || got.Observation.Coverage["genres"] != CoverageEmpty {
		t.Fatalf("observation = %+v, want UTC with explicit empty genres", got.Observation)
	}
	var extension map[string]any
	if err := json.Unmarshal(got.Observation.Extension, &extension); err != nil {
		t.Fatal(err)
	}
	if extension["FutureProviderFact"] == nil || extension["SafeURL"] == nil {
		t.Fatalf("safe unknown facts were not retained: %s", got.Observation.Extension)
	}
	for _, key := range []string{"ApiKey", "PlaybackSessionId", "UnsafeURL"} {
		if _, exists := extension[key]; exists {
			t.Fatalf("secret-bearing %s survived sanitization: %s", key, got.Observation.Extension)
		}
	}
	streams := got.Sources[0].Observation.Facts.Streams
	if streams[0].Index != 0 || streams[1].Index != 2 {
		t.Fatalf("streams = %+v, want stable global-index order", streams)
	}
}

func TestValidateSnapshotRejectsCredentialsAndAmbiguousStreams(t *testing.T) {
	now := time.Now()
	for _, mutate := range []func(*Snapshot){
		func(snapshot *Snapshot) { snapshot.Sources[0].Locator.Path = "https://server/video?api_key=secret" },
		func(snapshot *Snapshot) { snapshot.Sources[0].Observation.Facts.Streams[1].Index = 2 },
		func(snapshot *Snapshot) { snapshot.Observation.Coverage["genres"] = "unknown" },
	} {
		snapshot := validSnapshot(now)
		mutate(&snapshot)
		if _, err := ValidateSnapshot(snapshot); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ValidateSnapshot error = %v, want ErrInvalid", err)
		}
	}
}

func TestResolveSourceUsesFreshExactRevisionMeasurement(t *testing.T) {
	now := time.Now().UTC()
	repository := &memoryRepository{item: Item{
		ID: "item-1", Sources: []Source{{
			ID: "source-1", ItemID: "item-1", Kind: SourceLibraryOriginal, Revision: "rev-2",
			Origins: []SourceOrigin{{
				Key:     OriginKey{Authority: "library-main", ExternalItemID: "item-7"},
				Locator: Locator{Authority: "library-main", ExternalItemID: "item-7", ExternalSourceID: "source-1"},
				Observation: Observation[SourceFacts]{SchemaVersion: 1, ObservedAt: now.Add(-time.Hour),
					Coverage: map[string]Coverage{"streams": CoveragePresent},
					Facts:    SourceFacts{Streams: []Stream{{Index: 0, Kind: StreamAudio, Language: "eng"}}}},
			}},
			Measurement: &Measurement{SourceID: "source-1", Revision: "rev-2",
				Observation: Observation[SourceFacts]{SchemaVersion: 1, ObservedAt: now,
					Coverage: map[string]Coverage{"streams": CoveragePresent},
					Facts:    SourceFacts{Streams: []Stream{{Index: 0, Kind: StreamAudio, Language: "jpn"}}}}},
		}},
	}}
	resolved, ok, err := New(repository).ResolveSource(context.Background(), SourceRequest{
		Item: ItemRef{ID: "item-1"}, Now: now, MaxAge: time.Minute, RequireStreams: true,
	})
	if err != nil || !ok {
		t.Fatalf("ResolveSource = (%+v, %v, %v), want hit", resolved, ok, err)
	}
	if got := resolved.Observation.Facts.Streams[0].Language; got != "jpn" {
		t.Fatalf("resolved language = %q, want measured jpn", got)
	}
	repository.item.Sources[0].Measurement.Revision = "rev-1"
	if _, ok, err := New(repository).ResolveSource(context.Background(), SourceRequest{
		Item: ItemRef{ID: "item-1"}, Now: now, MaxAge: time.Minute, RequireStreams: true,
	}); err != nil || ok {
		t.Fatalf("stale imported observation plus old measurement = hit %v, err %v; want miss", ok, err)
	}
}
