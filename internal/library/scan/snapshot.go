package scan

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
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
//	28: "Series 1", "Staffel 1", "Season 2 [1080p]" and "<Show> Season 1" are season folders, and
//	    "S01E01-03", "S01E01+E02", "S01E01-S01E02" and "1x01-1x02" hold episodes through the last.
//	29: a file with no episode mark in a show's season folder ("Specials") is one of its episodes.
//	30: a release folder names its show by the words before its season ("Kettle.Bay.S02.1080p").
//	31: a movie whose file name has no year is named by its folder's, when that has one.
//	32: a season folder inside a season folder ("Show/Season 1/Season 1") files under the show.
//	33: a library named the way Sonarr, Radarr, Plex, Jellyfin, Emby or Kodi name one reads as
//	    that app means it.
//	34: an anime episode numbered across its show ("Show - 013") is placed by its seasons.
//	35: a movie in parts ("cd1", "cd2") notes each file's part, and every part of it plays.
//	36: a show whose folder gives no year takes the one its episodes' names agree on
//	    ("Show (1963) - S01E01").
//	37: a season folder's "101", "- 03", "3.05" and "E03" place its episodes; "10x10 (2018)" is a
//	    title; DivX, Flash, OGM, RealMedia and VOB files are read; an episode titled "Each-Other" and
//	    a catch-all's "Shorts" folder are not extras.
//	38: a movie in parts is one title before any catalog names it, and its parts may each have a
//	    folder of their own ("CD1/", "Disc 2/").
//	39: season folders spelled out, in other languages, with notes or the show's initials, or with
//	    a disc or codec folder inside, place their episodes; a NAS's snapshot and recycle folders, a
//	    downloader's unfinished ones and a film's interviews and featurettes are skipped; a day
//	    alone makes no show; a year after a dot, a dash or the release words is the year; release
//	    notes, a collection's place and a tracker's name are not the title; "1x2", "S01Ep02",
//	    "T01E02" and "Staffel 1 Folge 2" are marks.
//	40: an extras folder outside a title's own folder is a folder of titles; a folder whose
//	    episodes name several shows, none its own, names none; extras named in words beside a film
//	    are skipped; "Show E01" in the show's folder, a year's folder in a show's, "Season
//	    Specials", "1st Season", "Folge 1" and a fansub's "Show S2 - 03" place their episodes.
//	41: a word a title may share with a release ("Tangled Web", "The Cam") ends it only where a
//	    release's words are under way; a genre folder's one show is its episodes' own; a dated
//	    release loose at the top or in a catch-all is its show's; "Capítulo 5", "Серия 9", "Cap.117"
//	    and "第11話" are marks; a season pack in its show's folder, a fansub pack's folder and an
//	    unmarked file beside a show's episodes are the show's; descriptive extras, Media Center
//	    recordings and a Mac's share leavings are read as they are.
//	42: an extra named by its kind is one beside its title, however it is named, and a title of its
//	    own with none beside it; specials loose beside a show's season folders and episodes numbered
//	    alone in a show's folder are its; "Complete Series" folders, "第1期", "시즌 1" and "Series 7
//	    Club" are read as they mean; ".45 …" and "@Home …" are titles, not hidden folders.
//	43: an extra of a title with an apostrophe in its name ("…'s … (2010)-trailer") is one.
//	44: a show's episodes in its folder's unnamed or misspelt folders, or a copy of it ("Show
//	    Complete/", "Show (Season 2)/"), are the show's; loose names written in lower case are
//	    one show with their capitalised spelling; fansub numbers written " 19", " - E20", " -
//	    Episode 17", " - 05.5" and " - 01-03" are episodes, as are a show's numbers alone in a
//	    shows' catch-all; a film in its own folder in a films' catch-all is a film; "The Sample",
//	    "Paper-Clip" and a title holding "Extra" are titles; genre folders are catch-alls.
//	45: a miniseries' parts in a shows' catch-all are its episodes; "Title (Year) Trailer" alone,
//	    and "Show - Making Of" beside its episode, are extras; a show's folder with one marked
//	    episode holds episodes; "Promo", "TV Spot", "Blooper Reel" and galleries are extras.
//	46: a file among films is a film unless "S01E01" or a show's season folder says otherwise;
//	    every video in a show's folder in a shows' catch-all is an episode; names are read
//	    without their release's notes at their end; a remake's and a sequel's folders are their
//	    own shows; MakeMKV's, Media Center's, old scene and phone names; trackers' sites.
//	47: a title named by its file keeps its release's notes in its name, as before 46; the
//	    catalog's search sets them aside.
//	48: catch-alls are films', shows' or mixed; a show's loose folder of named episodes is a
//	    series; TV captures' parts are episodes; a site's promo, a sneak peek, a recap, a bonus
//	    and a preview are extras; an anime's dashed number wins over one in its name, a dated
//	    anime and a batch's folder are read; "S01E01" alone among films is a film.
//	49: a film's parts, a file named as its own folder with its year, sequels and a saga's films
//	    are films wherever they are; a season folder's "- 03" and "1.05" are its own; a "Show
//	    Season 2" folder gives its season, as does "Season IV"; a season pack's release folder
//	    stands for its show's folder; "Part 1 of 3" is a miniseries'; plural kinds, a trailer in
//	    a film's folder, a day's file in a show's folder and a DVR's dated name are read.
//	50: an extra's kind must end its name, and a dated name is an episode, so "Hilltop - Scene
//	    of the Crime" and "Show - 2018-04-02 - Recap" play; an anime folder tags its releases;
//	    a shows' catch-all's dated names are episodes; "movie.mkv" and "1.mkv" take their
//	    folder's title; a show's season folders are one show; a franchise's films are films;
//	    "1x01" among films, "3 - 01" in Season 3, "(Season 1-3)", "Kausi 1", "Ep01" are read; a
//	    film's or show's "<title> Extras" folder is its extras.
//	51: a loose episode beside its show's folder is that show's; "Season 5 Episode 2" names its
//	    season wherever it is; "(TV Series)" and "(Miniseries)" are no part of a show's name; a
//	    franchise's folder of two shows holds each as its own; three digits whose hundred is the
//	    season are its episode; editions before a year are set aside; "Easter Eggs", "Credits"
//	    and "Intro" are a film's extras, but an episode's titles beside marked episodes; a
//	    season's extras folder and a scene's proof are skipped; "Christmas Specials" are season
//	    0; "Saga Two" is a title, not a saga's folder.
//	52: a shelf by letter ("TV/C/") or by what is watched ("To Watch/") names no show; files
//	    numbered after a name they share ("Fizzy Fox 01", "Fizzy Cat - 02 - Paws", "01" beside
//	    "02") are that show's episodes wherever they are; a group's tag last, "{Grp}" or "(Grp)"
//	    first, tags a release; a fansub's "SP01" and "OVA 2" are season 0's; a year with a
//	    title's words after it is the title's ("Back to 1985 Again"); a season's disc label
//	    ("SHOW_S1_D1") holds its episodes; MakeMKV's "title00" and a recorded film are named by
//	    their folder and film; "シーズン1" is a season folder.
//	53: a tracker's promo ("ETRG.mp4"), a film's credits and Q&A, and a ".partial" download are
//	    no title; one show's loose episodes key alike with their year bare or bracketed, and join
//	    the show a season pack beside them stands for; a franchise's films numbered in order are
//	    films; a full mark beats a title's "2x4", and a season folder beats a "2x4" naming another
//	    season; "108 Rain" in Season 1 is its eighth beside "07 Sleet"; "1of3" with no year is a
//	    miniseries' episode; an OVA with no number is season 0's; an edition last is set aside
//	    with no year; "Widescreen" is a release's; "VTS_01_1" is named by its folder.
//	54: a show's packs, disc labels and loose copies at the top or in a shows' catch-all join one
//	    show kept by its catch-all ("TV/Kettle Bay"); a film's unnamed extras beside it are
//	    extras, and a show's trailer is no episode; a sequel keeps its own name; "Part 1 - Tides"
//	    titles an episode; a daily show filed by year and month is one show; a recorded film
//	    keeps its film's name; "Criterion" is a release's.
//	55: a show's folder of episodes named by their titles on an audience's shelf ("Kids/") is a
//	    show's; a miniseries' folder among films numbered "Episode 1", "E01" or "Night 1" holds
//	    episodes; "Show.E05.240301" is an episode; "第05集" loose beside its siblings is numbered;
//	    a film's disc or version folder ("Disc 1/", "4K/") names no film; a doubled show folder,
//	    a volume's ("Vol 2"), a "(TV)" tag and a bracketed day each join their show.
//	56: a root named as a shelf ("/media/TV", "/media/Movies", "/media/Kids", "/media/Anime") is
//	    read as one, as a folder of that name in a root is.
//	57: a MakeMKV title named by its film keeps its folder's year; a title in brackets alone is
//	    the title; a recorded film with its channel is the film; "SITE.txt.mp4" is junk; a
//	    season's disc folder in its show's folder is that show's; an anime's "Ep 05" and
//	    "01-02" batch read as the show's.
//	58: an episode titled after a spaced dash ("S01E04 - sample") is kept in any case; a show's
//	    specials' folder named by it, and an anime's "OVA", are its season 0; an anime release's
//	    creditless, clean, menu, preview and trailer extras are skipped; a recording named by its
//	    day first is its show's on a shows' shelf; a show and its remake numbering an episode alike
//	    stay two; a show's film kept with it is a film; three title-named videos with only
//	    catch-alls above are a show's.
//	59: a show's folder grouped by volume, book, chapter, year, DVD or collection holds its
//	    episodes; a season's pack, or a video named "Episode N" alone, on a films' shelf is a
//	    show's, and a release pack's "S02" gives its bare numbers' season; a show called a saga on
//	    a shows' shelf is a show; a film's "Specials" and any "Subs" folder are skipped; an anime
//	    release's bracketed notes leave its show's name; "Sports/" is a shows' shelf.
//	60: a shelf no list names is read by its words ("Christmas Movies", "TV-Shows", "Movies2",
//	    "DVD Rips", "Toddlers"); a show's own film beside its titled episodes leaves them its; a
//	    bare "E01" in a show's folder, and a lone number in a named season's, are episodes; a
//	    film's padded place, "03 Title", leaves its title; a camera's clip is its own video.
//	61: a show kept in several places (its folder, a second shelf's, loose downloads, a release's
//	    batch folders) is one show when their names and years agree (joinShows); a film's copy
//	    named by its cut, framing, colour or voice is a version, never skipped; ".m2t" is a
//	    video; "Kame Ep 5" in a shelf is Kame's; a show's film leaves its named episodes its; a
//	    release's language and a show folder's "DVD" leave the name; "Kame Specials - 01" is S0.
//	62: shows told apart by a bracketed tag ("(US)", "(Animated)") never join; a release's short
//	    file name takes its folder's fuller title of the same year; "Show E01" beside "E02" at the
//	    top is a show's; a "Creditless" folder is extras; "第1話" alone is its folder's episode; a
//	    film named as its own folder stays a film; a video site's id leaves the title; a file's own
//	    <movie> NFO makes a film of one only its folder placed as an episode.
//	63: an edition's word right after a film's year ends its title ("Gloam.Moss.2011.Theatrical");
//	    a year set first by a dash, dot or underscore is the year ("2010 - Hob Crag"); a span of
//	    years is the title's; loose "Show - Title" files sharing a show in a catch-all are its
//	    episodes; a show folder its episodes name whole ("Unit S9") is no season pack; one file in
//	    a season's pack is an episode; "Show Ep01" among films is one; a disc label's "WS" goes.
//	64: a dashed file name with a release's mark ("pratt-wood-1080p") is its folder's film; an
//	    episode's NFO <showtitle> names its show; Kodi's "Show - 214 - Title" is S2E14; an anime's
//	    "Show - Special - 01" and "- Recap -" are its show's; "[Show] S01E09" names its show; a
//	    show's yearless and dated folders join unless both number an episode; "Show 1-05" in its
//	    folder and a "(S07)" folder's "- 01" number their episodes.
//	65: a kids' show numbered after its name ("Gumdrop Garage 1") with none named so alone is a
//	    show; an anime's film in its release's folder is a film; a count after a year ("Part 2",
//	    "Week 1") runs on a title; "S01E02.Trailer" is the episode's extra.
//	66: a disc's MakeMKV titles number no episode; a show's yearless seasons apart from its dated
//	    ones stay another show's; "Show.E11.1080p.WEB" is an episode anywhere; a year after an
//	    anime's padded number is the release's; a folder's American days settle its others.
const writeVersion = 66

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
		// Notes can number a file differently when a file beside it comes or goes.
		Notes folderNotes `json:",omitzero"`
		// Part changes when another part of its movie comes or goes.
		Part int `json:",omitzero"`
	}{writeVersion, reader, m.records, m.series, embedded, file.Sidecars, seriesSidecars, file.Notes, file.Part})
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
	// Part is which part of a movie in parts this file is, from 1; 0 for a whole file.
	Part int `json:"part,omitempty"`
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
// movie has an edition or parts.
func originExtensionOf(kind mp.Kind, res resolution, series inventory.OriginKey, file fileNotes) json.RawMessage {
	r := res.resolved
	if kind != mp.KindMovie {
		file.Edition, file.Part = "", 0
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
	// Present beats missing, then a copy grounded in the catalog beats one known only by its name
	// (two copies of one title, one with an NFO naming its TMDB id: the title is named by that one,
	// whichever copy was written last), then newer beats older; the key breaks a tie, so the pick is
	// stable.
	grounded := func(o inventory.ItemOrigin) bool {
		listed := readOriginExtension(o.Observation.Extension).Listed
		return listed != nil && listed.Catalog.String() != ""
	}
	better := func(o, than inventory.ItemOrigin) bool {
		if o.MissingAt.IsZero() != than.MissingAt.IsZero() {
			return o.MissingAt.IsZero()
		}
		if g := grounded(o); g != grounded(than) {
			return g
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
		// A movie in parts' own mark only holds its parts together; no catalog knows it.
		if id.Namespace != partsNamespace {
			it.IDs = append(it.IDs, librarystore.LibraryID{Namespace: id.Namespace, Value: id.Value})
		}
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
				Bitrate: f.Bitrate, DurationMs: f.DurationMillis, Edition: exts[o.Key].Edition, Part: exts[o.Key].Part}
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
		func(i int) int { return files[i].src.Part },
		func(i int) bool { return files[i].managed }, func(i int) int {
			for _, st := range files[i].src.Streams {
				if st.Kind == string(inventory.StreamVideo) {
					return st.Width * st.Height
				}
			}
			return 0
		}, func(i int) int64 { return files[i].src.SizeBytes }, func(i int) int64 { return files[i].src.DurationMs })
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

var (
	// makeMKVTitle matches MakeMKV's mark for a title it ripped, at a name's end: "title_t00",
	// "GILDED_MOOR_t01", and its older versions' "title00".
	makeMKVTitle = regexp.MustCompile(`(?i)(?:^title_?t?|_t)\d{2}$`)
	// discLabel matches the disc's place, or its screen's shape, a disc's label ends in: "OSTREVIN
	// D1", "OSTREVIN DISC 2", "GLOAM RISE WS", "GLOAM RISE FS".
	discLabel = regexp.MustCompile(`(?i)[ ]+(?:d(?:isc)?[ ]?\d|ws|fs|widescreen|fullscreen)$`)
)

// genericStem matches a file's name that says what it is, never which: "movie", "film", "video",
// "main", "feature", "main feature", "1", "01", and a DVD's title set copied out of its disc,
// "VTS_01_1".
var genericStem = regexp.MustCompile(`(?i)^(?:movie|film|video|main|feature|main[ ._-]?feature|the[ ._-]movie|\d{1,2}|vts[ ._]\d{2}[ ._]\d)$`)

// runsOn says whether a title is another's with more words after it: "Brook 2" and "Heath Part 2"
// run on from "Brook" and "Heath".
// filmFolder is the folder that names the film a file in dir is: dir, or the film's folder above a
// disc's or a version's own, which names no film ("Holm Fen (2004)/Disc 1/", "Ashlee Vane
// (2015)/4K/").
func filmFolder(dir string) string {
	for dir != "." && path.Dir(dir) != "." {
		words := strings.Fields(folderKey(path.Base(dir)))
		if !partFolder.MatchString(path.Base(dir)) && (len(words) == 0 || slices.ContainsFunc(words, func(w string) bool { return !releaseToken(w) })) {
			break
		}
		dir = path.Dir(dir)
	}
	return dir
}

func runsOn(title, other string) bool {
	words, own := strings.Fields(normName(title)), strings.Fields(normName(other))
	return len(own) > 0 && len(words) > len(own) && slices.Equal(words[:len(own)], own)
}

// fallbackName is a title from a file name when no source names the item. A movie whose file name
// has no year, in a folder whose name has one ("Ashgrove (2015)/asg-1080p.mkv"), is named by the
// folder: a release or rip names its folder and leaves the file a code. A part of a movie in parts
// (part > 0) is named without its part mark; one whose part has a folder of its own ("CD1/") is
// named by the movie's folder, the one name all its parts share.
func fallbackName(rel string, kind mp.Kind, part int) string {
	clean := func(name string) string {
		return strings.Join(strings.Fields(editionTag.ReplaceAllString(embeddedID.ReplaceAllString(name, ""), "")), " ")
	}
	// The name keeps its release's notes, which the catalog's search sets aside; what the name is
	// is read without them ("bc [1080p]" is the folder's initials).
	name, plain := stem(path.Base(rel)), plainStem(path.Base(rel))
	if part > 0 {
		name, plain = withoutPart(name), withoutPart(plain)
		if dir := path.Dir(rel); partFolder.MatchString(path.Base(dir)) && path.Dir(dir) != "." {
			return clean(path.Base(path.Dir(dir)))
		}
	}
	// A recorder names a film it recorded by the film, then its channel and when it began: "Iron
	// Spire (2011)_Film4_2022_05_01_21_00_00" and "Grey Harbour (1958) 2022-01-01" are "Iron Spire
	// (2011)" and "Grey Harbour (1958)".
	if kind == mp.KindMovie && recordedFilm(plain, path.Dir(rel)) {
		return clean(recordedTitle(plain))
	}
	if kind == mp.KindMovie && (recording.MatchString(plain) || recordedAt.MatchString(plain)) {
		return clean(datedShow(plain))
	}
	// MakeMKV names a title it rips "title_t00", or its disc's label and "_t00": the label, or
	// the folder it was ripped into, names the film ("GILDED_MOOR/title_t00.mkv").
	if m := makeMKVTitle.FindStringSubmatchIndex(plain); m != nil && kind == mp.KindMovie {
		label := strings.Trim(strings.ReplaceAll(plain[:m[0]], "_", " "), " ")
		dir := filmFolder(path.Dir(rel))
		// A label that is the film's own folder's title, "Ferrisdale (1996)/Ferrisdale_t00", takes
		// the folder's name, year and all.
		title, year, _ := searchQuery(path.Base(dir), 0)
		if dir != "." && !catchAllFolder(path.Base(dir)) &&
			(len(label) <= 3 || strings.EqualFold(label, "title") || year != 0 && sameWords(discLabel.ReplaceAllString(label, ""), title)) {
			label = strings.ReplaceAll(path.Base(dir), "_", " ")
		}
		return clean(discLabel.ReplaceAllString(label, ""))
	}
	// A camera's clip is its own video, never the film its folder ("Holiday 2018/") seems to name.
	if dir := filmFolder(path.Dir(rel)); kind == mp.KindMovie && dir != "." && !cameraClip.MatchString(plain) {
		folder := clean(path.Base(dir))
		// A file named by its folder's initials, "Brindle Cove/bc.mkv", is named by the folder.
		// So is one named by what it is rather than what it is called: "Pikestaff/movie.mkv",
		// "Pikefen/1.mkv", "main.mkv", "feature.mkv".
		if !catchAllFolder(folder) && (len(plain) > 1 && strings.EqualFold(plain, initials(folder)) || genericStem.MatchString(plain)) {
			return folder
		}
		// A sequel kept in the first film's folder, "Brook (2010)/Brook 2.mkv", says more than the
		// folder's title: it is its own.
		if fileTitle, fileYear, _ := searchQuery(name, 0); fileYear == 0 {
			if title, year, _ := searchQuery(folder, 0); year != 0 && title != "" && !runsOn(fileTitle, title) {
				return folder
			}
		} else if title, year, _ := searchQuery(folder, 0); year == fileYear && runsOn(title, fileTitle) {
			// A release's file named short, "Wren.Harbor.2003…-GRP/wren.2003.1080p.mkv", is the film
			// its folder names in full, of the same year.
			return folder
		}
	}
	return clean(name)
}
