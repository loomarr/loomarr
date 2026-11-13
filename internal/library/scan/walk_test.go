package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A walk lists media and disc images, skips by-products, extras and ignored folders, and follows a
// symlink only inside the root, listing one out of it or gone as a problem.
func TestWalkRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	touch(t, root, "India/India.mkv")
	touch(t, root, "India/India.nfo")
	touch(t, root, "India/poster.jpg")
	touch(t, root, "India/trailers/Teaser.mp4")
	touch(t, root, "India/India-featurette.mkv")
	touch(t, root, "India/backdrops/Theme.mkv")
	touch(t, root, "India/trickplay/India.mkv")
	// A show named like an extras folder, at the top of the root, is a show.
	touch(t, root, "Shorts/Season 01/Shorts S01E01.mkv")
	touch(t, root, "@eaDir/India.mkv")
	touch(t, root, "Juliet/.ignore")
	touch(t, root, "Juliet/Juliet.mkv")
	touch(t, root, "Kilo/VIDEO_TS/VTS_01_1.VOB")
	touch(t, root, "Lima/Lima.iso")
	touch(t, root, "Lima/notes.txt")
	touch(t, outside, "Mike/Mike.mkv")
	touch(t, root, "November/November.mkv")
	touch(t, root, "November/Papa.nfo")
	touch(t, root, "#recycle/Quebec.mkv")
	touch(t, root, ".Trash-1000/files/Romeo.mkv")
	touch(t, root, "lost+found/Sierra.mkv")
	if err := os.Symlink(filepath.Join(outside, "Mike"), filepath.Join(root, "Mike")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "November"), filepath.Join(root, "Oscar")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Gone", "Tango.mkv"), filepath.Join(root, "Tango.mkv")); err != nil {
		t.Fatal(err)
	}

	w, err := walkRoot(context.Background(), root, nil, nil)
	if err != nil || !w.Complete {
		t.Fatalf("walk: %v complete=%v", err, w.Complete)
	}
	got := map[string]File{}
	for _, f := range w.Files {
		got[f.Rel] = f
	}
	want := []string{
		"India/India.mkv", "Kilo/VIDEO_TS", "Lima/Lima.iso", "Mike", "November/November.mkv",
		"Shorts/Season 01/Shorts S01E01.mkv", "Tango.mkv",
	}
	var rels []string
	for rel := range got {
		rels = append(rels, rel)
	}
	slices.Sort(rels)
	if !slices.Equal(rels, want) {
		t.Fatalf("walked %v, want %v", rels, want)
	}
	// A skipped extra beside the movie does not make it share the folder: the poster is still its.
	if india := got["India/India.mkv"]; !india.Sole {
		t.Error("the movie lost its folder to a skipped extra")
	} else if _, ok := india.Sidecars[filepath.Join(root, "India", "poster.jpg")]; !ok {
		t.Errorf("folder poster not the movie's: %v", india.Sidecars)
	}
	if got["Kilo/VIDEO_TS"].Problem != ProblemDiscImage || got["Lima/Lima.iso"].Problem != ProblemDiscImage {
		t.Error("disc images not listed as problems")
	}
	// A link the walk can't follow is listed, not passed over: one out of the root, one gone.
	if got["Mike"].Problem != ProblemLinkOutside || got["Tango.mkv"].Problem != ProblemBrokenLink {
		t.Errorf("links not listed as problems: Mike %q, Tango %q", got["Mike"].Problem, got["Tango.mkv"].Problem)
	}
	if _, ok := got["India/India.mkv"].Sidecars[filepath.Join(root, "India", "India.nfo")]; !ok {
		t.Errorf("sidecars = %v", got["India/India.mkv"].Sidecars)
	}
	// A sibling's NFO says nothing about the file, so its changes do not touch it.
	if s := got["November/November.mkv"].Sidecars; len(s) != 0 {
		t.Errorf("a sibling's NFO is a sidecar: %v", s)
	}
}

// An unreadable folder makes the walk incomplete, so its files are not taken as removed.
func TestWalkUnreadableFolderIsIncomplete(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	root := t.TempDir()
	touch(t, root, "Papa/Papa.mkv")
	if err := os.Chmod(filepath.Join(root, "Papa"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "Papa"), 0o755) })
	w, err := walkRoot(context.Background(), root, nil, nil)
	if err != nil || w.Complete {
		t.Fatalf("walk: %v complete=%v", err, w.Complete)
	}
}

func TestIDsAndNumberingInNames(t *testing.T) {
	ids := idsIn("Alpha (1999) {tmdb-90000603} [imdbid-tt90000603] {tvdb-0} [tmdbid-x1]")
	want := []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000603"}, {Namespace: mp.NamespaceIMDB, Value: "tt90000603"}}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v", ids)
	}
	cases := map[string]mp.Numbering{
		"Show S01E02.mkv":       {Season: 1, Episode: 2},
		"Show.s10e100.mkv":      {Season: 10, Episode: 100},
		"Show - S02E03-E05.mkv": {Season: 2, Episode: 3, EpisodeEnd: 5},
		"Show S02E03E04.mkv":    {Season: 2, Episode: 3, EpisodeEnd: 4},
		// A file holding three or more episodes is read through its last, not its second.
		"Show.S01E01E02E03.mkv":     {Season: 1, Episode: 1, EpisodeEnd: 3},
		"Show.S01E01-E02-E03.mkv":   {Season: 1, Episode: 1, EpisodeEnd: 3},
		"Show S01E01-S01E02-S01E04": {Season: 1, Episode: 1, EpisodeEnd: 4},
		"Show 3x07 Title.mkv":       {Season: 3, Episode: 7},
		"Show S00E01 Special.mkv":   {Season: 0, Episode: 1},
	}
	for name, want := range cases {
		if got, ok := numberingIn(name); !ok || got != want {
			t.Errorf("%s = %+v %v, want %+v", name, got, ok, want)
		}
	}
	for _, name := range []string{"Movie (2001).mkv", "Movie 1080x720.mkv", "Pressed (2011).mkv"} {
		if got, ok := numberingIn(name); ok {
			t.Errorf("%s parsed as %+v", name, got)
		}
	}
}

// An episode's series folder is the one above its season folder, else the nearest named by an id,
// else its own; it is never assumed to be the root's first folder. One with no series folder takes
// the show its name gives, and a season folder never names a show.
func TestSeriesFolder(t *testing.T) {
	cases := map[string]string{
		"Kilo/Season 02/Kilo S02E01.mkv":                      "Kilo",
		"TV/Kilo {tvdb-90081190}/Season 2/Kilo S02E01.mkv":    "TV/Kilo {tvdb-90081190}",
		"TV/Kilo {tvdb-90081190}/Kilo S02E01.mkv":             "TV/Kilo {tvdb-90081190}",
		"TV/Kids/Lima {tvdb-90081191}/S01/Lima S01E01.mkv":    "TV/Kids/Lima {tvdb-90081191}",
		"TV/Kids/Lima {tvdb-90081191}/Disc 1/Lima S01E01.mkv": "TV/Kids/Lima {tvdb-90081191}",
		"Mike/Mike S01E01.mkv":                                "Mike",
		"Mike S01E01.mkv":                                     "Mike",
		"Season 1/Mike S01E01.mkv":                            "Mike",
		"November.Show.S01E02.mkv":                            "November Show",
		"Oscar_Show - 1x03 - Pilot.mkv":                       "Oscar Show",
		"S01E02.mkv":                                          "",
		"Season 1/S01E02.mkv":                                 "",
	}
	for rel, want := range cases {
		if got := seriesFolder(rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

// A file named by the day it aired, in a folder, is an episode of that folder's show, placed by the
// day. A season and number in the name stand over a day; a day that is no real date, or a dated file
// loose at the top with no show folder, leaves the file what it was.
func TestAFileNamedByItsAirDateIsAnEpisode(t *testing.T) {
	for _, tc := range []struct {
		rel, day, seriesDir string
		kind                mp.Kind
	}{
		{"Papa Hour/Papa Hour - 2013-02-15 - February 15, 2013.mkv", "2013-02-15", "Papa Hour", mp.KindEpisode},
		{"Papa Hour {tmdb-90000802}/Papa.Hour.2014.03.07.mkv", "2014-03-07", "Papa Hour {tmdb-90000802}", mp.KindEpisode},
		{"Papa Hour/Season 2014/Papa Hour 2014 03 21.mkv", "2014-03-21", "Papa Hour", mp.KindEpisode},
		{"Papa Hour/Papa Hour S02E05 2013-02-15.mkv", "", "Papa Hour", mp.KindEpisode},
		{"Papa Hour/Papa Hour - 2013-02-30.mkv", "", "", mp.KindMovie},
		{"Papa Hour/Papa Hour - 2013-13-01.mkv", "", "", mp.KindMovie},
		{"Papa Hour - 2013-02-15.mkv", "", "", mp.KindMovie},
		{"Quebec (2013)/Quebec (2013).mkv", "", "", mp.KindMovie},
		{"Quebec (2013)/Quebec.2013.1080p.mkv", "", "", mp.KindMovie},
	} {
		rec, seriesDir := embeddedRecord(File{Rel: tc.rel})
		id := identity{kind: rec.Kind, embedded: rec}
		if rec.Kind != tc.kind || id.airDate() != tc.day || seriesDir != tc.seriesDir {
			t.Errorf("%s = %v day %q in %q, want %v day %q in %q", tc.rel, rec.Kind, id.airDate(), seriesDir,
				tc.kind, tc.day, tc.seriesDir)
		}
	}
}

// A root whose top-level count came back empty (its listing took longer than the count waits, as a
// busy share's can) is counted when the walk lists it: step 1 shows its folders, rather than reading
// full while the root's every file is still to come (a 533-folder TV root read "2,268 of 2,268"
// for minutes).
func TestStep1CountsARootItsEarlyCountMissed(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Tango", "Uniform", "Victor", "Whiskey"} {
		touch(t, root, name+"/"+name+" S01E01.mkv")
	}
	tr := NewTracker(nil, nil)
	tr.Begin(StepWalking, 0)
	other := &walkReport{tracker: tr, name: "Movies", entries: 6}
	tr.Grow(StepWalking, other.entries)
	other.listed(6)
	other.wrote(6)

	late := &walkReport{tracker: tr, name: "TV"} // its count timed out: 0 entries
	if _, err := walkRoot(context.Background(), root, nil, late); err != nil {
		t.Fatal(err)
	}
	got := tr.Snapshot(time.Now())
	if got.Total != 10 {
		t.Fatalf("step 1 total = %d after the late root was listed, want 10 (6 + 4)", got.Total)
	}
	if got.Done >= got.Total {
		t.Errorf("step 1 = %d of %d before the late root's files are written, want it short of full", got.Done, got.Total)
	}
	late.writing()
	late.wrote(4)
	if got := tr.Snapshot(time.Now()); got.Done != 10 {
		t.Errorf("step 1 done = %d once the late root is written, want 10", got.Done)
	}
}

// A root that holds a few big folders ("/media" with Movies and TV) is counted by what is in them:
// step 1's bar moves a movie or show at a time, not 0, half, then full over hours.
func TestStep1CountsAFewBigFoldersByWhatTheyHold(t *testing.T) {
	root := t.TempDir()
	for i := range 30 {
		name := fmt.Sprintf("Lima %02d (2001)", i)
		touch(t, root, "Movies/"+name+"/"+name+".mkv")
	}
	for i := range 20 {
		touch(t, root, fmt.Sprintf("TV/Mike %02d/Season 01/Mike %02d S01E01.mkv", i, i))
	}
	touch(t, root, "readme.txt")
	var mu sync.Mutex
	seen := map[int]bool{}
	var tr *Tracker
	tr = NewTracker(nil, func(bool) {
		s := tr.Snapshot(time.Now())
		mu.Lock()
		seen[s.Done] = true
		mu.Unlock()
	})
	tr.Begin(StepWalking, 0)
	report := &walkReport{tracker: tr, name: "media"}
	walk, err := walkRoot(context.Background(), root, nil, report)
	if err != nil {
		t.Fatal(err)
	}
	if got := tr.Snapshot(time.Now()); got.Total != 51 {
		t.Fatalf("step 1 total = %d, want 51: 30 movies, 20 shows and the loose file", got.Total)
	}
	report.writing()
	for _, f := range walk.Files {
		report.wrote(f.Top)
	}
	report.close()
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 25 {
		t.Errorf("step 1 moved through %d values, want it to move a folder at a time", len(seen))
	}
	if got := tr.Snapshot(time.Now()); got.Done != 51 {
		t.Errorf("step 1 done = %d once written, want 51", got.Done)
	}
}

// A movie takes ids from its own name or folder, the one above a part's own folder: not from a
// collection or series folder above it, and never a tvdb id.
func TestMovieIDsComeFromItsOwnFolder(t *testing.T) {
	cases := map[string][]mp.ExternalID{
		"Saga {tmdb-90000700}/November (2001)/November (2001).mkv":   nil,
		"November (2001) {tmdb-90000703}/CD1/nov-a.avi":              {{Namespace: mp.NamespaceTMDB, Value: "90000703"}},
		"Saga {tmdb-90000700}/November (2001)/CD1/nov-a.avi":         nil,
		"Saga/November (2001) {tmdb-90000701}/November (2001).mkv":   {{Namespace: mp.NamespaceTMDB, Value: "90000701"}},
		"Saga/November (2001)/November (2001) {imdb-tt90000702}.mkv": {{Namespace: mp.NamespaceIMDB, Value: "tt90000702"}},
		"Oscar {tvdb-90081192}/Oscar.mkv":                            nil,
	}
	for rel, want := range cases {
		rec, _ := embeddedRecord(File{Rel: rel})
		if rec.Kind != mp.KindMovie || !slices.Equal(rec.IDs, want) {
			t.Errorf("%s = %v %v, want %v", rel, rec.Kind, rec.IDs, want)
		}
	}
}
