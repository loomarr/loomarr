package scan

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Library → Items (#2151 step 10): the movies and series the scanner wrote, and one title's page.

// ItemDetail is one title as its page shows it.
type ItemDetail struct {
	store.LibraryItemSummary
	// Facts is what the scanner resolved for it: overview, genres, rating, runtime.
	Facts inventory.ItemFacts
	// From is where its details came from.
	From DetailsFrom
	// EpisodeNamesFrom is where most of a series' episode names came from.
	EpisodeNamesFrom mp.ID
	// Files is a movie's present files; a series lists its episodes in Seasons instead.
	Files []ItemFile
	// Seasons is a series' episodes by season, in order; season 0 is the specials.
	Seasons []Season
}

// ItemFile is one of a movie's files, with what its header says once read; zero until then.
type ItemFile struct {
	store.LibraryFile
	LengthMillis int64
	// Picture is its first video stream; nil for none.
	Picture *Picture
	// AudioChannels is the sound that plays: the default audio stream, else the first.
	AudioChannels int
	// Subtitles is each subtitle stream's language in order, "" where the file doesn't say.
	Subtitles []string
	// Edition is the labelled version of the title it is, "" for the default (decision 54).
	Edition string
	// Extra is a copy that never plays, because another of the title's files in its edition does
	// (decisions 37, 54).
	Extra bool
	// managed is true when its arr lists it.
	managed bool
}

// markExtras marks every file but the one that plays in its edition as an extra copy (decisions
// 37, 54). One file is never an extra, nor is a title's only copy of an edition.
func markExtras(files []ItemFile) {
	if len(files) < 2 {
		return
	}
	plays := playingCopies(len(files), func(i int) string { return files[i].Edition },
		func(i int) bool { return files[i].managed }, func(i int) int {
			if files[i].Picture == nil {
				return 0
			}
			return files[i].Picture.Width * files[i].Picture.Height
		}, func(i int) int64 { return files[i].Size })
	for i := range files {
		files[i].Extra = !plays[i]
	}
}

// playingCopy is which of a title's n copies plays (decision 37): the file its arr lists, else
// the sharpest picture, else the biggest file; the first of equals.
func playingCopy(n int, managed func(int) bool, pixels func(int) int, size func(int) int64) int {
	best := 0
	for i := 1; i < n; i++ {
		if c := cmp.Or(compareBool(managed(i), managed(best)), cmp.Compare(pixels(i), pixels(best)),
			cmp.Compare(size(i), size(best))); c > 0 {
			best = i
		}
	}
	return best
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

// Picture is a video stream's size and range.
type Picture struct {
	Width, Height int
	HDR           bool
}

// itemFile is f with what its header facts say.
func itemFile(f store.LibraryFile, facts inventory.SourceFacts) ItemFile {
	out := ItemFile{LibraryFile: f, LengthMillis: facts.DurationMillis}
	var audio *inventory.Stream
	for i, s := range facts.Streams {
		switch s.Kind {
		case inventory.StreamVideo:
			if out.Picture == nil {
				out.Picture = &Picture{Width: s.Width, Height: s.Height, HDR: s.HDR}
			}
		case inventory.StreamAudio:
			if audio == nil || (s.Disposition.Default && !audio.Disposition.Default) {
				audio = &facts.Streams[i]
			}
		case inventory.StreamSubtitle:
			out.Subtitles = append(out.Subtitles, s.Language)
		}
	}
	if audio != nil {
		out.AudioChannels = audio.Channels
	}
	return out
}

// Season is one season's episodes in order.
type Season struct {
	Number   int
	Episodes []Episode
}

// Episode is one episode a series' files hold.
type Episode struct {
	ID     string
	Name   string
	Number int
	// End is the last episode a multi-episode file holds, 0 for one.
	End int
	// Ready is false while any of its files' headers is unread.
	Ready bool
}

// Items lists the movies and series with a present file.
func (l *Library) Items(ctx context.Context, q store.LibraryItemQuery) (store.LibraryItemPage, error) {
	return l.ItemStore.ListLibraryItems(ctx, q)
}

// Item is one title's page; store.ErrNotFound when it has no present file.
func (l *Library) Item(ctx context.Context, id string) (ItemDetail, error) {
	summary, err := l.ItemStore.LibraryItem(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	detail := ItemDetail{LibraryItemSummary: summary}
	item, err := l.scanned(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	detail.Facts, detail.From = item.facts, item.from
	files, err := l.ItemStore.LibraryItemFiles(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	if summary.Kind != string(kindSeries) {
		for _, f := range files {
			source := item.sources[f.Path]
			file := itemFile(f, source.facts)
			file.managed, file.Edition = source.managed, source.edition
			detail.Files = append(detail.Files, file)
		}
		markExtras(detail.Files)
		return detail, nil
	}
	detail.Seasons, detail.EpisodeNamesFrom, err = l.seasons(ctx, files)
	return detail, err
}

// Decider is what decides which title an item is, as "Wrong title?" needs it (decision 26).
type Decider string

const (
	// DecidedByApp is the arr that manages it: the title changes there.
	DecidedByApp Decider = "app"
	// DecidedByFile is the file itself: an id in its name, or an NFO beside it.
	DecidedByFile Decider = "file"
	// DecidedByYou is the maintainer's answer.
	DecidedByYou Decider = "you"
	// DecidedBySearch is a catalog search that agreed on name, year and length (decision 56).
	DecidedBySearch Decider = "search"
	// DecidedByNothing is nothing yet: the title needs the maintainer.
	DecidedByNothing Decider = "none"
)

// DecidedBy is what decides which title m says an item is.
func DecidedBy(m store.LibraryMatch) Decider {
	switch mp.Basis(m.Basis) {
	case mp.BasisManager:
		return DecidedByApp
	case mp.BasisEmbeddedID, mp.BasisSidecar:
		return DecidedByFile
	case mp.BasisLink:
		return DecidedByYou
	case mp.BasisSearch:
		return DecidedBySearch
	default:
		return DecidedByNothing
	}
}

// UpdateDetails is "Update details" (decision 20): the title's catalog record is fetched now, and
// its files are written again, so what its arr and its sidecars say now is read too. A title with
// no catalog record, or a catalog with no key, is written again from what the files and arrs say.
func (l *Library) UpdateDetails(ctx context.Context, id string) error {
	item, err := l.ItemStore.LibraryItem(ctx, id)
	if err != nil {
		return err
	}
	if item.Catalog.String() != "" && l.Enricher != nil {
		if err := l.Enricher.Update(ctx, item.Catalog); err != nil && !errors.Is(err, ErrNoCatalog) {
			return err
		}
	}
	files, err := l.NeedStore.RewriteLibraryItemFiles(ctx, id)
	if err != nil {
		return err
	}
	l.pass(files)
	return nil
}

// CheckFiles is "Check files again": the title's files are opened and their headers read again,
// for a file that was replaced in place or failed to read.
func (l *Library) CheckFiles(ctx context.Context, id string) error {
	if _, err := l.ItemStore.LibraryItem(ctx, id); err != nil {
		return err
	}
	files, err := l.ItemStore.RecheckLibraryItemFiles(ctx, id)
	if err != nil {
		return err
	}
	l.pass(files)
	return nil
}

// pass asks the scanner for a pass over the folders holding files.
func (l *Library) pass(files []store.LibraryFilePath) {
	if l.Scanner != nil {
		rewriteFolders(files, l.Scanner.PassFolders)
	}
}

// seasons groups a series' episode files by the episode item each was written as, and says where
// most of the episode names came from.
func (l *Library) seasons(ctx context.Context, files []store.LibraryFile) ([]Season, mp.ID, error) {
	ready := map[string]bool{}
	var order []string
	for _, f := range files {
		if _, seen := ready[f.ItemID]; !seen {
			order = append(order, f.ItemID)
			ready[f.ItemID] = true
		}
		if f.Header == store.LibraryHeaderPending {
			ready[f.ItemID] = false
		}
	}
	bySeason := map[int][]Episode{}
	named := map[mp.ID]int{}
	for _, id := range order {
		episode, err := l.scanned(ctx, id)
		if err != nil {
			return nil, "", err
		}
		facts := episode.facts
		bySeason[facts.SeasonNumber] = append(bySeason[facts.SeasonNumber], Episode{
			ID: id, Name: facts.Name, Number: facts.EpisodeNumber, End: facts.EpisodeEnd, Ready: ready[id],
		})
		if episode.from.Name != "" {
			named[episode.from.Name]++
		}
	}
	var namesFrom mp.ID
	for p, n := range named {
		if n > named[namesFrom] || (n == named[namesFrom] && p < namesFrom) {
			namesFrom = p
		}
	}
	seasons := make([]Season, 0, len(bySeason))
	for n, eps := range bySeason {
		slices.SortFunc(eps, func(a, b Episode) int {
			return cmp.Or(cmp.Compare(a.Number, b.Number), strings.Compare(a.Name, b.Name))
		})
		seasons = append(seasons, Season{Number: n, Episodes: eps})
	}
	slices.SortFunc(seasons, func(a, b Season) int { return cmp.Compare(a.Number, b.Number) })
	return seasons, namesFrom, nil
}

// EpisodesOnDisk is, for each season of a series on disk, the highest episode number its files
// hold: what a series search is checked against (decision 57). Specials (season 0) and files
// without a number are left out, since nothing lays them out the same way twice.
func (l *Library) EpisodesOnDisk(ctx context.Context, seriesID string) (map[int]int, error) {
	files, err := l.ItemStore.LibraryItemFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	highest := map[int]int{}
	for _, f := range files {
		if seen[f.ItemID] {
			continue
		}
		seen[f.ItemID] = true
		episode, err := l.scanned(ctx, f.ItemID)
		if err != nil {
			return nil, err
		}
		facts := episode.facts
		if facts.SeasonNumber > 0 && facts.EpisodeNumber > 0 {
			highest[facts.SeasonNumber] = max(highest[facts.SeasonNumber], facts.EpisodeNumber, facts.EpisodeEnd)
		}
	}
	return highest, nil
}

// scannedItem is what the scanner wrote about an item: its newest present scan origin's facts and
// their provenance, and each present file by path.
type scannedItem struct {
	facts   inventory.ItemFacts
	from    DetailsFrom
	sources map[string]scannedSource
}

// scannedSource is one of an item's present files: its header facts, and whether its arr lists it.
type scannedSource struct {
	facts   inventory.SourceFacts
	managed bool
	edition string
}

// scanned is what the scanner wrote about an item; zero when the inventory has none.
func (l *Library) scanned(ctx context.Context, id string) (scannedItem, error) {
	item, ok, err := l.Inventory.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(id)})
	if err != nil || !ok {
		return scannedItem{}, err
	}
	present := func(key inventory.OriginKey, missing time.Time) bool {
		return strings.HasPrefix(string(key.Authority), authorityPrefix) && missing.IsZero()
	}
	out := scannedItem{sources: map[string]scannedSource{}}
	ext := map[inventory.OriginKey]originExtension{}
	var best *inventory.ItemOrigin
	for i, o := range item.Origins {
		if !present(o.Key, o.MissingAt) {
			continue
		}
		ext[o.Key] = readOriginExtension(o.Observation.Extension)
		if best == nil || o.Observation.ObservedAt.After(best.Observation.ObservedAt) {
			best = &item.Origins[i]
		}
	}
	if best != nil {
		out.facts, out.from = best.Observation.Facts, ext[best.Key].From
	}
	// A file's source origin shares its item origin's key, which says whether an arr lists it.
	for _, s := range item.Sources {
		for _, o := range s.Origins {
			if present(o.Key, o.MissingAt) && o.Locator.Path != "" {
				out.sources[o.Locator.Path] = scannedSource{facts: o.Observation.Facts, managed: ext[o.Key].Managed,
					edition: ext[o.Key].Edition}
			}
		}
	}
	return out, nil
}
