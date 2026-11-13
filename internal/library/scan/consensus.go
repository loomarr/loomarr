package scan

import (
	"path"
	"slices"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// settleFolders lets a show's own folder settle what the folders inside it are. Each file is read
// alone (embeddedRecord), and a folder a show keeps its episodes in by a name no rule knows ("Round
// 01 - Harbour/", "Reel B/") reads as a show of that name. Only the whole pass sees that the folder
// above it is a show's home, with episodes of its own, so the pass settles it after reading every
// file: a folder inside a show's home is the home's when nothing says it is a show of its own.
//
// A folder is a show of its own when its name runs on the home's ("Star Ridge/Star Ridge
// Origins/"), when it keeps season folders of its own, when its episodes name it or another show,
// when it names a catalog id, when an arr manages it, or when it numbers an episode the home
// numbers too: a spin-off kept in its franchise's folder starts its own first season.
func (b *builder) settleFolders(ids, others []identity) {
	episodes := map[string]int{}
	marks := map[string]map[[2]int]bool{}
	for _, id := range slices.Concat(others, ids) {
		if id.kind != mp.KindEpisode || id.seriesDir == "" {
			continue
		}
		episodes[id.seriesDir]++
		if n := id.numbering(); n != nil && n.Episode > 0 {
			if marks[id.seriesDir] == nil {
				marks[id.seriesDir] = map[[2]int]bool{}
			}
			marks[id.seriesDir][[2]int{n.Season, n.Episode}] = true
		}
	}
	// A folder that keeps season folders of its own, or years' (seasonOf), is a show's: a folder
	// that groups a show's episodes holds them, not seasons.
	seasoned := map[string]bool{}
	for d := range b.dirs {
		if seasonOf(d) {
			seasoned[path.Dir(d)] = true
		}
	}
	join := map[string]string{}
	for dir := range episodes {
		s := b.series[dir]
		if s == nil || s.managed != nil || len(s.ids) > 0 || !b.dirs[dir] || seasoned[dir] {
			continue
		}
		home := homeAbove(dir, func(d string) bool { return episodes[d] >= 2 && b.dirs[d] && b.series[d] != nil })
		if home == "" || ownShow(dir, home, s) || sharesAnyMark(marks[dir], marks[home]) {
			continue
		}
		join[dir] = home
	}
	b.applyJoins(join, ids, others)
}

// splitNamed lets a show's folder settle which of its episodes are another show's. A show's
// folder names its episodes' show whatever their files say, as its files may spell it their own
// way ("Corrow Fen/CF S01E01.mkv"). But an episode its file names as another show, numbered as
// one of the folder's own, is that show's: two first episodes of season one are two shows'
// ("Corrow Fen/Season 1/Brindle Point S01E01.mkv" beside "Corrow Fen S01E01.mkv"), and a
// spin-off kept in its franchise's folder starts its own first season. So is one its file names as
// another show in a film's folder, numbered as anything: "Movies/Yarrow Lake (2001)/Yarrow Lake -
// The Series S01E01.mkv" is the series'. The folder's own episodes are the ones named as it is or
// not named at all.
func (b *builder) splitNamed(ids, others []identity) {
	type key struct {
		dir, name string
	}
	own := map[string]map[[2]int]bool{}
	for _, list := range [][]identity{others, ids} {
		for _, id := range list {
			if n := id.numbering(); id.kind == mp.KindEpisode && n != nil && n.Episode > 0 && ownEpisode(id) {
				if own[id.seriesDir] == nil {
					own[id.seriesDir] = map[[2]int]bool{}
				}
				own[id.seriesDir][[2]int{n.Season, n.Episode}] = true
			}
		}
	}
	split := map[key]bool{}
	for _, list := range [][]identity{others, ids} {
		for _, id := range list {
			n := id.numbering()
			s := b.series[id.seriesDir]
			if id.kind != mp.KindEpisode || n == nil || ownEpisode(id) || s == nil || s.managed != nil ||
				len(s.ids) > 0 || !b.dirs[id.seriesDir] || !own[id.seriesDir][[2]int{n.Season, n.Episode}] && !amongFilms(id.seriesDir) {
				continue
			}
			split[key{id.seriesDir, normName(id.named)}] = true
		}
	}
	if len(split) == 0 {
		return
	}
	for _, list := range [][]identity{others, ids} {
		for i := range list {
			id := &list[i]
			if id.kind != mp.KindEpisode || !split[key{id.seriesDir, normName(id.named)}] {
				continue
			}
			from := b.series[id.seriesDir]
			to := showKey(path.Dir(id.seriesDir), id.named)
			s := b.series[to]
			if s == nil {
				s = &seriesSeen{dir: to, last: map[int]int{}, years: map[int]int{}, named: map[string]int{}}
				b.series[to] = s
			}
			if n := id.numbering(); n != nil && n.Episode > 0 {
				s.last[n.Season] = max(s.last[n.Season], n.Episode, n.EpisodeEnd)
			}
			name := normName(id.named)
			s.named[name]++
			if from.named[name]--; from.named[name] <= 0 {
				delete(from.named, name)
			}
			id.seriesDir = to
			b.seen[seriesOrigin(b.root, to)] = true
		}
	}
}

// ownEpisode says whether an episode is its show folder's own by its file's name: named as the
// folder is, its year or not ("Dark Ridge (2017) - S03E02" in "Dark Ridge/"), or not named at all.
func ownEpisode(id identity) bool {
	if id.named == "" {
		return true
	}
	bare := func(name string) string {
		title, _, _ := searchQuery(name, 0)
		return title
	}
	for _, folder := range []string{showFolderName(path.Base(id.seriesDir)), withoutShowLabels(path.Base(id.seriesDir))} {
		if sameWords(folder, id.named) || sameWords(bare(folder), bare(id.named)) {
			return true
		}
	}
	return false
}

// homeAbove is the nearest folder above dir that isHome says is a show's home, "" for none before a
// catch-all or the root: a shelf ("TV/", "Kids/") is no show's.
func homeAbove(dir string, isHome func(string) bool) string {
	for d := path.Dir(dir); d != "."; d = path.Dir(d) {
		if shelfAt(d) {
			return ""
		}
		if isHome(d) {
			return d
		}
	}
	return ""
}

// ownShow says whether the folder dir inside a show's home is a show of its own by its name or its
// episodes': its name runs on the home's, or its episodes name it, or name a show other than the
// home's.
func ownShow(dir, home string, s *seriesSeen) bool {
	own, homeName := showFolderName(path.Base(dir)), showFolderName(path.Base(home))
	if relatedName(own, homeName) || s.named[normName(own)] > 0 {
		return true
	}
	for name := range s.named {
		if !relatedName(name, homeName) {
			return true
		}
	}
	return false
}

// sharesAnyMark says whether two shows' episodes number any one episode alike.
func sharesAnyMark(a, c map[[2]int]bool) bool {
	for mark := range a {
		if c[mark] {
			return true
		}
	}
	return false
}
