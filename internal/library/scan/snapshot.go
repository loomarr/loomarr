package scan

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Item kinds the scanner writes. Movie, series and episode match the media server's kinds, so the
// two origins of one title agree.
const (
	kindMovie   inventory.ItemKind = "movie"
	kindSeries  inventory.ItemKind = "series"
	kindEpisode inventory.ItemKind = "episode"
)

// authorityPrefix starts every scan root's authority.
const authorityPrefix = "scan:"

// Authority is the inventory authority of one scan root: every file under it is an origin of that
// authority, so a walk of the root marks exactly its own files unseen.
func Authority(root string) inventory.AuthorityID {
	digest := sha256.Sum256([]byte(root))
	return inventory.AuthorityID(authorityPrefix + hex.EncodeToString(digest[:12]))
}

// fileOrigin is a media file's origin: its path within its root.
func fileOrigin(root, rel string) inventory.OriginKey {
	digest := sha256.Sum256([]byte(rel))
	return inventory.OriginKey{Authority: Authority(root), ExternalItemID: "file:" + hex.EncodeToString(digest[:16])}
}

// seriesOrigin is a series' origin within a root: its folder. Its ids are not part of the key, so
// learning them never forks the series, and finding it present needs no sidecar read.
func seriesOrigin(root, dir string) inventory.OriginKey {
	digest := sha256.Sum256([]byte(dir))
	return inventory.OriginKey{Authority: Authority(root), ExternalItemID: "series:dir:" + hex.EncodeToString(digest[:16])}
}

// sortKey is the key a title sorts by: its sort title in lower case with the accents taken off,
// so "À Nous la Liberté" files under A rather than after Z. The few letters that are not an accent
// on another one read as the letters people file them under: "Øster" under O, "Æon" under AE.
func sortKey(title string) string {
	decomposed := norm.NFD.String(strings.ToLower(title))
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
		case sortLetters[r] != "":
			b.WriteString(sortLetters[r])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sortLetters are the lower-case letters with no decomposition that sort as other letters.
var sortLetters = map[rune]string{'ø': "o", 'æ': "ae", 'œ': "oe", 'ß': "ss", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th"}

// revision is a file's inventory revision: its stat. A new size or mtime is new bytes.
func revision(st Stat) string { return fmt.Sprintf("stat:%d:%d", st.Size, st.ModifiedNs) }

// writeVersion changes when what a write produces from the same inputs changes, so every file is
// written once more with the new shape; its header is kept, so none is opened again.
//
//	2: titles name the catalog row their details come from (library_items.catalog).
//	3: titles note their scan origin and whether anything grounds them (Needs you).
//	4: titles note what said which title they are, and its page (Wrong title?).
//	5: each origin notes where its shown details came from (Technical details).
//	6: a file's origin notes whether its arr lists it (the copy that plays).
//	7: a multi-episode file carries every episode's ids, not only its first's.
//	8: extras are not titles (0047); the digest drops what only an extra used.
//	9: NFOs are read in full (decision 59): older id tags, tags, ratings, people and sets.
//	10: a movie's or series' origin carries how it is listed; the store projects its row (0047).
//	12: every title, episodes too, is listed with its facts and who supplied each (decision 40).
//	13: an episode with a TVDB id asks the catalog for TMDB's view of it (decision 53).
//	14: every title's files, tracks and artwork are listed with it (decisions 41, 44).
//	15: an episode with no TVDB id takes TMDB's entry for its season and number, when TMDB numbers
//	    the series as the disk does (decision 53).
//	16: an audio track whose container states no channel layout is labelled as ffmpeg states it.
//	17: a movie's file notes its edition, and each edition has its own copy that plays (decision 54).
//	18: a series carries a poster for each season it has one for (decisions 45, 61).
//	19: a file notes the trickplay files beside it (decision 61).
//	20: a file notes its external subtitle and audio files and its .edl markers, and lists its
//	    chapters and its tracks' commentary, SDH and descriptive flags (decisions 45, 46, 61).
//	21: a title lists every studio or network, and its tagline, countries and trailer (decision 46).
//	22: a title lists its people with their role, character, TMDB id and photo (decision 49).
//	23: a movie's set is listed as the catalog row the catalog provider fetches it by.
//	24: a person is linked by any catalog's id (namespace and value), not a TMDB id.
//	25: an episode its show's catalog can't number is marked, and one whose show has an answer is
//	    numbered by the order picked (decision 53).
//	26: a title sorts by its letters with the accents taken off ("À Nous" under A).
//	27: a show with no name of its own is named by its own folder, not the folders above it.
const writeVersion = 27

// identityDigest digests everything that says what a file is, so a change to any of it, and
// nothing else, rewrites the file's inventory item. reader is the header.Version, so a reader that
// learns more reaches every file. seriesSidecars are an episode's series folder's tvshow.nfo and
// folder art (#3226); omitted when empty, so a movie's digest is unchanged by them.
func identityDigest(file File, m managed, embedded mp.Record, reader int, seriesSidecars map[string]Stat) string {
	payload, _ := json.Marshal(struct {
		Version        int
		Header         int
		Records        []mp.Record
		Series         *mp.Record
		Embedded       mp.Record
		Sidecars       map[string]Stat
		SeriesSidecars map[string]Stat `json:",omitempty"`
	}{writeVersion, reader, m.records, m.series, embedded, file.Sidecars, seriesSidecars})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:16])
}

func pickedIDs(picked []precedence.Picked[mp.ExternalID]) []mp.ExternalID {
	out := make([]mp.ExternalID, 0, len(picked))
	for _, p := range picked {
		out = append(out, p.Value)
	}
	return out
}

func externalIDs(picked []precedence.Picked[mp.ExternalID]) []inventory.ExternalID {
	out := make([]inventory.ExternalID, 0, len(picked))
	for _, p := range picked {
		out = append(out, inventory.ExternalID{Namespace: string(p.Value.Namespace), Value: p.Value.Value})
	}
	return out
}

// DetailsFrom names the provider each detail a title page shows came from, for Technical details'
// "Where details come from" (#2151 step 10d). "" is no provider: the detail is unknown, or the
// scanner made it up from the path.
type DetailsFrom struct {
	Name        mp.ID `json:"name,omitempty"`
	Description mp.ID `json:"description,omitempty"`
	Rating      mp.ID `json:"rating,omitempty"`
	// Year, Premiere, Score and Runtime feed the library_items `_from` columns (decision 40).
	Year     mp.ID `json:"year,omitempty"`
	Premiere mp.ID `json:"premiere,omitempty"`
	Score    mp.ID `json:"score,omitempty"`
	Runtime  mp.ID `json:"runtime,omitempty"`
}

// originExtension is what the scanner keeps in an origin's Observation.Extension.
type originExtension struct {
	From DetailsFrom `json:"from"`
	fileNotes
	// Listed is how a title is listed in the Library; nil for an origin written before it carried
	// one (a movie or series before write version 10, an episode before 12), whose row stays as it
	// was until it is written again.
	Listed *listedExtension `json:"listed,omitempty"`
}

// fileNotes is what the scanner notes about a file itself, beside what its title resolves to.
type fileNotes struct {
	// Managed is true for a file its arr lists: the arr's current file, the one that plays when a
	// title has several (decision 37).
	Managed bool `json:"managed,omitempty"`
	// Edition is the labelled version of a movie this file is, "" for the default (decision 54).
	Edition string `json:"edition,omitempty"`
	// Trickplay are the trickplay files beside it, by path: referenced, never imported (decision 61).
	Trickplay []string `json:"trickplay,omitempty"`
	// External are the subtitle and audio files beside it, as their names describe them; Markers
	// the spans its .edl marks (decision 61).
	External []externalTrack `json:"external,omitempty"`
	Markers  []marker        `json:"markers,omitempty"`
}

// listedExtension is what a title's library row needs that its facts do not say.
type listedExtension struct {
	Catalog librarystore.LibraryCatalogKey `json:"catalog,omitzero"`
	// Set is a movie's ready-made set, as the catalog row it is fetched by.
	Set   librarystore.LibraryCatalogKey `json:"set,omitzero"`
	Match librarystore.LibraryMatch      `json:"match,omitzero"`
	// Series is an episode's series by its scan origin.
	Series librarystore.LibraryOrigin `json:"series,omitzero"`
	// Unnumbered is an episode its show's catalog could not number, whose show nobody has
	// answered for (decision 53).
	Unnumbered bool `json:"unnumbered,omitempty"`
}

// originExtensionOf is res's provenance, how the title is listed (series is an episode's series
// origin) and what the scanner notes about the file, as an origin's Observation.Extension. Only a
// movie has an edition.
func originExtensionOf(kind mp.Kind, res resolution, series inventory.OriginKey, file fileNotes) json.RawMessage {
	r := res.resolved
	if kind != mp.KindMovie {
		file.Edition = ""
	}
	ext := originExtension{From: DetailsFrom{
		Name: r.Title.From.Provider, Description: r.Overview.From.Provider, Rating: r.Certification.From.Provider,
		Year: r.Year.From.Provider, Premiere: r.PremiereDate.From.Provider,
		Score: r.CommunityRating.From.Provider, Runtime: r.Runtime.From.Provider,
	}, fileNotes: file}
	switch kind {
	case mp.KindMovie:
		ext.Listed = &listedExtension{Catalog: res.catalog, Set: res.set, Match: res.match}
	case mp.KindSeries:
		ext.Listed = &listedExtension{Catalog: res.catalog, Match: res.match}
	case mp.KindEpisode:
		ext.Listed = &listedExtension{Catalog: res.catalog, Series: libraryOrigin(series), Unnumbered: res.unnumbered}
	}
	raw, _ := json.Marshal(ext)
	return raw
}

// ProjectLibraryItem is the scanner's store.LibraryProjector: a title's library row, from its
// newest present scanner origin (or, with none present, its newest scanner origin). An item with
// no scanner origin, or whose origin predates its listing, is not listed by it.
func ProjectLibraryItem(item inventory.Item) (librarystore.LibraryItem, bool) {
	if item.Kind != kindMovie && item.Kind != kindSeries && item.Kind != kindEpisode {
		return librarystore.LibraryItem{}, false
	}
	// Present beats missing, then newer beats older; the key breaks a tie, so the pick is stable.
	better := func(o, than inventory.ItemOrigin) bool {
		if o.MissingAt.IsZero() != than.MissingAt.IsZero() {
			return o.MissingAt.IsZero()
		}
		if !o.Observation.ObservedAt.Equal(than.Observation.ObservedAt) {
			return o.Observation.ObservedAt.After(than.Observation.ObservedAt)
		}
		return o.Key.ExternalItemID < than.Key.ExternalItemID
	}
	var best *inventory.ItemOrigin
	for i, o := range item.Origins {
		if strings.HasPrefix(string(o.Key.Authority), authorityPrefix) && (best == nil || better(o, *best)) {
			best = &item.Origins[i]
		}
	}
	if best == nil {
		return librarystore.LibraryItem{}, false
	}
	ext := readOriginExtension(best.Observation.Extension)
	listed := ext.Listed
	if listed == nil {
		return librarystore.LibraryItem{}, false
	}
	facts := best.Observation.Facts
	it := librarystore.LibraryItem{
		ID: string(item.ID), Kind: string(item.Kind), Name: facts.Name,
		SortName: sortKey(facts.SortTitle), Year: facts.ProductionYear,
		Catalog: listed.Catalog, Set: listed.Set,
		Origin: libraryOrigin(best.Key), Match: listed.Match,
		OriginalTitle: facts.OriginalTitle, Overview: facts.Overview, PremiereDate: facts.PremiereDate,
		Rating: facts.OfficialRating, Score: facts.CommunityScore, RuntimeMs: facts.RuntimeMillis,
		Season: facts.SeasonNumber, Episode: facts.EpisodeNumber, EpisodeEnd: facts.EpisodeEnd,
		Series: listed.Series, Unnumbered: listed.Unnumbered,
		Tags: facts.Tags, Tagline: facts.Tagline, Trailer: facts.Trailer,
		From: librarystore.LibraryFrom{
			Name: string(ext.From.Name), Year: string(ext.From.Year), Overview: string(ext.From.Description),
			Premiere: string(ext.From.Premiere), Rating: string(ext.From.Rating), Score: string(ext.From.Score),
			Runtime: string(ext.From.Runtime),
		},
	}
	// An episode's genres, studios, countries and cast are its show's (decision 42).
	if item.Kind != kindEpisode {
		it.Genres, it.Studios, it.Countries = facts.Genres, facts.Studios, facts.Countries
		for _, p := range facts.People {
			it.Credits = append(it.Credits, librarystore.LibraryCredit{Name: p.Name, Role: p.Role, Character: p.Character,
				ID: librarystore.LibraryID{Namespace: p.ID.Namespace, Value: p.ID.Value}, Photo: p.Photo})
		}
	}
	for _, id := range item.ExternalIDs {
		it.IDs = append(it.IDs, librarystore.LibraryID{Namespace: id.Namespace, Value: id.Value})
	}
	for _, a := range facts.Artwork {
		it.Art = append(it.Art, librarystore.LibraryArt{Kind: a.Kind, Season: a.Season, Ref: a.Ref})
	}
	it.Sources = librarySources(item)
	return it, true
}

// librarySources is each of an item's present files with its tracks, in path order, the copy
// that plays marked (decision 37).
func librarySources(item inventory.Item) []librarystore.LibrarySource {
	exts := map[inventory.OriginKey]originExtension{}
	for _, o := range item.Origins {
		exts[o.Key] = readOriginExtension(o.Observation.Extension)
	}
	type file struct {
		src     librarystore.LibrarySource
		managed bool
	}
	var files []file
	for _, s := range item.Sources {
		for _, o := range s.Origins {
			if !strings.HasPrefix(string(o.Key.Authority), authorityPrefix) || !o.MissingAt.IsZero() || o.Locator.Path == "" {
				continue
			}
			f := o.Observation.Facts
			src := librarystore.LibrarySource{Path: o.Locator.Path, Container: f.Container, SizeBytes: f.SizeBytes,
				Bitrate: f.Bitrate, DurationMs: f.DurationMillis, Edition: exts[o.Key].Edition}
			for _, ref := range exts[o.Key].Trickplay {
				src.Trickplay = append(src.Trickplay, librarystore.LibraryTrickplay{Ref: ref, Format: "bif"})
			}
			next := 0
			for _, st := range f.Streams {
				src.Streams = append(src.Streams, libraryStream(st))
				next = max(next, st.Index+1)
			}
			for i, t := range exts[o.Key].External {
				src.Streams = append(src.Streams, externalStream(t, next+i))
			}
			for _, c := range f.Chapters {
				src.Chapters = append(src.Chapters, librarystore.LibraryChapter{StartMs: c.StartMillis, EndMs: c.EndMillis, Title: c.Title})
			}
			for _, m := range exts[o.Key].Markers {
				src.Markers = append(src.Markers, librarystore.LibraryMarker{StartMs: m.StartMs, EndMs: m.EndMs, Kind: m.Kind})
			}
			files = append(files, file{src: src, managed: exts[o.Key].Managed})
			break
		}
	}
	if len(files) == 0 {
		return nil
	}
	slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.src.Path, b.src.Path) })
	plays := playingCopies(len(files), func(i int) string { return files[i].src.Edition },
		func(i int) bool { return files[i].managed }, func(i int) int {
			for _, st := range files[i].src.Streams {
				if st.Kind == string(inventory.StreamVideo) {
					return st.Width * st.Height
				}
			}
			return 0
		}, func(i int) int64 { return files[i].src.SizeBytes })
	out := make([]librarystore.LibrarySource, len(files))
	for i, f := range files {
		out[i] = f.src
		out[i].Plays = plays[i]
	}
	return out
}

// libraryStream is a header stream with its labels.
func libraryStream(st inventory.Stream) librarystore.LibraryStream {
	out := librarystore.LibraryStream{
		Index: st.Index, Kind: string(st.Kind), Codec: st.Codec, Profile: st.Profile, Title: st.Title,
		Default: st.Disposition.Default, Forced: st.Disposition.Forced,
		Commentary: st.Disposition.Comment, HearingImpaired: st.Disposition.HearingImpaired,
		Descriptive: st.Disposition.VisualImpaired,
		Channels:    st.Channels, ChannelLayout: st.ChannelLayout, SampleRate: st.SampleRate,
		Width: st.Width, Height: st.Height, FrameRate: st.FrameRate, PixelFormat: st.PixelFormat,
		ColorTransfer: st.ColorTransfer,
	}
	out.Language, out.LanguageName = languageLabel(st.Language)
	switch st.Kind {
	case inventory.StreamVideo:
		out.Resolution, out.HDR = resolutionLabel(st.Width, st.Height), hdrLabel(st.ColorTransfer, st.HDR, st.DolbyVision)
	case inventory.StreamAudio:
		out.AudioFormat = audioFormatLabel(st.Codec, st.Profile)
		out.ChannelLayout = cmp.Or(st.ChannelLayout, header.ChannelLayout(st.Codec, st.Channels))
	}
	return out
}

// externalStream is an external track as a row, at index, after the file's own tracks.
func externalStream(t externalTrack, index int) librarystore.LibraryStream {
	out := librarystore.LibraryStream{Index: index, Kind: t.Kind, Codec: t.Codec, Default: t.Default, Forced: t.Forced,
		HearingImpaired: t.HearingImpaired, Commentary: t.Commentary, ExternalRef: t.Ref}
	out.Language, out.LanguageName = languageLabel(t.Language)
	if t.Kind == string(inventory.StreamAudio) {
		out.AudioFormat = audioFormatLabel(t.Codec, "")
	}
	return out
}

// readOriginExtension reads originExtensionOf back; an origin written before it says nothing.
func readOriginExtension(raw json.RawMessage) originExtension {
	var ext originExtension
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &ext)
	}
	return ext
}

// itemFacts projects resolved values into the inventory's item facts, with coverage for each
// family a source answered.
func itemFacts(r precedence.Resolved) (inventory.ItemFacts, map[string]inventory.Coverage) {
	f := inventory.ItemFacts{
		Name: r.Title.Value, OriginalTitle: r.OriginalTitle.Value, SortTitle: r.SortTitle.Value,
		Overview: r.Overview.Value, Genres: r.Genres.Value, ProductionYear: r.Year.Value,
		PremiereDate: r.PremiereDate.Value, OfficialRating: r.Certification.Value.Rating,
		RuntimeMillis: r.Runtime.Value.Milliseconds(), Tags: r.Keywords.Value,
		CommunityScore: r.CommunityRating.Value,
	}
	for _, p := range r.People.Value {
		f.People = append(f.People, inventory.Person{Name: p.Name, Role: string(p.Role), Character: p.Character,
			ID: inventory.ExternalID{Namespace: string(p.ID.Namespace), Value: p.ID.Value}, Photo: p.Photo})
	}
	f.Studios, f.Countries, f.Tagline, f.Trailer = r.Studios.Value, r.Countries.Value, r.Tagline.Value, r.Trailer.Value
	if r.Numbering.Known() {
		f.SeasonNumber, f.EpisodeNumber, f.EpisodeEnd = r.Numbering.Value.Season, r.Numbering.Value.Episode, r.Numbering.Value.EpisodeEnd
	}
	for _, a := range r.Artwork {
		f.Artwork = append(f.Artwork, inventory.ArtworkRef{Kind: a.Value.Kind, Ref: a.Value.Ref, Season: a.Value.Season})
	}
	for _, p := range r.Parent {
		f.Relationships = append(f.Relationships, inventory.Relationship{
			Kind: string(kindSeries), ExternalID: string(p.Value.Namespace) + ":" + p.Value.Value,
		})
	}
	// A movie's ready-made set is its "you have 3 of 4" on the Collections tab.
	if c := r.Collection.Value.ID; c.Namespace != "" && c.Value != "" {
		f.Relationships = append(f.Relationships, inventory.Relationship{
			Kind: string(mp.KindCollection), ExternalID: string(c.Namespace) + ":" + c.Value,
		})
	}
	coverage := map[string]inventory.Coverage{}
	if r.Title.Known() {
		coverage["descriptive"] = inventory.CoveragePresent
	}
	if r.Numbering.Known() {
		coverage["numbering"] = inventory.CoveragePresent
	}
	if r.Certification.Known() {
		coverage["certification"] = inventory.CoveragePresent
	}
	return f, coverage
}

// sourceSnapshot is a file as the inventory stores a source: its stat, plus its header facts once
// read. The observation's schema version is the header.Version that read them (1 before any was),
// so lastHeader can tell facts an older reader left.
func sourceSnapshot(file File, read *inventory.SourceFacts, version int, at time.Time) inventory.SourceSnapshot {
	facts := inventory.SourceFacts{Protocol: "file", SizeBytes: file.Stat.Size, ModifiedUnixNano: file.Stat.ModifiedNs}
	coverage := map[string]inventory.Coverage{"sourceStat": inventory.CoveragePresent}
	schema := 1
	if read != nil {
		facts.Container, facts.DurationMillis, facts.Bitrate, facts.Streams, facts.Chapters =
			read.Container, read.DurationMillis, read.Bitrate, read.Streams, read.Chapters
		coverage["streams"] = inventory.CoveragePresent
		coverage["audioStreams"] = inventory.CoverageEmpty
		for _, s := range read.Streams {
			if s.Kind == inventory.StreamAudio {
				coverage["audioStreams"] = inventory.CoveragePresent
				break
			}
		}
		schema = version
	}
	return inventory.SourceSnapshot{
		ExternalSourceID: "file", Kind: inventory.SourceLocalFile, Revision: revision(file.Stat),
		Locator: inventory.Locator{Path: file.Path},
		Observation: inventory.Observation[inventory.SourceFacts]{
			SchemaVersion: schema, ObservedAt: at, Coverage: coverage, Facts: facts,
		},
	}
}

// fallbackName is a title from a file name when no source names the item.
func fallbackName(rel string) string {
	name := rel[strings.LastIndex(rel, "/")+1:]
	if dot := strings.LastIndex(name, "."); dot > 0 {
		name = name[:dot]
	}
	return strings.TrimSpace(editionTag.ReplaceAllString(embeddedID.ReplaceAllString(name, ""), ""))
}
