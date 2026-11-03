package filler_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// countingCatalog counts catalog reads, so a test can say how many a report took.
type countingCatalog struct {
	clips []filler.Clip
	reads int
}

func (c *countingCatalog) AllClips(context.Context) ([]filler.Clip, error) {
	c.reads++
	return c.clips, nil
}

// Bookends are catalog-wide, so PoolCounts owns the count: a catalog with no bumper or station ID
// reads as a role gap (#749) whatever channels exist.
func TestPoolCounts_CountsTheBookends(t *testing.T) {
	catalog := []filler.Clip{
		{Path: "bump.mp4", Kind: filler.Bumper, DurationMs: 5_000},
		{Path: "ident.mp4", Kind: filler.StationID, DurationMs: 4_000},
		{Path: "ad.mp4", Kind: filler.Commercial, DurationMs: 30_000},
	}
	if got := filler.PoolCounts(catalog, filler.Policy{}).Bookends; got != 2 {
		t.Errorf("Bookends = %d, want 2 — the bumper and the station ID, not the commercial", got)
	}
	if got := filler.PoolCounts(catalog[2:], filler.Policy{}).Bookends; got != 0 {
		t.Errorf("Bookends = %d, want 0 for a catalog of commercials only", got)
	}
}

// The strip's diagnosis line names Channels[0], so equally-bad channels must keep one order.
func TestOrderWorstFirst_LevelThenNumber(t *testing.T) {
	at := func(id string, number int, level filler.MatchLevel) filler.ChannelCoverage {
		return filler.ChannelCoverage{ChannelID: id, Number: number, Report: filler.CoverageReport{Level: level}}
	}
	channels := []filler.ChannelCoverage{
		at("exact-3", 3, filler.MatchExact),
		at("card-9", 9, filler.MatchBumperCard),
		at("widened-5", 5, filler.MatchWidened),
		at("card-2", 2, filler.MatchBumperCard),
	}

	filler.OrderWorstFirst(channels)

	var got []string
	for _, c := range channels {
		got = append(got, c.ChannelID)
	}
	want := []string{"card-2", "card-9", "widened-5", "exact-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// One catalog read answers the counts and every channel; the report lists channels worst first
// and carries the selection each channel was given.
func TestPoolFor_OneCatalogReadForEveryChannel(t *testing.T) {
	catalog := &countingCatalog{clips: []filler.Clip{
		{Hash: "a", Path: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000, Era: 1992},
		{Hash: "i", Path: "i.mp4", Kind: filler.StationID, DurationMs: 4_000, Era: 1992},
	}}
	adapter := filler.NewPodAdapter(catalog, nil, nil, slog.New(slog.DiscardHandler))

	report, err := adapter.PoolFor(context.Background(), []filler.PoolChannel{
		{ChannelID: "ch-b", Name: "B", Number: 8},
		{ChannelID: "ch-a", Name: "A", Number: 4},
		{ChannelID: "ch-c", Name: "C", Number: 6},
	})
	if err != nil {
		t.Fatal(err)
	}

	if catalog.reads != 1 {
		t.Errorf("catalog reads = %d, want 1 — the counts and every channel share one snapshot", catalog.reads)
	}
	if report.Clips != 2 || report.Bookends != 1 {
		t.Errorf("counts = %d clips, %d bookends; want 2 and 1", report.Clips, report.Bookends)
	}
	if len(report.Channels) != 3 {
		t.Fatalf("channels = %d, want 3", len(report.Channels))
	}
	for i := 1; i < len(report.Channels); i++ {
		prev, cur := report.Channels[i-1], report.Channels[i]
		if filler.LevelWorseThan(cur.Report.Level, prev.Report.Level) {
			t.Errorf("%s (%s) listed before %s (%s): not worst first",
				prev.ChannelID, prev.Report.Level, cur.ChannelID, cur.Report.Level)
		}
		if prev.Report.Level == cur.Report.Level && prev.Number > cur.Number {
			t.Errorf("equal levels out of number order: %d before %d", prev.Number, cur.Number)
		}
	}
}
