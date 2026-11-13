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
//
// It is read in two steps. First the file's place: what its folders say before its name is read
// (folderPlace). Then its name, by the rules in numberingRules, in their order: the first rule
// that decides gives the numbering, or says there is none. The order is the rules' precedence,
// stated in one place, in four tiers:
//
//  1. A name that numbers itself whatever its folder: "Season 5 Episode 2", "Show - Episode 3",
//     "show.312.hdtv", "Part 2 of 6", Kodi's "Show - 214 - Title".
//  2. A name at the top, with no folder to read it by.
//  3. A name in a season's folder, read by that season: "3.05", "Show 06", "Larchmere 1 - The Gate".
//  4. A name read by what its folder's videos say together (folderNotes): a show's spelled
//     episodes, its numbers run together ("101", "201"), its leading numbers, its "E01".
//
// A name no rule decides is numbered after its show's name, if at all (showNumbered).

// partWord matches a part spelled out at a name's end: "Ridge Fall - Part One".
var partWord = regexp.MustCompile(`(?i)(?:^|[ ._-])part[ ._-]+([a-z]+)$`)

// folderPlace is what a file's folders say before its name is read.
type folderPlace struct {
	file File
	// dir is the file's folder, relative to the root.
	dir string
	// raw is the file's stem; name is it less its release's notes at its end: "Show - Part One
	// [1080p]" is "Show - Part One". A fansub's notes say what it is, so its rules read raw.
	raw, name string
	// show is the folder named as the file's show: the one above its season's folder, or its own.
	show string
	// season is the season its folders give, 1 when none does.
	season int
	// inSeason says a season's folder holds it; namedFolder that its own folder names its show
	// and season, "Puddle Pals Season 2/" or a pack's "Pike.Moss.S02.1080p/".
	inSeason, namedFolder bool
	// anime says an anime's shelf holds it.
	anime bool
}

// placeOf reads a file's place (folderPlace).
func placeOf(file File) folderPlace {
	dir := path.Dir(file.Rel)
	p := folderPlace{
		file: file, dir: dir, raw: stem(path.Base(file.Rel)), name: plainStem(path.Base(file.Rel)),
		show: path.Base(dir), season: 1, anime: inAnime(shelved(file, dir)),
	}
	if seasonDir := seasonDirOf(dir); seasonDir != "" {
		p.inSeason, p.show = true, path.Base(aboveSeasons(seasonDir))
		if n, ok := seasonIn(path.Base(seasonDir)); ok {
			p.season = n
		} else if showSpecials.MatchString(path.Base(seasonDir)) {
			p.season = 0
		} else if m := namedSeason.FindStringSubmatch(path.Base(seasonDir)); m != nil {
			p.season, _ = seasonNumberWord(matched(m[2], m[3]))
		}
	} else if n := namedSeasonOf(p.show); n > 0 {
		// A show's season folder with no show folder above it, "Puddle Pals Season 2/", is the
		// show's and gives the season.
		p.season, p.namedFolder = n, true
	} else if n := packSeason(p.show); n > 0 && packShow(dir) != "" {
		// So does a season pack's release folder, "Pike.Moss.S02.1080p/".
		p.season, p.namedFolder = n, true
	}
	return p
}

// episode is the place's season's episode e; ok is false for none.
func (p folderPlace) episode(e int) (mp.Numbering, bool) {
	return mp.Numbering{Season: p.season, Episode: e}, e > 0
}

// tagged says the name carries a release group's tag first, "[Kumo] Show - 05": a fansub's,
// numbered across its show (animeRelease).
func (p folderPlace) tagged() bool { return leadingTags.MatchString(p.name) }

// numberingRule reads one way a name numbers itself in its place. decided says the rule settles
// the file's numbering, as n and ok, or that it has none (ok false); undecided passes it on.
type numberingRule func(p folderPlace) (n mp.Numbering, ok, decided bool)

// decide is a rule's answer that settles a file's numbering.
func decide(n mp.Numbering, ok bool) (mp.Numbering, bool, bool) { return n, ok, true }

// undecided is a rule's answer that passes the file on to the next.
func undecided() (mp.Numbering, bool, bool) { return mp.Numbering{}, false, false }

// numberingRules are the rules a name is read by, in their precedence (see the file's comment).
var numberingRules = []numberingRule{
	// 1. A name that numbers itself whatever its folder.
	makeMKVTitleRule,
	seasonEpisodeWordsRule,
	dashedEpisodeRule,
	animeEpisodeWordRule,
	sceneNumberRule,
	capturedEpisodeRule,
	partOfWordRule,
	countedPartRule,
	titledPartRule,
	shelvedEpisodeRule,
	kodiTitledRule,
	dashedSeasonRule,
	// 2. A name at the top, with no folder to read it by.
	topRule,
	// 3. A name in a season's folder.
	seasonDashEpisodeRule,
	seasonedRule,
	// A fansub's release name, read as its group writes it, before the folder's notes.
	animeReleaseRule,
	// 4. A name read by what its folder's videos say together.
	catchAllRule,
	spelledEpisodeRule,
	capMarkRule,
	looseSeasonDotRule,
	episodeWordRule,
	episodeOnlyRule,
	cjkEpisodeRule,
	partOfRule,
	compositeRule,
	leadingNumberRule,
	seasonBareRule,
	ownBareEpisodeRule,
}

// numberingInFolder is the numbering a file's folders let its name give, where the name alone does
// not say it is an episode: a show's file "Ep02", "2of6" or "Season 1 Episode 2"; a season folder's
// "05 - Tidewater"; the show's name and a number, "Kettle Bay 104" (season 1, episode 4, as Kodi
// reads it) or, as Sonarr writes an anime's, "Kettle Bay - 013" (episode 13 of all). A season
// folder gives the season; with none it is the first. A file loose at the top has no show to say so.
func numberingInFolder(file File) (mp.Numbering, bool) {
	p := placeOf(file)
	for _, rule := range numberingRules {
		if n, ok, decided := rule(p); decided {
			return throughLast(n, p.name), ok
		}
	}
	n, ok := p.showNumbered()
	return throughLast(n, p.name), ok
}

// throughLast is a folder's numbering for a file that holds its episode and the next few, joined
// on after a dash, "&" or "and": "Brackwold - 03-04", "Brackwold 05 & 06" run through 4 and 6.
// "Show 03-14" is no file's run of twelve episodes, and "Show 2-1" runs nowhere.
func throughLast(n mp.Numbering, name string) mp.Numbering {
	if n.Episode == 0 || n.EpisodeEnd != 0 {
		return n
	}
	for _, m := range numberRun.FindAllStringSubmatch(name, -1) {
		if last := atoi(m[2]); atoi(m[1]) == n.Episode && last > n.Episode && last-n.Episode <= 3 {
			n.EpisodeEnd = last
			break
		}
	}
	return n
}

// numberRun matches two numbers a file runs between: "03-04", "05 & 06", "7 and 8".
var numberRun = regexp.MustCompile(`(?i)(?:^|[^0-9a-z])(\d{1,3})(?:[ ._]*[-&+][ ._]*|[ ._]+and[ ._]+)(\d{1,3})(?:[^0-9]|$)`)

// makeMKVTitleRule: MakeMKV's title, "Kite Shore Disc 1_t01", numbers its disc's titles, never the
// show's episodes: every title on disc 1 would claim the first. Its length or the catalog places it.
func makeMKVTitleRule(p folderPlace) (mp.Numbering, bool, bool) {
	if makeMKVTitle.MatchString(p.name) {
		return decide(mp.Numbering{}, false)
	}
	return undecided()
}

// seasonEpisodeWordsRule: "Bellcote - Season 5 Episode 2" and "Dunbar.Season.3.Episode.1" name
// their season too, which is theirs before the folder's.
func seasonEpisodeWordsRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := seasonEpisodeWords.FindStringSubmatch(p.name); m != nil {
		return decide(mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, atoi(m[2]) > 0)
	}
	return undecided()
}

// dashedEpisodeRule: "Spotty Dog - Episode 3" names its show and its episode wherever it is, loose
// or in a catch-all: its season folder's, or the first season's. A fansub's, "[Kumo] Show -
// Episode 17", is its number in the whole show (looseAnime).
func dashedEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := dashedEpisode.FindStringSubmatch(p.name); m != nil && !p.tagged() {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// animeEpisodeWordRule: on an anime's shelf, an untagged "Mizu no Oto Ep 05" is as "Tsukishiro Kan
// Episode 09" is.
func animeEpisodeWordRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := episodeWord.FindStringSubmatch(p.name); m != nil && !p.tagged() && p.anime {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// sceneNumberRule: a scene release's "larchmont.road.312.hdtv-lol" is season 3, episode 12.
func sceneNumberRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := sceneNumber.FindStringSubmatch(p.raw); m != nil && atoi(m[2]) > 0 {
		return decide(mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true)
	}
	return undecided()
}

// capturedEpisodeRule: a TV capture's part, "Zelston.Fall.Pt1.HDTV", "Ashlington.Rise.3of6.HDTV" or
// "Jevington.E01.HDTV", is a broadcast's episode, never a film's part.
func capturedEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if n, ok := capturedEpisode(p.raw); ok {
		return decide(p.episode(n))
	}
	return undecided()
}

// partOfWordRule: a miniseries' "Part 2 of 6" after its name says what it is wherever it is.
func partOfWordRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := partOfWord.FindStringSubmatch(p.name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// countedPartRule: "Docs/Wild Moor 1of3" is a miniseries' first of three wherever it is; a film's
// part gives its year, "Ferrow Lane 2 of 3 (2010)".
func countedPartRule(p folderPlace) (mp.Numbering, bool, bool) {
	if n := countedPart(p.name); n > 0 {
		return decide(p.episode(n))
	}
	return undecided()
}

// titledPartRule: a miniseries' part with its own title after it, "Wild Shore (2020) - Part 1 -
// Tides"; a film's part never has one.
func titledPartRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := titledPart.FindStringSubmatch(p.name); m != nil && atoi(m[2]) > 0 {
		return decide(p.episode(atoi(m[2])))
	}
	return undecided()
}

// shelvedEpisodeRule: a shows' catch-all's "TV/Lower Wick 1-01.mkv" (shelvedEpisode).
func shelvedEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if _, n, ok := shelvedEpisode(path.Base(p.file.Rel), p.dir); ok {
		return decide(n, true)
	}
	return undecided()
}

// kodiTitledRule: Kodi's "Cutters Bay - 214 - Late" is season 2's fourteenth: three digits between
// dashes, its episode's title after. An anime's number across its show is padded, "- 014 -", or on
// its shelf.
func kodiTitledRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := kodiTitled.FindStringSubmatch(p.name); m != nil && !p.tagged() && !p.anime {
		if n := atoi(m[1]); n%100 > 0 && (!p.inSeason || n/100 == p.season) {
			return decide(mp.Numbering{Season: n / 100, Episode: n % 100}, true)
		}
	}
	return undecided()
}

// dashedSeasonRule: "Fennick 1-05" in Fennick's own folder is the show's name, then its season and
// episode apart by a dash.
func dashedSeasonRule(p folderPlace) (mp.Numbering, bool, bool) {
	m := dashedSeason.FindStringSubmatch(p.name)
	if m == nil || p.dir == "." || p.inSeason || atoi(m[3]) == 0 || p.tagged() || p.anime {
		return undecided()
	}
	before, _, _ := searchQuery(m[1], 0)
	if own, _, _ := searchQuery(showFolderName(p.show), 0); relatedName(before, own) {
		return decide(mp.Numbering{Season: atoi(m[2]), Episode: atoi(m[3])}, true)
	}
	return undecided()
}

// topRule: loose at the top, "Bramblecourt E01" beside "Bramblecourt E02" number one show's
// episodes; nothing else there is numbered by its folder, as it has none. A fansub's "[Kumo] Show
// - E20" is numbered across its show (looseAnime).
func topRule(p folderPlace) (mp.Numbering, bool, bool) {
	if p.dir != "." {
		return undecided()
	}
	if m := spelledEpisode.FindStringSubmatch(p.name); m != nil && p.file.Notes.Spelled && !p.tagged() {
		return decide(p.episode(atoi(m[2])))
	}
	return decide(mp.Numbering{}, false)
}

// seasonDashEpisodeRule: "Tiddlywinks Farm - Series 2 - 05 - Muddy Day" is series 2's fifth.
func seasonDashEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := seasonDashEpisode.FindStringSubmatch(p.name); m != nil && atoi(m[2]) > 0 {
		return decide(mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true)
	}
	return undecided()
}

// seasonedRule reads a name by its season's folder, or by a folder named by its show and season,
// "Bracken Hall (S07)", but for an anime's pack.
func seasonedRule(p folderPlace) (mp.Numbering, bool, bool) {
	seasoned := p.inSeason || p.namedFolder && !leadingTags.MatchString(path.Base(p.dir)) && !p.anime
	if !seasoned || p.season <= 0 || p.tagged() {
		return undecided()
	}
	// "Kettle Bay - 3.05" or "1.05 Hedge" in Season 3 or 1, before "1" reads as a number alone;
	// and the season's number then the episode's apart by a dash, "Bramley Court - 3 - 01" or
	// "2-03" in Season 3 or 2.
	for _, re := range []*regexp.Regexp{seasonDotEpisode, seasonDashedEpisode} {
		if m := re.FindStringSubmatch(p.name); m != nil && atoi(m[1]) == p.season {
			return decide(p.episode(atoi(m[2])))
		}
	}
	// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is
	// the season's episode: an anime's number across its show is three digits, as Sonarr writes
	// it, and a fansub's carries its group's tag.
	// A show's season folder named by both, "Larchmere Series 2", names the show before its season.
	own := showFolderName(p.show)
	if m := namedSeason.FindStringSubmatch(p.show); p.namedFolder && m != nil {
		own = strings.TrimSpace(m[1])
	}
	n, ok := numberAfterShow(p.name, own, p.file.Notes.Numbered)
	// Three digits whose hundred is the folder's season, "Harbor 24 - 103" in Season 1, are its
	// episode as Kodi writes it.
	if ok && n.Absolute >= 100 && n.Absolute < 1000 && n.Absolute/100 == p.season && n.Absolute%100 > 0 {
		return decide(p.episode(n.Absolute % 100))
	}
	if e, ok := titledDigitAfter(p.name, own); ok {
		return decide(p.episode(e))
	}
	if ok && n.Absolute > 0 && n.Absolute < 100 && seasonTwoDigits.MatchString(p.name) {
		return decide(p.episode(n.Absolute))
	}
	return undecided()
}

// animeReleaseRule: a fansub's release name, "[Kumo] Show - 07 [1080p]" (animeRelease).
func animeReleaseRule(p folderPlace) (mp.Numbering, bool, bool) {
	if _, n, ok := animeRelease(path.Base(p.file.Rel), p.anime); ok {
		// Its number across the show is its season's episode where it can be nothing else: in the
		// first season's folder the two count alike, and a number below its season's, "Season 3/[Grp]
		// Shiroi Kumo - 01", was never counted across the show. Sonarr leaves "[Grp] Show - 13" in
		// Season 2 when 13 is the second season's first, so a number there is the catalog's to place.
		if abs := n.Absolute; p.inSeason && n.Season == 0 && n.Episode == 0 && abs > 0 && abs < 100 &&
			(p.season == 1 || p.season > 1 && abs < p.season) {
			return decide(p.episode(abs))
		}
		return decide(n, true)
	}
	return undecided()
}

// catchAllRule: a catch-all's file ("Movies/Ferrow Lane Part 2 of 3 (2010).mkv") has no show
// folder to number it after, unless it is one of several numbered after a name they share
// ("Cartoons/Fizzy Fox 01.mp4" beside "… 02").
func catchAllRule(p folderPlace) (mp.Numbering, bool, bool) {
	if !p.inSeason && catchAllFolder(p.show) && !p.file.Notes.Numbered && !p.file.Notes.Spelled {
		return decide(mp.Numbering{}, false)
	}
	return undecided()
}

// spelledEpisodeRule: "Rook Hall Mini E01" beside "… E02", and "Ash Vale - Night 1" beside "…
// Night 2" (folderNotes.Spelled).
func spelledEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := spelledEpisode.FindStringSubmatch(p.name); m != nil && p.file.Notes.Spelled {
		return decide(p.episode(atoi(m[2])))
	}
	return undecided()
}

// capMarkRule: a season and episode written as capMark writes them, but for a year.
func capMarkRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := capMark.FindStringSubmatch(p.name); m != nil && atoi(m[2]) > 0 && !yearToken.MatchString(m[1]+m[2]) {
		return decide(mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true)
	}
	return undecided()
}

// looseSeasonDotRule: "1.01 Arrival" and "Fallowmere - 1.09" in a show's own folder, with no
// season folder to say the season, write it before the episode as "3.05" does in one. A day,
// "2019.10.05", is no season's.
func looseSeasonDotRule(p folderPlace) (mp.Numbering, bool, bool) {
	m := seasonDotEpisode.FindStringSubmatch(p.name)
	if m != nil && !p.inSeason && !p.namedFolder && atoi(m[1]) > 0 && atoi(m[2]) > 0 && airDateIn(p.name) == "" {
		return decide(mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true)
	}
	return undecided()
}

// episodeWordRule: an episode's word and its number, "Ep02", in a show's folder.
func episodeWordRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := episodeWord.FindStringSubmatch(p.name); m != nil {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// episodeOnlyRule: "Hollow Sea/E01.mkv", a name that is its episode's mark alone, is its folder's
// show's.
func episodeOnlyRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := episodeOnly.FindStringSubmatch(p.name); m != nil {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// cjkEpisodeRule: an episode numbered as Japanese and Chinese write it, "第05話".
func cjkEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := cjkEpisode.FindStringSubmatch(p.name); m != nil {
		return decide(p.episode(atoi(matched(m[1], m[2]))))
	}
	return undecided()
}

// partOfRule: a miniseries' "2of6" or "2 of 6".
func partOfRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := partOf.FindStringSubmatch(p.name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// compositeRule: "201 Return" beside "101 Pilot" in a show's own folder is season 2's first
// (folderNotes.Composite).
func compositeRule(p folderPlace) (mp.Numbering, bool, bool) {
	if m := leadingComposite.FindStringSubmatch(p.name); m != nil && !p.inSeason && p.file.Notes.Composite {
		n := atoi(m[1])
		return decide(mp.Numbering{Season: n / 100, Episode: n % 100}, true)
	}
	return undecided()
}

// leadingNumberRule: a show's episodes numbered alone, first in their names, in a season's folder
// or its own ("Ocean Halls/01 - The Deep.mkv", folderNotes.Leading).
func leadingNumberRule(p folderPlace) (mp.Numbering, bool, bool) {
	m := leadingNumber.FindStringSubmatch(p.name)
	if m == nil || !p.inSeason && !p.namedFolder && !p.file.Notes.Leading {
		return undecided()
	}
	// "101 - Tidewater" in Season 1 is its first episode, when every numbered name in the folder
	// is in its hundred; "101" among "99" and "100" is the hundred and first.
	if n := atoi(m[1]); p.season > 0 && p.file.Notes.Hundreds == p.season {
		return decide(p.episode(n % 100))
	}
	// A number alone with no season folder, "Fizzy Hen/01.mp4", is its place in the whole show.
	if !p.inSeason && !p.namedFolder && digits(p.name) {
		return decide(mp.Numbering{Absolute: atoi(m[1])}, atoi(m[1]) > 0)
	}
	return decide(p.episode(atoi(m[1])))
}

// seasonBareRule: "Kettle Bay - 3.05" in Season 3, and "Kettle Bay E03" in any season.
func seasonBareRule(p folderPlace) (mp.Numbering, bool, bool) {
	if !p.inSeason || p.season <= 0 {
		return undecided()
	}
	if m := seasonDotEpisode.FindStringSubmatch(p.name); m != nil && atoi(m[1]) == p.season {
		return decide(p.episode(atoi(m[2])))
	}
	if m := bareEpisode.FindStringSubmatch(p.name); m != nil {
		return decide(p.episode(atoi(m[1])))
	}
	return undecided()
}

// ownBareEpisodeRule: "Kettle Bay E01" in the show's own folder, with no season folder, is its
// first season's.
func ownBareEpisodeRule(p folderPlace) (mp.Numbering, bool, bool) {
	m := bareEpisode.FindStringSubmatchIndex(p.name)
	if p.inSeason || m == nil {
		return undecided()
	}
	before, _, _ := searchQuery(p.name[:m[0]], 0)
	if own, _, _ := searchQuery(showFolderName(p.show), 0); relatedName(before, own) {
		return decide(p.episode(atoi(p.name[m[2]:m[3]])))
	}
	return undecided()
}

// showNumbered is the numbering a name no rule decides gives by a number after its show's name
// (numberAfterShow), or, in a show's own folder, by a part or a number alone.
func (p folderPlace) showNumbered() (mp.Numbering, bool) {
	n, ok := numberAfterShow(p.name, showFolderName(p.show), p.file.Notes.Numbered)
	// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is the
	// season's episode: an anime's number across its show is three digits, as Sonarr writes it. So
	// is one in a folder named by its show and season, "TV/Owlbeck Season 2/Owlbeck 01".
	if ok && n.Absolute > 0 && (p.inSeason || p.namedFolder) && p.season > 0 && seasonTwoDigits.MatchString(p.name) {
		return p.episode(n.Absolute)
	}
	// A show's folder in a shows' catch-all numbers its episodes by a number alone or after the
	// show's name, "TV/Ivory Fen/02.mkv" and "Anime/Hoshizora/Hoshizora 12.mkv": its episode by
	// its place in the whole show. In a film's folder, "Marrow Point 2" is the sequel.
	if !ok && !p.inSeason && (underShows(shelved(p.file, p.dir)) || p.file.Notes.Series || showLabelled(path.Base(p.dir))) {
		// A miniseries' "Ridge Point Part 2" and "Ridge Fall - Part Two" are its episodes.
		// Its "Part 2" straight after the show's name is no film's sequel here (partIn's titlesOwnPart).
		if _, part, ok := partIn(p.name + ".mkv"); ok && part > 0 {
			return p.episode(part)
		}
		if m := capturedPart.FindStringSubmatch(p.name); m != nil && atoi(m[1]) > 0 {
			return p.episode(atoi(m[1]))
		}
		if m := partWord.FindStringSubmatch(p.name); m != nil {
			if part, ok := spelledNumbers[strings.ToLower(m[1])]; ok {
				return p.episode(part)
			}
		}
		words, own := strings.Fields(normName(p.name)), showWordsOf(p.show)
		if len(words) == len(own)+1 && slices.Equal(words[:len(own)], own) || len(words) == 1 {
			if last := words[len(words)-1]; digits(last) && len(last) <= 3 && atoi(last) > 0 {
				// A folder named by its show and season, "Pebble Pals Season 1", gives the season.
				if p.namedFolder {
					return p.episode(atoi(last))
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
