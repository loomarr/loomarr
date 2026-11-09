package scan

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// A pass rewrites what the id map pairs and reports the rest; the media server's hand-made
// collections come over once, and a ready-made one only when a channel names it.
func TestReferenceRewriterRewritesWhatMapsAndImportsCollections(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0)
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "lib-film", Kind: "movie", Name: "Film"},
		{ID: "lib-other", Kind: "movie", Name: "Other", IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: "90000103"}}},
	}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{{Path: "/m/other.mkv", Root: "/m", Size: 1, ModifiedNs: 1,
		Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: "lib-other"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceLibraryIDMap(ctx, []store.LibraryIDMapping{
		{MediaID: "90000001", Kind: "movie", ItemID: "lib-film"},
		{MediaID: "90000002", Kind: "movie", Reason: "no file"},
	}, at); err != nil {
		t.Fatal(err)
	}
	// 90000003 is an id the media server dropped when it re-added its library: the id map never
	// pairs it, but its title's tmdb id names a Library title.
	for key, id := range map[provision.Key]string{
		"movie:tmdb:90000101": "90000001", "movie:tmdb:90000102": "90000002", "movie:tmdb:90000103": "90000003",
	} {
		if err := st.UpsertTitle(ctx, provision.Record{Key: key, State: provision.Available, LibraryID: id,
			Title: provision.Title{MediaType: provision.Movie, Name: "T"}}); err != nil {
			t.Fatal(err)
		}
	}
	ch := channelstore.Channel{Name: "Ch", Number: 7, Strategy: schedule.Sequential, Status: schedule.StatusLive}
	ch.ID = "ch-1"
	ch.Policy.Scope.Collections = []string{"90000201", "90000299"}
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	r := &ReferenceRewriter{
		Store: st, Now: func() time.Time { return at }, Flavor: func() string { return "emby" },
		Collections: func(context.Context) ([]ServerCollection, error) {
			return []ServerCollection{
				{ID: "90000200", Name: "Rainy days"},
				{ID: "90000201", Name: "Film Collection", FromTMDB: true},
				{ID: "90000202", Name: "Other Collection", FromTMDB: true},
			}, nil
		},
		Members: func(_ context.Context, id string) ([]string, error) {
			return []string{"90000001", "90000002"}, nil
		},
	}
	report, err := r.Rewrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Imported != 2 || !slices.Equal(report.UnmappedItems, []string{"90000002"}) ||
		!slices.Equal(report.UnmappedCollections, []string{"90000299"}) {
		t.Errorf("report = %+v", report)
	}
	owned, err := st.ListLibraryCollections(ctx)
	if err != nil || len(owned) != 2 {
		t.Fatalf("household collections = %+v, %v; want the hand-made and the named one", owned, err)
	}
	byImport := map[string]librarystore.LibraryCollection{}
	for _, c := range owned {
		byImport[c.ImportedID] = c
	}
	hand, named := byImport["90000200"], byImport["90000201"]
	if hand.Name != "Rainy days" || hand.ImportedFrom != "emby" || !slices.Equal(hand.Members, []string{"lib-film"}) {
		t.Errorf("hand-made import = %+v, want its mapped title only", hand)
	}
	got, err := st.GetChannel(ctx, "ch-1")
	if err != nil || !slices.Equal(got.Policy.Scope.Collections, []string{named.ID, "90000299"}) {
		t.Errorf("scope = %v, %v; want the imported collection, the unknown one kept", got.Policy.Scope.Collections, err)
	}
	if title, _ := st.GetTitle(ctx, "movie:tmdb:90000101"); title.LibraryID != "lib-film" {
		t.Errorf("mapped title = %q", title.LibraryID)
	}
	if title, _ := st.GetTitle(ctx, "movie:tmdb:90000103"); title.LibraryID != "lib-other" {
		t.Errorf("title with a dropped id = %q, want the Library title its tmdb id names", title.LibraryID)
	}

	// The household deletes the hand-made import; a later pass doesn't bring it back.
	if err := st.DeleteLibraryCollection(ctx, hand.ID); err != nil {
		t.Fatal(err)
	}
	if report, err := r.Rewrite(ctx); err != nil || report.Imported != 0 || report.Rewritten != 0 {
		t.Errorf("second pass = %+v, %v; want nothing to do", report, err)
	}
}
