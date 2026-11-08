package library_test

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/testkit"
)

// ItemDurationMs resolves a single item's runtime via GET /Items?Ids=<id>&
// Fields=RunTimeTicks (NOT the bare /Items/<id> path, which Emby 4.10 rejects
// unless user-scoped — the live-smoke bug). Returns ms from RunTimeTicks, or 0
// when the item isn't found (so the scheduler falls back, never dead air).
func TestItemDurationMs_FromRunTimeTicks(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	ms.ItemRunTimeTicks = 81786880000 // a feature film: 2h16m
	c := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")

	dur, err := c.ItemDurationMs(context.Background(), "642595")
	if err != nil {
		t.Fatal(err)
	}
	if dur != 8178688 {
		t.Errorf("duration = %d ms, want 8178688 (81786880000 ticks / 10000)", dur)
	}

	// Not found → (0, nil): the scheduler falls back to the entry's own duration.
	ms.ItemRunTimeTicks = 0
	dur, err = c.ItemDurationMs(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if dur != 0 {
		t.Errorf("missing item duration = %d, want 0", dur)
	}
}

// LibraryFolders resolves the NAME an operator typed for a filler library to its folders, as the
// media server sees them. Written against a fixture CAPTURED from Emby 4.10.0.22 —
// `GET /Library/VirtualFolders` returns a bare array, not the `{"Items": […]}` envelope.
func TestLibraryFolders_ResolvesTheNameAnOperatorTyped(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	c := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")

	for _, tc := range []struct{ name, want string }{
		{"Movies", "/data/movies"},
		// ⚠ A library with NO CollectionType, as a hand-made commercials library usually is:
		// matching must not filter on it.
		{"Mixed content", "/data/fictional-backup/emby-live-recordings"},
		// Case-insensitive: the operator retyped something they read off a screen.
		{"  tv SHOWS  ", "/data/tv"},
	} {
		got, err := c.LibraryFolders(context.Background(), tc.name)
		if err != nil || !slices.Equal(got, []string{tc.want}) {
			t.Errorf("LibraryFolders(%q) = %v, %v; want [%s]", tc.name, got, err, tc.want)
		}
	}
}

// ⚠ An unknown name is (nil, nil) — "found nothing", never an error.
func TestLibraryFolders_UnknownNameIsNotAnError(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	c := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")

	for _, name := range []string{"Commercials", "", "   "} {
		got, err := c.LibraryFolders(context.Background(), name)
		if err != nil || got != nil {
			t.Errorf("LibraryFolders(%q) = %v, %v; want nothing", name, got, err)
		}
	}
}
