package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// Numbering in a folder: the season and episode a file's name gives once its folders say it is a
// show's (a season's folder, a show's own folder, a pack's), where the name alone would not.

// partWord matches a part spelled out at a name's end: "Ridge Fall - Part One".
var partWord = regexp.MustCompile(`(?i)(?:^|[ ._-])part[ ._-]+([a-z]+)$`)

// numberingInFolder is the numbering a file's folders let its name give, where the name alone does
// not say it is an episode: a show's file "Ep02", "2of6" or "Season 1 Episode 2"; a season folder's
// "05 - Tidewater"; the show's name and a number, "Kettle Bay 104" (season 1, episode 4, as Kodi
// reads it) or, as Sonarr writes an anime's, "Kettle Bay - 013" (episode 13 of all). A season
// folder gives the season; with none it is the first. A file loose at the top has no show to say so.
func numberingInFolder(file File) (mp.Numbering, bool) {
	dir := path.Dir(file.Rel)
	// The name is read without its release's notes at its end: "Show - Part One [1080p]" is
	// "Show - Part One". A fansub's notes say what it is, so looseAnime reads them.
	raw, name := stem(path.Base(file.Rel)), plainStem(path.Base(file.Rel))
	season, inSeason, namedFolder, show := 1, false, false, path.Base(dir)
	if seasonDir := seasonDirOf(dir); seasonDir != "" {
		inSeason, show = true, path.Base(aboveSeasons(seasonDir))
		if n, ok := seasonIn(path.Base(seasonDir)); ok {
			season = n
		} else if showSpecials.MatchString(path.Base(seasonDir)) {
			season = 0
		} else if m := namedSeason.FindStringSubmatch(path.Base(seasonDir)); m != nil {
			season, _ = seasonNumberWord(matched(m[2], m[3]))
		}
	} else if n := namedSeasonOf(show); n > 0 {
		// A show's season folder with no show folder above it, "Puddle Pals Season 2/", is the
		// show's and gives the season.
		season, namedFolder = n, true
	} else if n := packSeason(show); n > 0 && packShow(dir) != "" {
		// So does a season pack's release folder, "Pike.Moss.S02.1080p/".
		season, namedFolder = n, true
	}
	episode := func(e int) (mp.Numbering, bool) { return mp.Numbering{Season: season, Episode: e}, e > 0 }
	// MakeMKV's title, "Kite Shore Disc 1_t01", numbers its disc's titles, never the show's episodes:
	// every title on disc 1 would claim the first. Its length or the catalog places it.
	if makeMKVTitle.MatchString(name) {
		return mp.Numbering{}, false
	}
	// "Bellcote - Season 5 Episode 2" and "Dunbar.Season.3.Episode.1" name their season too, which
	// is theirs before the folder's.
	if m := seasonEpisodeWords.FindStringSubmatch(name); m != nil {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, atoi(m[2]) > 0
	}
	// "Spotty Dog - Episode 3" names its show and its episode wherever it is, loose or in a
	// catch-all: its season folder's, or the first season's. A fansub's, "[Kumo] Show - Episode
	// 17", is its number in the whole show (looseAnime).
	if m := dashedEpisode.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) {
		return episode(atoi(m[1]))
	}
	// On an anime's shelf, an untagged "Mizu no Oto Ep 05" is as "Tsukishiro Kan Episode 09" is.
	if m := episodeWord.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) && inAnime(shelved(file, dir)) {
		return episode(atoi(m[1]))
	}
	// So does a scene release's "larchmont.road.312.hdtv-lol": season 3, episode 12.
	if m := sceneNumber.FindStringSubmatch(raw); m != nil && atoi(m[2]) > 0 {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	// A TV capture's part, "Zelston.Fall.Pt1.HDTV", "Ashlington.Rise.3of6.HDTV" or
	// "Jevington.E01.HDTV", is a broadcast's episode, never a film's part.
	if n, ok := capturedEpisode(raw); ok {
		return episode(n)
	}
	if m := partOfWord.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	// "Docs/Wild Moor 1of3" is a miniseries' first of three wherever it is; a film's part gives its
	// year, "Ferrow Lane 2 of 3 (2010)".
	if countedPart(name) > 0 {
		return episode(countedPart(name))
	}
	if m := titledPart.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 {
		return episode(atoi(m[2]))
	}
	if _, n, ok := shelvedEpisode(path.Base(file.Rel), dir); ok {
		return n, true
	}
	// Kodi's "Cutters Bay - 214 - Late" is season 2's fourteenth: three digits between dashes, its
	// episode's title after. An anime's number across its show is padded, "- 014 -", or on its shelf.
	if m := kodiTitled.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) && !inAnime(shelved(file, dir)) {
		if n := atoi(m[1]); n%100 > 0 && (!inSeason || n/100 == season) {
			return mp.Numbering{Season: n / 100, Episode: n % 100}, true
		}
	}
	// So is "Fennick 1-05" in Fennick's own folder: the show's name, then its season and episode
	// apart by a dash.
	if m := dashedSeason.FindStringSubmatch(name); m != nil && dir != "." && !inSeason && atoi(m[3]) > 0 &&
		!leadingTags.MatchString(name) && !inAnime(shelved(file, dir)) {
		before, _, _ := searchQuery(m[1], 0)
		if own, _, _ := searchQuery(showFolderName(show), 0); relatedName(before, own) {
			return mp.Numbering{Season: atoi(m[2]), Episode: atoi(m[3])}, true
		}
	}
	if dir == "." {
		// Loose at the top, "Bramblecourt E01" beside "Bramblecourt E02" number one show's episodes.
		// A fansub's "[Kumo] Show - E20" is numbered across its show (looseAnime).
		if m := spelledEpisode.FindStringSubmatch(name); m != nil && file.Notes.Spelled && !leadingTags.MatchString(name) {
			return episode(atoi(m[2]))
		}
		return mp.Numbering{}, false
	}
	// "Tiddlywinks Farm - Series 2 - 05 - Muddy Day" is series 2's fifth.
	if m := seasonDashEpisode.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	// So is a folder named by its show and season, "Bracken Hall (S07)", but for an anime's pack.
	seasoned := inSeason || namedFolder && !leadingTags.MatchString(path.Base(dir)) && !inAnime(shelved(file, dir))
	if seasoned && season > 0 && !leadingTags.MatchString(name) {
		// "Kettle Bay - 3.05" or "1.05 Hedge" in Season 3 or 1, before "1" reads as a number alone;
		// and the season's number then the episode's apart by a dash, "Bramley Court - 3 - 01" or
		// "2-03" in Season 3 or 2.
		for _, re := range []*regexp.Regexp{seasonDotEpisode, seasonDashedEpisode} {
			if m := re.FindStringSubmatch(name); m != nil && atoi(m[1]) == season {
				return episode(atoi(m[2]))
			}
		}
		// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is
		// the season's episode: an anime's number across its show is three digits, as Sonarr writes
		// it, and a fansub's carries its group's tag.
		// A show's season folder named by both, "Larchmere Series 2", names the show before its season.
		own := showFolderName(show)
		if m := namedSeason.FindStringSubmatch(show); namedFolder && m != nil {
			own = strings.TrimSpace(m[1])
		}
		n, ok := numberAfterShow(name, own, file.Notes.Numbered)
		// Three digits whose hundred is the folder's season, "Harbor 24 - 103" in Season 1, are its
		// episode as Kodi writes it.
		if ok && n.Absolute >= 100 && n.Absolute < 1000 && n.Absolute/100 == season && n.Absolute%100 > 0 {
			return episode(n.Absolute % 100)
		}
		if e, ok := titledDigitAfter(name, own); ok {
			return episode(e)
		}
		if ok && n.Absolute > 0 && n.Absolute < 100 && seasonTwoDigits.MatchString(name) {
			return episode(n.Absolute)
		}
	}
	if _, n, ok := animeRelease(path.Base(file.Rel), inAnime(shelved(file, dir))); ok {
		return n, true
	}
	// A catch-all's file ("Movies/Ferrow Lane Part 2 of 3 (2010).mkv") has no show folder to number
	// it after, unless it is one of several numbered after a name they share ("Cartoons/Fizzy Fox
	// 01.mp4" beside "… 02").
	if !inSeason && catchAllFolder(show) && !file.Notes.Numbered && !file.Notes.Spelled {
		return mp.Numbering{}, false
	}
	// "Rook Hall Mini E01" beside "… E02", and "Ash Vale - Night 1" beside "… Night 2".
	if m := spelledEpisode.FindStringSubmatch(name); m != nil && file.Notes.Spelled {
		return episode(atoi(m[2]))
	}
	if m := capMark.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 && !yearToken.MatchString(m[1]+m[2]) {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	// "1.01 Arrival" and "Fallowmere - 1.09" in a show's own folder, with no season folder to say
	// the season, write it before the episode as "3.05" does in one.
	if m := seasonDotEpisode.FindStringSubmatch(name); m != nil && !inSeason && !namedFolder && atoi(m[1]) > 0 && atoi(m[2]) > 0 &&
		airDateIn(name) == "" {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	if m := episodeWord.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	// "Hollow Sea/E01.mkv": a name that is its episode's mark alone is its folder's show's.
	if m := episodeOnly.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := cjkEpisode.FindStringSubmatch(name); m != nil {
		return episode(atoi(matched(m[1], m[2])))
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	// "201 Return" beside "101 Pilot" in a show's own folder is season 2's first (compositeNumbers).
	if m := leadingComposite.FindStringSubmatch(name); m != nil && !inSeason && file.Notes.Composite {
		n := atoi(m[1])
		return mp.Numbering{Season: n / 100, Episode: n % 100}, true
	}
	// So do a show's episodes numbered alone in its own folder ("Ocean Halls/01 - The Deep.mkv").
	if m := leadingNumber.FindStringSubmatch(name); (inSeason || namedFolder || file.Notes.Leading) && m != nil {
		// "101 - Tidewater" in Season 1 is its first episode, when every numbered name in the folder
		// is in its hundred; "101" among "99" and "100" is the hundred and first.
		if n := atoi(m[1]); season > 0 && file.Notes.Hundreds == season {
			return episode(n % 100)
		}
		// A number alone with no season folder, "Fizzy Hen/01.mp4", is its place in the whole show.
		if !inSeason && !namedFolder && digits(name) {
			return mp.Numbering{Absolute: atoi(m[1])}, atoi(m[1]) > 0
		}
		return episode(atoi(m[1]))
	}
	if inSeason && season > 0 {
		// "Kettle Bay - 3.05" in Season 3, and "Kettle Bay E03" in any season.
		if m := seasonDotEpisode.FindStringSubmatch(name); m != nil && atoi(m[1]) == season {
			return episode(atoi(m[2]))
		}
		if m := bareEpisode.FindStringSubmatch(name); m != nil {
			return episode(atoi(m[1]))
		}
	}
	// "Kettle Bay E01" in the show's own folder, with no season folder, is its first season's.
	if m := bareEpisode.FindStringSubmatchIndex(name); !inSeason && m != nil {
		before, _, _ := searchQuery(name[:m[0]], 0)
		own, _, _ := searchQuery(showFolderName(show), 0)
		if relatedName(before, own) {
			return episode(atoi(name[m[2]:m[3]]))
		}
	}
	n, ok := numberAfterShow(name, showFolderName(show), file.Notes.Numbered)
	// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is the
	// season's episode: an anime's number across its show is three digits, as Sonarr writes it. So
	// is one in a folder named by its show and season, "TV/Owlbeck Season 2/Owlbeck 01".
	if ok && n.Absolute > 0 && (inSeason || namedFolder) && season > 0 && seasonTwoDigits.MatchString(name) {
		return episode(n.Absolute)
	}
	// A show's folder in a shows' catch-all numbers its episodes by a number alone or after the
	// show's name, "TV/Ivory Fen/02.mkv" and "Anime/Hoshizora/Hoshizora 12.mkv": its episode by
	// its place in the whole show. In a film's folder, "Marrow Point 2" is the sequel.
	if !ok && !inSeason && (underShows(shelved(file, dir)) || file.Notes.Series || showLabelled(path.Base(dir))) {
		// A miniseries' "Ridge Point Part 2" and "Ridge Fall - Part Two" are its episodes.
		// Its "Part 2" straight after the show's name is no film's sequel here (partIn's titlesOwnPart).
		if _, part, ok := partIn(name + ".mkv"); ok && part > 0 {
			return episode(part)
		}
		if m := capturedPart.FindStringSubmatch(name); m != nil && atoi(m[1]) > 0 {
			return episode(atoi(m[1]))
		}
		if m := partWord.FindStringSubmatch(name); m != nil {
			if part, ok := spelledNumbers[strings.ToLower(m[1])]; ok {
				return episode(part)
			}
		}
		words, own := strings.Fields(normName(name)), showWordsOf(show)
		if len(words) == len(own)+1 && slices.Equal(words[:len(own)], own) || len(words) == 1 {
			if last := words[len(words)-1]; digits(last) && len(last) <= 3 && atoi(last) > 0 {
				// A folder named by its show and season, "Pebble Pals Season 1", gives the season.
				if namedFolder {
					return episode(atoi(last))
				}
				return mp.Numbering{Absolute: atoi(last)}, true
			}
		}
	}
	return n, ok
}

// numberAfterShow reads a number after a show's name: "Kettle Bay - 013" and "Kettle Bay 013" are
// its absolute episode 13, and "Kettle.Bay.102" its season 1, episode 2, as are the files of a
// folder that number theirs after a name they share ("kb 101", "kb 201"). A year is never one.
func numberAfterShow(name, show string, numbered bool) (mp.Numbering, bool) {
	words := func(s string) []string {
		return strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(s)))
	}
	// The show is its title without its year, and the name's year is passed over: "Kettle Bay (2019)
	// - 013" in "Kettle Bay (2019)".
	title, _, _ := searchQuery(show, 0)
	nameWords, showWords := slices.DeleteFunc(words(name), func(w string) bool {
		return len(w) == 6 && w[0] == '(' && w[5] == ')' && yearToken.MatchString(w[1:5])
	}), words(title)
	at := len(showWords)
	if at == 0 || len(nameWords) <= at || !slices.Equal(nameWords[:at], showWords) {
		if !numbered {
			return mp.Numbering{}, false
		}
		at = slices.IndexFunc(nameWords, func(w string) bool { return digits(w) })
		if at < 1 {
			return mp.Numbering{}, false
		}
	}
	dashed := nameWords[at] == "-"
	if dashed && len(nameWords) > at+1 {
		at++
	}
	num, _, _ := strings.Cut(nameWords[at], "-") // "001-003" starts at 1
	if !digits(num) || len(num) > 4 || yearToken.MatchString(num) {
		return mp.Numbering{}, false
	}
	n := atoi(num)
	switch {
	case dashed || num[0] == '0':
		return mp.Numbering{Absolute: n}, n > 0
	case len(num) >= 3:
		return mp.Numbering{Season: n / 100, Episode: n % 100}, n%100 > 0
	}
	// "Marrow Point 2" is a sequel as likely as an episode.
	return mp.Numbering{}, false
}

// titledDigitAfter is the episode a name in a season's folder numbers with one digit after its
// show's name and its title after a dash, as a British series' episodes are named: "Larchmere 1 -
// The Gate" in "Larchmere Series 2". Outside a season "Marrow Point 2" is as likely a sequel.
func titledDigitAfter(name, show string) (int, bool) {
	words := func(s string) []string {
		return strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(s)))
	}
	title, _, _ := searchQuery(show, 0)
	nameWords, showWords := words(name), words(title)
	at := len(showWords)
	if at == 0 || len(nameWords) < at+3 || !slices.Equal(nameWords[:at], showWords) {
		return 0, false
	}
	num := nameWords[at]
	if len(num) != 1 || !digits(num) || num == "0" || nameWords[at+1] != "-" {
		return 0, false
	}
	return atoi(num), true
}
