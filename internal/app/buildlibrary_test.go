package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The app starts the scanner once it is built: a file in an in-use folder reaches the scanner's
// table with no request, and the kill switch keeps it out.
func TestBuildLibrary_StartsTheScanner(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    map[string]string
		scanned bool
	}{
		{name: "on by default", scanned: true},
		{name: "kill switch", rows: map[string]string{"library.scanner.enabled": "false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(ctx, testkit.MigratedSQLiteDSN(t), true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "Golf (2013)"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Golf (2013)", "Golf (2013).mkv"), []byte("not a header"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := st.AddLibraryRoot(ctx, librarystore.LibraryRootCheck{Path: root, Readable: true}, time.Now()); err != nil {
				t.Fatal(err)
			}

			owner := newGenerationLifecycle(ctx)
			t.Cleanup(func() { _ = owner.shutdown(context.Background()) })
			svc := buildLibrary(st, visionSet(t, tc.rows), libraryBuild{
				log: slog.New(slog.NewTextHandler(io.Discard, nil)), owner: owner,
			})
			lib, ok := svc.roots.(*scan.Library)
			if !ok || lib.Scanner == nil || svc.hooks == nil {
				t.Fatalf("library = %+v, want a *scan.Library with a scanner, and the webhooks", svc)
			}
			// No arr is connected here, so registering finds nothing to do.
			if got := svc.hooks.Statuses(); len(got) != 2 || got[0].State != scan.HookNotConfigured {
				t.Fatalf("hooks = %+v", got)
			}
			if lib.Enabled() != tc.scanned {
				t.Fatalf("Enabled = %v, want %v", lib.Enabled(), tc.scanned)
			}
			owner.startBuilt()

			// The first pass starts at once and takes milliseconds here; a second is ample to see
			// that none wrote anything.
			wait := 10 * time.Second
			if !tc.scanned {
				wait = time.Second
			}
			deadline := time.Now().Add(wait)
			for {
				counts, err := lib.Counts(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if counts.Files > 0 {
					if !tc.scanned {
						t.Fatal("the scanner wrote a file with the kill switch off")
					}
					return
				}
				if time.Now().After(deadline) {
					if tc.scanned {
						t.Fatal("the scanner did not run once the app was built")
					}
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}
