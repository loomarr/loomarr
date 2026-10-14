package suggest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// An owned pick carries its library file size as an exact fact, read from the grounded
// candidate at proposal creation (#1817). A title that still has to be acquired has no
// size: its fields are absent on the wire, never a 0 that reads as an empty file.
func TestFromCandidate_LibrarySize(t *testing.T) {
	owned := fromCandidate(catalog.Candidate{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol",
		InLibrary: true, LibraryItemID: "lib-90000603", SizeBytes: 21_914_038_799,
	}, "", 0)
	if owned.SizeBytes != 21_914_038_799 || owned.SizeConfidence != storagegovernor.SizeExact {
		t.Errorf("owned pick = %d bytes (%q), want 21914038799 exact", owned.SizeBytes, owned.SizeConfidence)
	}

	cases := map[string]catalog.Candidate{
		// A series has no media source of its own; summing episodes would need new calls.
		"owned series": {MediaType: provision.Series, TMDBID: 90063639, Name: "Ferrocene Station", InLibrary: true, LibraryItemID: "lib-1"},
		"acquisition":  {MediaType: provision.Movie, TMDBID: 90000604, Name: "Marigold Protocol II"},
		// A size on a title we do not own is not a library fact; it must not be shown as one.
		"stray size": {MediaType: provision.Movie, TMDBID: 90000605, Name: "Marigold Protocol III", SizeBytes: 9},
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			item := fromCandidate(candidate, "", 0)
			if item.SizeBytes != 0 || item.SizeConfidence != "" {
				t.Errorf("size = %d (%q), want unavailable", item.SizeBytes, item.SizeConfidence)
			}
			blob, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(blob), "size") {
				t.Errorf("unavailable size reached the wire: %s", blob)
			}
		})
	}
}
