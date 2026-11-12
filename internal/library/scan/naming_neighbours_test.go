package scan

import (
	"path"
	"strings"
	"testing"
)

// namingNeighbours are ways a household writes the same file's name that say nothing new about it:
// another container, a release's note, dots for spaces, lower case. A row's titles read the same
// whichever way their files are written, so no rule reads only the spelling its own row has.
var namingNeighbours = []struct {
	name   string
	rename func(file string) string
}{
	{"container", func(file string) string {
		if strings.EqualFold(path.Ext(file), ".mkv") {
			return stem(file) + ".mp4"
		}
		return stem(file) + ".mkv"
	}},
	{"note", func(file string) string { return stem(file) + " [1080p]" + path.Ext(file) }},
	{"dots", func(file string) string {
		if strings.ContainsAny(file, "()[]{}") {
			return file
		}
		return strings.ReplaceAll(stem(file), " ", ".") + path.Ext(file)
	}},
	{"lower", strings.ToLower},
	{"underscores", func(file string) string {
		if strings.ContainsAny(file, "()[]{}") {
			return file
		}
		return strings.ReplaceAll(stem(file), " ", "_") + path.Ext(file)
	}},
	{"spaces", func(file string) string {
		if strings.Contains(stem(file), " ") || strings.Contains(strings.ToLower(file), "www.") {
			return file
		}
		return strings.ReplaceAll(stem(file), ".", " ") + path.Ext(file)
	}},
}

// namingNeighboursToday is what a neighbour reads today where it reads otherwise than its row, keyed
// "<row id> <neighbour> <file>". A neighbour fixed drops out.
var namingNeighboursToday = map[string]string{
	// A concert's day written with dots reads as its year; spaced, the day stays in its title. Each
	// names the one concert.
	"W11 dots Concerts/Ferrow Band - Live at Ashgrove Hall 2019.10.05.mkv":        "movie Ferrow Band - Live at Ashgrove Hall 2019",
	"W11 underscores Concerts/Ferrow Band - Live at Ashgrove Hall 2019.10.05.mkv": "movie Ferrow Band - Live at Ashgrove Hall 2019",
	// "CAM" last is a release's word in a name written with dots (releaseCut); spaced, it may be
	// the title's.
	"W14 spaces Movies/Ashgrove.CAM.avi": "movie Ashgrove CAM -",
	// An episode titled "Sample" is told from a release's sample by its capital: in lower case,
	// "thornbury s01e07 - sample" is the sample a release names so.
	"W79 lower TV/Thornbury/Season I/Thornbury S01E07 - Sample.mkv": "skip",
}

// TestNamingNeighbours reads each matrix row again with its titles' file names written each
// neighbour's way; a skipped file keeps its own name, as an extra's mark is its spelling.
func TestNamingNeighbours(t *testing.T) {
	for _, row := range namingMatrix {
		if len(row.nfo) > 0 {
			continue // a sidecar is named by its video
		}
		for _, n := range namingNeighbours {
			t.Run(row.id+"/"+n.name, func(t *testing.T) {
				t.Parallel()
				renamed := namingRow{id: row.id, files: map[string]string{}}
				from := map[string]string{}
				for rel, want := range row.files {
					to := rel
					if want != "skip" && want != "problem" {
						to = path.Join(path.Dir(rel), n.rename(path.Base(rel)))
					}
					if _, taken := renamed.files[to]; taken {
						t.Skipf("%s writes two files one way", n.name)
					}
					renamed.files[to], from[to] = want, rel
				}
				got := namingOutcomes(t, renamed)
				for to, want := range renamed.files {
					key := row.id + " " + n.name + " " + from[to]
					today, known := namingNeighboursToday[key]
					switch read := neighbourRead(got[to]); {
					case known && read == neighbourRead(want):
						t.Errorf("%s reads right now; drop it from namingNeighboursToday", key)
					case known && read != neighbourRead(today):
						t.Errorf("%s = %q, was %q (want %q)", key, got[to], today, want)
					case !known && read != neighbourRead(want):
						t.Errorf("%s as %q = %q, want %q", from[to], to, got[to], want)
					}
				}
			})
		}
	}
}

// neighbourRead is an outcome as a neighbour is held to it: its words, whatever their case or the
// separators between them, so "movie Ferrow.Lane 2019" reads as "movie Ferrow Lane 2019".
func neighbourRead(outcome string) string { return normName(outcome) }
