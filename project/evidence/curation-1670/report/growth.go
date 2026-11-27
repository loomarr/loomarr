package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

// GrowthParams are the numbers the simulator sets for #4140 step 2 (#4333), shown to the
// maintainer before growth ships.
type GrowthParams struct {
	Margin       int           // years either side of the seeds' span
	Bar          float64       // share of the line's genres a clear fit carries
	MaxShowShare float64       // the largest share of a day one show may take
	DailyAirtime time.Duration // programme time a day carries, breaks excluded
}

// GrowthRow is one channel's line and what the library holds against it, in counts only.
type GrowthRow struct {
	Channel                         string
	Number                          int
	Seeds, OddSeeds                 int
	Fixed                           bool
	Franchise                       bool
	Line                            string
	Clear, Unclear, Unrated, Out    int
	ClearEpisodes, UnclearEpisodes  int
	ClearShows, ClearFilms          int
	Episodes, Shows, Films          int // what the channel carries now
	Target                          growth.Target
	ShortAfterClear, ShortAfterBoth string
}

// candidate is a library series or film, keyed the way lineups are.
type candidate struct {
	title    growth.Title
	name     string
	episodes int
}

// admitted is the library titles the channel's stored scope and ceiling admit, and on a holiday
// channel only those that fit its holidays: what the nightly fill reads before the line. A policy
// this build cannot read admits everything, so an older snapshot still reports.
func admitted(c Channel, library []candidate) map[provision.Key]bool {
	out := make(map[provision.Key]bool, len(library))
	var policy schedule.ChannelPolicy
	if err := json.Unmarshal([]byte(c.PolicyJSON), &policy); err != nil || c.PolicyJSON == "" {
		for _, cand := range library {
			out[cand.title.Key] = true
		}
		return out
	}
	entries := make([]schedule.LineupEntry, 0, len(library))
	for _, cand := range library {
		entries = append(entries, schedule.LineupEntry{Key: cand.title.Key, Title: cand.name,
			OfficialRating: cand.title.Rating, Genres: cand.title.Genres, Year: cand.title.Year})
	}
	vocab := holidayvocab.Default()
	for _, e := range schedule.Admitted(entries, policy, schedule.Strategy(c.Strategy)) {
		if schedule.FitsHolidayChannel(e, policy, vocab) {
			out[e.Key] = true
		}
	}
	return out
}

func runGrowth(ctx context.Context, db *sql.DB, s Snapshot, p GrowthParams, w io.Writer) ([]GrowthRow, error) {
	seeds, err := loadSeeds(ctx, db)
	if err != nil {
		return nil, err
	}
	library, err := loadCandidates(ctx, db)
	if err != nil {
		return nil, err
	}
	var rows []GrowthRow
	for _, c := range s.Channels {
		rows = append(rows, growthRow(c, seeds[c.ID], library, p))
	}
	if err := printGrowth(w, rows, p); err != nil {
		return nil, err
	}
	// The bar is one of the simulator's numbers: how the clear and unclear counts move with it.
	out := &printer{w: w}
	out.printf("\nclear bar\tclear titles\tunclear titles\tchannels still short\n")
	for _, bar := range []float64{0.25, 0.34, 0.5, 0.67, 1} {
		q := p
		q.Bar = bar
		clear, unclear, short := 0, 0, 0
		for _, c := range s.Channels {
			r := growthRow(c, seeds[c.ID], library, q)
			clear, unclear = clear+r.Clear, unclear+r.Unclear
			if r.ShortAfterClear != "-" {
				short++
			}
		}
		out.printf("%.2f\t%d\t%d\t%d\n", bar, clear, unclear, short)
	}
	return rows, out.err
}

func growthRow(c Channel, seeds []growth.Title, library []candidate, p GrowthParams) GrowthRow {
	units := map[string]LineupTitle{}
	for _, t := range c.Lineup {
		units[t.Key] = t
	}
	for i := range seeds {
		seeds[i].Airable = units[string(seeds[i].Key)].Units > 0
	}
	l := growth.DrawLine(seeds, p.Margin)
	row := GrowthRow{Channel: c.Name, Number: c.Number, Seeds: len(seeds), OddSeeds: len(l.Odd), Fixed: l.Fixed,
		Franchise: l.Franchise > 0, Line: describeLine(l)}

	var seriesMs, filmMs int64
	var seriesUnits, films int
	for _, t := range c.Lineup {
		switch {
		case t.Units == 0:
		case strings.HasPrefix(t.Key, "series:"):
			row.Shows++
			seriesUnits += t.Units
			seriesMs += t.RuntimeMs
		default:
			films++
			filmMs += t.RuntimeMs
		}
	}
	row.Episodes, row.Films = seriesUnits, films
	need := growth.Need{DailyAirtime: p.DailyAirtime}
	if total := seriesMs + filmMs; total > 0 {
		need.FilmShare = float64(filmMs) / float64(total)
	}
	if seriesUnits > 0 {
		need.MeanEpisode = time.Duration(seriesMs/int64(seriesUnits)) * time.Millisecond
	}
	if films > 0 {
		need.MeanFilm = time.Duration(filmMs/int64(films)) * time.Millisecond
	}
	row.Target = growth.TargetFor(need, p.MaxShowShare)

	allowed := admitted(c, library)
	for _, cand := range library {
		if _, on := units[string(cand.title.Key)]; on || !allowed[cand.title.Key] {
			continue
		}
		switch l.Classify(cand.title, p.Bar).Fit {
		case growth.Clear:
			row.Clear++
			row.ClearEpisodes += cand.episodes
			if cand.title.Kind == growth.Series {
				row.ClearShows++
			} else {
				row.ClearFilms++
			}
		case growth.Unclear:
			row.Unclear++
			row.UnclearEpisodes += cand.episodes
		case growth.Unrated:
			row.Unrated++
		default:
			row.Out++
		}
	}
	if l.Fixed { // a single-show channel is that size by design (decision 6): it is never short
		row.Target = growth.Target{}
	}
	row.ShortAfterClear = shortfall(row.Target, row.Episodes+row.ClearEpisodes, row.Shows+row.ClearShows, row.Films+row.ClearFilms)
	row.ShortAfterBoth = shortfall(row.Target, row.Episodes+row.ClearEpisodes+row.UnclearEpisodes, row.Shows+row.ClearShows, row.Films+row.ClearFilms)
	return row
}

// shortfall says what the channel still lacks against its target, or "-" when it reaches it.
func shortfall(t growth.Target, episodes, shows, films int) string {
	var parts []string
	if t.Episodes > episodes {
		parts = append(parts, fmt.Sprintf("%d ep", t.Episodes-episodes))
	}
	if t.Shows > shows && t.Episodes > 0 {
		parts = append(parts, fmt.Sprintf("%d shows", t.Shows-shows))
	}
	if t.Films > films {
		parts = append(parts, fmt.Sprintf("%d films", t.Films-films))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func describeLine(l growth.Line) string {
	switch {
	case l.Fixed:
		return "fixed"
	case l.Franchise > 0:
		return "one franchise"
	}
	kinds := make([]string, 0, len(l.Kinds))
	for _, k := range l.Kinds {
		kinds = append(kinds, string(k))
	}
	years := "any year"
	if l.YearTo != 0 {
		years = fmt.Sprintf("%d-%d", l.YearFrom, l.YearTo)
	}
	r := string(l.MaxRating)
	if r == "" {
		r = "any rating"
	}
	return fmt.Sprintf("%s, %s, to %s, %d genres", strings.Join(kinds, "+"), years, r, len(l.Genres))
}

func printGrowth(w io.Writer, rows []GrowthRow, p GrowthParams) error {
	out := &printer{w: w}
	out.printf("\nGrowth (#4333): margin %d y, clear bar %.2f, max show share %.2f, %v a day\n",
		p.Margin, p.Bar, p.MaxShowShare, p.DailyAirtime)
	if out.err != nil {
		return out.err
	}
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', 0)
	table := &printer{w: tw}
	table.printf("ch\tseeds\todd\tline\tclear\tunclear\tunrated\toutside\tnow ep/shows/films\ttarget ep/shows/films\tshort after clear\tshort after both\n")
	for _, r := range rows {
		table.printf("%d\t%d\t%d\t%s\t%d (+%d ep)\t%d (+%d ep)\t%d\t%d\t%d/%d/%d\t%d/%d/%d\t%s\t%s\n",
			r.Number, r.Seeds, r.OddSeeds, r.Line, r.Clear, r.ClearEpisodes, r.Unclear, r.UnclearEpisodes, r.Unrated, r.Out,
			r.Episodes, r.Shows, r.Films, r.Target.Episodes, r.Target.Shows, r.Target.Films, r.ShortAfterClear, r.ShortAfterBoth)
	}
	if table.err != nil {
		return table.err
	}
	return tw.Flush()
}

// loadSeeds reads every channel's lineup entries as growth titles. Every entry is a seed until
// growth adds titles of its own.
func loadSeeds(ctx context.Context, db *sql.DB) (map[string][]growth.Title, error) {
	rows, err := db.QueryContext(ctx, `SELECT channel_id, title_key, entry FROM channel_lineup ORDER BY channel_id, position`)
	if err != nil {
		return nil, fmt.Errorf("seeds: %w", err)
	}
	out := map[string][]growth.Title{}
	return out, each(rows, func(rows *sql.Rows) error {
		var ch, key, entry string
		if err := rows.Scan(&ch, &key, &entry); err != nil {
			return err
		}
		var e struct {
			Genres         []string
			OfficialRating string
			Year           int
			CollectionID   int
		}
		_ = json.Unmarshal([]byte(entry), &e) // a malformed entry is a seed with no facts
		out[ch] = append(out[ch], growth.Title{Key: keyOf(key), Kind: kindOf(key), Year: e.Year,
			Rating: rating.Normalize(e.OfficialRating), Genres: e.Genres, Franchise: max(e.CollectionID, 0)})
		return nil
	})
}

// loadCandidates reads the library's series and films with what growth needs. A title is keyed by
// its TMDB id (films) or TVDB id (series), the namespaces lineups use; a title with neither cannot
// be keyed and is left out. The library holds media-server collections, not TMDB franchises, so a
// candidate's franchise is unknown here.
func loadCandidates(ctx context.Context, db *sql.DB) ([]candidate, error) {
	const q = `
SELECT t.id, t.kind, t.name, t.year, t.rating,
       coalesce((SELECT n.namespace || ':' || n.value FROM library_ids n WHERE n.item_id = t.id
                 AND n.namespace = CASE t.kind WHEN 'movie' THEN 'tmdb' ELSE 'tvdb' END LIMIT 1), ''),
       coalesce((SELECT group_concat(g.name, '|') FROM library_genres g WHERE g.item_id = t.id), ''),
       (SELECT count(*) FROM library_items e WHERE e.series_id = t.id AND e.kind = 'episode')
FROM library_items t WHERE t.kind IN ('movie', 'series')`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("candidates: %w", err)
	}
	var out []candidate
	err = each(rows, func(rows *sql.Rows) error {
		var id, kind, name, r, ns, genres string
		var year, episodes int
		if err := rows.Scan(&id, &kind, &name, &year, &r, &ns, &genres, &episodes); err != nil {
			return err
		}
		if ns == "" || (kind == "series" && episodes == 0) {
			return nil
		}
		c := candidate{title: growth.Title{Key: keyOf(kind + ":" + ns), Kind: growth.Kind(kind), Year: year,
			Rating: rating.Normalize(r), Airable: true}, name: name, episodes: episodes}
		if genres != "" {
			c.title.Genres = strings.Split(genres, "|")
		}
		out = append(out, c)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].title.Key < out[j].title.Key })
	return out, err
}

func keyOf(key string) provision.Key { return provision.Key(key) }

func kindOf(key string) growth.Kind {
	if strings.HasPrefix(key, "series:") {
		return growth.Series
	}
	return growth.Film
}
