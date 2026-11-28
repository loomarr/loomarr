package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Whose photo is imported (#4476), when it stops being due, and what a title's credits carry.
func testLibraryPersonPhotos(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		t.Skip("reads rows directly")
	}
	at := time.UnixMilli(1_800_000_000_000)
	person := func(id, role string) librarystore.LibraryCredit {
		return librarystore.LibraryCredit{Name: "Person " + id, Role: role,
			ID:    librarystore.LibraryID{Namespace: "tmdb", Value: id},
			Photo: "https://image.tmdb.org/t/p/original/" + id + ".jpg"}
	}
	credits := []librarystore.LibraryCredit{person("1", "actor")}
	for i := range 9 {
		credits = append(credits, librarystore.LibraryCredit{Name: fmt.Sprintf("Extra %d", i), Role: "actor"})
	}
	plain := person("6", "director")
	plain.Photo = "http://nas.example.org/6.jpg"
	// Billed eleventh and after: an actor, a writer, a director, a creator.
	credits = append(credits, person("2", "actor"), person("3", "writer"), person("4", "director"), person("5", "creator"), plain)
	items := []librarystore.LibraryItem{
		{ID: "m1", Kind: "movie", Name: "One", Credits: credits},
		{ID: "m2", Kind: "movie", Name: "No File", Credits: []librarystore.LibraryCredit{person("7", "director")}},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`INSERT INTO library_files
		(path, root, size_bytes, modified_ns, seen_at_ms, inventory_item_id) VALUES ('/m/m1.mkv', '/m', 1, 1, ?, 'm1')`),
		at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	due := func(after string, limit int) []string {
		t.Helper()
		got, err := st.DueLibraryPersonPhotos(ctx, after, limit)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range got {
			out = append(out, p.PersonID)
		}
		return out
	}
	// The top-billed actor, the director and the creator of a listed title with an https photo.
	if got := due("", 10); !slices.Equal(got, []string{"tmdb:1", "tmdb:4", "tmdb:5"}) {
		t.Fatalf("due = %v", got)
	}
	if got := due("tmdb:1", 1); !slices.Equal(got, []string{"tmdb:4"}) {
		t.Fatalf("due after the first = %v", got)
	}

	hash := fmt.Sprintf("%064d", 1)
	if err := st.PutImage(ctx, clipstore.Image{Hash: hash, Origin: "remote", Role: "poster", Visibility: "member",
		SourceURL: "https://image.tmdb.org/t/p/w185/1.jpg", CreatedAt: at, UpdatedAt: at, LastUsedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutLibraryPersonPhoto(ctx, "tmdb:1", "poster", hash,
		"https://image.tmdb.org/t/p/original/1.jpg"); err != nil {
		t.Fatal(err)
	}
	// Adopted for the URL held, the lead is no longer due.
	if got := due("", 10); !slices.Equal(got, []string{"tmdb:4", "tmdb:5"}) {
		t.Fatalf("due after adopting = %v", got)
	}
	detail, err := st.LibraryItemDetail(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Credits[0].PhotoHash != hash || detail.Credits[0].Photo != "" || detail.Credits[1].PhotoHash != "" {
		t.Fatalf("credits = %+v", detail.Credits[:2])
	}
	// Re-keyed to the bytes' hash, the ref keeps what it was adopted for, even where the person
	// already holds those bytes for an earlier URL.
	content := fmt.Sprintf("%064d", 2)
	if err := st.PutImage(ctx, clipstore.Image{Hash: content, Origin: "remote", Role: "poster", Visibility: "member",
		OriginFetchedAt: at, CreatedAt: at, UpdatedAt: at, LastUsedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutLibraryPersonPhoto(ctx, "tmdb:1", "poster", content, "https://image.example.org/old.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := st.RepointImageRefs(ctx, hash, content); err != nil {
		t.Fatal(err)
	}
	if err := st.SettleLibraryPersonPhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if got := due("", 10); !slices.Equal(got, []string{"tmdb:4", "tmdb:5"}) {
		t.Fatalf("due after the bytes landed = %v", got)
	}

	// A new photo is due again.
	changed := items[0]
	changed.Credits = []librarystore.LibraryCredit{person("1", "actor")}
	changed.Credits[0].Photo = "https://image.tmdb.org/t/p/original/new.jpg"
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{changed}, at); err != nil {
		t.Fatal(err)
	}
	if got := due("", 10); !slices.Equal(got, []string{"tmdb:1"}) {
		t.Fatalf("due after a new photo = %v", got)
	}

	// Removing a person lets their photo go.
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{{ID: "m1", Kind: "movie", Name: "One"}}, at); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PruneLibraryPeople(ctx); err != nil {
		t.Fatal(err)
	}
	held, err := st.ImagesForOwner(ctx, librarystore.LibraryPersonImageOwner, "tmdb:1")
	if err != nil || len(held) != 0 {
		t.Fatalf("a pruned person holds %v, %v", held, err)
	}
}
