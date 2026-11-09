package demolibrary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/library"
)

// The stand-in is only useful if Loomarr's REAL media-server client reads it correctly, so every
// test here drives internal/library against it rather than asserting on raw JSON. Loomarr scans
// the demo's files itself (#2151); it reads the media server only for its folders, users and Live
// TV, so those are what these tests drive.
func demoClient(t *testing.T, lib *demolibrary.Library) (*library.Client, demolibrary.Layout) {
	t.Helper()
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: lib, Token: "demo-token"})
	t.Cleanup(srv.Close)
	return library.New(library.Emby, srv.URL, "demo-token", "demo-test"), l
}

// Scan-root discovery finds the folders the demo's films and shows sit in.
func TestStandInNamesItsLibraryFolders(t *testing.T) {
	c, l := demoClient(t, demolibrary.Invented())
	for name, want := range map[string]string{"Movies": l.Movies(), "Shows": l.Shows()} {
		folders, err := c.LibraryFolders(context.Background(), name)
		if err != nil || len(folders) != 1 {
			t.Fatalf("%s folders = %v, %v", name, folders, err)
		}
		if abs, _ := filepath.Abs(want); folders[0] != abs {
			t.Errorf("%s folder = %q, want %q", name, folders[0], abs)
		}
	}
}

// The filler library points at the generated fillers, or at the open pack's download folder.
func TestStandInNamesTheFillerFolder(t *testing.T) {
	open, err := demolibrary.Load(demolibrary.ContentOpen)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		lib  *demolibrary.Library
		want func(demolibrary.Layout) string
	}{
		{"invented", demolibrary.Invented(), func(l demolibrary.Layout) string {
			return filepath.Dir(l.Filler(demolibrary.Fillers[0].ID))
		}},
		{"open pack", open, func(l demolibrary.Layout) string { return filepath.Dir(l.PackFiller(open.Fillers[0].ID)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, l := demoClient(t, tc.lib)
			folders, err := c.LibraryFolders(context.Background(), demolibrary.FillerLibraryName)
			if err != nil || len(folders) != 1 {
				t.Fatalf("filler library folders = %v, %v", folders, err)
			}
			if want, _ := filepath.Abs(tc.want(l)); folders[0] != want {
				t.Fatalf("filler folder = %q, want %q", folders[0], want)
			}
		})
	}
}

func TestStandInRefusesAWrongToken(t *testing.T) {
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: demolibrary.Invented(), Token: "demo-token"})
	t.Cleanup(srv.Close)
	c := library.New(library.Emby, srv.URL, "wrong", "demo-test")
	if _, err := c.ListUsers(context.Background()); err == nil {
		t.Fatal("a wrong token listed the users")
	}
	resp, err := http.Get(srv.URL + "/System/Info/Public")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public info = %d, want 200 without a token", resp.StatusCode)
	}
}
