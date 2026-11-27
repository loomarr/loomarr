package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// joinShows makes each show one show however many places its episodes are kept in: the loose
// episodes a download left ("Downloads/Gull.Rock.S02E02.mkv"), a release's batch folder
// ("Anime/[Grp] Usagi Hana S2 (13-24) [1080p]/") and a second shelf's copy ("TV 2/Haven Hill/")
// join the show's own folder ("TV/Haven Hill/"). Each file is read alone (embeddedRecord); only
// the whole pass sees that the folders name one show, so the pass joins them after reading every
// file and before writing any.
//
// Shows join by the name and year their folders give, the year and all: "Blue.Fen.2020.S01E01"
// may be a remake of the "Blue Fen" kept with no year, so it keeps its own. A show an arr manages
// keeps its own, as do two that name different catalog ids.
//
// others are the identities of files the pass knows but doesn't walk (a folder pass's rest): they
// count toward where a show lives, so a folder pass joins a show as a full walk does.
func (b *builder) joinShows(ids, others []identity) {
	byName := map[string][]*showPlace{}
	places := map[string]*showPlace{}
	for _, id := range slices.Concat(others, ids) {
		if id.kind != mp.KindEpisode || id.seriesDir == "" {
			continue
		}
		p := places[id.seriesDir]
		if p == nil {
			s := b.series[id.seriesDir]
			if s == nil || s.managed != nil {
				continue
			}
			name, year := showIdentity(path.Base(id.seriesDir))
			if name == "" {
				continue
			}
			p = &showPlace{dir: id.seriesDir, year: year, real: b.dirs[id.seriesDir], tagged: leadingTags.MatchString(path.Base(id.seriesDir)),
				named: map[string]int{}, marks: map[[2]int]bool{}}
			places[id.seriesDir] = p
			byName[name] = append(byName[name], p)
		}
		p.episodes++
		if id.named != "" {
			title, _, _ := searchQuery(id.named, 0)
			p.named[normName(title)]++
		}
		if n := id.numbering(); n != nil && n.Episode > 0 {
			p.marks[[2]int{n.Season, n.Episode}] = true
		}
	}
	retypo(byName)
	join := map[string]string{}
	for _, group := range byName {
		if len(group) < 2 {
			continue
		}
		years := map[int]bool{}
		for _, p := range group {
			years[p.year] = true
		}
		// The show's own folder is its home: a real, untagged folder before a release's or a loose
		// name's, the one with the most episodes, then the first by name.
		home := func(ps []*showPlace) *showPlace {
			return slices.MinFunc(ps, func(a, c *showPlace) int {
				switch {
				case a.real && !a.tagged != (c.real && !c.tagged):
					if a.real && !a.tagged {
						return -1
					}
					return 1
				case a.episodes != c.episodes:
					return c.episodes - a.episodes
				}
				return strings.Compare(a.dir, c.dir)
			})
		}
		homes := map[int]*showPlace{}
		for year := range years {
			var same []*showPlace
			for _, p := range group {
				if p.year == year {
					same = append(same, p)
				}
			}
			homes[year] = home(same)
			b.joinInto(homes[year].dir, same, join)
		}
		// A show's folders kept with and without its year, "TV/Dunmore/Season 1/" beside "TV/Dunmore
		// (2016)/Season 2/", are one show when it is the only dated one of its name, they number no
		// episode alike, and their seasons run on (seasonsApart): a remake kept beside its original
		// numbers its own first season again.
		if len(years) != 2 || !years[0] {
			continue
		}
		delete(homes, 0)
		for _, dated := range homes {
			var yearless []*showPlace
			for _, p := range group {
				if p.year == 0 {
					yearless = append(yearless, p)
				}
			}
			if !slices.ContainsFunc(yearless, func(p *showPlace) bool { return sharesMark(p, group, dated.year) }) &&
				!seasonsApart(seasonsOf(yearless), seasonsOf(slices.DeleteFunc(slices.Clone(group), func(p *showPlace) bool { return p.year != dated.year }))) {
				b.joinInto(dated.dir, append(yearless, dated), join)
			}
		}
	}
	b.applyJoins(join, ids, others)
}

// retypo moves a show folder named with a slip of the keys, "TV/Brackenfiled/", to the show its
// episodes spell, when another folder is named so: every episode in it names "Brackenfield", one
// slip from its folder's name, and "TV/Brackenfield/" is a show's. A short name ("Lost", "Last") is
// too near other words to say it was a slip.
func retypo(byName map[string][]*showPlace) {
	for name, group := range byName {
		if len(group) != 1 || len(name) < 6 {
			continue
		}
		p := group[0]
		if len(p.named) != 1 {
			continue
		}
		for spelled, n := range p.named {
			if n == p.episodes && len(byName[spelled]) > 0 && oneSlip(name, spelled) {
				byName[spelled] = append(byName[spelled], p)
				delete(byName, name)
			}
		}
	}
}

// applyJoins moves every episode of each show join names (from → to) to the show it joins, and
// that show's counts with it.
func (b *builder) applyJoins(join map[string]string, ids, others []identity) {
	if len(join) == 0 {
		return
	}
	for _, list := range [][]identity{others, ids} {
		for i := range list {
			to, ok := join[list[i].seriesDir]
			if !ok {
				continue
			}
			from := list[i].seriesDir
			list[i].seriesDir = to
			if n := list[i].numbering(); n != nil && n.Episode > 0 {
				s := b.series[to]
				s.last[n.Season] = max(s.last[n.Season], n.Episode, n.EpisodeEnd)
			}
			b.seen[seriesOrigin(b.root, to)] = true
			delete(b.seen, seriesOrigin(b.root, from))
		}
	}
	for from, to := range join {
		s, home := b.series[from], b.series[to]
		for year, n := range s.years {
			home.years[year] += n
		}
		for name, n := range s.named {
			home.named[name] += n
		}
		if len(home.ids) == 0 {
			home.ids = s.ids
		}
		delete(b.series, from)
	}
}

// joinInto records that each of places but home joins home, unless the catalog ids their folders
// name differ from home's.
func (b *builder) joinInto(home string, places []*showPlace, join map[string]string) {
	for _, p := range places {
		if p.dir == home || !sameIDs(b.series[p.dir].ids, b.series[home].ids) {
			continue
		}
		join[p.dir] = home
	}
}

// showPlace is a folder a show's episodes are kept in, or a loose show's key, as joinShows weighs
// it: real is a folder on disk, tagged a release's folder ("[Grp] …").
type showPlace struct {
	dir      string
	year     int
	real     bool
	tagged   bool
	episodes int
	// named counts the shows its episodes' file names give, by normName.
	named map[string]int
	// marks are the season and episode each of its episodes numbers.
	marks map[[2]int]bool
}

// seasonsOf are the seasons places' episodes number.
func seasonsOf(places []*showPlace) []int {
	var seasons []int
	for _, p := range places {
		for mark := range p.marks {
			seasons = append(seasons, mark[0])
		}
	}
	return seasons
}

// sharesMark says whether p numbers an episode as any of group's places of year does.
func sharesMark(p *showPlace, group []*showPlace, year int) bool {
	for _, o := range group {
		if o.year != year {
			continue
		}
		for mark := range p.marks {
			if o.marks[mark] {
				return true
			}
		}
	}
	return false
}

// sameIDs says whether two shows' ids agree: one names none, or they share one.
func sameIDs(a, c []mp.ExternalID) bool {
	if len(a) == 0 || len(c) == 0 {
		return true
	}
	for _, x := range a {
		if slices.Contains(c, x) {
			return true
		}
	}
	return false
}

// showIdentity is the name and year a show's folder, or the key a loose show is kept by, gives it:
// "[Grp] Usagi Hana S2 (13-24) [1080p]" is "usagi hana", "Haven Hill (2019)" "haven hill" 2019.
func showIdentity(base string) (string, int) {
	s := leadingTags.ReplaceAllString(base, "")
	tagged := s != base
	year := 0
	if m := bracketYear.FindString(s); m != "" {
		year = atoi(m[1 : len(m)-1])
	}
	// A release's batch notes and its season: "(01-12)", "[1080p]", "(Season 2)", "S2". Any other
	// bracket is the show's name: "Quiet Office (US)" and "Quiet Office (UK)" are two shows, as are
	// "Tin Cup" and "Tin Cup (Animated)".
	s = bracketGroup.ReplaceAllStringFunc(s, func(g string) string {
		inner := strings.TrimSpace(g[1 : len(g)-1])
		if bracketYear.MatchString(g) || episodeRange.MatchString(g) || releaseNotes.MatchString(g) || batchNote.MatchString(inner) ||
			trailingSeason.MatchString(" "+inner) || digits(strings.ReplaceAll(inner, "-", "")) || releaseCRC.MatchString(inner) {
			return " "
		}
		return g
	})
	s = trailingSeason.ReplaceAllString(strings.TrimSpace(s), "")
	// A tagged release's folder holding one episode, "[Grp] Ame Furu - 07 [1080p] [ABCD1234]", is
	// its show's: the number is the episode's.
	if m := animeDashed.FindStringSubmatch(s); tagged && m != nil && strings.TrimSpace(m[1]) != "" {
		s = m[1]
	}
	title, y, _ := searchQuery(showFolderName(strings.TrimSpace(s)), 0)
	if year == 0 {
		year = y
	}
	return normName(title), year
}

var (
	// batchNote matches a release's note that it holds a run whole: "[Batch]", "(Complete)", "[Fin]".
	batchNote = regexp.MustCompile(`(?i)^(?:batch|complete|fin|final|end)$`)
	// releaseCRC matches a release's checksum, eight hex digits: "ABCD1234".
	releaseCRC = regexp.MustCompile(`^[0-9A-Fa-f]{8}$`)
	// bracketGroup matches a bracketed group anywhere in a folder's name.
	bracketGroup = regexp.MustCompile(`[(\[][^)\]]*[)\]]`)
	// trailingSeason matches a season's mark at a folder's name's end: "S2", "Season 2", "- S02".
	trailingSeason = regexp.MustCompile(`(?i)[ ._-]+(?:s\d{1,2}|season[ ._]*\d{1,2})$`)
)
