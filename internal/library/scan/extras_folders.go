package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// extrasFolder says whether a folder named name in parent is an extras folder: named by its kind
// (extrasKinds), "Extras", "Bloopers", "Making Of", or by its title then its kind, "Brine
// Rock/Brine Rock S01 Extras". In a title's own folder one is skipped: only movies and episodes
// become titles (0047). Elsewhere it is a folder of titles: a show named "Shorts" at the top,
// "Movies/Kids/Shorts" or "TV/Cartoons/Clips".
func extrasFolder(name, parent string) bool {
	if extrasName(name) {
		return true
	}
	// A release's material in a title's folder, "[Grp] Kasa Hoshi [BD]/CDs/", "…/Scans/", holds
	// its soundtrack and its booklet's pages, never an episode; at the top or among shelves the
	// name may be a shelf's.
	if packMaterial[normName(name)] && parent != "." && !shelfAt(parent) {
		return true
	}
	// Its kind first, then its disc's label: "Extras - Disc 2", "Bonus Features Disc 3".
	if words := strings.Fields(normName(name)); parent != "." {
		for n := 1; n <= 2 && n < len(words); n++ {
			if partFolder.MatchString(strings.Join(words[len(words)-n:], " ")) && extrasName(strings.Join(words[:len(words)-n], " ")) {
				return true
			}
		}
	}
	head := extrasAfter(name)
	if len(head) == 0 || parent == "." {
		return false
	}
	// A disc's label names its title's extras as the title does: "Disc 2 - Extras".
	if partFolder.MatchString(strings.Join(head, " ")) {
		return true
	}
	// The title is its show's, or its film's without its year: "Sallowmere (2008)/Sallowmere Extras".
	title, _, _ := searchQuery(path.Base(parent), 0)
	for _, own := range [][]string{showWordsOf(path.Base(parent)), strings.Fields(normName(title))} {
		if len(own) > 0 && len(head) >= len(own) && slices.Equal(head[:len(own)], own) {
			return true
		}
	}
	return false
}

// packMaterial are the folders a release's pack keeps its material in beside its episodes, as
// normName writes them.
var packMaterial = map[string]bool{
	"cds": true, "cd": true, "scans": true, "scan": true, "ost": true, "osts": true, "soundtrack": true,
	"soundtracks": true, "booklet": true, "booklets": true, "fonts": true, "covers": true, "artwork": true,
}

// bonusDisc says whether a disc's folder ("Disc 2", "DVD2") beside a film's own video holds the
// film's extras: the folder of a film's parts holds no film of its own (partFolderOf), and a
// show's or a season's discs hold its episodes. videos are the folder's own videos.
func bonusDisc(name string, videos []string, shows bool) bool {
	return !shows && partFolder.MatchString(name) && len(videos) == 1 && !episodeMark.MatchString(videos[0])
}

// seasonExtrasFolder says whether a folder is a season's extras by its own name, wherever it is:
// "Corran.Hill.S04.Extras", "Corran Hill Season 4 Extras", "Kettle.Moor.S01.Bonus.Features",
// "Pike.Mere.S02.Extras.720p".
func seasonExtrasFolder(name string) bool {
	head := extrasAfter(name)
	if len(head) < 2 {
		return false
	}
	return slices.ContainsFunc(head[1:], seasonToken.MatchString) || namedSeasonOf(strings.Join(head, " ")) > 0
}

// shelfExtras are the folders a films' shelf keeps its films' trailers and samples in.
var shelfExtras = map[string]bool{"trailers": true, "teasers": true, "samples": true}

// plexSuffix matches Plex's suffix for a film's extra, in lower case after a dash: "-short",
// "-behindthescenes".
var plexSuffix = regexp.MustCompile(`-(?:trailer|short|scene|clip|featurette|behindthescenes|deleted|interview|other|extra)$`)

// shelfBonusExtras is which videos in a films' shelf's bonus folder, "Movies/Extras/", "Movies/Bonus
// Features/", are its films' extras: those that name no year, "Tamber Sound Featurette". One that
// does, "Movies/Extras/Pinfold (2011).mkv", is a film kept there, never skipped unread. dir is the
// folder, relative to the root.
func shelfBonusExtras(dir string, names []string) map[string]bool {
	if dir == "." || path.Dir(dir) == "." || !shelfBonus[normName(path.Base(dir))] || !amongFilms(path.Dir(dir)) {
		return nil
	}
	out := map[string]bool{}
	for _, name := range names {
		if _, year, _ := searchQuery(plainStem(name), 0); year == 0 {
			out[name] = true
		}
	}
	return out
}

// shelfSamples is which videos in a films' shelf's "Sample/" are its films' samples, as its
// "Samples/" holds (shelfExtras): every one but a film named as the folder, "Movies/Sample/Sample.mkv".
// dir is the folder, relative to the root.
func shelfSamples(dir string, names []string) map[string]bool {
	if dir == "." || path.Dir(dir) == "." || normName(path.Base(dir)) != "sample" || !amongFilms(path.Dir(dir)) {
		return nil
	}
	out := map[string]bool{}
	for _, name := range names {
		if !namedAsFolder(name, path.Base(dir)) {
			out[name] = true
		}
	}
	return out
}

// shelfBonus are the folders a films' shelf keeps its films' bonus videos in, by normName.
var shelfBonus = map[string]bool{
	"extras": true, "bonus": true, "bonus features": true, "special features": true, "featurettes": true,
	"behind the scenes": true, "deleted scenes": true, "interviews": true, "bonus material": true,
}
