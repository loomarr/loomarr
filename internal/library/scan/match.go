package scan

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Matcher defaults (#2151 step 11, decision 27).
const (
	// MatchInterval is how often the Matcher looks for titles nothing grounds.
	MatchInterval = time.Minute
	// matchBatch caps the titles searched in one tick.
	matchBatch = 20
	// matchFailLimit is how many searches in a row may fail before a tick stops: one title the
	// catalog refuses is that title's problem, a run of them is the catalog out of reach.
	matchFailLimit = 3
	// matchSearched is how many search results are looked at closely; matchKept, how many are kept.
	matchSearched = 5
	matchKept     = 3
)

// SearchHit is one title a catalog search found.
type SearchHit struct {
	// Kind is movie or series.
	Kind string
	// ID is the title by the id of the catalog that found it.
	ID       mp.ExternalID
	Name     string
	Year     int
	Overview string
}

// Searcher finds catalog titles by name. It returns tmdb.ErrAPIKeyRequired when the catalog is
// not configured.
type Searcher func(ctx context.Context, term string, limit int) ([]SearchHit, error)

// MatchStore is where the Matcher finds titles nothing grounds and keeps what it found for them.
type MatchStore interface {
	LibraryNeeds(ctx context.Context) ([]librarystore.LibraryNeed, error)
	SaveLibraryCandidates(ctx context.Context, itemID, searched string, candidates []librarystore.LibraryCandidate) error
}

// Matcher searches the catalog for each title nothing grounds and nobody answered, and keeps its
// best candidates for Needs you: the ones whose name, year and length agree with what is on disk
// first. A title is searched again only when what it would be searched for changes.
type Matcher struct {
	Store  MatchStore
	Search Searcher
	// Provider fetches a candidate's details: its poster, and a movie's length.
	Provider mp.Provider
	// Inventory reads a title's length from its files' headers; nil knows none.
	Inventory inventory.Service
	// Enabled is the scanner's kill switch; nil is on.
	Enabled  func() bool
	Interval time.Duration
	Log      *slog.Logger
	// Sleep waits between catalog calls, per the provider's rate hint; nil sleeps on the clock.
	Sleep func(ctx context.Context, d time.Duration) error
	// Episodes is a series' highest episode number on disk in each season, specials left out; nil
	// knows none, and then no series is settled.
	Episodes func(ctx context.Context, seriesID string) (map[int]int, error)
	// Match answers a title a search settled: exactly one result agrees on everything it can be
	// checked by (decisions 56 and 57). Nil leaves every title to the maintainer.
	Match func(ctx context.Context, itemID string, id mp.ExternalID) error
}

// matchRules leads what a title was searched as, and changes when what settles a title does, so
// each title still waiting is searched once more under the new rules.
//
//	r2: a movie whose files name no year is settled by its name and length.
//	r3: names agree with their accents off, composed or not, and acronyms with their dots off.
const matchRules = "r3"

// evidence is what the files say about a title beyond its name: a movie's length, a series'
// episodes. A search result is checked against it.
type evidence struct {
	year int
	// whole is the name read with its year-like word as part of the title (searchQuery).
	whole    string
	length   time.Duration
	episodes map[int]int
}

// key is the part of the searched key the evidence adds, so a title is searched again when it
// grows: a movie once its header is read, a series when its episodes change.
func (e evidence) key() string {
	switch {
	case e.length > 0:
		return "|length"
	case len(e.episodes) > 0:
		seasons := make([]int, 0, len(e.episodes))
		for s := range e.episodes {
			seasons = append(seasons, s)
		}
		sort.Ints(seasons)
		var b strings.Builder
		b.WriteString("|episodes")
		for _, s := range seasons {
			b.WriteString(" " + strconv.Itoa(s) + ":" + strconv.Itoa(e.episodes[s]))
		}
		return b.String()
	}
	return ""
}

// fits is whether a result agrees with everything the files say. A movie: its runtime matches the
// file's length (decision 56). A series: every season on disk exists in its layout with at least
// as many episodes as the highest number found (decision 57). Containment, not equality: a show
// still airing, or a household missing episodes, fits; a show TMDB lays out differently from the
// files does not, and waits for the maintainer.
func (e evidence) fits(kind string, rec mp.Record) bool {
	if rec.Descriptive == nil {
		return false
	}
	if kind == string(mp.KindMovie) {
		return sameLength(rec.Descriptive.Runtime, e.length)
	}
	if len(e.episodes) == 0 {
		return false
	}
	size := map[int]int{}
	for _, s := range rec.Descriptive.Seasons {
		size[s.Number] = s.Episodes
	}
	for season, highest := range e.episodes {
		if size[season] < highest {
			return false
		}
	}
	return true
}

// settles is whether a candidate agrees on everything: the name; the year (a title whose files name
// none is checked without it); and the length or the episodes. A remake of about the same length
// agrees as well, and two that agree wait for the maintainer.
func (e evidence) settles(c librarystore.LibraryCandidate) bool {
	return c.NameMatch && (c.YearMatch || e.year == 0) && c.LengthMatch
}

// Run matches until ctx ends.
func (m *Matcher) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(m.Interval, MatchInterval))
	defer tick.Stop()
	for {
		if err := m.Tick(ctx); err != nil && ctx.Err() == nil {
			m.log().Warn("library match: tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick searches for the titles whose search is due. It stops early when the catalog cannot be
// reached; they are searched next tick.
func (m *Matcher) Tick(ctx context.Context) error {
	if m.Enabled != nil && !m.Enabled() {
		return nil
	}
	needs, err := m.Store.LibraryNeeds(ctx)
	if err != nil {
		return err
	}
	// failed counts the searches in a row that failed.
	done, failed := 0, 0
	for _, need := range needs {
		title, year, whole := searchQuery(need.Name, need.Year)
		ev := evidence{year: year, whole: whole, length: m.length(ctx, need), episodes: m.episodes(ctx, need)}
		searched := matchRules + "|" + title + "|" + strconv.Itoa(year) + ev.key()
		if title == "" || need.Searched == searched {
			continue
		}
		if done == matchBatch {
			return nil
		}
		done++
		candidates, settled, err := m.candidates(ctx, need, title, ev)
		if errors.Is(err, tmdb.ErrAPIKeyRequired) {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Not marked searched, so the next tick searches it again. One title whose search
			// fails holds back no other; matchFailLimit in a row is the catalog not answering.
			failed++
			if failed == matchFailLimit {
				m.log().Warn("library match: the catalog is not answering, searching again next tick", "err", err)
				return nil
			}
			m.log().Debug("library match: search failed", "item", need.ID, "err", err)
			continue
		}
		failed = 0
		if settled.Value != "" && m.Match != nil {
			if err := m.Match(ctx, need.ID, settled); err != nil {
				// Not marked searched: the next tick searches and settles it again.
				m.log().Warn("library match: settled title not answered", "item", need.ID, "err", err)
				searched = ""
			}
		}
		if err := m.Store.SaveLibraryCandidates(ctx, need.ID, searched, candidates); err != nil {
			return err
		}
	}
	return nil
}

// candidates searches for one title and ranks what it finds. settled is the catalog id of the one
// candidate that agrees on everything the title can be checked by (decisions 56 and 57), or zero
// when none or more than one does: then the maintainer decides (decision 58).
func (m *Matcher) candidates(ctx context.Context, need librarystore.LibraryNeed, title string, ev evidence) (
	_ []librarystore.LibraryCandidate, settled mp.ExternalID, _ error,
) {
	if err := m.wait(ctx); err != nil {
		return nil, mp.ExternalID{}, err
	}
	hits, err := m.Search(ctx, title, 20)
	if err != nil {
		return nil, mp.ExternalID{}, err
	}
	var out []librarystore.LibraryCandidate
	for _, hit := range hits {
		if hit.Kind != need.Kind {
			continue
		}
		if len(out) == matchSearched {
			break
		}
		c := librarystore.LibraryCandidate{
			Kind: hit.Kind, ID: libraryID(hit.ID), Name: hit.Name, Year: hit.Year, Overview: hit.Overview,
			NameMatch: sameName(hit.Name, title) || sameName(hit.Name, ev.whole), YearMatch: sameYear(hit.Year, ev.year),
		}
		if rec, ok := m.details(ctx, hit); ok {
			c.Poster = posterURL(rec)
			if c.Overview == "" && rec.Descriptive != nil {
				c.Overview = rec.Descriptive.Overview
			}
			c.LengthMatch = ev.fits(hit.Kind, rec)
		}
		out = append(out, c)
	}
	agreeing := 0
	for _, c := range out {
		if ev.settles(c) {
			agreeing++
			settled = externalID(c.ID)
		}
	}
	if agreeing != 1 {
		settled = mp.ExternalID{}
	}
	sort.SliceStable(out, func(i, j int) bool { return agreement(out[i]) > agreement(out[j]) })
	if len(out) > matchKept {
		out = out[:matchKept]
	}
	return out, settled, nil
}

// agreement ranks a candidate: the name counts most, then the year, then the length.
func agreement(c librarystore.LibraryCandidate) int {
	n := 0
	if c.NameMatch {
		n += 4
	}
	if c.YearMatch {
		n += 2
	}
	if c.LengthMatch {
		n++
	}
	return n
}

// details fetches a candidate's record. One that cannot be fetched is kept without its poster or
// length.
func (m *Matcher) details(ctx context.Context, hit SearchHit) (mp.Record, bool) {
	if m.Provider == nil {
		return mp.Record{}, false
	}
	if err := m.wait(ctx); err != nil {
		return mp.Record{}, false
	}
	rec, err := m.Provider.Fetch(ctx, mp.Ref{Kind: mp.Kind(hit.Kind), ID: hit.ID})
	if err != nil {
		m.log().Debug("library match: candidate details not fetched", "namespace", hit.ID.Namespace, "id", hit.ID.Value, "err", err)
		return mp.Record{}, false
	}
	return rec, true
}

// length is a title's length from its files' headers: the longest, for a movie with an extra
// copy, and its parts' together for a movie in parts. Zero when no header is read yet.
func (m *Matcher) length(ctx context.Context, need librarystore.LibraryNeed) time.Duration {
	if m.Inventory == nil || need.Kind != string(mp.KindMovie) {
		return 0
	}
	item, ok, err := m.Inventory.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(need.ID)})
	if err != nil || !ok {
		return 0
	}
	// A file's source origin shares its item origin's key, which notes its edition and part.
	exts := map[inventory.OriginKey]originExtension{}
	for _, o := range item.Origins {
		exts[o.Key] = readOriginExtension(o.Observation.Extension)
	}
	var longest int64
	// Each part's longest copy, by edition and part.
	longestPart := map[string]map[int]int64{}
	for _, src := range item.Sources {
		for _, o := range src.Origins {
			ms := o.Observation.Facts.DurationMillis
			longest = max(longest, ms)
			if part := exts[o.Key].Part; part > 0 {
				edition := editionKey(exts[o.Key].Edition)
				if longestPart[edition] == nil {
					longestPart[edition] = map[int]int64{}
				}
				longestPart[edition][part] = max(longestPart[edition][part], ms)
			}
		}
	}
	for _, byPart := range longestPart {
		var total int64
		for _, ms := range byPart {
			total += ms
		}
		longest = max(longest, total)
	}
	return time.Duration(longest) * time.Millisecond
}

// episodes is a series' episodes on disk by season; nil for a movie, or when they can't be read.
func (m *Matcher) episodes(ctx context.Context, need librarystore.LibraryNeed) map[int]int {
	if m.Episodes == nil || need.Kind != string(mp.KindSeries) {
		return nil
	}
	found, err := m.Episodes(ctx, need.ID)
	if err != nil {
		m.log().Debug("library match: episodes not read", "item", need.ID, "err", err)
		return nil
	}
	return found
}

// wait keeps to the provider's rate hint between catalog calls.
func (m *Matcher) wait(ctx context.Context) error {
	if m.Provider == nil {
		return nil
	}
	gap := m.Provider.Capabilities().Rate.MinInterval
	if gap <= 0 {
		return nil
	}
	if m.Sleep != nil {
		return m.Sleep(ctx, gap)
	}
	t := time.NewTimer(gap)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (m *Matcher) log() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

// posterURL is a record's first poster; "" for none.
func posterURL(rec mp.Record) string {
	for _, a := range rec.Artwork {
		if a.Kind == "poster" && strings.HasPrefix(a.Ref, "https://") {
			return a.Ref
		}
	}
	return ""
}

var (
	// leadingYearTag matches a year in brackets before a title: "(2015) Ashgrove", "[2015] Ashgrove".
	leadingYearTag = regexp.MustCompile(`^\s*[(\[]((?:19|20)\d{2})[)\]][ ._-]*`)
	// leadingYearSet matches a year that starts a name, set apart by a dash, a dot or an
	// underscore: "2010 - ", "2010.", "2010_".
	leadingYearSet = regexp.MustCompile(`^\s*((?:19|20)\d{2})(?:\s*-\s*|[._])`)
	// yearSpan matches a word that is a span of years, "1939-1945".
	yearSpan = regexp.MustCompile(`^(?:19|20)\d{2}[-–](?:19|20)\d{2}$`)
	// yearAnywhere matches a year as a word of its own anywhere in a name.
	yearAnywhere = regexp.MustCompile(`(?:^|[^0-9])(?:19|20)\d{2}(?:[^0-9]|$)`)
	// trackerPrefix matches the site a download names itself after, before the title:
	// "www.TrackerNet.org - Ashgrove", "[www.TrackerNet.org] Ashgrove", "[ www.Torrent.Example ] -
	// Ashgrove", and, with no "www.", a site's name with its domain and a dash: "Tracker.io -
	// Ashgrove". The site ends at a bracket, a space, an underscore or a dash, never inside a dotted
	// title.
	trackerPrefix = regexp.MustCompile(`(?i)^\s*\[?\s*(?:www\.(?:[a-z0-9-]+\.)+[a-z]{2,10}(?:\s*\][ ._]*(?:-[ ._]*)?|[ ._]*-[ ._]*|\s+|_+)|(?:[a-z0-9-]+\.)+(?:com|org|net|io|to|cc|me|tv|info|xyz|ru|se|nu|ws|club|lol|biz|co|example)\s*\]?[ ._]*-[ ._]*)`)
	// indexPrefix matches a collection's place for a film, before its title: "01 - Ashgrove",
	// "1. Ashgrove", "(1) Ashgrove", "03 Ashgrove" padded as a place is. A number that starts a
	// title, "10 Kettles" or "7 Lanterns", has none of these.
	indexPrefix = regexp.MustCompile(`^\s*(?:\(\d{1,2}\)|\d{1,2}\.|\d{1,2}[ ._]+-|0\d)[ ._]+`)
	// bracketNote matches a bracketed group after a title, which noteWords may say is a note on the
	// release rather than part of its name: "(Dual Audio)", "[HD]", "(2016 Remaster)".
	bracketNote = regexp.MustCompile(`[(\[]([^()\[\]]*)[)\]]`)
	// noteWord is a word a bracketed note on a release is made of.
	noteWord = regexp.MustCompile(`(?i)^(directors?|director's|cut|uncut|extended|unrated|theatrical|special|edition|version|remaster|remastered|restored|dual|audio|multi|subs?|subbed|dubbed|eng|english|hd|sd|fhd|imax|criterion|anniversary|complete|\d+(?:st|nd|rd|th))$`)
	// releaseMark is a word only a release name carries after its title: a resolution, source, codec
	// or rip. The title ends before the first.
	releaseMark = regexp.MustCompile(`(?i)^(\d{3,4}x\d{3,4}|\d{3,4}p|4k|uhd|hdr\d*|blu-?ray|bdrip|brrip|web-?dl|webrip|hdtv|dvdrip|x26[45]|h\.?26[45]|hevc|xvid|aac\d*|ac3|dts|ddp?\d*|atmos|truehd|10bit|8bit|repack|camrip|hdcam|hdrip|hdts|telesync|dvdscr|vostfr|vost|fhd|dl|widescreen|fullscreen)$`)
	// releaseWord is a word a release name carries after its title that a title may carry too:
	// "Tangled Web", "The Cam", "Mister Proper", "The German". It ends the title only where a
	// release's words are plainly under way: after the year, before another release word, or last
	// in a name written with dots.
	releaseWord = regexp.MustCompile(`(?i)^(dv|sdr|bd|web|dvd|remux|avc|proper|extended|unrated|uncut|remastered|imax|internal|limited|cam|screener|multi|subbed|dubbed|hd|german|french|ita|spanish|dutch|hindi|tamil|telugu|malayalam|kannada|bengali|punjabi|marathi|eng|dual|amzn|nf|dsnp|hmax|atvp|pcok|hulu|3d|hsbs|hou|sbs|mkv|mp4|avi|criterion)$`)
)

// releaseToken says whether a word is a release's: a releaseMark, or a releaseWord.
func releaseToken(w string) bool { return releaseMark.MatchString(w) || releaseWord.MatchString(w) }

// releaseCut is where a name's title ends among its words: at its first releaseMark, or at a
// releaseWord after the year, before another release word, or last in a name written with dots
// ("Ashgrove.CAM"). In a name whose title is written in mixed case, a release writes its word in
// capitals ("Ashgrove WEB 1080p") and a title does not ("Tangled Web 1080p", "The.Cam", "The
// German 1080p"): there, only a word in capitals ends the title before the year.
func releaseCut(words []string, dotted bool) int {
	first := func(w string) string { return strings.SplitN(w, "-", 2)[0] }
	mixed := mixedCase(words)
	for i, w := range words {
		if i == 0 {
			continue
		}
		if releaseMark.MatchString(w) {
			return i // "Web-DL" whole
		}
		w = first(w)
		switch {
		case releaseMark.MatchString(w):
			return i
		case mixed && releaseWord.MatchString(w) && titleCased(w) && !releaseYear(first(words[i-1])) &&
			(i+1 == len(words) || !strings.EqualFold(words[i+1], "dl") &&
				(titleCased(first(words[i+1])) || !releaseWord.MatchString(first(words[i+1])))):
			// A language's "German DL", and a release's word in capitals after it, are a release's.
			continue
		// Radarr's "{Movie.CleanTitle}.{Release.Year}.{Edition.Tags}": an edition's word right
		// after the year ends the title however it is written, "Gloam.Moss.2011.Theatrical".
		case releaseYear(first(words[i-1])) && versionWords[strings.ToLower(w)] && w != "and":
			return i
		case !releaseWord.MatchString(w):
			continue
		case releaseYear(first(words[i-1])),
			i+1 < len(words) && releaseToken(first(words[i+1])),
			i+1 == len(words) && dotted:
			return i
		}
	}
	return len(words)
}

// mixedCase says whether a name's words are written in mixed case, some capitalised and some not
// all in capitals: "Tangled Web 1080p" is, "tangled.web.1080p" and "TANGLED WEB" are not.
func mixedCase(words []string) bool {
	titled := false
	for _, w := range words {
		if !strings.ContainsFunc(w, unicode.IsLetter) || releaseMark.MatchString(w) {
			continue
		}
		if w == strings.ToLower(w) {
			continue
		}
		if w != strings.ToUpper(w) {
			titled = true
		}
	}
	return titled
}

// titleCased says whether a word is written as a title's: its first letter alone in capitals,
// "Web", not "WEB" or a release's "MULTi".
func titleCased(w string) bool {
	r, size := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r) && w[size:] == strings.ToLower(w[size:])
}

// releaseNote says whether a bracketed group's words are a note on the release ("Dual Audio",
// "2016 Remaster", "HD"), not part of the title. A year alone is the title's year, kept.
func releaseNote(inner string) bool {
	words := strings.Fields(inner)
	if len(words) == 0 || len(words) == 1 && yearToken.MatchString(words[0]) {
		return false
	}
	for _, w := range words {
		if !noteWord.MatchString(w) && !releaseToken(w) && !yearToken.MatchString(w) {
			return false
		}
	}
	return true
}

// dottedWords splits a word joined with dots into its words, "Lane.2019" and "Ferrow.Lane", where
// the name is otherwise spaced. Letters written apart ("S.W.A.T.") and a number ("3.14") stay whole.
func dottedWords(w string) []string {
	parts := strings.Split(w, ".")
	if len(parts) < 2 || slices.Contains(parts, "") {
		return []string{w}
	}
	for _, p := range parts {
		if letters := strings.IndexFunc(p, unicode.IsLetter) >= 0; letters && utf8.RuneCountInString(p) > 1 {
			return parts
		}
	}
	return []string{w}
}

var yearToken = regexp.MustCompile(`^(19|20)\d{2}$`)

// videoSiteID matches a video site's id in brackets at a name's end, as yt-dlp writes it: eleven
// letters, digits, dashes or underscores, "[abcDEF12345]" (withoutVideoID).
var videoSiteID = regexp.MustCompile(`\s*\[([A-Za-z0-9_-]{11})\]$`)

// withoutVideoID is a name without a video site's id at its end: one of eleven characters in both
// cases or with a digit ("[abcDEF12345]", "[abcdef12345]"), never a word or a release's notes
// ("[Remastered]").
func withoutVideoID(name string) string {
	m := videoSiteID.FindStringSubmatchIndex(name)
	if m == nil {
		return name
	}
	// An id carries a digit or both cases, as a word or a release's note doesn't.
	id := name[m[2]:m[3]]
	if strings.IndexFunc(id, unicode.IsDigit) < 0 && (strings.ToLower(id) == id || strings.ToUpper(id) == id) ||
		strings.IndexFunc(id, unicode.IsLetter) < 0 {
		return name
	}
	return name[:m[0]]
}

// releaseYear is whether a year-like word could be a release's year: none is dated after next year,
// so "Harbor Lights 2049" is a title.
func releaseYear(word string) bool {
	year, _ := strconv.Atoi(word)
	return yearToken.MatchString(word) && year <= time.Now().Year()+1
}

// searchQuery is what a title nothing grounds is searched for: its name up to the release words,
// and its year. known is a year already known (from a sidecar); otherwise the last year-like word
// after the first one is the year ("Harbor Lights 2049 (2017)").
//
// whole is the name with that word kept, when it was not bracketed: "Lantern Keeper 1984" is a film
// of 1984 or a film called that, and only the catalog can say which, so a hit may match either.
func searchQuery(name string, known int) (title string, year int, whole string) {
	// Full-width letters and digits ("ＦＥＲＲＯＷ（２０１９）") are searched as the plain ones; "³",
	// "½" and "…" stay as written.
	name = width.Fold.String(name)
	name = editionTag.ReplaceAllString(embeddedID.ReplaceAllString(name, " "), " ")
	// A release's notes at its end say nothing of its title: "Ashgrove.CAM [1080p]" is
	// "Ashgrove.CAM".
	name = trimReleaseNotes(strings.TrimSpace(name))
	// A video site's id after the title, "How to throw a bowl [abcDEF12345]", is no part of it.
	name = withoutVideoID(name)
	// A leading year is the year: "(2015) Ashgrove". Any other leading tag is a release group's.
	leadingYear := 0
	if m := leadingYearTag.FindStringSubmatch(name); m != nil {
		leadingYear, name = atoi(m[1]), name[len(m[0]):]
	} else if m := leadingYearSet.FindStringSubmatch(name); m != nil && releaseYear(m[1]) &&
		!yearAnywhere.MatchString(name[len(m[0]):]) && strings.IndexFunc(name[len(m[0]):], unicode.IsLetter) == 0 {
		// So is one set apart by a dash, a dot or an underscore, as a collection sorted by year
		// names its films: "2010 - Hob Crag", "2010.Hob.Cove", "2010_Hob_Corrie". A year with only
		// a space after it may start a title ("1984 Revisited"), and one with a year of its own
		// after it is its title's ("2001.Gloam.Odyssey.1968").
		leadingYear, name = atoi(m[1]), name[len(m[0]):]
	}
	name = trackerPrefix.ReplaceAllString(name, "")
	// A leading tag is a group's only when a title follows it: "[Carrow Fell] (2017)" is the title
	// in brackets.
	if bare := leadingTags.ReplaceAllString(name, ""); strings.IndexFunc(bare, unicode.IsLetter) >= 0 {
		name = bare
	} else {
		name = strings.NewReplacer("[", "", "]", "", "{", "", "}", "").Replace(name)
	}
	name = strings.NewReplacer("【", "[", "】", "]", "「", "[", "」", "]").Replace(name)
	// A copy a household made by hand: "Copy of Ashgrove (2015)".
	if strings.HasPrefix(strings.ToLower(name), "copy of ") {
		name = name[len("copy of "):]
	}
	if loc := indexPrefix.FindStringIndex(name); loc != nil && strings.IndexFunc(name[loc[1]:], unicode.IsLetter) >= 0 {
		name = name[loc[1]:]
	}
	// A name written with dots is one still when a bracketed note sits among them:
	// "Ashgrove.2015.[1080p]".
	dotted := !strings.Contains(name, " ")
	name = bracketNote.ReplaceAllStringFunc(name, func(group string) string {
		if releaseNote(group[1 : len(group)-1]) {
			return " "
		}
		return group
	})
	if dotted {
		name = strings.NewReplacer(".", " ", "_", " ", "+", " ").Replace(name)
	}
	// A name joined only with dashes, "ferrow-lane-2019" or "ferrow-lane-1080p", is spaced where a
	// year or a release's mark shows it is words; "Spider-Lane" stays one.
	if !strings.Contains(strings.TrimSpace(name), " ") && slices.ContainsFunc(strings.Split(name, "-")[1:], func(w string) bool {
		w = strings.Trim(w, "()[]")
		return releaseYear(w) || releaseMark.MatchString(w)
	}) {
		name = strings.ReplaceAll(name, "-", " ")
	}
	var words []string
	for _, w := range strings.FieldsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("()[]{}_", r)
	}) {
		words = append(words, dottedWords(w)...)
	}
	cut := releaseCut(words, dotted)
	// A year among the release words, "Ashgrove 1080p 2015", is the year when the title gives none.
	after := 0
	for _, w := range words[cut:] {
		if w = strings.SplitN(w, "-", 2)[0]; releaseYear(w) {
			after = atoi(w)
			break
		}
	}
	words = words[:cut]
	clean := func(words []string) string { return strings.Trim(strings.Join(words, " "), " -.") }
	year = known
	for j := len(words) - 1; j > 0; j-- {
		// A year glued to a release group, "2015-GRP", or set between dashes, "-2015-", is the year.
		if w := strings.SplitN(strings.TrimLeft(words[j], "-"), "-", 2)[0]; releaseYear(w) {
			// A year with a title's words after it, "Back to 1985 Again" or "Class of 1999 Kettle", is
			// the title's. A release's year has only its notes after it ("Ashgrove.2015.3D.HSBS",
			// "Kettle Bay 2021 AMZN") or an edition, or is in brackets.
			bracketed := strings.Contains(name, "("+w+")") || strings.Contains(name, "["+w+"]")
			if !bracketed && titleWords(words[j+1:], name) {
				continue
			}
			// A span of years, "Gannet Wars 1939-1945", is what the title is about, not its year.
			if yearSpan.MatchString(words[j]) {
				continue
			}
			if year == 0 {
				year, _ = strconv.Atoi(w)
			}
			if !strings.Contains(name, "("+w+")") && !strings.Contains(name, "["+w+"]") {
				whole = articleFirst(clean(words))
			}
			// An edition between the title and its year is the file's, not the title's:
			// "Mirefield.Extended.Edition.1985", "Kilner Special Edition (2006)".
			words = withoutEdition(words[:j])
			break
		}
	}
	// With no year in its words, an edition last is still the file's: "Brackenfold Special Edition".
	if j := slices.IndexFunc(words, func(w string) bool { return releaseYear(strings.SplitN(strings.TrimLeft(w, "-"), "-", 2)[0]) }); j < 1 {
		words = withoutEdition(words)
	}
	if year == 0 {
		year = leadingYear
	}
	if year == 0 {
		year = after
	}
	return articleFirst(clean(words)), year, whole
}

// editionWords are the editions a release names after its title, in lower case.
var editionWords = [][]string{
	{"extended", "edition"}, {"extended", "cut"}, {"directors", "cut"}, {"director's", "cut"}, {"special", "edition"},
	{"ultimate", "edition"}, {"collectors", "edition"}, {"collector's", "edition"}, {"anniversary", "edition"},
	{"theatrical", "cut"}, {"remastered"}, {"unrated"}, {"extended"},
}

// titleWords says whether words, the last of name's, run on a title: each a word written as name
// writes its title's, "Again" ("again" in a name all in lower case), or a small one between them,
// "of", "the", and not an edition or a release's word. A release's notes are in capitals or carry
// digits: "AMZN", "3D".
func titleWords(words []string, name string) bool {
	if len(words) == 0 || editionOnly(words) {
		return false
	}
	asTitle := func(w string) bool {
		if strings.ContainsFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
			return false
		}
		switch {
		case name == strings.ToLower(name), name == strings.ToUpper(name):
			return true
		}
		r, size := utf8.DecodeRuneInString(w)
		return unicode.IsUpper(r) && w[size:] != "" && strings.ToLower(w[size:]) == w[size:]
	}
	titled := false
	for i, w := range words {
		switch {
		case releaseToken(w):
			return false
		case smallWords[strings.ToLower(w)]:
		// A count after its word runs on a title too: "Holiday 2019 Part 2", "League 2023 Week 1".
		case i > 0 && countWords[strings.ToLower(words[i-1])] && digits(w) && len(w) <= 2:
		case asTitle(w):
			titled = true
		default:
			return false
		}
	}
	return titled
}

// smallWords are the words a title keeps in lower case between its others.
var smallWords = map[string]bool{
	"of": true, "the": true, "and": true, "a": true, "an": true, "in": true, "to": true, "on": true, "at": true,
	"vs": true, "v": true,
}

// countWords are the words a title counts its parts by, before the count: "Part 2", "Week 1".
var countWords = map[string]bool{
	"part": true, "week": true, "day": true, "round": true, "game": true, "match": true, "leg": true,
	"stage": true, "race": true, "chapter": true,
}

// editionOnly says whether words are an edition and nothing else: "Directors Cut", "Extended".
func editionOnly(words []string) bool {
	return slices.ContainsFunc(editionWords, func(edition []string) bool {
		return slices.EqualFunc(words, edition, strings.EqualFold)
	})
}

// withoutEdition is a title's words with an edition at their end taken off; a title that is
// nothing but one, or but an article and one, keeps it: "Extended (2010)", "The Uncut (2004)".
func withoutEdition(words []string) []string {
	for _, edition := range editionWords {
		n := len(words) - len(edition)
		if n > 0 && slices.EqualFunc(words[n:], edition, strings.EqualFold) {
			if n == 1 && slices.Contains([]string{"the", "a", "an"}, strings.ToLower(words[0])) {
				return words
			}
			// The edition's article and the dash setting it off are its own: "Ashwood - The
			// Directors Cut" is "Ashwood".
			if n > 2 && strings.EqualFold(words[n-1], "the") && words[n-2] == "-" {
				n -= 2
			} else if n > 1 && words[n-1] == "-" {
				n--
			}
			return words[:n]
		}
	}
	return words
}

// trailingArticle matches a title Radarr and Plex sort by writing its article last: "Quiet Signal,
// The".
var trailingArticle = regexp.MustCompile(`(?i)^(.+), (the|a|an)$`)

// articleFirst is a title with its article back in front: "Quiet Signal, The" is "The Quiet Signal".
func articleFirst(title string) string {
	if m := trailingArticle.FindStringSubmatch(title); m != nil {
		return m[2] + " " + m[1]
	}
	return title
}

// sameName is whether two names say the same title, ignoring case, punctuation and "&" for "and".
func sameName(a, b string) bool {
	return a != "" && normName(a) == normName(b)
}

// Its letters are read as sortKey reads them, accents off and composed or not ("Amélie" is
// "amelie"), and letters written apart are one word ("S.W.A.T." and "M*A*S*H" are "swat" and
// "mash").
func normName(s string) string {
	s = strings.ReplaceAll(sortKey(s), "&", " and ")
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// "Harbor's" is "Harbors".
		default:
			flush()
		}
	}
	flush()
	// Single letters in a row are one word written apart.
	var out []string
	for i := 0; i < len(words); i++ {
		j := i
		for j < len(words) && utf8.RuneCountInString(words[j]) == 1 && unicode.IsLetter([]rune(words[j])[0]) {
			j++
		}
		if j-i >= 2 {
			out = append(out, strings.Join(words[i:j], ""))
			i = j - 1
			continue
		}
		out = append(out, words[i])
	}
	return strings.Join(out, " ")
}

// sameYear is whether a candidate's year agrees with the file's, a year apart at most: a release
// year and a premiere year often differ by one.
func sameYear(candidate, file int) bool {
	if candidate == 0 || file == 0 {
		return false
	}
	d := candidate - file
	return d >= -1 && d <= 1
}

// sameLength is whether a catalog's runtime agrees with the file's: within five minutes, or 8%
// for a long film.
func sameLength(runtime, file time.Duration) bool {
	if runtime <= 0 || file <= 0 {
		return false
	}
	d := runtime - file
	if d < 0 {
		d = -d
	}
	return d <= max(5*time.Minute, runtime*8/100)
}
