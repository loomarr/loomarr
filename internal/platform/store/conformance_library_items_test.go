package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Library → Items lists what has present files, counts a series' episodes toward it, searches by
// name with LIKE's wildcards taken literally, and pages in sort order.
func testLibraryItems(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "The 100% Film", SortName: "100% film", Year: 1999},
		{ID: "m2", Kind: "movie", Name: "Another Film", Year: 2004},
		{ID: "s1", Kind: "series", Name: "A Show", SortName: "show", Year: 2001},
		{ID: "gone", Kind: "movie", Name: "Gone Film"},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
		{ID: "e2", Kind: "episode", Name: "Second"},
	}, at); err != nil {
		t.Fatal(err)
	}
	// m1's poster is the image its ref names; a fanart ref is not a poster.
	poster, fanart := imageAt("m1-poster", at), imageAt("m2-fanart", at)
	for _, img := range []clipstore.Image{poster, fanart} {
		if err := st.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []clipstore.ImageRef{
		{ImageHash: poster.Hash, OwnerKind: librarystore.LibraryItemImageOwner, OwnerID: "m1", Role: "poster"},
		{ImageHash: fanart.Hash, OwnerKind: librarystore.LibraryItemImageOwner, OwnerID: "m2", Role: "backdrop"},
	} {
		if err := st.PutImageRef(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	file := func(path, item, series, header string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	missing := file("/media/gone.mkv", "gone", "", librarystore.LibraryHeaderRead)
	missing.MissingAt = at
	problem := file("/media/bad.iso", "m2", "", librarystore.LibraryHeaderFailed)
	problem.Problem = "disc_image"
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/100.mkv", "m1", "", librarystore.LibraryHeaderRead),
		file("/media/another.mkv", "m2", "", librarystore.LibraryHeaderPending),
		file("/media/show/e1.mkv", "e1", "s1", librarystore.LibraryHeaderRead),
		file("/media/show/e2.mkv", "e2", "s1", librarystore.LibraryHeaderPending),
		missing, problem,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %+v, want three present items", page)
	}
	want := []librarystore.LibraryItemSummary{
		{LibraryItem: LibraryItem{ID: "m1", Kind: "movie", Name: "The 100% Film", SortName: "100% film", Year: 1999}, Poster: poster.Hash, Files: 1},
		// m2's fanart is its backdrop, never its poster.
		{LibraryItem: LibraryItem{ID: "m2", Kind: "movie", Name: "Another Film", SortName: "another film", Year: 2004}, Backdrop: fanart.Hash, Files: 1, Pending: 1},
		{LibraryItem: LibraryItem{ID: "s1", Kind: "series", Name: "A Show", SortName: "show", Year: 2001}, Files: 2, Pending: 1},
	}
	for i, it := range page.Items {
		if !reflect.DeepEqual(it, want[i]) {
			t.Errorf("item %d = %+v, want %+v", i, it, want[i])
		}
	}

	series, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Kind: "series"})
	if err != nil || series.Total != 1 || series.Items[0].ID != "s1" {
		t.Fatalf("series = %+v, %v", series, err)
	}
	literal, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Search: "100%"})
	if err != nil || literal.Total != 1 || literal.Items[0].ID != "m1" {
		t.Fatalf("search 100%% = %+v, %v", literal, err)
	}
	if wild, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Search: "_"}); err != nil || wild.Total != 0 {
		t.Fatalf("search _ = %+v, %v; want the underscore taken literally", wild, err)
	}
	second, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Limit: 1, Offset: 1})
	if err != nil || second.Total != 3 || len(second.Items) != 1 || second.Items[0].ID != "m2" {
		t.Fatalf("second page = %+v, %v", second, err)
	}

	one, err := st.LibraryItem(ctx, "s1")
	if err != nil || one.Files != 2 {
		t.Fatalf("item s1 = %+v, %v", one, err)
	}
	if _, err := st.LibraryItem(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an item with no present file = %v, want ErrNotFound", err)
	}
	files, err := st.LibraryItemFiles(ctx, "s1")
	if err != nil || len(files) != 2 || files[0].Path != "/media/show/e1.mkv" || files[1].SeriesID != "s1" {
		t.Fatalf("series files = %+v, %v", files, err)
	}
	// Many titles' files are one read: each title its own present files, in path order, and a
	// title with none (or no such title) has none.
	many, err := st.LibraryItemsFiles(ctx, []string{"s1", "m1", "gone", "nobody"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string][]string{}
	for id, fs := range many {
		for _, f := range fs {
			paths[id] = append(paths[id], f.Path)
		}
	}
	if want := map[string][]string{"s1": {"/media/show/e1.mkv", "/media/show/e2.mkv"}, "m1": {"/media/100.mkv"}}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("many titles' files = %v, want %v", paths, want)
	}

	// A file Loomarr moved away goes missing at once, unheld; a missing one keeps when it went.
	held := file("/media/show/e2.mkv", "e2", "s1", librarystore.LibraryHeaderPending)
	held.Held = true
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{held}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	for _, path := range []string{"/media/show/e2.mkv", "/media/gone.mkv"} {
		if err := st.RemoveLibraryFile(ctx, path, later); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.LibraryFiles(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch r.Path {
		case "/media/show/e2.mkv":
			if !r.MissingAt.Equal(later) || r.Held {
				t.Errorf("removed file = %+v, want missing at %v and unheld", r, later)
			}
		case "/media/gone.mkv":
			if !r.MissingAt.Equal(at) {
				t.Errorf("an already missing file = %+v, want it to keep %v", r, at)
			}
		}
	}
	if one, err := st.LibraryItem(ctx, "s1"); err != nil || one.Files != 1 {
		t.Errorf("series after a removal = %+v, %v; want one file", one, err)
	}
}

// A file whose header couldn't be read counts as unplayable toward its item and its series, and
// the problems list keeps just the titles that need a look: one with an unplayable file, or a movie
// with a copy that does not play, sized by its file. A skipped file (a disc image) is neither, and
// nor is a movie whose two editions each play. A broken copy beside one that plays is an extra,
// not a title that can't be played; a title whose only file is broken can't be played.
func testLibraryItemProblems(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "broken", Kind: "movie", Name: "Broken Film"},
		{ID: "twice", Kind: "movie", Name: "Twice Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/twice/a.mkv", SizeBytes: 5, Plays: true}, {Path: "/media/twice/b.mkv", SizeBytes: 30}}},
		{ID: "cuts", Kind: "movie", Name: "Cuts Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/cuts/a.mkv", SizeBytes: 5, Plays: true},
			{Path: "/media/cuts/b.mkv", SizeBytes: 7, Edition: "Extended", Plays: true}}},
		{ID: "copy", Kind: "movie", Name: "Copy Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/copy/main.mkv", SizeBytes: 5, Plays: true}, {Path: "/media/copy/uhd.mkv", SizeBytes: 9}}},
		{ID: "lone", Kind: "movie", Name: "Lone Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/lone/only.mkv", SizeBytes: 5, Plays: true}}},
		{ID: "fine", Kind: "movie", Name: "Fine Film"},
		{ID: "show", Kind: "series", Name: "A Show"},
		{ID: "shows", Kind: "series", Name: "Fine Show"},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
		{ID: "e2", Kind: "episode", Name: "Second"},
		{ID: "f1", Kind: "episode", Name: "Fine Pilot"},
		{ID: "f2", Kind: "episode", Name: "Fine Second"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	sized := func(f librarystore.LibraryFile, size int64) librarystore.LibraryFile {
		f.Size = size
		return f
	}
	disc := file("/media/fine/disc.iso", "fine", "", librarystore.LibraryHeaderFailed)
	disc.Problem = "disc_image"
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/broken.mkv", "broken", "", librarystore.LibraryHeaderFailed),
		file("/media/twice/a.mkv", "twice", "", librarystore.LibraryHeaderRead),
		sized(file("/media/twice/b.mkv", "twice", "", librarystore.LibraryHeaderRead), 30),
		file("/media/cuts/a.mkv", "cuts", "", librarystore.LibraryHeaderRead),
		file("/media/cuts/b.mkv", "cuts", "", librarystore.LibraryHeaderRead),
		file("/media/copy/main.mkv", "copy", "", librarystore.LibraryHeaderRead),
		sized(file("/media/copy/uhd.mkv", "copy", "", librarystore.LibraryHeaderFailed), 40),
		file("/media/lone/only.mkv", "lone", "", librarystore.LibraryHeaderFailed),
		file("/media/fine/fine.mkv", "fine", "", librarystore.LibraryHeaderRead),
		disc,
		file("/media/show/e1.mkv", "e1", "show", librarystore.LibraryHeaderRead),
		file("/media/show/e2.mkv", "e2", "show", librarystore.LibraryHeaderFailed),
		file("/media/shows/e1.mkv", "f1", "shows", librarystore.LibraryHeaderRead),
		file("/media/shows/e2.mkv", "f2", "shows", librarystore.LibraryHeaderRead),
	}); err != nil {
		t.Fatal(err)
	}

	if one, err := st.LibraryItem(ctx, "show"); err != nil || one.Unplayable != 1 || one.Files != 2 || one.Pending != 0 {
		t.Fatalf("show = %+v, %v; want one of two episodes unplayable", one, err)
	}
	page, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Problems: true})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range page.Items {
		got = append(got, it.ID)
	}
	// Sorted by name: A Show, Broken Film, Copy Film, Lone Film, Twice Film.
	want := []string{"show", "broken", "copy", "lone", "twice"}
	if page.Total != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %v (total %d), want %v", got, page.Total, want)
	}
	type counts struct {
		Unplayable, ExtraCopies int
		ExtraBytes              int64
	}
	for i, c := range []counts{{1, 0, 0}, {1, 0, 0}, {0, 1, 40}, {1, 0, 0}, {0, 1, 30}} {
		it := page.Items[i]
		if have := (counts{it.Unplayable, it.ExtraCopies, it.ExtraBytes}); have != c {
			t.Errorf("%s counts = %+v, want %+v", it.ID, have, c)
		}
	}
	if one, err := st.LibraryItem(ctx, "copy"); err != nil || one.Unplayable != 0 || one.ExtraCopies != 1 || one.ExtraBytes != 40 {
		t.Errorf("copy = %+v, %v; want its broken 40-byte copy an extra, nothing unplayable", one, err)
	}
	if all, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{}); err != nil || all.Total != 8 {
		t.Errorf("every title = %d, %v; want eight", all.Total, err)
	}
	if paged, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Problems: true, Limit: 1}); err != nil ||
		paged.Total != len(want) || len(paged.Items) != 1 {
		t.Errorf("a page of problems = %d of %d, %v; want one of five", len(paged.Items), paged.Total, err)
	}

	// Each file's state comes from the same rule as the counts.
	for id, want := range map[string][]librarystore.LibraryCopy{
		"broken": {{Path: "/media/broken.mkv", State: librarystore.LibraryCopyUnplayable}},
		"twice": {{Path: "/media/twice/a.mkv", State: librarystore.LibraryCopyPlays},
			{Path: "/media/twice/b.mkv", State: librarystore.LibraryCopyExtra, InPlaceOf: []string{"/media/twice/a.mkv"}}},
		"cuts": {{Path: "/media/cuts/a.mkv", State: librarystore.LibraryCopyPlays},
			{Path: "/media/cuts/b.mkv", State: librarystore.LibraryCopyPlays}},
		"copy": {{Path: "/media/copy/main.mkv", State: librarystore.LibraryCopyPlays},
			{Path: "/media/copy/uhd.mkv", State: librarystore.LibraryCopyExtra, InPlaceOf: []string{"/media/copy/main.mkv"}}},
		"lone": {{Path: "/media/lone/only.mkv", State: librarystore.LibraryCopyUnplayable}},
		"fine": {{Path: "/media/fine/fine.mkv", State: librarystore.LibraryCopyReading}},
		"show": {},
	} {
		got, err := st.LibraryItemCopies(ctx, id)
		if err != nil || len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Errorf("%s copies = %+v, %v; want %+v", id, got, err, want)
		}
	}
}

// A listed copy that does not play is an extra only while a listed copy plays in its place: one in
// its edition with its part, or a whole file of its edition for a part. A file with no source row
// yet is never an extra, and never plays in another's place.
func testLibraryItemCopiesInPlace(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	type copy struct {
		edition string
		part    int
		plays   bool
		// unsourced is a listed file with no source row yet.
		unsourced bool
	}
	cases := map[string]struct {
		copies []copy
		extras []bool
	}{
		"a second copy":             {[]copy{{plays: true}, {}}, []bool{false, true}},
		"a lone copy":               {[]copy{{}}, []bool{false}},
		"parts beside a whole":      {[]copy{{plays: true}, {part: 1}, {part: 2}}, []bool{false, true, true}},
		"a second copy of a part":   {[]copy{{part: 1, plays: true}, {part: 1}, {part: 2, plays: true}}, []bool{false, true, false}},
		"a part whose part is gone": {[]copy{{part: 1}, {part: 2, plays: true}}, []bool{false, false}},
		"another edition plays":     {[]copy{{edition: "Extended", plays: true}, {}}, []bool{false, false}},
		"no source row yet":         {[]copy{{unsourced: true}, {}}, []bool{false, false}},
		"unsourced in another edition": {
			[]copy{{edition: "Extended", unsourced: true}, {}}, []bool{false, false},
		},
		"two copies both playing": {[]copy{{plays: true}, {plays: true}}, []bool{false, false}},
		"none playing":            {[]copy{{}, {}}, []bool{false, false}},
		"a stale whole beside playing parts": {
			[]copy{{}, {part: 1, plays: true}, {part: 2, plays: true}}, []bool{false, false, false},
		},
	}
	var items []LibraryItem
	var files []librarystore.LibraryFile
	n := 0
	for name, tc := range cases {
		n++
		id := fmt.Sprintf("m%d", n)
		it := LibraryItem{ID: id, Kind: "movie", Name: name}
		for i, c := range tc.copies {
			path := fmt.Sprintf("/media/%s/%d.mkv", id, i)
			if !c.unsourced {
				it.Sources = append(it.Sources, librarystore.LibrarySource{Path: path, Edition: c.edition, Part: c.part, Plays: c.plays})
			}
			files = append(files, librarystore.LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie",
				Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: id})
		}
		items = append(items, it)
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		tc := cases[it.Name]
		got, err := st.LibraryItemCopies(ctx, it.ID)
		if err != nil || len(got) != len(tc.extras) {
			t.Fatalf("%s: copies = %+v, %v", it.Name, got, err)
		}
		extras := 0
		for i, want := range tc.extras {
			if (got[i].State == librarystore.LibraryCopyExtra) != want {
				t.Errorf("%s: file %d = %s, want extra %v", it.Name, i, got[i].State, want)
			}
			if want {
				extras++
			}
		}
		if sum, err := st.LibraryItem(ctx, it.ID); err != nil || sum.ExtraCopies != extras || sum.ExtraBytes != int64(extras) {
			t.Errorf("%s: summary = %d extras of %d bytes, %v; want %d", it.Name, sum.ExtraCopies, sum.ExtraBytes, err, extras)
		}
	}
}

// A copy that doesn't play waits while the copy that plays in its place is gone from the list or
// can't be read: it is neither an extra nor a file that can't be played.
func testLibraryItemCopiesWaiting(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "gone", Kind: "movie", Name: "Gone Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/gone/a.mkv", Plays: true}, {Path: "/media/gone/b.mkv"}}},
		{ID: "bad", Kind: "movie", Name: "Bad Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/bad/a.mkv", Plays: true}, {Path: "/media/bad/b.mkv"}}},
		{ID: "new", Kind: "movie", Name: "New Film", Sources: []librarystore.LibrarySource{
			{Path: "/media/new/a.mkv", Plays: true}}},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, header string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item}
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/gone/b.mkv", "gone", librarystore.LibraryHeaderRead),
		file("/media/bad/a.mkv", "bad", librarystore.LibraryHeaderFailed),
		file("/media/bad/b.mkv", "bad", librarystore.LibraryHeaderRead),
		file("/media/new/a.mkv", "new", librarystore.LibraryHeaderRead),
		file("/media/new/b.mkv", "new", librarystore.LibraryHeaderRead),
	}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string][]librarystore.LibraryCopy{
		"gone": {{Path: "/media/gone/b.mkv", State: librarystore.LibraryCopyWaiting}},
		"bad": {{Path: "/media/bad/a.mkv", State: librarystore.LibraryCopyUnplayable},
			{Path: "/media/bad/b.mkv", State: librarystore.LibraryCopyWaiting}},
		"new": {{Path: "/media/new/a.mkv", State: librarystore.LibraryCopyPlays},
			{Path: "/media/new/b.mkv", State: librarystore.LibraryCopyReading}},
	} {
		if got, err := st.LibraryItemCopies(ctx, id); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s copies = %+v, %v; want %+v", id, got, err, want)
		}
	}
	if bad, err := st.LibraryItem(ctx, "bad"); err != nil || bad.Unplayable != 1 || bad.ExtraCopies != 0 {
		t.Errorf("bad = %+v, %v; want its broken playing copy unplayable and no extra", bad, err)
	}
}

// "Ready so far" lists the titles whose files are all read, the last written first; a title's
// page names the channels whose lineup holds it, once each, by number.
func testLibraryItemsReadyAndChannels(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	for i, it := range []LibraryItem{
		{ID: "r1", Kind: "movie", Name: "Read First"},
		{ID: "r2", Kind: "movie", Name: "Read Last"},
		{ID: "p1", Kind: "movie", Name: "Pending"},
	} {
		if err := st.SaveLibraryItems(ctx, []LibraryItem{it}, at.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	file := func(path, item, header string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header, SeenAt: at, ItemID: item}
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/r1.mkv", "r1", librarystore.LibraryHeaderRead),
		file("/media/r2.mkv", "r2", librarystore.LibraryHeaderRead),
		file("/media/p1.mkv", "p1", librarystore.LibraryHeaderPending),
	}); err != nil {
		t.Fatal(err)
	}
	ready, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Ready: true, Recent: true, Limit: 5})
	if err != nil || ready.Total != 2 || len(ready.Items) != 2 || ready.Items[0].ID != "r2" || ready.Items[1].ID != "r1" {
		t.Fatalf("ready, recent first = %+v, %v; want r2 then r1", ready, err)
	}

	lineup := []schedule.LineupEntry{{Key: "movie:tmdb:90000021", Title: "Read First"}, {Key: "movie:tmdb:90000022", Title: "Read Last"}}
	nine := sampleChannel("ch-nine", 9, time.Time{})
	nine.Lineup = lineup
	mustSaveChannel(t, st, nine)
	four := sampleChannel("ch-four", 4, time.Time{})
	four.Lineup = lineup[:1]
	mustSaveChannel(t, st, four)
	if err := st.UpsertTitle(ctx, provision.Record{Key: lineup[0].Key, State: provision.Available, LibraryID: "r1",
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 90000021, Name: "Read First"}}); err != nil {
		t.Fatal(err)
	}
	got, err := st.LibraryItemChannels(ctx, "r1")
	if err != nil || len(got) != 2 || got[0].ID != "ch-four" || got[0].Number != 4 || got[1].ID != "ch-nine" || got[1].Name != nine.Name {
		t.Fatalf("r1's channels = %+v, %v; want ch-four then ch-nine", got, err)
	}
	// A lineup title with no library title yet (pending) names no item.
	if none, err := st.LibraryItemChannels(ctx, "r2"); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("r2's channels = %#v, %v; want none", none, err)
	}
}
