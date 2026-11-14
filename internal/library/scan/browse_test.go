package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// browseTree lays out a share: two shelves, a NAS's and an OS's own folders, a file, a link to a
// shelf inside the share and a link out of it.
func browseTree(t *testing.T) (share, outside string) {
	t.Helper()
	tmp := t.TempDir()
	share, outside = filepath.Join(tmp, "share"), filepath.Join(tmp, "outside")
	for _, dir := range []string{
		"Movies/Quillon Fen (2001)", "Movies/Tarrow Bank (1998)", "TV/Hobb Lane/Season 1",
		"@eaDir", "#recycle", ".Trash-1000", "lost+found", "Subs",
	} {
		if err := os.MkdirAll(filepath.Join(share, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"notes.txt", "Movies/loose.mkv", "Movies/loose.nfo"} {
		if err := os.WriteFile(filepath.Join(share, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(share, "TV"), filepath.Join(share, "Shows")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(share, "Escape")); err != nil {
		t.Fatal(err)
	}
	return share, outside
}

func names(b Browsed) []string {
	out := make([]string, len(b.Folders))
	for i, f := range b.Folders {
		out[i] = f.Name
	}
	return out
}

// The top lists the folders an app names first, then the mounts; inside a mount, folders only,
// in name order, without what the scan never reads, and never a link out of every mount.
func TestTheBrowserShowsFoldersUnderTheMountsOnly(t *testing.T) {
	share, outside := browseTree(t)
	tv := filepath.Join(share, "TV")
	disk := fakeDisk{mounts: []string{share}, readable: map[string]bool{tv: true}}
	r := newRoots(t, disk, "", Source{Authority: "sonarr", Folders: folders(tv)})
	ctx := context.Background()
	if _, _, err := r.Discover(ctx); err != nil {
		t.Fatal(err)
	}

	top, err := r.Browse(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(top); !slices.Equal(got, []string{tv, share}) || !top.Folders[0].Named || top.Folders[1].Named {
		t.Fatalf("top = %v (%+v), want Sonarr's folder first, then the mount", got, top.Folders)
	}

	in, err := r.Browse(ctx, share)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(in); !slices.Equal(got, []string{"Movies", "Shows", "TV"}) {
		t.Fatalf("in the share = %v, want Movies, Shows (a link inside it), TV", got)
	}
	if in.Parent != "" {
		t.Fatalf("a mount's parent = %q, want the top", in.Parent)
	}
	movies, err := r.Browse(ctx, filepath.Join(share, "Movies"))
	if err != nil {
		t.Fatal(err)
	}
	if movies.Parent != share {
		t.Fatalf("Movies' parent = %q, want the share", movies.Parent)
	}

	for _, p := range []string{outside, filepath.Join(share, "Escape"), filepath.Dir(share), "/"} {
		if _, err := r.Browse(ctx, p); !errors.Is(err, ErrNotBrowsable) {
			t.Errorf("Browse(%s) = %v, want ErrNotBrowsable", p, err)
		}
	}
	if _, err := r.Browse(ctx, "relative"); !errors.Is(err, ErrNotAbsolute) {
		t.Errorf("a relative path: %v, want ErrNotAbsolute", err)
	}
}

// Each folder's summary is counted after the list is answered, heard once, and answered from
// memory with the list after that.
func TestTheBrowserCountsEachFolderOnceInTheBackground(t *testing.T) {
	share, _ := browseTree(t)
	heard := make(chan FolderSummary, 8)
	r := newRoots(t, fakeDisk{mounts: []string{share}}, "")
	r.Summarized = func(s FolderSummary) { heard <- s }
	ctx := context.Background()

	if _, err := r.Browse(ctx, share); err != nil {
		t.Fatal(err)
	}
	got := map[string]FolderSummary{}
	for range 3 {
		select {
		case s := <-heard:
			got[filepath.Base(s.Path)] = s
		case <-time.After(5 * time.Second):
			t.Fatalf("heard %v, want three summaries", got)
		}
	}
	if m := got["Movies"]; m.State != SummaryCounted || m.Folders != 2 || m.Videos != 1 {
		t.Errorf("Movies = %+v, want 2 folders and 1 video", m)
	}
	if tv := got["TV"]; tv.Folders != 1 || tv.Videos != 0 {
		t.Errorf("TV = %+v, want 1 folder", tv)
	}

	again, err := r.Browse(ctx, share)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range again.Folders {
		if f.Summary == nil || f.Summary.State != SummaryCounted {
			t.Errorf("%s on the second look: %+v, want its summary from memory", f.Name, f.Summary)
		}
	}
	select {
	case s := <-heard:
		t.Errorf("%s was counted again", s.Path)
	case <-time.After(100 * time.Millisecond):
	}
}

// A folder inside one in use, or holding some, is refused; Replace takes a folder in place of the
// ones inside it in one step. The browser says which is which.
func TestAFolderOverlappingOneInUseIsRefused(t *testing.T) {
	share, _ := browseTree(t)
	movies, tv := filepath.Join(share, "Movies"), filepath.Join(share, "TV")
	film := filepath.Join(movies, "Quillon Fen (2001)")
	disk := fakeDisk{mounts: []string{share}, readable: map[string]bool{share: true, movies: true, tv: true, film: true}}
	r := newRoots(t, disk, "")
	ctx := context.Background()
	for _, p := range []string{movies, tv} {
		if err := r.Add(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	var overlap *RootOverlap
	if err := r.Add(ctx, film); !errors.As(err, &overlap) || overlap.Within != movies {
		t.Fatalf("a folder inside Movies: %v, want refused as part of Movies", err)
	}
	if err := r.Replace(ctx, film); !errors.As(err, &overlap) || overlap.Within != movies {
		t.Fatalf("Replace inside Movies: %v, want refused as part of Movies", err)
	}
	if err := r.Add(ctx, share); !errors.As(err, &overlap) || !slices.Equal(overlap.Holds, []string{movies, tv}) {
		t.Fatalf("the share: %v, want refused as holding Movies and TV", err)
	}

	top, err := r.Browse(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if f := top.Folders[0]; !slices.Equal(f.Holds, []string{movies, tv}) || f.InUse {
		t.Fatalf("the share at the top: %+v, want it holding Movies and TV", f)
	}
	in, err := r.Browse(ctx, movies)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range in.Folders {
		if f.Within != movies {
			t.Errorf("%s: within %q, want Movies", f.Name, f.Within)
		}
	}

	if err := r.Replace(ctx, share); err != nil {
		t.Fatal(err)
	}
	all, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var used []string
	for _, root := range all.Roots {
		if root.InUse {
			used = append(used, root.Path)
		}
	}
	if !slices.Equal(used, []string{share}) {
		t.Fatalf("in use after Replace = %v, want the share alone", used)
	}
}
