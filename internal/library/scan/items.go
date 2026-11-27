package scan

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Library → Items (#2151 step 10): the movies and series the scanner wrote, and one title's page.
// The page reads the Library's own rows (#4476); each origin's observation JSON is evidence only
// (decision 39).

// ItemDetail is one title as its page shows it.
type ItemDetail struct {
	librarystore.LibraryItemSummary
	// Facts is what the scanner resolved for it: name, year, original title, overview, tagline,
	// premiere date, score, genres, studios, countries, rating, runtime, and People (each credit's
	// name, role and character, in billing order).
	Facts inventory.ItemFacts
	// From is where its details came from.
	From DetailsFrom
	// EpisodeNamesFrom is where most of a series' episode names came from.
	EpisodeNamesFrom mp.ID
	// Files is a movie's present files; a series lists its episodes in Seasons instead.
	Files []ItemFile
	// Seasons is a series' episodes by season, in order; season 0 is the specials.
	Seasons []Season
	// Channels are the channels whose lineup holds it, by number.
	Channels []librarystore.LibraryItemChannel
	// Still is the image hash of an episode's still (#4476); "" for none, and for a movie or series.
	Still string
}

// ItemFile is one of a movie's files, with what its header says once read; zero until then.
type ItemFile struct {
	librarystore.LibraryFile
	LengthMillis int64
	// Picture is its first video stream; nil for none.
	Picture *Picture
	// AudioChannels is the sound that plays: the file's default audio stream, else its first.
	AudioChannels int
	// Subtitles is each subtitle track's language in order, the file's own then those beside it,
	// "" where it doesn't say.
	Subtitles []string
	// Edition is the labelled version of the title it is, "" for the default (decision 54).
	Edition string
	// Part is which part of a movie in parts it is, from 1; 0 for a whole file.
	Part int
	// Container is the header's format name; Bitrate its overall rate in bit/s, 0 unknown.
	Container string
	Bitrate   int64
	// Streams are every track in index order, the file's own then those beside it; Chapters its
	// chapter marks in order.
	Streams  []librarystore.LibraryStream
	Chapters []librarystore.LibraryChapter
	// Extra is a copy that never plays, because another of the title's listed files plays in its
	// place (decisions 37, 54); see extraCopy.
	Extra bool
	// sourced is true when it has a source row; plays is that row's plays.
	sourced, plays bool
}

// playsInPlaceOf is the listed files that play in files[i]'s place: each other file with a source
// row that plays, in the same edition and either the same part or as a whole file (an edition
// with a whole file plays it, not its parts). It is none when files[i] plays or has no source row
// yet. The source rows follow the inventory and the list follows the walk, and the two disagree
// while a removal waits (a pass that could not hear from an arr marks no origin unseen) and while
// a new file waits for its first write. So a copy whose playing file has gone from the list has
// none, and a file with no source row is neither removable nor in another's place.
func playsInPlaceOf(files []ItemFile, i int) []ItemFile {
	f := files[i]
	if !f.sourced || f.plays {
		return nil
	}
	var out []ItemFile
	for j, g := range files {
		if j != i && g.sourced && g.plays && editionKey(g.Edition) == editionKey(f.Edition) && (g.Part == f.Part || g.Part == 0) {
			out = append(out, g)
		}
	}
	return out
}

// extraCopy is whether files[i] is an extra copy: a listed file plays in its place, so a title's
// last file is never offered for removal.
func extraCopy(files []ItemFile, i int) bool { return len(playsInPlaceOf(files, i)) > 0 }

// PlayingCopy is the file of d that plays when the title airs: of the files whose source row
// plays, the default edition's before a labelled one, then a whole file or its first part, then
// list order. ok is false for a series, which lists its episodes instead, and for a title none
// of whose files plays yet.
func (d ItemDetail) PlayingCopy() (f ItemFile, ok bool) {
	rank := func(g ItemFile) int {
		r := 0
		if g.Edition != "" {
			r += 2
		}
		if g.Part > 1 {
			r++
		}
		return r
	}
	for _, g := range d.Files {
		if g.sourced && g.plays && (!ok || rank(g) < rank(f)) {
			f, ok = g, true
		}
	}
	return f, ok
}

// Picture is a video stream's size and range.
type Picture struct {
	Width, Height int
	HDR           bool
}

// itemFile is f with what its source row says; a file with no row yet is zero beside f.
func itemFile(f librarystore.LibraryFile, src librarystore.LibrarySource, ok bool) ItemFile {
	out := ItemFile{LibraryFile: f}
	if !ok {
		return out
	}
	out.sourced = true
	out.LengthMillis, out.Edition, out.Part, out.plays = src.DurationMs, src.Edition, src.Part, src.Plays
	out.Container, out.Bitrate, out.Streams, out.Chapters = src.Container, src.Bitrate, src.Streams, src.Chapters
	var audio *librarystore.LibraryStream
	for i, s := range src.Streams {
		own := s.ExternalRef == ""
		switch inventory.StreamKind(s.Kind) {
		case inventory.StreamVideo:
			if own && out.Picture == nil {
				out.Picture = &Picture{Width: s.Width, Height: s.Height, HDR: s.HDR != ""}
			}
		case inventory.StreamAudio:
			if own && (audio == nil || (s.Default && !audio.Default)) {
				audio = &src.Streams[i]
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

// sourceOf is the row of the file at path among sources.
func sourceOf(sources []librarystore.LibrarySource, path string) (librarystore.LibrarySource, bool) {
	for _, s := range sources {
		if s.Path == path {
			return s, true
		}
	}
	return librarystore.LibrarySource{}, false
}

// detailsFrom is the page's provenance from a library row's _from columns. An episode's rating
// taken from its show (librarystore.RatingFromShow) names no provider.
func detailsFrom(f librarystore.LibraryFrom) DetailsFrom {
	rating := mp.ID(f.Rating)
	if f.Rating == librarystore.RatingFromShow {
		rating = ""
	}
	return DetailsFrom{Name: mp.ID(f.Name), Description: mp.ID(f.Overview), Rating: rating,
		Year: mp.ID(f.Year), Premiere: mp.ID(f.Premiere), Score: mp.ID(f.Score), Runtime: mp.ID(f.Runtime)}
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
	// FromFile is an episode named and numbered by its file until its show's order is said
	// (#2151 decision 53).
	FromFile bool
	// Named is true when a catalogue or sidecar gave its name; false while it is its file's.
	Named bool
	// Overview is its own story. LengthMillis is the measured length of its files that play once
	// each is read, else the catalog's runtime; 0 when neither is known.
	Overview     string
	LengthMillis int64
	// Still is the image hash of its still (#4476); "" for none.
	Still string
}

// Items lists the movies and series with a present file.
func (l *Library) Items(ctx context.Context, q librarystore.LibraryItemQuery) (librarystore.LibraryItemPage, error) {
	return l.ItemStore.ListLibraryItems(ctx, q)
}

// Genres lists the genres of the listed titles of kind, with how many carry each; for member,
// only the titles a household member reads.
func (l *Library) Genres(ctx context.Context, kind string, member bool) ([]librarystore.LibraryGenre, error) {
	return l.ItemStore.LibraryGenres(ctx, kind, member)
}

// Item is one title's page; store.ErrNotFound when it has no present file, and for a member when
// a household member doesn't read it (librarystore.MemberLibraryItem).
func (l *Library) Item(ctx context.Context, id string, member bool) (ItemDetail, error) {
	read := l.ItemStore.LibraryItem
	if member {
		read = l.ItemStore.MemberLibraryItem
	}
	summary, err := read(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	detail := ItemDetail{LibraryItemSummary: summary}
	if detail.Channels, err = l.ItemStore.LibraryItemChannels(ctx, id); err != nil {
		return ItemDetail{}, err
	}
	page, err := l.ItemStore.LibraryItemDetail(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	detail.Facts = inventory.ItemFacts{Name: summary.Name, ProductionYear: summary.Year, Overview: page.Overview,
		OriginalTitle: page.OriginalTitle, Tagline: page.Tagline, PremiereDate: page.PremiereDate,
		CommunityScore: page.Score, Genres: page.Genres, Studios: page.Studios, Countries: page.Countries,
		OfficialRating: page.Rating, RuntimeMillis: page.RuntimeMs}
	for _, c := range page.Credits {
		detail.Facts.People = append(detail.Facts.People, inventory.Person{Name: c.Name, Role: c.Role, Character: c.Character})
	}
	detail.From = detailsFrom(page.From)
	detail.Still = page.Still
	files, err := l.ItemStore.LibraryItemFiles(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	if summary.Kind != string(kindSeries) {
		for _, f := range files {
			src, ok := sourceOf(page.Sources, f.Path)
			detail.Files = append(detail.Files, itemFile(f, src, ok))
		}
		for i := range detail.Files {
			detail.Files[i].Extra = extraCopy(detail.Files, i)
		}
		return detail, nil
	}
	detail.Seasons, detail.EpisodeNamesFrom = seasons(files, page.Episodes)
	return detail, nil
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
func DecidedBy(m librarystore.LibraryMatch) Decider {
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
func (l *Library) pass(files []librarystore.LibraryFilePath) {
	if l.Scanner != nil {
		rewriteFolders(files, l.Scanner.PassFolders)
	}
}

// seasons groups a series' episode files by the item each was written as, and says where most of
// the episode names came from. A file not yet written as an item is an episode with no name or
// number.
func seasons(files []librarystore.LibraryFile, episodes map[string]librarystore.LibraryDetailEpisode) ([]Season, mp.ID) {
	ready := map[string]bool{}
	var order []string
	for _, f := range files {
		if _, seen := ready[f.ItemID]; !seen {
			order = append(order, f.ItemID)
			ready[f.ItemID] = true
		}
		if f.Header == librarystore.LibraryHeaderPending {
			ready[f.ItemID] = false
		}
	}
	bySeason := map[int][]Episode{}
	named := map[mp.ID]int{}
	for _, id := range order {
		row := episodes[id]
		bySeason[row.Season] = append(bySeason[row.Season], Episode{
			ID: id, Name: row.Name, Number: row.Episode, End: row.EpisodeEnd, Ready: ready[id], FromFile: row.Unnumbered,
			Named: row.NameFrom != "", Overview: row.Overview, LengthMillis: row.LengthMs, Still: row.Still,
		})
		if row.NameFrom != "" {
			named[mp.ID(row.NameFrom)]++
		}
	}
	var namesFrom mp.ID
	for p, n := range named {
		if n > named[namesFrom] || (n == named[namesFrom] && p < namesFrom) {
			namesFrom = p
		}
	}
	out := make([]Season, 0, len(bySeason))
	for n, eps := range bySeason {
		slices.SortFunc(eps, func(a, b Episode) int {
			return cmp.Or(cmp.Compare(a.Number, b.Number), strings.Compare(a.Name, b.Name))
		})
		out = append(out, Season{Number: n, Episodes: eps})
	}
	slices.SortFunc(out, func(a, b Season) int { return cmp.Compare(a.Number, b.Number) })
	return out, namesFrom
}

// EpisodesOnDisk is, for each season of a series on disk, the highest episode number its files
// hold: what a series search is checked against (decision 57). Specials (season 0) and files
// without a number are left out, since nothing lays them out the same way twice.
func (l *Library) EpisodesOnDisk(ctx context.Context, seriesID string) (map[int]int, error) {
	episodes, err := l.ItemStore.LibraryEpisodeNumbers(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	highest := map[int]int{}
	for _, e := range episodes {
		if e.Season > 0 && e.Episode > 0 {
			highest[e.Season] = max(highest[e.Season], e.Episode, e.EpisodeEnd)
		}
	}
	return highest, nil
}
