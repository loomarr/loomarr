package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// A library filler source that names a media-server library is rewritten, on the next sync, to
// the household library's folder for it: the library's folder as the media server reports it,
// through the scan roots' path map (#2151 decision 36). One with no mapped folder is left as it
// is and does no work.
func TestFillerLibraryCutoverRewritesANamedLibraryToItsFolder(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	ms := testkit.NewMediaServer(t)
	lib := mediaserver.New(mediaserver.Emby, ms.URL, ms.AdminToken, "dev-1")
	at := time.Unix(1_800_000_000, 0)

	// The captured fixture's mixed library, as a hand-made commercials library usually is.
	if err := st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{
			{Authority: "emby", AuthorityPath: "/data/fictional-backup/emby-live-recordings", RootPath: "/media/ads"},
			{Authority: "emby", AuthorityPath: "/data/movies"},
		},
		Complete: []string{"emby"},
	}, at); err != nil {
		t.Fatal(err)
	}
	for _, s := range []records.FillerSource{
		records.NewFillerSource("library:Mixed content", "library", "Mixed content", "Ads", at),
		records.NewFillerSource("library:Movies", "library", "Movies", "Films", at),
	} {
		if err := st.UpsertFillerSource(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	adapter := fillerScanSourceAdapter{st: st, cutover: &fillerLibraryCutover{
		st: st, roots: st, lib: lib, log: slog.New(slog.DiscardHandler), now: func() time.Time { return at },
		warned: map[string]bool{},
	}}
	for pass := range 2 {
		listed, err := adapter.ListScanSources(ctx)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]cliplibrary.ScanSource{}
		for _, s := range listed {
			byID[s.ID] = s
		}
		if got := byID["library:Mixed content"].URI; got != "/media/ads" {
			t.Errorf("pass %d: rewritten source lists %q, want its library folder", pass, got)
		}
		if got := byID["library:Movies"].URI; got != "Movies" {
			t.Errorf("pass %d: a library with no mapped folder lists %q, want its name kept", pass, got)
		}
	}

	rows, err := st.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == "library:Mixed content" && (r.URI != "/media/ads" || r.Label != "Ads" || !r.Enabled) {
			t.Errorf("stored row = %+v, want the folder with its label and switch kept", r)
		}
	}
}
