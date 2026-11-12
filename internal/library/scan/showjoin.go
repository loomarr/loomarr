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
		if p := places[id.seriesDir]; p != nil {
			p.episodes++
			continue
		}
		s := b.series[id.seriesDir]
		if s == nil || s.managed != nil {
			continue
		}
		name, year := showIdentity(path.Base(id.seriesDir))
		if name == "" {
			continue
		}
		p := &showPlace{dir: id.seriesDir, year: year, real: b.dirs[id.seriesDir], tagged: leadingTags.MatchString(path.Base(id.seriesDir)), episodes: 1}
		places[id.seriesDir] = p
		byName[name] = append(byName[name], p)
	}
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
		for year := range years {
			var same []*showPlace
			for _, p := range group {
				if p.year == year {
					same = append(same, p)
				}
			}
			b.joinInto(home(same).dir, same, join)
		}
	}
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
	year := 0
	if m := bracketYear.FindString(s); m != "" {
		year = atoi(m[1 : len(m)-1])
	}
	// A release's batch notes and its season: "(01-12)", "[1080p]", "(Season 2)", "S2". Any other
	// bracket is the show's name: "Quiet Office (US)" and "Quiet Office (UK)" are two shows, as are
	// "Tin Cup" and "Tin Cup (Animated)".
	s = bracketGroup.ReplaceAllStringFunc(s, func(g string) string {
		inner := strings.TrimSpace(g[1 : len(g)-1])
		if bracketYear.MatchString(g) || episodeRange.MatchString(g) || releaseNotes.MatchString(g) ||
			trailingSeason.MatchString(" "+inner) || digits(strings.ReplaceAll(inner, "-", "")) {
			return " "
		}
		return g
	})
	s = trailingSeason.ReplaceAllString(strings.TrimSpace(s), "")
	title, y, _ := searchQuery(showFolderName(strings.TrimSpace(s)), 0)
	if year == 0 {
		year = y
	}
	return normName(title), year
}

var (
	// bracketGroup matches a bracketed group anywhere in a folder's name.
	bracketGroup = regexp.MustCompile(`[(\[][^)\]]*[)\]]`)
	// trailingSeason matches a season's mark at a folder's name's end: "S2", "Season 2", "- S02".
	trailingSeason = regexp.MustCompile(`(?i)[ ._-]+(?:s\d{1,2}|season[ ._]*\d{1,2})$`)
)
