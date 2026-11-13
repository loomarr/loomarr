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
//
// folder is the folder's name, "" for none. In a film's own dated folder, "Wexcombe (2008)/", its
// title then "Part 1" is the film's part: a sequel would give its own year, and this one has the
// folder's.
func partsOf(folder string, names []string) map[string]int {
	type part struct {
		name   string
		number int
	}
	title, year, _ := searchQuery(folder, 0)
	sets := map[string][]part{}
	for _, name := range names {
		// A TV capture's parts are a broadcast's episodes (capturedEpisode).
		set, number, ok := partIn(name)
		if m := titledPartWord.FindStringSubmatch(plainStem(name)); !ok && m != nil && year != 0 && title != "" &&
			sameWords(m[1], title) && !yearAnywhere.MatchString(plainStem(name)) {
			set, number, ok = partIn(m[1] + " - Part " + m[2] + filepath.Ext(name))
		}
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

// titledPartWord matches a title then "Part" and its number set apart by a space: "Wexcombe Part 1".
var titledPartWord = regexp.MustCompile(`(?i)^(.*\S)[ ]+part[ ]*(\d{1,2})$`)

// partFolder matches a folder named by a part mark alone: "CD1", "Disc 2", "dvd-b".
var partFolder = regexp.MustCompile(`(?i)^(?:cd|dvd|part|pt|disc|disk)[ ._-]*([0-9]{1,2}|[a-d])$`)

// partFolderOf is which part of a movie in parts the folder dir holds, from 1, when its folder
// holds the movie one part to a folder ("Ashgrove (2015)/CD1/", "…/CD2/"); 0 otherwise. The set
// is its folder's subfolders named by part marks alone: whole, numbered from 1 with none missing,
// and each holding one video. The videos must be one title's: a box set's discs hold episodes
// ("Disc 1/Garnet Fold 1x01.mkv") or films of their own ("Disc 2/Hollin Rising.mkv"), and a
// file's own name outweighs its folder's. It is read from dir's own folder, so a pass over one
// part's folder alone finds it too.
func partFolderOf(dir string) int {
	if !partFolder.MatchString(filepath.Base(dir)) {
		return 0
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		return 0
	}
	var names []string
	title := ""
	for _, e := range entries {
		if !e.IsDir() || !partFolder.MatchString(e.Name()) {
			continue
		}
		videos := videosIn(filepath.Join(filepath.Dir(dir), e.Name()))
		if len(videos) != 1 || !oneTitlesPart(videos[0], &title, len(names) == 0) {
			return 0
		}
		names = append(names, e.Name())
	}
	return partsOf("", names)[filepath.Base(dir)]
}

// oneTitlesPart says whether a part folder's video can be one part of the title the set's first
// video names, setting title from the first: a video with an episode's mark is an episode, and a
// video named for another title than the first's is that title.
func oneTitlesPart(video string, title *string, first bool) bool {
	if _, marked := numberingIn(video); marked {
		return false
	}
	own := partLetter.ReplaceAllString(withoutPart(strings.TrimSuffix(video, filepath.Ext(video))), "")
	own = normName(own)
	if first {
		*title = own
	}
	return own == *title
}

// halfMark matches a film's half counted at its name's end with no part's word: "Frith Downs
// 1of2", "Frith.Downs.2.of.2". A count past two is a miniseries' episodes ("Cold Mere 1of4").
var halfMark = regexp.MustCompile(`(?i)^(.*?)[ ._-]+([12])[ ._-]*of[ ._-]*2$`)

// joinedLetter matches a part's letter joined to its film's name by a separator other than a
// space: "LORN_VALE_A", "asg-b". "Plan A", spaced, is a title.
var joinedLetter = regexp.MustCompile(`(?i)^(.*[^ ._-])[._-]([a-d])$`)

// trailingNotes matches a release's notes in square brackets at a name's end: " [1080p]".
var trailingNotes = regexp.MustCompile(`(?:[ ._-]*\[[^\]]*\])+$`)

// bareMarkPart is the set and part a stem's mark with no part's word puts it in (partIn): a half
// counted "1of2", or a letter joined on, before any notes in brackets. ok is false for none.
func bareMarkPart(stem, ext string) (set string, number int, ok bool) {
	stem = trailingNotes.ReplaceAllString(stem, "")
	if m := halfMark.FindStringSubmatch(stem); m != nil && strings.TrimSpace(m[1]) != "" {
		return strings.ToLower(m[1] + "\x00half\x00" + ext), atoi(m[2]), true
	}
	if m := joinedLetter.FindStringSubmatch(stem); m != nil {
		return strings.ToLower(m[1] + "\x00a\x00" + ext), int(unicode.ToLower(rune(m[2][0]))-'a') + 1, true
	}
	return "", 0, false
}

// partLetter matches a part's bare letter after its title's name: the "-a" of "asg-a". A bare
// number is a sequel's ("Hollin 2"), not a part's.
var partLetter = regexp.MustCompile(`(?i)[ ._-]+[a-d]$`)

// videosIn is the videos directly in dir, as a walk would list them.
func videosIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var videos []string
	for _, e := range entries {
		if lower := strings.ToLower(e.Name()); !e.IsDir() && videoExtensions[filepath.Ext(lower)] && !extraIn(e.Name(), filepath.Base(dir)) {
			videos = append(videos, e.Name())
		}
	}
	return videos
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
		// "Frith Downs 1of2" and "LORN_VALE_A [1080p]" are "Frith Downs" and "LORN_VALE"
		// (bareMarkPart).
		for _, bare := range []*regexp.Regexp{halfMark, joinedLetter} {
			if m := bare.FindStringSubmatch(trailingNotes.ReplaceAllString(stem, "")); m != nil && strings.TrimSpace(m[1]) != "" {
				return strings.TrimSpace(m[1])
			}
		}
		return stem
	}
	before := strings.TrimRight(stem[m[2]:m[3]], " ._-([")
	after := strings.TrimLeft(stem[m[6]:m[7]], " ._-)]")
	return strings.TrimSpace(before + " " + after)
}

// titlesOwnPart is whether a "part" or "pt" mark belongs to the title: it follows a title word
// after a plain space ("Harbor Saga Part 2", a sequel). A rip sets its mark off with a separator, a
// bracket or the year ("Ashgrove - pt1", "Ashgrove.part1", "Ashgrove (2015) Part 1"); a "cd",
// "disc" or "dvd" mark is never a title's.
//
// A title writes "Part", never "pt" ("Ashgrove pt1"), and its year follows its part: a year or a
// release's mark before the mark ends the title, "Ashgrove 2015 Part 1", "Ashgrove 1080p Part 1".
func titlesOwnPart(before, mark string) bool {
	if !strings.HasPrefix(strings.ToLower(mark), "part") {
		return false
	}
	words := strings.TrimRight(before, " ")
	if len(words) == len(before) {
		return false
	}
	if fields := strings.Fields(words); len(fields) > 0 {
		if last := fields[len(fields)-1]; releaseYear(last) || releaseMark.MatchString(last) {
			return false
		}
	}
	r, _ := utf8.DecodeLastRuneInString(words)
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// partIn is the set a file's name puts it in, the name with its part mark taken out, and its part
// number; ok is false for a name with no part mark. A mark joined to the word before it
// ("Counterpart2") or the word after it ("part about") is no mark. A letter counts as its place in
// the alphabet, and a set's numbers and letters are kept apart.
func partIn(name string) (set string, number int, ok bool) {
	ext := filepath.Ext(name)
	m := partToken.FindStringSubmatchIndex(strings.TrimSuffix(name, ext))
	if m == nil {
		return bareMarkPart(strings.TrimSuffix(name, ext), ext)
	}
	stem := strings.TrimSuffix(name, ext)
	before, value, after := stem[m[2]:m[3]], stem[m[4]:m[5]], stem[m[6]:m[7]]
	if r, _ := utf8.DecodeLastRuneInString(before); r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
		return "", 0, false
	}
	if r, _ := utf8.DecodeRuneInString(after); r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
		return "", 0, false
	}
	// "Part 1 of 3" counts a miniseries' episodes, and "- Part 1 - Tides" titles one: a film's
	// parts never say how many, nor have titles of their own.
	// A release's mark straight after the part's number, "Ashgrove Part 1 1080p", says a rip marks
	// its parts: a sequel's name gives its year first, "Harbor Saga Part 2 (2011) 1080p".
	ripped := false
	if words := strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(after)); len(words) > 0 {
		ripped = releaseMark.MatchString(words[0])
	}
	if partCount.MatchString(after) || titledPart.MatchString(stem) || !ripped && titlesOwnPart(before, stem[m[3]:m[4]]) {
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
