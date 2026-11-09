package scan

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/inventory"
)

// The report pairs the media server's files with the scanner's by path, through the path map.
// What the path map doesn't cover is explained, and a skipped trailer makes no finding; a file the
// media server lists that the scanner never found fails the gate.
func TestParityPairsFilesByPath(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.file("Lantern Keeper (2004) {tmdb-90000613}/Lantern Keeper (2004).mkv", 10)
	h.file("Lantern Keeper (2004) {tmdb-90000613}/Lantern Keeper-trailer.mkv", 10)
	h.run()

	files, roots, _, err := ParityFiles(ctx, h.st, h.inv)
	if err != nil {
		t.Fatal(err)
	}
	server := []library.ServerFile{
		{Kind: "movie", Name: "Lantern Keeper", Path: "/srv/media/Lantern Keeper (2004) {tmdb-90000613}/Lantern Keeper (2004).mkv",
			ExternalIDs: []inventory.ExternalID{{Namespace: "tmdb", Value: "90000613"}}, DurationMillis: 1_320_400},
		{Kind: "movie", Name: "Elsewhere", Path: "/elsewhere/Elsewhere.mkv"},
		{Kind: "movie", Name: "Gone Missing", Path: "/srv/media/Gone Missing (2001)/Gone Missing.mkv"},
	}
	r := Parity(server, files, library.ParsePathMap("/srv/media=>"+h.root), roots)

	if r.Paired != 1 || r.IdentityAgreed != 1 || r.DurationCompared != 1 || r.DurationAgreed != 1 {
		t.Fatalf("paired %d, identity %d, durations %d/%d; want one pair agreeing", r.Paired, r.IdentityAgreed, r.DurationAgreed, r.DurationCompared)
	}
	want := map[string]string{
		"Elsewhere":    "the path map doesn't cover it",
		"Gone Missing": "",
	}
	for _, f := range r.Findings {
		if strings.HasSuffix(f.Path, "-trailer.mkv") {
			t.Errorf("trailer: %+v, want no finding: extras are not imported", f)
			continue
		}
		explained, ok := want[f.Name]
		if !ok || f.Kind != FindingServerOnly || f.Explained != explained {
			t.Errorf("unexpected finding %+v", f)
		}
		delete(want, f.Name)
	}
	if len(want) != 0 {
		t.Errorf("findings missing for %v", want)
	}
	if r.Unexplained() != 1 || r.Passed() {
		t.Errorf("unexplained %d, passed %v; want the missing file to fail the gate", r.Unexplained(), r.Passed())
	}
}

// Identity agrees on a shared id with none contradicted. An episode bridges by its own ids even
// when the two catalogs number it differently, and otherwise by its show's ids and its numbering.
func TestParityIdentity(t *testing.T) {
	ids := func(pairs ...string) []inventory.ExternalID {
		var out []inventory.ExternalID
		for _, p := range pairs {
			ns, v, _ := strings.Cut(p, ":")
			out = append(out, inventory.ExternalID{Namespace: ns, Value: v})
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		server library.ServerFile
		file   ParityFile
		want   parityIdentity
	}{
		{"movie shares an id", library.ServerFile{Kind: "movie", ExternalIDs: ids("tmdb:90000001", "imdb:tt9000001")},
			ParityFile{ExternalIDs: ids("tmdb:90000001")}, identityAgrees},
		{"movie contradicts", library.ServerFile{Kind: "movie", ExternalIDs: ids("tmdb:90000001", "imdb:tt9000001")},
			ParityFile{ExternalIDs: ids("tmdb:90000002", "imdb:tt9000001")}, identityConflicts},
		{"scanner has no ids", library.ServerFile{Kind: "movie", ExternalIDs: ids("tmdb:90000001")},
			ParityFile{ExternalIDs: ids("file:abc")}, identityUngrounded},
		{"no id in common", library.ServerFile{Kind: "movie", ExternalIDs: ids("imdb:tt9000001")},
			ParityFile{ExternalIDs: ids("tmdb:90000001")}, identityNoCommonID},
		{"episode by its tvdb id, numbered apart",
			library.ServerFile{Kind: "episode", ExternalIDs: ids("tvdb:90000010"), Season: 0, Episode: 3},
			ParityFile{ExternalIDs: ids("tvdb:90000010"), Season: 1, Episode: 9}, identityAgrees},
		{"episode by its show and numbering",
			library.ServerFile{Kind: "episode", SeriesIDs: ids("tvdb:90000020"), Season: 2, Episode: 4},
			ParityFile{ExternalIDs: ids("tmdb:90000030"), SeriesIDs: ids("tvdb:90000020"), Season: 2, Episode: 4}, identityAgrees},
		{"episode numbered apart with no own id",
			library.ServerFile{Kind: "episode", SeriesIDs: ids("tvdb:90000020"), Season: 2, Episode: 4},
			ParityFile{SeriesIDs: ids("tvdb:90000020"), Season: 2, Episode: 5}, identityConflicts},
		{"the same Sonarr episode under another TVDB id",
			library.ServerFile{Kind: "episode", ExternalIDs: ids("sonarr:90046936", "tvdb:90004504", "tmdb:90001023")},
			ParityFile{ExternalIDs: ids("sonarr:90046936", "tvdb:90010576")}, identityRenumbered},
		{"the same Sonarr episode whose other ids name another show",
			library.ServerFile{Kind: "episode", ExternalIDs: ids("sonarr:90035846", "tvdb:90026169", "tmdb:90130766")},
			ParityFile{ExternalIDs: ids("sonarr:90035846", "tvdb:90026170", "tmdb:90046533")}, identityConflicts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, detail := identityVerdict(tc.server, tc.file); got != tc.want {
				t.Errorf("verdict %d (%s), want %d", got, detail, tc.want)
			}
		})
	}
}

// Durations within a second agree, and the gate needs 99.5% of the compared ones to.
func TestParityDurationGate(t *testing.T) {
	var server []library.ServerFile
	var files []ParityFile
	for i := range 1000 {
		path := "/m/" + strconv.Itoa(i) + ".mkv"
		tmdb := []inventory.ExternalID{{Namespace: "tmdb", Value: strconv.Itoa(90_000_000 + i)}}
		duration := int64(600_000)
		if i < 6 {
			duration += 5_000
		}
		server = append(server, library.ServerFile{Kind: "movie", Path: path, DurationMillis: duration, ExternalIDs: tmdb})
		files = append(files, ParityFile{Path: path, Kind: "movie", DurationMillis: 600_400, ExternalIDs: tmdb})
	}
	r := Parity(server, files, library.ParsePathMap("/m=>/m"), []string{"/m"})
	if r.DurationAgreed != 994 || r.Unexplained() != 0 || r.Passed() {
		t.Fatalf("agreed %d of %d, unexplained %d, passed %v; want 994, none, a failed gate",
			r.DurationAgreed, r.DurationCompared, r.Unexplained(), r.Passed())
	}
}
