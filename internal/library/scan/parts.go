package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// partToken matches the last part mark in a file's stem: "cd1", "part 2", "pt.B", "(disc 1)",
// "[dvd2]". Its groups are the words before it, its number or letter, and the words after it.
var partToken = regexp.MustCompile(`(?i)^(.*)(?:cd|dvd|part|pt|disc|disk)[ ._-]*([0-9]{1,2}|[a-d])[\)\]]?(.*)$`)

// partCount matches what follows a part's number when it says how many there are: " of 3".
var partCount = regexp.MustCompile(`(?i)^[ ._-]*of[ ._-]*\d`)

// partsOf is which of a folder's videos are parts of one movie, by name, and each one's number
// from 1. A file is a part only beside the rest of its set: two or more videos whose names are
// the same but for a part mark, numbered 1, 2, … (or A, B, …) with none missing. A name alone
// never says it, so "Kettle Part 2 (2011)", alone in its folder, is a movie of its own.
func partsOf(names []string) map[string]int {
	type part struct {
		name   string
		number int
	}
	sets := map[string][]part{}
	for _, name := range names {
		// A TV capture's parts are a broadcast's episodes (capturedEpisode).
		set, number, ok := partIn(name)
		if ok && !tvCapture.MatchString(name) {
			sets[set] = append(sets[set], part{name: name, number: number})
		}
	}
	var out map[string]int
	for _, set := range sets {
		if len(set) < 2 {
			continue
		}
		seen := make([]bool, len(set)+1)
		whole := true
		for _, p := range set {
			if p.number < 1 || p.number > len(set) || seen[p.number] {
				whole = false
				break
			}
			seen[p.number] = true
		}
		if !whole {
			continue
		}
		if out == nil {
			out = map[string]int{}
		}
		for _, p := range set {
			out[p.name] = p.number
		}
	}
	return out
}

// partFolder matches a folder named by a part mark alone: "CD1", "Disc 2", "dvd-b".
var partFolder = regexp.MustCompile(`(?i)^(?:cd|dvd|part|pt|disc|disk)[ ._-]*([0-9]{1,2}|[a-d])$`)

// partFolderOf is which part of a movie in parts the folder dir holds, from 1, when its folder
// holds the movie one part to a folder ("Ashgrove (2015)/CD1/", "…/CD2/"); 0 otherwise. The set
// is its folder's subfolders named by part marks alone: whole, numbered from 1 with none missing,
// and each holding one video. It is read from dir's own folder, so a pass over one part's folder
// alone finds it too.
func partFolderOf(dir string) int {
	if !partFolder.MatchString(filepath.Base(dir)) {
		return 0
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		return 0
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || !partFolder.MatchString(e.Name()) {
			continue
		}
		if videosIn(filepath.Join(filepath.Dir(dir), e.Name())) != 1 {
			return 0
		}
		names = append(names, e.Name())
	}
	return partsOf(names)[filepath.Base(dir)]
}

// videosIn counts the videos directly in dir, as a walk would list them.
func videosIn(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if lower := strings.ToLower(e.Name()); !e.IsDir() && videoExtensions[filepath.Ext(lower)] && !extraFile(e.Name()) {
			n++
		}
	}
	return n
}

// partsNamespace is the scanner's own id for a movie in parts: every part of one set carries the
// same value, so the parts are one title before any catalog names it. It is never listed.
const partsNamespace = "loomarr-parts"

// partsID is the id a set's parts share, from where the set is in its root.
func partsID(set string) string {
	sum := sha256.Sum256([]byte(set))
	return hex.EncodeToString(sum[:16])
}

// withoutPart is a part's stem with its part mark taken out: "Ashgrove (2015) - cd1" is
// "Ashgrove (2015)", the movie's name.
func withoutPart(stem string) string {
	m := partToken.FindStringSubmatchIndex(stem)
	if m == nil {
		return stem
	}
	before := strings.TrimRight(stem[m[2]:m[3]], " ._-([")
	after := strings.TrimLeft(stem[m[6]:m[7]], " ._-)]")
	return strings.TrimSpace(before + " " + after)
}

// partIn is the set a file's name puts it in, the name with its part mark taken out, and its part
// number; ok is false for a name with no part mark. A mark joined to the word before it
// ("Counterpart2") or the word after it ("part about") is no mark. A letter counts as its place in
// the alphabet, and a set's numbers and letters are kept apart.
func partIn(name string) (set string, number int, ok bool) {
	ext := filepath.Ext(name)
	m := partToken.FindStringSubmatchIndex(strings.TrimSuffix(name, ext))
	if m == nil {
		return "", 0, false
	}
	stem := strings.TrimSuffix(name, ext)
	before, value, after := stem[m[2]:m[3]], stem[m[4]:m[5]], stem[m[6]:m[7]]
	if r, _ := utf8.DecodeLastRuneInString(before); r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
		return "", 0, false
	}
	if r, _ := utf8.DecodeRuneInString(after); r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
		return "", 0, false
	}
	// "Part 1 of 3" counts a miniseries' episodes: a film's parts never say how many.
	if partCount.MatchString(after) {
		return "", 0, false
	}
	before = strings.TrimRight(before, " ._-([")
	kind := "n"
	if n, err := strconv.Atoi(value); err == nil {
		number = n
	} else {
		kind, number = "a", int(unicode.ToLower(rune(value[0]))-'a')+1
	}
	return strings.ToLower(before + "\x00" + kind + "\x00" + after + ext), number, true
}
