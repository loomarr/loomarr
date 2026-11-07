package scan

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/store"
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
			detail.Files = append(detail.Files, itemFile(f, item.sources[f.Path]))
		}
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

// scannedItem is what the scanner wrote about an item: its newest present scan origin's facts and
// their provenance, and each present file's header facts by path.
type scannedItem struct {
	facts   inventory.ItemFacts
	from    DetailsFrom
	sources map[string]inventory.SourceFacts
}

// scanned is what the scanner wrote about an item; zero when the inventory has none.
func (l *Library) scanned(ctx context.Context, id string) (scannedItem, error) {
	item, ok, err := l.Inventory.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(id)})
	if err != nil || !ok {
		return scannedItem{}, err
	}
	out := scannedItem{sources: map[string]inventory.SourceFacts{}}
	for _, s := range item.Sources {
		for _, o := range s.Origins {
			if strings.HasPrefix(string(o.Key.Authority), authorityPrefix) && o.MissingAt.IsZero() && o.Locator.Path != "" {
				out.sources[o.Locator.Path] = o.Observation.Facts
			}
		}
	}
	var best *inventory.ItemOrigin
	for i, o := range item.Origins {
		if !strings.HasPrefix(string(o.Key.Authority), authorityPrefix) || !o.MissingAt.IsZero() {
			continue
		}
		if best == nil || o.Observation.ObservedAt.After(best.Observation.ObservedAt) {
			best = &item.Origins[i]
		}
	}
	if best != nil {
		out.facts, out.from = best.Observation.Facts, detailsFromExtension(best.Observation.Extension)
	}
	return out, nil
}
