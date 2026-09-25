package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPreparedSourceAccessOpensFreshLibraryOriginalWithoutPersistingIt(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(`{
		"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
		"MediaSources":[{"Id":"source-4k","ETag":"rev-1","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"h264"},
			{"Index":1,"Type":"Audio","Language":"eng"}
		]}]
	}`)}
	r := &playoutResolver{lib: newTestLibraryClient(server), inventory: inventory.New(st), now: time.Now}

	source, hint, ok := r.ResolvePreparedSource(t.Context(), "item-1", nil)
	if !ok || source.ItemID == "" || source.SourceID == "" || source.Revision != "etag:rev-1" {
		t.Fatalf("resolved prepared source = (%+v, %v)", source, ok)
	}
	if !strings.Contains(hint, "MediaSourceId=source-4k") || !strings.Contains(hint, "api_key=") {
		t.Fatalf("transient source hint did not select the authenticated original: %q", hint)
	}
	input, err := r.OpenInput(t.Context(), source)
	if err != nil || !input.IsHTTP() {
		t.Fatalf("OpenInput = (%+v, %v)", input, err)
	}
	stable := fmt.Sprintf("%+v", source)
	if strings.Contains(stable, server.AdminToken) || strings.Contains(stable, server.URL) {
		t.Fatalf("durable prepared source exposed operational access: %s", stable)
	}
}

func TestPreparedSourceResolutionUsesLoomarrInventoryWithoutAnotherLibraryRequest(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(`{
		"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
		"MediaSources":[{"Id":"source-4k","ETag":"rev-1","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"h264"},
			{"Index":1,"Type":"Audio","Language":"eng"}
		]}]
	}`)}
	r := &playoutResolver{lib: newTestLibraryClient(server), inventory: inventory.New(st), now: time.Now}

	want, _, ok := r.ResolvePreparedSource(t.Context(), "item-1", nil)
	if !ok {
		t.Fatal("initial source import failed")
	}
	requestsAfterImport := len(server.Requests())
	got, hint, ok := r.ResolvePreparedSourceFromInventory(t.Context(), "item-1", nil)
	if !ok || got != want {
		t.Fatalf("inventory source = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	if !strings.Contains(hint, "MediaSourceId=source-4k") {
		t.Fatalf("inventory source hint did not preserve exact original: %q", hint)
	}
	if gotRequests := len(server.Requests()); gotRequests != requestsAfterImport {
		t.Fatalf("inventory resolution made %d new Library requests", gotRequests-requestsAfterImport)
	}
	if _, _, ok := r.ResolvePreparedSourceFromInventory(
		t.Context(), "item-1", library.ParsePathMap("/data=>/mnt/media"),
	); ok {
		t.Fatal("inventory Library original bypassed configured direct-disk resolution")
	}
}

func TestPreparedSourceAccessRejectsChangedLibraryRevision(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	item := func(revision string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{
			"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
			"MediaSources":[{"Id":"source-1","ETag":%q,"MediaStreams":[
				{"Index":0,"Type":"Video","Codec":"h264"},
				{"Index":1,"Type":"Audio","Language":"eng"}
			]}]
		}`, revision))
	}
	server.InventoryItems = map[string]json.RawMessage{"item-1": item("rev-1")}
	r := &playoutResolver{lib: newTestLibraryClient(server), inventory: inventory.New(st), now: time.Now}
	source, _, ok := r.ResolvePreparedSource(t.Context(), "item-1", nil)
	if !ok {
		t.Fatal("initial Library source did not resolve")
	}
	server.InventoryItems["item-1"] = item("rev-2")
	if _, err := r.OpenInput(t.Context(), source); !errors.Is(err, prepared.ErrSourceChanged) {
		t.Fatalf("changed Library OpenInput error = %v, want ErrSourceChanged", err)
	}
}

func TestPreparedSourceAccessRejectsChangedLocalRevision(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	service := inventory.New(st)
	path := t.TempDir() + "/movie.mkv"
	if err := os.WriteFile(path, []byte("first bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	revision := localInventoryRevision(info)
	itemID, err := service.ApplySnapshot(t.Context(), inventory.Snapshot{
		Origin: inventory.OriginKey{Authority: "local-test", ExternalItemID: "movie"}, Kind: "movie",
		Observation: inventory.Observation[inventory.ItemFacts]{SchemaVersion: 1, ObservedAt: time.Now()},
		Sources: []inventory.SourceSnapshot{{
			ExternalSourceID: "file", Kind: inventory.SourceLocalFile, Revision: revision,
			Locator:     inventory.Locator{Path: path},
			Observation: inventory.Observation[inventory.SourceFacts]{SchemaVersion: 1, ObservedAt: time.Now()},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, ok, err := service.Item(t.Context(), inventory.ItemRef{ID: itemID})
	if err != nil || !ok || len(item.Sources) != 1 {
		t.Fatalf("inventory item = (%+v, %v, %v)", item, ok, err)
	}
	selected := prepared.Source{
		ItemID: string(item.ID), SourceID: string(item.Sources[0].ID), Revision: revision,
	}
	r := &playoutResolver{inventory: service}
	if input, err := r.OpenInput(t.Context(), selected); err != nil || input.IsHTTP() {
		t.Fatalf("local OpenInput = (%+v, %v)", input, err)
	}
	if err := os.WriteFile(path, []byte("changed source bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.OpenInput(t.Context(), selected); !errors.Is(err, prepared.ErrSourceChanged) {
		t.Fatalf("changed local OpenInput error = %v, want ErrSourceChanged", err)
	}
}

// The prepared rendition must record whether tone-mapping applies, decided by live's own rule:
// an HDR source AND a build that can tone-map. A build without zscale prepares flat, as live does.
func TestPreparedRuntimeToneMapFollowsSourceAndBuild(t *testing.T) {
	t.Parallel()
	now := time.Unix(10_000, 0)
	for _, tc := range []struct {
		name         string
		hdr, capable bool
		want         bool
	}{
		{"HDR on a capable build tone-maps", true, true, true},
		{"SDR on a capable build does not", false, true, false},
		{"HDR without zscale prepares flat like live", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{
				"internal": {{Kind: schedule.SlotProgram, LibraryItemID: "shared", Start: now.Add(-time.Minute), Stop: now.Add(time.Hour)}},
			}}
			inputs := &preparedInputsFake{
				sources: map[string]library.InputSource{"shared": {URL: "/media/shared.mkv", Kind: library.InputFile}},
				hdr:     map[string]bool{"shared": tc.hdr},
			}
			preparedLibrary, err := prepared.NewLibrary(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			readiness, err := prepared.OpenReadiness(preparedLibrary)
			if err != nil {
				t.Fatal(err)
			}
			r := newPreparedRuntimeResolver(preparedRuntimeDependencies{
				Channels: preparedChannels{channels: []store.Channel{{Channel: schedule.Channel{ID: "internal"}}}},
				Timeline: timeline, Sources: inputs, Lookup: preparedLookupFake{}, Now: func() time.Time { return now },
				Policy: func() string { return "policy" }, GlobalBackend: func() string { return "internal" },
				Rendition: func() prepared.RenditionContract { return playout.CanonicalPreparedRendition(playout.TierBalanced) },
				Tonemap:   func() bool { return tc.capable }, Readiness: readiness,
			})
			plan, err := r.Plan(t.Context(), now, now.Add(6*time.Hour))
			if err != nil || len(plan.Candidates) != 1 {
				t.Fatalf("plan = (%+v, %v)", plan, err)
			}
			if got := plan.Candidates[0].Request.Rendition.ToneMap; got != tc.want {
				t.Fatalf("ToneMap = %v, want %v", got, tc.want)
			}
		})
	}
}

// The resolver reads HDR from the Loomarr Inventory facts it already resolved, not a second probe.
func TestPreparedSourceResolutionReportsHDRFromInventory(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{
		"hdr": json.RawMessage(`{"Id":"hdr","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
			"MediaSources":[{"Id":"s","ETag":"r","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"hevc","ColorTransfer":"smpte2084"},
			{"Index":1,"Type":"Audio","Language":"eng"}]}]}`),
		"sdr": json.RawMessage(`{"Id":"sdr","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
			"MediaSources":[{"Id":"s","ETag":"r","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"h264"},
			{"Index":1,"Type":"Audio","Language":"eng"}]}]}`),
	}
	r := &playoutResolver{lib: newTestLibraryClient(server), inventory: inventory.New(st), now: time.Now}
	for item, want := range map[string]bool{"hdr": true, "sdr": false} {
		source, _, ok := r.ResolvePreparedSource(t.Context(), item, nil)
		if !ok || source.HDR != want {
			t.Errorf("%s: source = (%+v, %v), want HDR=%v", item, source, ok, want)
		}
		fromInventory, _, ok := r.ResolvePreparedSourceFromInventory(t.Context(), item, nil)
		if !ok || fromInventory.HDR != want {
			t.Errorf("%s: inventory source = (%+v, %v), want HDR=%v", item, fromInventory, ok, want)
		}
	}
}
