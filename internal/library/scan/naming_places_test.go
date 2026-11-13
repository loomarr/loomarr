package scan

import (
	"path"
	"regexp"
	"strings"
	"testing"
)

// namingPlaces are folders a household keeps a whole library under, each a catch-all that names
// nothing of what it holds: a share's "Media/", a NAS's "Media/Library/". A row's titles read the
// same however deep their library sits, so no rule reads a title by how far it is from the root.
var namingPlaces = []string{"Media/", "Media/Library/"}

// TestNamingPlaces reads each matrix row again with its whole library moved under each place.
func TestNamingPlaces(t *testing.T) {
	for _, row := range namingMatrix {
		if len(row.nfo) > 0 {
			continue // a sidecar's path is its video's
		}
		for _, place := range namingPlaces {
			t.Run(row.id+"/"+place, func(t *testing.T) {
				t.Parallel()
				moved := namingRow{id: row.id, root: row.root, files: map[string]string{}}
				for rel, want := range row.files {
					moved.files[place+rel] = want
				}
				got := namingOutcomes(t, moved)
				for rel, want := range moved.files {
					if neighbourRead(got[rel]) != neighbourRead(want) {
						t.Errorf("%s = %q, want %q", rel, got[rel], want)
					}
				}
			})
		}
	}
}

// seasonInNumber matches a season 1 written into an episode's number: "101", "1.05", "1x02".
var seasonInNumber = regexp.MustCompile(`(?:^|[^0-9])1(?:[.x]\d{2}|\d{2})(?:[^0-9]|$)`)

// TestNamingSeasons reads each matrix row's first season again as its third: "Season 1/" is
// "Season 3/", "S01E02" is "S03E02", and an episode read as season 1's is season 3's. A rule that
// reads the season's number from the folder, or from the name, reads every season's.
func TestNamingSeasons(t *testing.T) {
	for _, row := range namingMatrix {
		if len(row.nfo) > 0 {
			continue
		}
		moved := namingRow{id: row.id, root: row.root, files: map[string]string{}}
		changed := false
		for rel, want := range row.files {
			// A name that writes the season into its number, "101 - Tidewater" or "1.05 Hedge", is
			// season 1's whatever its folder says; it stays where it is.
			name := strings.ToLower(path.Base(rel))
			ownSeason := !strings.Contains(name, "s01e") && seasonInNumber.MatchString(name)
			// A season's extras move with it: "S01E02 Trailer" is skipped beside "S01E02" because
			// they sit together.
			if strings.Contains(rel, "/Season 1/") && (strings.Contains(want, " S1E") || want == "skip") && !ownSeason {
				rel = strings.Replace(rel, "/Season 1/", "/Season 3/", 1)
				rel = strings.ReplaceAll(strings.ReplaceAll(rel, "S01E", "S03E"), "s01e", "s03e")
				want, changed = strings.Replace(want, " S1E", " S3E", 1), true
			}
			moved.files[rel] = want
		}
		if !changed {
			continue
		}
		t.Run(row.id, func(t *testing.T) {
			t.Parallel()
			got := namingOutcomes(t, moved)
			for rel, want := range moved.files {
				if neighbourRead(got[rel]) != neighbourRead(want) {
					t.Errorf("%s = %q, want %q", rel, got[rel], want)
				}
			}
		})
	}
}
