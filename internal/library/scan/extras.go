package scan

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// extraName reports whether a video file's lower-cased name marks it as no title whatever is beside
// it: a sample, an anime's creditless opening, a copy still being written ("~"), or a hidden file
// such as a Mac's "._" shadow of a real one. A name with a space after its dot (".45 Calibre
// Kings") is a title's. An extra named by its kind ("- Trailer") is one only beside its title:
// siblingExtras.
func extraName(lowerName string) bool {
	base := strings.TrimRight(strings.TrimSuffix(lowerName, filepath.Ext(lowerName)), "0123456789")
	hidden := strings.HasPrefix(lowerName, "._") || strings.HasPrefix(lowerName, ".") && !strings.Contains(lowerName, " ")
	if hidden || strings.HasPrefix(lowerName, "~") || base == "sample" || strings.HasPrefix(base, "sample-") ||
		creditless.MatchString(lowerName) || (leadingTags.MatchString(lowerName) && animeOpening.MatchString(lowerName)) ||
		siteName(strings.TrimSuffix(lowerName, filepath.Ext(lowerName))) ||
		promoNames[strings.TrimSuffix(lowerName, filepath.Ext(lowerName))] || strings.HasSuffix(base, ".partial") ||
		otherKind.MatchString(strings.TrimSuffix(lowerName, filepath.Ext(lowerName))) {
		return true
	}
	// Joined with a dash, a release's sample: "Movie (2001)-sample.mkv"; and its proof after its year
	// or tags, "Brigg (2008)-proof.mkv" ("Bullet-proof" is a title's word).
	// Bracketed, it is a release's note: "Gullfen S01E05 [sample]", "Ashgrove (2015) (Sample)".
	if strings.HasSuffix(base, "-sample") || strings.HasSuffix(base, "[sample]") || strings.HasSuffix(base, "(sample)") ||
		releaseProof.MatchString(base) {
		return true
	}
	// An episode's title set apart by a spaced dash after its mark, "Thornbury S01E07 - Sample", is
	// the episode's in any case: a release joins its sample to its name ("….x264-grp.sample").
	if before, ok := strings.CutSuffix(base, " - sample"); ok && episodeMark.MatchString(before) {
		return false
	}
	// Otherwise it follows a release's own words: "Movie (2001) sample", "Kettle Bay S01E02
	// sample", "kettle.bay.s01e02.720p.hdtv.x264-grp.sample"; "The Last Sample", "The.Last.Sample"
	// and "Free Sample" are titles.
	for _, sep := range []string{".", "_", " "} {
		before, ok := strings.CutSuffix(base, sep+"sample")
		if !ok {
			continue
		}
		fields := strings.FieldsFunc(strings.TrimRight(before, " ._-"), func(r rune) bool { return r == ' ' || r == '.' || r == '_' })
		if len(fields) == 0 {
			return false
		}
		last := fields[len(fields)-1]
		if strings.HasSuffix(last, ")") || strings.HasSuffix(last, "]") || episodeMark.MatchString(last) || yearToken.MatchString(last) {
			return true
		}
		// The release's words, and its group's after a dash: "x264-grp".
		return slices.ContainsFunc(strings.Split(last, "-"), releaseToken)
	}
	return false
}

// extraFile is extraName for a file's name as it is written: an episode titled as a household
// titles one, "Thornbury S01E07 - Sample", is that episode, where a release's lower-case "kettle bay
// s01e02 - sample" is its sample.
func extraFile(name string) bool {
	return extraName(strings.ToLower(name)) && !titledSample.MatchString(stem(name))
}

// extraIn is extraFile for a file in its folder, named folder: a video named as its own folder is
// that folder's title whatever word it is, "Movies/Sample/Sample.mkv", "Sample (2021)/Sample.mkv".
func extraIn(name, folder string) bool {
	return extraFile(name) && !namedAsFolder(name, folder)
}

// namedAsFolder says whether a video is named as its folder is, with or without the folder's year
// and notes: "Sample/Sample.mkv", "Featurette (1999)/Featurette.mkv". folder is "" for a root's top.
// A hidden file, "._Bog Wizard (2010).mkv", is a Mac's shadow of one, never named so.
func namedAsFolder(name, folder string) bool {
	if folder == "" || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.EqualFold(stem(name), folder) {
		return true
	}
	// Only its year and its release's notes may go: "Ashgrove (2015)-sample" is no "Ashgrove".
	own := strings.TrimSpace(bracketYear.ReplaceAllString(trimReleaseNotes(plainStem(name)), ""))
	title, _, _ := searchQuery(folder, 0)
	return title != "" && sameWords(own, title)
}

// titledSample matches an episode's mark, then its title "Sample", capitalised, after a dash set
// apart: "S01E07 - Sample", "S01E07.-.Sample".
var titledSample = regexp.MustCompile(`(?i:s\d{1,4}[ ._]?e\d{1,4})[ ._]-[ ._]Sample$`)

// siteName says whether a video is named by nothing but a site, the promo a tracker or a release
// group leaves in a download: "www.Torrent.to.mkv", and a known promo's "RARBG.com.mp4". Any other
// name a site's ending follows is a title's, "Lovebirds.com", "Trust.Me", "Nine.To"; and so is a
// release's name that ends in its site, "Orrin.Gate.2018.1080p-www.site.com", read as its release.
func siteName(lowerStem string) bool {
	if wwwSite.MatchString(lowerStem) {
		return true
	}
	head, tld, ok := strings.Cut(lowerStem, ".")
	return ok && promoNames[head] && siteEnding.MatchString(tld)
}

var (
	// wwwSite matches a site's whole address: "www.torrent.to".
	wwwSite = regexp.MustCompile(`^www\.[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,6}$`)
	// siteEnding matches a site's ending: "com", "to", "co.uk".
	siteEnding = regexp.MustCompile(`^[a-z]{2,6}(?:\.[a-z]{2})?$`)
)

// otherKind matches a name that ends in another kind of file's extension before a video's, as a
// site's junk does to pass for one: "RARBG.txt.mp4", "Read Me.url.mkv", "setup.exe.avi". ".com" is
// far more often a site's ending, "… RARBG.com.mp4", than a program's: siteName reads it.
var otherKind = regexp.MustCompile(`\.(?:txt|nfo|url|lnk|exe|scr|bat|htm|html|pdf|jpg|jpeg|png|gif|torrent|zip|rar)$`)

// releaseProof matches a release's proof clip, joined after its year, tags or group: "brigg
// (2008)-proof", "brigg.2008.1080p.bluray.x264-grp-proof".
var releaseProof = regexp.MustCompile(`(?:[)\]]|\d{4}|x26[45]|-[a-z0-9]+)-proof$`)

// promoNames are the names a release group's or tracker's promo clip goes by, in lower case, with
// no site's ending: "ETRG.mp4", "YTS.mp4". A download still being written ends ".partial".
var promoNames = map[string]bool{
	"etrg": true, "rarbg": true, "yts": true, "yify": true, "torrentgalaxy": true, "tgx": true,
	"extratorrent": true, "1337x": true, "rarbg_do_not_mirror": true, "torrent downloaded from": true,
}

// extraKinds are the words an extra is named by, as Jellyfin, Plex and households name them; a
// longer one is tried before a word it starts with.
var extraKinds = [][]string{
	{"behind", "the", "scenes"}, {"making", "of"}, {"gag", "reel"}, {"music", "video"}, {"alternate", "ending"},
	{"short", "film"}, {"deleted", "scenes"}, {"deleted", "scene"}, {"official", "trailer"},
	{"teaser", "trailer"}, {"theatrical", "trailer"}, {"directors", "commentary"}, {"director", "commentary"},
	{"audio", "commentary"}, {"blooper", "reel"}, {"tv", "spot"}, {"photo", "gallery"},
	{"promo"}, {"storyboards"}, {"storyboard"}, {"gallery"}, {"sample"}, {"sneak", "peek"},
	{"extended", "scene"}, {"extended", "scenes"}, {"easter", "eggs"}, {"easter", "egg"},
	{"opening", "credits"}, {"end", "credits"}, {"closing", "credits"}, {"q&a"}, {"credits"}, {"intro"},
	{"recap"}, {"bonus"}, {"preview"}, {"trailers"}, {"teasers"}, {"featurettes"}, {"interviews"},
	{"promos"}, {"previews"}, {"recaps"}, {"clips"}, {"extras"},
	{"trailer"}, {"teaser"}, {"featurette"}, {"interview"}, {"behindthescenes"}, {"deletedscene"},
	{"deletedscenes"}, {"deleted"}, {"bloopers"}, {"outtakes"}, {"commentary"}, {"scene"}, {"short"},
	{"clip"}, {"extra"}, {"other"}, {"theme"}, {"creditless"}, {"textless"}, {"ncop"}, {"nced"},
}

// extraKindAt is where in a name's words its extra's kind starts, -1 for none. A number joined on
// is its count: "trailer2".
func extraKindAt(words []string) int {
	bare := make([]string, len(words))
	for i, w := range words {
		bare[i] = strings.NewReplacer("'", "", "’", "").Replace(strings.TrimRight(w, "0123456789"))
	}
	// The kind follows the title, which may hold a kind's word of its own: "Ferry Extra (1982)
	// 1080p-trailer" is a trailer.
	last := -1
	for i := 0; i < len(bare); i++ {
		for _, kind := range extraKinds {
			if len(bare)-i >= len(kind) && slices.Equal(bare[i:i+len(kind)], kind) {
				last = i
				i += len(kind) - 1
				break
			}
		}
	}
	return last
}

// kindEnds says whether a name's words from its extra's kind are that kind alone, with a count or a
// release's words after it: "Trailer 2", "Deleted Scenes", "trailer 1080p"; "Scene of the Crime"
// and "Clip Show" are titles. A kind first, "Featurette Costumes", is a household's extra named by
// what it shows, read beside its title (siblingExtras).
func kindEnds(words []string) bool {
	for _, kind := range extraKinds {
		bare := func(w string) string { return strings.TrimRight(w, "0123456789") }
		if len(words) < len(kind) || !slices.EqualFunc(words[:len(kind)], kind, func(w, k string) bool { return bare(w) == k }) {
			continue
		}
		// "Making of Season 1" is its season's extra.
		if _, season := seasonIn(strings.Join(words[len(kind):], " ")); season && len(words) > len(kind) {
			return true
		}
		return !slices.ContainsFunc(words[len(kind):], func(w string) bool { return !digits(w) && !releaseToken(w) })
	}
	return false
}

// extrasBeside is which of a folder's videos are extras of the episodes or the season beside them,
// read whole by the folder:
//   - An episode's name titled by a kind of extra alone after its mark, "S01E02 - Sample",
//     "S01E01 - Commentary", "S01E01 Trailer", "S01E02.Recap", is that episode's extra when
//     another video beside it numbers the same episode. Alone, it is the episode, titled so. So is
//     one titled by a kind only an extra is named by, "S01E01 - Behind the Episode", beside that
//     episode titled otherwise.
//   - A show's season and a kind of extra with no episode's mark, "Fernhollow.S01.Bloopers",
//     "Gullhaven - Season 1 Promo", is the season's extra, as a folder so named is
//     (seasonExtrasFolder).
func extrasBeside(names []string) map[string]bool {
	var out map[string]bool
	add := func(name string) {
		if out == nil {
			out = map[string]bool{}
		}
		out[name] = true
	}
	// What follows its last mark, set apart any way: "S01E02 - Sample", "S01E01 Trailer",
	// "S01E02.Preview".
	afterMark := func(name string) []string {
		marks := episodeMark.FindAllStringIndex(stem(name), -1)
		if len(marks) == 0 {
			return nil
		}
		return extraWords(stem(name)[marks[len(marks)-1][1]:])
	}
	titledExtra := func(name string) bool {
		words := afterMark(name)
		return len(words) > 0 && extraKindAt(words) == 0 && kindEnds(words)
	}
	// A title a kind only an extra is named by starts, "S01E01 - Behind the Episode", "S01E01 -
	// Making the Pilot": beside a copy of the episode titled otherwise it is the episode's extra.
	// Alone, or beside a copy titled as it is, "S01E01 - Behind Enemy Lines" is the episode.
	kindTitled := func(name string) bool { return strongExtraKind(afterMark(name)) }
	for _, name := range names {
		mark, marked := numberingIn(name)
		if !marked {
			if seasonExtrasFolder(stem(name)) || seasonKindFirst(name) || dashedExtra(name) {
				add(name)
			}
			continue
		}
		titled, kinded := titledExtra(name), kindTitled(name)
		if !titled && !kinded {
			continue
		}
		if slices.ContainsFunc(names, func(other string) bool {
			n, ok := numberingIn(other)
			return other != name && ok && !titledExtra(other) && (titled || !kindTitled(other)) &&
				n.Season == mark.Season && n.Episode == mark.Episode
		}) {
			add(name)
		}
	}
	return out
}

// seasonKindFirst says whether a video is named by its show, its season with no episode, then a
// kind of extra and what it shows: "Saltcove.S02.Bonus.Cast.Q&A", "Gullhaven S1 Featurette
// Costumes". A season's mark and a title, "Saltcove S02 The Long Night", is no extra, nor is one
// whose title starts with an everyday word a kind is also: "Saltcove S02 Short Stories".
func seasonKindFirst(name string) bool {
	words := extraWords(name)
	for i := 1; i < len(words)-1; i++ {
		if !seasonToken.MatchString(words[i]) {
			continue
		}
		rest := words[i+1:]
		return !everydayKinds[rest[0]] && slices.ContainsFunc(extraKinds, func(kind []string) bool {
			return len(rest) >= len(kind) && slices.Equal(rest[:len(kind)], kind)
		})
	}
	return false
}

// dashedExtra says whether a video with no episode's mark is named by its title, a dash, then a
// kind of extra: first, of the kinds no title starts with, "Ashlow Run - Featurette - Sets", "Kame
// Tsuki - Creditless ED", or after a word that says whose it is, "Ashlow Run - Cast Interview",
// "[Grp] Kame Tsuki - Next Episode Preview". "Ashlow Run - The Interview" and "Hilltop - Interview
// with a Ghost" may be episodes' titles, and stay them; so does a recap, its show's season 0's.
func dashedExtra(name string) bool {
	head, after, ok := strings.Cut(stem(leadingTags.ReplaceAllString(name, "")), " - ")
	if !ok || strings.TrimSpace(head) == "" {
		return false
	}
	words := extraWords(trimReleaseNotes(after))
	if len(words) == 0 {
		return false
	}
	if extraFirst[words[0]] {
		return true
	}
	for i := 1; i < len(words); i++ {
		if extraOwners[words[i-1]] && kindEnds(words[i:]) {
			return true
		}
	}
	return false
}

// extraFirst are the kinds of extra no episode's title starts with.
var extraFirst = map[string]bool{
	"featurette": true, "featurettes": true, "creditless": true, "textless": true, "ncop": true, "nced": true,
	"bloopers": true, "outtakes": true, "storyboards": true, "behindthescenes": true, "deletedscenes": true,
}

// extraOwners are words before a kind of extra that say whose it is: "Cast Interview", "Next
// Episode Preview", "Creditless Opening".
var extraOwners = map[string]bool{
	"cast": true, "crew": true, "director": true, "directors": true, "creator": true, "episode": true,
	"season": true, "series": true, "creditless": true, "textless": true, "extended": true, "bonus": true,
}

// everydayKinds are kinds of extra that are everyday words too, and so start titles.
var everydayKinds = map[string]bool{
	"short": true, "scene": true, "clip": true, "other": true, "theme": true, "extra": true, "intro": true,
	"credits": true, "end": true, "opening": true, "closing": true, "music": true, "easter": true,
}

// episodeExtra matches Jellyfin's mark for an episode's own extra, joined to its name with a dash:
// "Kettle Bay - S01E02-trailer", "… - Tidewater-deleted". " - Trailer", spaced, is the episode's
// title. So is one joined with a dot straight after the episode's mark, "Coldharbour
// S01E02.Trailer": a title follows its mark apart, " - Trailer".
var episodeExtra = regexp.MustCompile(`(?i)(?:-|[ ._]s\d{1,2}e\d{1,3}\.)(?:trailer|featurette|behindthescenes|deleted|deletedscene|interview|scene|short|clip|extra)\d*$`)

// titleWordExtra matches an episode's title that ends in an everyday word joined on with a dash,
// as a hyphenated title does: "Behind-the-Scene", "Cut-Short", "Paper-Clip", "Re-Interview". A
// mark joined to the episode's number ("S01E02-short") is Jellyfin's, as is one it writes in lower
// case after a title ("Tidewater-clip").
var titleWordExtra = regexp.MustCompile(`[a-zA-Z]-(?:(?i:short|scene)|[A-Z][a-z]+)$`)

// extraWords are a name's lower-cased words, split at its separators and brackets; an apostrophe
// stays in its word ("barney's"), as the title it is matched against keeps it.
func extraWords(name string) []string {
	return strings.FieldsFunc(strings.ToLower(stem(name)), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("._-()[]{}#,", r)
	})
}

// siblingExtras is which of a folder's videos are extras of a title beside them: named by their
// title then their kind ("Ashgrove (2015) - Official Trailer", "Ashgrove.2015.Trailer.1080p",
// "Ashgrove (2015) [Trailer]"), where the folder holds that title's own video or is named as it
// is with its year; or named by their kind alone ("Trailer 2", "Gag Reel", "theme") in a title's
// own folder. A name with no title beside it is a title of its own, kind words and all
// ("Night Trailer", "Cut-Short", "The Other Side (2019)"), as is one with its own year after its
// kind ("Ashgrove Short (2017)"), and an episode, whose mark says what it is ("S02E08 - Trailer").
// folder is the folder's name, "" for a root's top.
func siblingExtras(folder string, names []string, shows bool) map[string]bool {
	// A title is keyed by its letters alone: "E.T.A. the Ferry" and its extra's words, "e t a the
	// ferry", are one.
	compact := func(title string) string { return strings.ReplaceAll(normName(title), " ", "") }
	key := func(words []string) string {
		title, _, _ := searchQuery(strings.Join(words, " "), 0)
		return compact(title)
	}
	titles := map[string]bool{}
	titled := false
	if title, year, _ := searchQuery(folder, 0); folder != "" && title != "" && year != 0 {
		titles[compact(title)], titled = true, true
	}
	type marked struct {
		name  string
		words []string
		at    int
	}
	var kinds []marked
	out := map[string]bool{}
	episodes := slices.ContainsFunc(names, episodeMark.MatchString)
	// The film a film's own folder holds, named as the folder is: "Corbel Ridge (2013).mkv". One
	// ending in a kind of extra, "Corbel Ridge (2013) 1080p-trailer", is no film; "Sample Lane
	// (2011)" is.
	film := titled && slices.ContainsFunc(names, func(name string) bool {
		words := extraWords(name)
		at := extraKindAt(words)
		return !strings.Contains(stem(name), " - ") && titles[key(strings.Fields(stem(name)))] && (at <= 0 || !kindEnds(words[at:]))
	})
	// A video named as a household names a film, not as MakeMKV numbers what it rips, is the film:
	// the disc's other titles left beside it, "Gilded Road_t03", are the disc's extras.
	renamed := !shows && !episodes && slices.ContainsFunc(names, func(name string) bool {
		return !makeMKVTitle.MatchString(plainStem(name))
	})
	for _, name := range names {
		words := extraWords(name)
		at := extraKindAt(words)
		if renamed && makeMKVTitle.MatchString(plainStem(name)) {
			out[name] = true
			continue
		}
		// In a release's pack, its group's tag on the folder, "[Grp] Tora Michi [01-24]/Tora Michi -
		// Menu.mkv" is the release's extra as a tagged name's is (extraName).
		if leadingTags.MatchString(folder) && animeOpening.MatchString(stem(name)) {
			out[name] = true
			continue
		}
		// A hidden copy of a video beside it, ".Fenwick Garth (2003).mkv", is a shadow a copy or a
		// sync left, never a second copy; ".45 Calibre Kings" alone is a title.
		if strings.HasPrefix(name, ".") && slices.ContainsFunc(names, func(other string) bool {
			return other != name && strings.EqualFold(plainStem(other), plainStem(name[1:]))
		}) {
			out[name] = true
			continue
		}
		// Beside that film, Plex's own suffix, joined on in lower case, says its extra whatever the
		// name before it: "Ashwick Short Film-short", "… Gala-scene". "Cut-Short" is a title, and so
		// is a name all in lower case, "cut-short", which can't say which it is.
		if film && plexSuffix.MatchString(stem(name)) && name != strings.ToLower(name) {
			out[name] = true
			continue
		}
		// Beside that film, "<film> - <anything else>" is its extra, whatever the household called
		// it ("- Animatic", "- Press Conference"), unless what follows says which copy of the film
		// it is: "- 1080p", "- Directors Cut", "- Part 2".
		if before, after, ok := strings.Cut(stem(name), " - "); film && ok && titles[key(strings.Fields(before))] && !filmVersion(name, after) &&
			!episodeMark.MatchString(name) && airDateIn(stem(name)) == "" {
			out[name] = true
			continue
		}
		// In a show's folder, a name its kind ends that only an extra is named by, with no episode's
		// mark, is the show's extra: "Gannet Rock - Official Trailer", "Gannet Rock S02 Trailer",
		// "trailer.mkv". "Gannet Rock S01E05 - The Trailer" is an episode.
		if shows && at >= 0 && strongExtraKind(words[at:]) && kindEnds(words[at:]) && !episodeMark.MatchString(name) {
			out[name] = true
			continue
		}
		// A name written all in lower case can't say "Paper-Clip" from Jellyfin's "Tidewater-clip":
		// it is the episode, never skipped unread.
		if episodeMark.MatchString(name) && episodeExtra.MatchString(stem(name)) && !titleWordExtra.MatchString(stem(name)) &&
			name != strings.ToLower(name) {
			out[name] = true
			continue
		}
		// A kind's word before the title's year is the title's: "Ferry Extra (1982) 1080p". Among a
		// show's files, a kind's word with more after it is a title's, "Hilltop - Scene of the Crime",
		// and so is a dated name's, "Quillon Hall - 2018-04-02 - Recap": a day says an episode as its
		// mark does. In a film's own folder, "… - Interview with Cast" is its extra.
		// Beside a show's marked episodes, a word an episode is as often titled by, "Hilltop -
		// Credits", "… - Recap", is its title: only a kind only an extra is named by is an extra.
		noKind := at < 0
		numbered := episodeMark.MatchString(name) || airDateIn(stem(name)) != ""
		yearAfterKind := !noKind && slices.ContainsFunc(words[at:], releaseYear)
		titledByKind := !noKind && at > 0 && !kindEnds(words[at:]) && (!titled || episodes)
		episodeTitleWord := !noKind && at > 0 && episodes && slices.Contains(titleKinds, words[at])
		if noKind || numbered || yearAfterKind || titledByKind || episodeTitleWord {
			titles[key(words)] = true
			// An episode's show is a title its extras name: "Cold Coast - Making Of" beside
			// "Cold Coast S01E01".
			if show := looseSeries(name); show != "" {
				title, _, _ := searchQuery(show, 0)
				titles[compact(title)] = true
			}
			continue
		}
		kinds = append(kinds, marked{name, words, at})
	}
	for _, k := range kinds {
		if k.at == 0 {
			// Its kind alone, and what a household adds: "Trailer 2", "Featurette Costumes", in a
			// title's own folder: one named with its year, or holding that one title. In a catch-all
			// ("Movies/Outtakes.mkv") it is a title of its own, and so is one named as its own
			// folder's title: "Featurette (1999)/Featurette.mkv" is the film.
			if (titled || len(titles) == 1 && !catchAllFolder(folder)) && !namedAsFolder(k.name, folder) {
				out[k.name] = true
			}
			continue
		}
		// A title's year, then a kind only an extra is named by, says an extra with no title beside
		// it: "Movies/Gravel Peak (2003) Trailer.mp4".
		// In a film's own folder, one named with its year, a trailer is an extra whichever film it
		// names, "Glimmerfold Sequel Teaser" or another's, "Ironvale Trailer"; one named as the
		// folder's film is the film: "Night Trailer (2010)/Night Trailer.mkv".
		if titles[key(k.words[:k.at])] || slices.ContainsFunc(k.words[:k.at], releaseYear) && strongExtraKind(k.words[k.at:]) ||
			titled && strongExtraKind(k.words[k.at:]) && !titles[key(k.words)] {
			out[k.name] = true
		}
	}
	return out
}

// rippedExtras is which of a film's disc titles, as MakeMKV left them, are its extras: no name says
// which title is the feature, so its size does. A title under half the largest's size is the
// disc's bonus ("title_t01" beside the feature, "title_t00"); titles near the largest's size are
// the film's own copies, as a disc's cuts are. names are the folder's videos left once siblingExtras
// has read it; sizeOf is a video's size, 0 when unknown. A season's disc ("KETTLE_BAY_S1_D1") holds
// episodes, and its play-all title is the largest, so a show's folder is never read this way.
func rippedExtras(folder string, names []string, sizeOf func(name string) int64) map[string]bool {
	if len(names) < 2 || seasonDisc.MatchString(folder) || slices.ContainsFunc(names, func(name string) bool {
		return !makeMKVTitle.MatchString(plainStem(name))
	}) {
		return nil
	}
	sizes := make(map[string]int64, len(names))
	var largest int64
	for _, name := range names {
		sizes[name] = sizeOf(name)
		largest = max(largest, sizes[name])
	}
	out := map[string]bool{}
	for name, size := range sizes {
		if size > 0 && size < largest/2 {
			out[name] = true
		}
	}
	return out
}

// bonusBeside says whether a video in a film's own dated folder, beside other videos, is named as
// something other than that film: "Gale Spit (2004)/Cast Q and A.mkv", "…/Return to Gale Spit.mkv".
// Its name has no year of its own, is not the film's, does not run on from its title as a sequel's
// does ("Brook (2010)/Brook 2.mkv"), is not the film's name cut short ("asg-1080p", "movie.mkv"),
// and has no episode's or day's mark; a set's part is named before this is asked. It keeps its own name (fallbackName): an unlabelled
// bonus feature waits in Needs you, and a film kept there is its own, never skipped.
func bonusBeside(folder, name string) bool {
	plain := plainStem(name)
	title, _, _ := searchQuery(folder, 0)
	own, _, _ := searchQuery(name, 0)
	if title == "" || own == "" || yearAnywhere.MatchString(plain) || markIn(name) != nil || airDateIn(plain) != "" ||
		makeMKVTitle.MatchString(plain) || genericStem.MatchString(plain) {
		return false
	}
	if sameWords(own, title) || runsOn(own, title) || abbreviates(own, title) || strings.EqualFold(own, initials(title)) {
		return false
	}
	return true
}

// filmVersion says whether what follows a film's name and a dash, after, says which copy of the
// film name is: its quality ("1080p", "4K HDR"), an edition ("Directors Cut") or a part ("Part 2",
// "cd1"), as Plex and Radarr name a film's versions.
func filmVersion(name, after string) bool {
	if editionIn(name) != "" {
		return true
	}
	if _, part, ok := partIn(name); ok && part > 0 {
		return true
	}
	// Radarr's "- [Remux-1080p][DTS-HD MA 5.1][h264]-HiFi": notes in brackets and the group that
	// released it say which copy it is, whatever their words.
	if strings.TrimSpace(releaseGroupTail.ReplaceAllString(bracketedNotes.ReplaceAllString(after, ""), "")) == "" &&
		strings.Contains(after, "[") {
		return true
	}
	words := strings.FieldsFunc(after, func(r rune) bool { return strings.ContainsRune(" ._-[]()", r) })
	if len(words) == 0 {
		return false
	}
	rest := slices.DeleteFunc(slices.Clone(words), releaseToken)
	unmarked := slices.DeleteFunc(slices.Clone(words), releaseMark.MatchString)
	// "- Final Cut", "- Open Matte", "- Black and White", "- English Dub", "- Workprint": words
	// that say how a copy was cut, framed, coloured or voiced say which copy it is.
	versioned := len(rest) > 0 && !slices.ContainsFunc(rest, func(w string) bool { return !versionWords[strings.ToLower(w)] })
	return len(rest) == 0 || editionOnly(rest) || editionOnly(unmarked) || versioned
}

// bracketedNotes matches a release's notes in square brackets anywhere in a name: "[Remux-1080p]".
var bracketedNotes = regexp.MustCompile(`\[[^\]]*\]`)

// releaseGroupTail matches the group a release names last, after a dash: "-HiFi".
var releaseGroupTail = regexp.MustCompile(`^[ ._]*-?[ ._]*[A-Za-z0-9]+$|^[ ._-]*$`)

// versionWords are the words a household names a film's copy by: its cut, framing, colour, voice
// or release. A label of these alone is a version, never an extra; any other word ("Animatic",
// "Press Conference") makes it the film's extra.
var versionWords = map[string]bool{
	"cut": true, "edit": true, "version": true, "final": true, "alternate": true, "alt": true, "rough": true,
	"workprint": true, "open": true, "matte": true, "black": true, "and": true, "white": true, "b&w": true,
	"colorized": true, "colourized": true, "color": true, "colour": true, "theatrical": true, "original": true,
	"home": true, "release": true, "uncut": true, "restored": true, "remaster": true, "remastered": true,
	"redux": true, "censored": true, "uncensored": true, "broadcast": true, "tv": true, "cinema": true,
	"director's": true, "directors": true, "extended": true, "special": true, "edition": true, "ultimate": true,
	"dub": true, "dubbed": true, "sub": true, "subbed": true, "english": true, "language": true,
	"french": true, "german": true, "spanish": true, "italian": true, "japanese": true, "hindi": true,
	"tamil": true, "telugu": true, "korean": true, "mandarin": true, "cantonese": true, "dutch": true,
	"widescreen": true, "fullscreen": true, "pan": true, "scan": true, "3d": true, "imax": true, "hdr": true,
	"sdr": true, "dv": true, "dolby": true, "vision": true, "1st": true, "2nd": true, "us": true, "uk": true,
	"international": true, "domestic": true, "television": true, "longer": true, "hybrid": true, "fan": true,
	"rus": true, "eng": true, "ukr": true, "ger": true, "fre": true, "spa": true, "ita": true, "jpn": true,
	"kor": true, "multi": true,
}

// titleKinds are the kinds an episode is as often titled by: "Credits", "Recap", "Intro".
var titleKinds = []string{"credits", "intro", "recap", "recaps", "preview", "previews", "bonus", "extra", "extras", "other", "theme", "clip", "clips", "scene", "short"}

// strongExtraKinds are the kinds only an extra is named by; "Short", "Clip" or "Theme" after a
// year may be a title's or an edition's.
var strongExtraKinds = []string{
	"trailer", "teaser", "featurette", "behind", "making", "deleted", "bloopers", "outtakes", "gag", "official",
	"theatrical", "behindthescenes", "deletedscene", "deletedscenes", "trailers", "teasers", "featurettes",
}

// strongExtraKind says whether the words an extra's kind starts are a kind only an extra is
// named by.
func strongExtraKind(words []string) bool {
	return len(words) > 0 && slices.Contains(strongExtraKinds, strings.TrimRight(words[0], "0123456789"))
}

var (
	// creditless matches an anime's creditless opening or ending, "NCOP1", "NCED2v2": an extra.
	creditless = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])nc(?:op|ed)\d*(?:v\d+)?(?:[^a-z0-9]|$)`)
	// animeOpening matches a release's extra in its episode's place, " - OP", " - ED2", " -
	// Creditless OP", " - Clean ED", " - Menu 01", " - Preview 03", " - Picture Drama 01", " -
	// Trailer", " - Ending Theme", " - Opening Movie 2": an extra, when a release group's tag says
	// the name is a release's.
	animeOpening = regexp.MustCompile(`(?i)[ ._]-[ ._]+(?:(?:creditless|clean|textless|non[ ._-]?credit)[ ._]+)?(?:op|ed|opening|ending|pv|cm|menu|preview|trailer|teaser|spot|picture[ ._]+drama)(?:[ ._]+(?:theme|song|movie|animation))?(?:[ ._]*\d+)?(?:v\d+)?(?:[ ._]*[\[(.]|$)`)
)
