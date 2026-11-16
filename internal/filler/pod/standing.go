package pod

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
)

// Standing is how the live channels stand toward the clips on air, read when a full budget needs
// room (#3855, decision 13 of #3838). Each list is ordered most-aired first, ties by hash, so the
// clip a household has seen most goes first and the order is the same on every read.
type Standing struct {
	// Idle is the clips that fit no live channel. A holiday clip out of season is not idle: it is
	// kept through the off-season (decision 9), so it is in neither list.
	Idle []string
	// Surplus is the clips some channel could air but none needs: each channel keeps the
	// evening-target clips nearest its era, least aired first, and what no channel keeps is surplus.
	// A bumper or station ID a channel can air is never surplus, and neither is a pinned clip.
	Surplus []string
}

// StandingFor reads Standing over one catalog load. A channel's target is the evening it needs
// (PoolTargets.For over SizePool, the same number fetching steers by): enough clips for the
// household's longest watched evening without a repeat, and never fewer than spacing needs. With no
// live channel it reports nothing, since no channel is no reason to call every clip idle.
func (a *Adapter) StandingFor(ctx context.Context, channels []PoolChannel, airings []clips.Airing, spacing time.Duration, now time.Time) (Standing, error) {
	if len(channels) == 0 {
		return Standing{}, nil
	}
	catalog, err := a.catalog.AllClips(ctx)
	if err != nil {
		return Standing{}, err
	}
	pol := a.pol()
	played := map[string]int{}
	for _, airing := range airings {
		played[airing.ClipHash]++
	}

	// The era windows are read as CoverageFrom reads them, so the target a channel is sized by is
	// the one the pool strip and fetching see.
	type live struct {
		w       Window
		windows []EraRange
	}
	lives := make([]live, len(channels))
	eraWindows := make(map[string][]EraRange, len(channels))
	for i, ch := range channels {
		lives[i].w = a.fitWindow(ch.ChannelID, ch.Selection, now)
		if ws := lives[i].w.eraWindows(); len(ws) > 0 && !ws[0].Any() {
			lives[i].windows = NormalizeEraWindows(ws)
		}
		eraWindows[ch.ChannelID] = lives[i].windows
	}
	targets := SizePool(airings, eraWindows, spacing, now)

	fits := map[string]bool{}      // some channel can air it
	kept := map[string]bool{}      // some channel needs it
	offSeason := map[string]bool{} // a holiday clip out of season on a channel
	for _, l := range lives {
		w, windows := l.w, l.windows
		type candidate struct {
			hash  string
			rung  int
			plays int
		}
		var body []candidate
		for _, c := range catalog {
			fit := FitFor(c, w, pol)
			switch {
			case fit.Reason == FitSeason:
				offSeason[c.ID()] = true
				continue
			case fit.Reason != "":
				continue
			}
			fits[c.ID()] = true
			if fit.Pinned || c.EffectivePlacement() != PlacementBreakBody {
				kept[c.ID()] = true // a pin is the operator's word; a bookend is not drawn from the commercial ladder
				continue
			}
			rung := 2
			switch fit.Level {
			case MatchExact:
				rung = 0
			case MatchWidened:
				rung = 1
			}
			body = append(body, candidate{hash: c.ID(), rung: rung, plays: played[c.ID()]})
		}
		slices.SortFunc(body, func(x, y candidate) int {
			return cmp.Or(cmp.Compare(x.rung, y.rung), cmp.Compare(x.plays, y.plays), cmp.Compare(x.hash, y.hash))
		})
		for _, c := range body[:min(targets.For(windows, now.Year()), len(body))] {
			kept[c.hash] = true
		}
	}

	var out Standing
	for _, c := range catalog {
		hash := c.ID()
		switch {
		case offSeason[hash] && !fits[hash]:
		case !fits[hash]:
			out.Idle = append(out.Idle, hash)
		case !kept[hash]:
			out.Surplus = append(out.Surplus, hash)
		}
	}
	mostAired := func(x, y string) int { return cmp.Or(cmp.Compare(played[y], played[x]), cmp.Compare(x, y)) }
	slices.SortFunc(out.Idle, mostAired)
	slices.SortFunc(out.Surplus, mostAired)
	return out, nil
}
