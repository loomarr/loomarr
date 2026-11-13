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
