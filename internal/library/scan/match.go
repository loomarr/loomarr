package scan

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Matcher defaults (#2151 step 11, decision 27).
const (
	// MatchInterval is how often the Matcher looks for titles nothing grounds.
	MatchInterval = time.Minute
	// matchBatch caps the titles searched in one tick.
	matchBatch = 20
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

// evidence is what the files say about a title beyond its name: a movie's length, a series'
// episodes. A search result is checked against it.
type evidence struct {
	year     int
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

// settles is whether a candidate agrees on everything: the name; the year (a series without one
// in its folder name is checked without it); and the length or the episodes.
func (e evidence) settles(c librarystore.LibraryCandidate) bool {
	yearOK := c.YearMatch || (c.Kind == string(mp.KindSeries) && e.year == 0)
	return c.NameMatch && yearOK && c.LengthMatch
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
	done := 0
	for _, need := range needs {
		title, year := searchQuery(need.Name, need.Year)
		ev := evidence{year: year, length: m.length(ctx, need), episodes: m.episodes(ctx, need)}
		searched := title + "|" + strconv.Itoa(year) + ev.key()
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
			m.log().Debug("library match: search failed", "item", need.ID, "err", err)
			return nil
		}
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
			NameMatch: sameName(hit.Name, title), YearMatch: sameYear(hit.Year, ev.year),
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
// copy. Zero when no header is read yet.
func (m *Matcher) length(ctx context.Context, need librarystore.LibraryNeed) time.Duration {
	if m.Inventory == nil || need.Kind != string(mp.KindMovie) {
		return 0
	}
	item, ok, err := m.Inventory.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(need.ID)})
	if err != nil || !ok {
		return 0
	}
	var longest int64
	for _, src := range item.Sources {
		for _, o := range src.Origins {
			longest = max(longest, o.Observation.Facts.DurationMillis)
		}
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

// releaseToken is a word a release name carries after the title: the quality, source, codec or
// edition. The title ends before the first.
var releaseToken = regexp.MustCompile(`(?i)^(\d{3,4}p|4k|uhd|hdr\d*|dv|sdr|blu-?ray|bd|bdrip|brrip|web|web-?dl|webrip|hdtv|dvd|dvdrip|remux|x26[45]|h\.?26[45]|hevc|avc|xvid|aac\d*|ac3|dts|ddp?\d*|atmos|truehd|10bit|8bit|proper|repack|extended|unrated|remastered|imax|internal|limited)$`)

var yearToken = regexp.MustCompile(`^(19|20)\d{2}$`)

// searchQuery is what a title nothing grounds is searched for: its name up to the release words,
// and its year. known is a year already known (from a sidecar); otherwise the last year-like word
// after the first one is the year ("Harbor Lights 2049 (2017)").
func searchQuery(name string, known int) (string, int) {
	name = editionTag.ReplaceAllString(embeddedID.ReplaceAllString(name, " "), " ")
	if !strings.Contains(name, " ") {
		name = strings.NewReplacer(".", " ", "_", " ").Replace(name)
	}
	words := strings.FieldsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("()[]{}_", r)
	})
	cut := len(words)
	for i, w := range words {
		if i > 0 && releaseToken.MatchString(strings.SplitN(w, "-", 2)[0]) {
			cut = i
			break
		}
	}
	words = words[:cut]
	year := known
	for j := len(words) - 1; j > 0; j-- {
		if yearToken.MatchString(words[j]) {
			if year == 0 {
				year, _ = strconv.Atoi(words[j])
			}
			words = words[:j]
			break
		}
	}
	return strings.Trim(strings.Join(words, " "), " -."), year
}

// sameName is whether two names say the same title, ignoring case, punctuation and "&" for "and".
func sameName(a, b string) bool {
	return a != "" && normName(a) == normName(b)
}

func normName(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", " and ")
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		case r == '\'' || r == '’':
			// "Harbor's" is "Harbors".
		default:
			space = true
		}
	}
	return b.String()
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
