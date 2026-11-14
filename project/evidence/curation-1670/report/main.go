// Command report measures how repetitive a Loomarr install's channels really were, from a copy of
// its database (#4140 step 0). It reads what aired, not a forecast: the scheduled ledger, the
// watched ledger, the household's viewings and the watched commercial breaks. Both #4140 and #3838
// gate on its "Done when" table.
//
// It never opens a live database. Take a copy first, on the host that runs Loomarr:
//
//	sqlite3 'file:loomarr.db?mode=ro' "VACUUM INTO '/tmp/loomarr-snapshot.db'"
//
// then, wherever the copy is:
//
//	go run ./project/evidence/curation-1670/report -db /tmp/loomarr-snapshot.db \
//	  -tz America/New_York -out /tmp/repeats.json
//
// With no -from/-to it covers everything the scheduled ledger holds (it keeps about eight days).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

func main() {
	dbPath := flag.String("db", "", "a copy of the install's SQLite database (required)")
	fromFlag := flag.String("from", "", "start of the span, RFC 3339 (default: the oldest scheduled airing)")
	toFlag := flag.String("to", "", "end of the span, RFC 3339 (default: just after the newest one)")
	tz := flag.String("tz", "Local", "the household's time zone, which lays out days and evenings")
	out := flag.String("out", "", "also write the report as JSON to this file")
	flag.Parse()
	if err := run(*dbPath, *fromFlag, *toFlag, *tz, *out, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(1)
	}
}

func run(dbPath, fromFlag, toFlag, tz, out string, w io.Writer) error {
	if dbPath == "" {
		return fmt.Errorf("-db is required")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return err
	}
	db, err := openSnapshot(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }() // a read-only snapshot: nothing to lose on a failed close
	s, err := Load(context.Background(), db)
	if err != nil {
		return err
	}
	from, to, err := span(s, fromFlag, toFlag)
	if err != nil {
		return err
	}
	r := Compute(s, from, to, loc)
	if err := printReport(w, r, loc); err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o600)
}

func span(s Snapshot, fromFlag, toFlag string) (time.Time, time.Time, error) {
	var from, to time.Time
	for _, a := range s.Scheduled {
		if from.IsZero() || a.AiredAt.Before(from) {
			from = a.AiredAt
		}
		if a.AiredAt.After(to) {
			to = a.AiredAt
		}
	}
	to = to.Add(time.Second)
	var err error
	if fromFlag != "" {
		if from, err = time.Parse(time.RFC3339, fromFlag); err != nil {
			return from, to, fmt.Errorf("-from: %w", err)
		}
	}
	if toFlag != "" {
		if to, err = time.Parse(time.RFC3339, toFlag); err != nil {
			return from, to, fmt.Errorf("-to: %w", err)
		}
	}
	if !from.Before(to) {
		return from, to, fmt.Errorf("the span is empty: nothing in the scheduled ledger, or -from is not before -to")
	}
	return from, to, nil
}

// printer writes formatted text and keeps the first error, so a report written to a closed pipe
// fails once instead of on every line.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func printReport(w io.Writer, r Report, loc *time.Location) error {
	out := &printer{w: w}
	out.printf("Span %s to %s (%s)\n\n", r.From.In(loc).Format(time.DateTime), r.To.In(loc).Format(time.DateTime), loc)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	table := &printer{w: tw}
	table.printf("ch\tname\torder\ttitles\tlibrary units\thours\tR24h\tR7d\tmin gap h\tshows/day\tmax share\tre-dealt\tguards gone\tnever aired\tfiller excl\tin-era ads\n")
	for _, c := range r.Channels {
		gap := "none" // the kit leaves the gap at zero when nothing aired twice
		if c.Scheduled.ReairGapMinH > 0 {
			gap = fmt.Sprintf("%.1f", c.Scheduled.ReairGapMinH)
		}
		table.printf("%d\t%s\t%s\t%d\t%d\t%.0f\t%s\t%s\t%s\t%d\t%s\t%d/%d\t%t\t%d\t%d/%d\t%s\n",
			c.Number, c.Name, c.Ordering, c.LineupTitles, c.LibraryUnits, c.LibraryHours,
			pct(c.Scheduled.RepeatRate24h), pct(c.Scheduled.RepeatRate7d), gap,
			c.ShowsPerDay, pct(c.MaxShowShare), len(c.RedealDays), c.ChannelDays, c.RelaxedToNothing,
			len(c.NeverScheduled), c.FillerExcluded, c.FillerAirings, count(c.InEraCommercials))
	}
	if table.err != nil {
		return table.err
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	h := r.Household
	source := "learned from viewing"
	if !h.EveningCutLearned {
		source = "no viewing to learn from"
	}
	out.printf("\nEvenings turn over at %02d:00 local (%s)\n", h.EveningCutHour, source)
	out.printf("Worst watched evening: %d clip airings, %d distinct clips, one clip %d times (%s)\n",
		h.WorstEvening.Plays, h.WorstEvening.DistinctClips, h.WorstEvening.MostPlays, h.WorstEvening.Start.In(loc).Format(time.DateTime))
	out.printf("\nDone when\n")
	for _, row := range doneWhen(r) {
		out.printf("  %-62s %s\n", row[0], row[1])
	}
	return out.err
}

// doneWhen is the "Done when" table of #4140 and the filler measures of #3838, as read today.
func doneWhen(r Report) [][2]string {
	var films []string
	minFilmGap := -1.0
	maxShare, relaxed, multi, redeal, days, never := 0.0, 0, 0, 0, 0, 0
	withRules, inEraMin, replaced := 0, -1, 0
	for _, c := range r.Channels {
		if c.FilmOnly {
			films = append(films, c.Name)
			if g := c.Scheduled.ReairGapMinH; g > 0 && (minFilmGap < 0 || g < minFilmGap) {
				minFilmGap = g
			}
		}
		if c.LineupTitles > 1 {
			multi++
			if c.RelaxedToNothing {
				relaxed++
			}
			if !c.FilmOnly && c.MaxShowShare > maxShare {
				maxShare = c.MaxShowShare
			}
		}
		redeal += len(c.RedealDays)
		replaced += c.ReplacedDeals
		days += c.ChannelDays
		never += len(c.NeverScheduled)
		if c.Rules > 0 {
			withRules++
		}
		if c.InEraCommercials >= 0 && c.FillerAirings > 0 && (inEraMin < 0 || c.InEraCommercials < inEraMin) {
			inEraMin = c.InEraCommercials
		}
	}
	excluded, fillerAirings := 0, 0
	for _, c := range r.Channels {
		excluded += c.FillerExcluded
		fillerAirings += c.FillerAirings
	}
	h := r.Household
	return [][2]string{
		{"A film's shortest gap between airings (" + strings.Join(films, ", ") + ")", hours(minFilmGap)},
		{"One show's largest share of a multi-show channel's day", pct(maxShare)},
		{"Channels with repeat guards relaxed to nothing", fmt.Sprintf("%d of %d", relaxed, multi)},
		{"Channel-days re-dealt on air", fmt.Sprintf("%d of %d", redeal, days)},
		{"Ledger rows replaced before they aired", fmt.Sprint(replaced)},
		{"Lineup titles that never aired", fmt.Sprint(never)},
		{"Channels with a scheduling rule", fmt.Sprintf("%d of %d", withRules, len(r.Channels))},
		{"The same episode on two channels within a week", fmt.Sprint(h.SameEpisodeTwoChannels)},
		{"Watched rows recorded only by a tune-in under a minute", fmt.Sprint(h.ShortTuneInSeen)},
		{"Watched evenings with an ad aired twice", fmt.Sprintf("%d of %d", h.EveningsWithRepeat, h.Evenings)},
		{"Ad airings the channel's rules would refuse", fmt.Sprintf("%d of %d", excluded, fillerAirings)},
		{"Fewest airable in-era commercials on a watched channel", count(inEraMin)},
		{"Airable clips", fmt.Sprintf("%d (%.0f min)", h.AirableClips, h.AirableMinutes)},
		{"Clips held for a person", fmt.Sprintf("%d (%.0f min)", h.HeldClips, h.HeldMinutes)},
	}
}

func pct(v float64) string {
	if v < 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", v*100)
}

func hours(v float64) string {
	if v < 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f h", v)
}

func count(n int) string {
	if n < 0 {
		return "–"
	}
	return fmt.Sprint(n)
}
