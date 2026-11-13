package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// Anime releases: a fansub's or a group's own name, "[Group] Show - 07 [1080p]", read as its
// show's episode by its absolute number, its specials and recaps as season 0's, and a shows'
// shelf's "Show 1-01" by its season and episode.

// looseAnime reads a fansub release's own name, "[Group] Show - 07 [1080p]" or
// "[Group][Show][07][1080p]": its show and its absolute episode, or, where the show's name ends in
// its season ("Show S2 - 03", "Show 2nd Season - 03"), that season's episode. Without the group's
// tag a name is not read so: "Show - 07" could be a title of its own.
func looseAnime(name string) (string, mp.Numbering, bool) { return animeRelease(name, false) }

// trailingEpisodeWord matches an episode's word last in a show's name, before the number it reads:
// "Mizu no Oto Ep", "Mizu no Oto Episode".
var trailingEpisodeWord = regexp.MustCompile(`(?i)[ ._-]+(?:ep|episode)\.?$`)

// animeRelease is looseAnime for a file in an anime's folder too ("Anime/"), where a name is a
// release's as if its group had tagged it: "Anime/Usagi Den 07 [1080p].mkv", "Anime/Kumo Tetsu - 07
// (BD 1080p) [Grp].mkv", "Anime/Usagi.Den.08.1080p.mkv", read without its release's words.
func animeRelease(name string, inAnime bool) (string, mp.Numbering, bool) {
	s := stem(name)
	if inAnime && !leadingTags.MatchString(s) {
		words := strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(s))
		if at := slices.IndexFunc(words, releaseToken); at > 0 {
			words = words[:at]
		}
		s = strings.Join(words, " ")
	}
	// A group's tag last, "Show - 06 [Kumo]", says a release as one first does, for a number written
	// as an anime's is, padded or after a dash: "Apollo Ridge 13 [Criterion]" is a film's.
	grouped := false
	if plain := trimReleaseNotes(s); !leadingTags.MatchString(plain) {
		if m := trailingGroup.FindStringSubmatchIndex(plain); m != nil && groupTag(plain[m[2]:m[3]]) {
			s, grouped = plain[:m[0]], true
		}
	}
	tagged := inAnime || leadingTags.MatchString(s)
	// A release's special, "[Kumo] Show - SP01" or "Show - OVA 2 [Kumo]", is the show's season 0's.
	// An OVA with no number, "[Kumo] Show - OVA", is still the season's; a "Special" alone may be a
	// title's word.
	if m := animeSpecial.FindStringSubmatch(leadingTags.ReplaceAllString(s, "")); m != nil && (tagged || grouped) &&
		(atoi(m[2]) > 0 || m[2] == "" && ovaWord.MatchString(m[0])) {
		return showSpelling(m[1]), mp.Numbering{Season: 0, Episode: atoi(m[2])}, true
	}
	// A release's recap with no number, "[Grp] Hoshino Rail - Recap", is the show's season 0's, as
	// "- Recap - 06.5" is: no film is tagged so.
	if bare := leadingTags.ReplaceAllString(s, ""); tagged || grouped {
		if m := animeAside.FindStringSubmatch(strings.TrimSpace(trimReleaseNotes(bare))); m != nil && !strings.EqualFold(m[2], "special") {
			return showSpelling(m[1]), mp.Numbering{Season: 0}, true
		}
	}
	show, number, zero := "", 0, false
	if m := animeBrackets.FindStringSubmatch(s); m != nil {
		show, number = m[1], atoi(m[2])
	} else if tagged || grouped || paddedAbsolute.MatchString(plainStem(name)) ||
		releaseNotes.MatchString(s) && dashedLast.MatchString(plainStem(name)) {
		// With no group's tag, a number written as Sonarr writes an anime's ("Show - 08", "Show -
		// 013"), or a dashed number last before a release's notes ("Show - 7 [1080p]"), say a name
		// is a release's; "Show - 7", "Show 11 [1080p]" and "Show - 03 - Tidewater [1080p]" may be
		// titles or a season's episodes. A year says a film, "[Tracker] Show 11 (2001)", unless a
		// dashed number follows it: "[Kumo] Show (2019) - 07" is the 2019 show's episode.
		bare := leadingTags.ReplaceAllString(s, "")
		m := animeDashed.FindStringSubmatch(bare)
		if m == nil {
			m = animeNumber.FindStringSubmatch(bare)
		}
		// A year after a dashed number of two digits or more is the release's, "[Grp] Kaze Tori - 03
		// (2019)", "- 12 (2019)", "- 112 (2019)": a film is never numbered so.
		if m != nil && bracketYear.MatchString(s) && (m[2] == "" || !bracketYear.MatchString(m[1]) && len(m[2]) < 2) {
			m = nil
		}
		if m != nil && (tagged || m[2] != "" || grouped && strings.HasPrefix(m[3], "0")) && !yearToken.MatchString(matched(m[2], m[3])) {
			show, number = m[1], atoi(matched(m[2], m[3]))
			// A tagged release's "- 00" is its prologue, an episode before the first: the show's,
			// placed by the catalog.
			zero = number == 0 && (tagged || grouped) && m[2] != ""
		}
	}
	// A tagged release numbered as Japanese and Chinese write it: "[Kumo] Hoshi Kettle 第03話".
	if bare := leadingTags.ReplaceAllString(s, ""); number == 0 && bare != s {
		if m := cjkEpisode.FindStringSubmatch(bare); m != nil {
			show, number = strings.TrimSpace(bare[:strings.Index(bare, m[0])]), atoi(matched(m[1], m[2]))
		}
	}
	if number == 0 && !zero || show == "" {
		return "", mp.Numbering{}, false
	}
	// "Mizu no Oto Ep 05" is "Mizu no Oto"'s: the word before a number is the number's.
	if bare := trailingEpisodeWord.ReplaceAllString(show, ""); strings.TrimSpace(bare) != "" {
		show = bare
	}
	// "[Grp] Kaze no Brindle (BD 1080p) - 05" is "Kaze no Brindle"'s: its notes are the release's.
	if bare := trimReleaseNotes(show); strings.TrimSpace(bare) != "" {
		show = bare
	}
	show = showSpelling(show)
	// "[Grp] Kame Specials - 01" and "[Grp] Kame OVA - 02" are Kame's season 0's.
	if m := showSpecials.FindStringSubmatch(show); m != nil {
		return strings.TrimSpace(m[1]), mp.Numbering{Season: 0, Episode: number}, true
	}
	// So are "[Grp] Kame - Special - 01" and its "[Grp] Kame - Recap - 06.5": a recap's number is
	// the episode it follows, never its own.
	if m := animeAside.FindStringSubmatch(show); m != nil {
		if strings.EqualFold(m[2], "special") {
			return strings.TrimSpace(m[1]), mp.Numbering{Season: 0, Episode: number}, true
		}
		return strings.TrimSpace(m[1]), mp.Numbering{Season: 0}, true
	}
	if m := animeSeason.FindStringSubmatch(show); m != nil {
		season := atoi(matched(m[2], m[3], m[4]))
		return strings.TrimSpace(m[1]), mp.Numbering{Season: season, Episode: number}, true
	}
	return show, mp.Numbering{Absolute: number}, true
}

var (
	// titledPart matches a miniseries' part with its own title after it: "Wild Shore (2020) - Part
	// 1 - Tides". A film's part never has one.
	titledPart = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+part[ ._]*(\d{1,2})[ ._]+-[ ._]+\S`)
	// dashedSeason matches a show's name, then its season and episode apart by a dash: "Lower Wick
	// 1-01".
	dashedSeason = regexp.MustCompile(`^(.+?)[ ._]+(\d{1,2})-(\d{2,3})(?:[ ._]|$)`)
)

// shelvedEpisode is the show and numbering a name gives as a show's file in a shows' catch-all,
// "TV/Lower Wick 1-01.mkv": its season and episode apart by a dash. Anywhere else "1-01" may be a
// title's. An anime's "Shiro Neko - 01-02", after a dash, a group's tag or on an anime's shelf, is
// a batch of episodes from its first (animeRelease).
func shelvedEpisode(name, dir string) (string, mp.Numbering, bool) {
	m := dashedSeason.FindStringSubmatch(plainStem(name))
	if m == nil || dir == "." || !showsFolderNames[folderKey(path.Base(dir))] || atoi(m[3]) == 0 ||
		strings.HasSuffix(strings.TrimSpace(m[1]), "-") || leadingTags.MatchString(m[1]) || animeFolderNames[folderKey(path.Base(dir))] {
		return "", mp.Numbering{}, false
	}
	return showSpelling(m[1]), mp.Numbering{Season: atoi(m[2]), Episode: atoi(m[3])}, true
}

// countedPart is the part a name with no year counts out of its whole, "Wild Moor 1of3" 1: a
// miniseries' episode. 0 for a name that counts none, or gives a year, as a film's part does.
func countedPart(name string) int {
	m := partOf.FindStringSubmatch(name)
	if m == nil || atoi(m[1]) < 1 || atoi(m[1]) > atoi(m[2]) || atoi(m[2]) < 2 {
		return 0
	}
	if _, year, _ := searchQuery(name, 0); year != 0 || bracketYear.MatchString(name) {
		return 0
	}
	return atoi(m[1])
}

// groupTag says whether a bracketed word at a name's end is a release group's: not a release's
// note ("1080p", "HEVC"), an edition ("Extended") or a year.
func groupTag(inner string) bool {
	return !releaseToken(inner) && !releaseNote(inner) && !editionOnly([]string{inner}) && !yearToken.MatchString(inner)
}

// animeSeason matches a fansub show's name that ends in its season, bracketed or not: "Show S2",
// "Show 2nd Season", "Show Season 2", "Show (Season 2)".
var animeSeason = regexp.MustCompile(`(?i)^(.+?)[ ._]+[(\[]?(?:s(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)[ ._]+season|season[ ._]+(\d{1,2}))[)\]]?$`)

// kodiTitled matches Kodi's episode named by its show, its season and episode in three digits,
// and its title, apart by dashes: "Cutters Bay - 214 - Late".
var kodiTitled = regexp.MustCompile(`^.+?[ ._]+-[ ._]+([1-9]\d{2})[ ._]+-[ ._]+\S`)
