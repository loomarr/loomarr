package api

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

var hlT0 = time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)

// lineup builds one channel's airings from a compact script: "S1E1" is an episode of `series`,
// "M" a movie, "F" a filler break, "X:<series>:S2E1" an episode of another series. Each block is
// 30 minutes, back to back from hlT0.
func lineup(series string, script ...string) []playout.Broadcast {
	var out []playout.Broadcast
	at := hlT0
	for _, s := range script {
		b := playout.Broadcast{Start: at, Stop: at.Add(30 * time.Minute)}
		switch s {
		case "F":
			b.Kind = schedule.SlotFiller
		case "M":
			b.Kind, b.Title, b.Key = schedule.SlotProgram, "A film", "movie:tmdb:1"
		default:
			name := series
			if rest, ok := strings.CutPrefix(s, "X:"); ok {
				name, s, _ = strings.Cut(rest, ":")
			}
			var season, episode int
			_, _ = fmt.Sscanf(s, "S%dE%d", &season, &episode)
			b.Kind, b.SeriesTitle, b.Title = schedule.SlotProgram, name, s
			b.Key = provision.Key("series:tvdb:" + name)
			b.Season, b.Episode = season, episode
		}
		out = append(out, b)
		at = b.Stop
	}
	return out
}

func describePicks(picks []highlightPick) string {
	var parts []string
	for _, p := range picks {
		s := fmt.Sprintf("%s:%s@%s", p.channelID, p.reason, p.airing.Start.Sub(hlT0))
		if p.episodes > 0 {
			s += fmt.Sprintf("x%d-until-%s", p.episodes, p.until.Sub(hlT0))
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func TestHighlightsFindPremieresAndMarathons(t *testing.T) {
	got := pickHighlights([]channelAirings{
		// A series premiere; two episodes back to back isn't a marathon, so it carries no run.
		{channelID: "ch-a", broadcasts: lineup("Show A", "S1E1", "F", "S1E2")},
		// Four back to back with breaks between: a marathon until the last one ends.
		{channelID: "ch-b", broadcasts: lineup("Show B", "S3E4", "F", "S3E5", "S3E6", "F", "S3E7", "M")},
		// A season premiere that opens a run of three: the premiere, carrying the run.
		{channelID: "ch-c", broadcasts: lineup("Show C", "M", "S4E1", "S4E2", "S4E3")},
		// Two episodes is not a marathon; a movie and specials are not highlights.
		{channelID: "ch-d", broadcasts: lineup("Show D", "S2E5", "S2E6", "M", "S0E1")},
	}, 10)
	want := "ch-a:series_premiere@0s ch-b:marathon@0sx4-until-3h0m0s ch-c:season_premiere@30m0sx3-until-2h0m0s"
	if describePicks(got) != want {
		t.Fatalf("highlights =\n %s\nwant\n %s", describePicks(got), want)
	}
}

// Another show between two runs of the same show ends the run.
func TestHighlightsARunIsOneShowBackToBack(t *testing.T) {
	got := pickHighlights([]channelAirings{
		{channelID: "ch-a", broadcasts: lineup("Show A", "S1E5", "S1E6", "X:Other:S1E9", "S1E7", "S1E8", "F", "S1E9", "S1E10")},
	}, 10)
	if want := "ch-a:marathon@1h30m0sx4-until-4h0m0s"; describePicks(got) != want {
		t.Fatalf("highlights = %s, want %s", describePicks(got), want)
	}
}

// On a channel dealing its episodes out of order, an episode 1 is chance, not a premiere. The
// ordering is asked at each airing's own start: a rule can shuffle one part of the evening only.
func TestHighlightsNoPremieresWhileShuffled(t *testing.T) {
	firstHourShuffled := func(at time.Time) bool { return at.Before(hlT0.Add(time.Hour)) }
	always := func(time.Time) bool { return true }
	got := pickHighlights([]channelAirings{
		// Shuffled at 0: the episode 1 opens a plain marathon. In order again by 2h: a premiere.
		{channelID: "ch-a", shuffledAt: firstHourShuffled, broadcasts: lineup("Show A", "S1E1", "S1E2", "S1E3", "M", "X:Other:S2E1")},
		// Always shuffled and too short for a marathon: nothing.
		{channelID: "ch-b", shuffledAt: always, broadcasts: lineup("Show B", "S1E1", "S1E2")},
	}, 10)
	if want := "ch-a:marathon@0sx3-until-1h30m0s ch-a:season_premiere@2h0m0s"; describePicks(got) != want {
		t.Fatalf("highlights = %s, want %s", describePicks(got), want)
	}
}

// A channel looping a short pool airs the same premiere and the same marathon every cycle. The
// rerun is not news: one highlight per show on a channel, its best one. The same show on another
// channel is its own highlight.
func TestHighlightsOnePerShowPerChannel(t *testing.T) {
	got := pickHighlights([]channelAirings{
		{channelID: "ch-a", broadcasts: lineup("Show A", "S1E1", "S1E2", "S1E3", "M", "S1E1", "S1E2", "S1E3")},
		{channelID: "ch-b", broadcasts: lineup("Show A", "S2E4", "S2E5", "S2E6", "M", "S2E4", "S2E5", "S2E6", "M", "S2E1")},
		{channelID: "ch-c", broadcasts: lineup("Show B", "S3E2", "S3E3", "S3E4", "M", "S3E2", "S3E3", "S3E4")},
	}, 10)
	if want := "ch-a:series_premiere@0sx3-until-1h30m0s ch-c:marathon@0sx3-until-1h30m0s ch-b:season_premiere@4h0m0s"; describePicks(got) != want {
		t.Fatalf("highlights = %s, want %s", describePicks(got), want)
	}
}

// Home shows a handful: premieres outrank marathons, one per channel before any channel gets a
// second, and the chosen few come back in airtime order.
func TestHighlightsRankSpreadAndOrder(t *testing.T) {
	got := pickHighlights([]channelAirings{
		{channelID: "ch-a", broadcasts: lineup("Show A", "S2E8", "S2E9", "S2E10", "F", "X:Other:S1E1")},
		{channelID: "ch-b", broadcasts: lineup("Show B", "M", "M", "S5E1")},
		{channelID: "ch-c", broadcasts: lineup("Show C", "S1E2", "S1E3", "S1E4", "S1E5", "S1E6")},
	}, 3)
	// ch-a's premiere and ch-b's premiere rank first; ch-c's marathon beats ch-a's second pick.
	if want := "ch-c:marathon@0sx5-until-2h30m0s ch-b:season_premiere@1h0m0s ch-a:series_premiere@2h0m0s"; describePicks(got) != want {
		t.Fatalf("highlights = %s, want %s", describePicks(got), want)
	}
}

// Property: over random lineups, every pick is justified by the airings, the set is bounded, spread
// across channels when it can be, and in airtime order.
func TestHighlightsInvariantsProperty(t *testing.T) {
	shows := []string{"Show A", "Show B", "Show C"}
	for seed := range uint64(400) {
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
		var channels []channelAirings
		for c := range 1 + rng.IntN(6) {
			var script []string
			for range rng.IntN(16) {
				switch r := rng.IntN(10); {
				case r < 2:
					script = append(script, "F")
				case r < 3:
					script = append(script, "M")
				default:
					script = append(script, fmt.Sprintf("X:%s:S%dE%d", shows[rng.IntN(len(shows))], rng.IntN(3), 1+rng.IntN(4)))
				}
			}
			ch := channelAirings{channelID: fmt.Sprintf("ch-%d", c), broadcasts: lineup("", script...)}
			if rng.IntN(2) == 0 {
				cut := hlT0.Add(time.Duration(rng.IntN(16)) * 30 * time.Minute)
				ch.shuffledAt = func(at time.Time) bool { return at.Before(cut) }
			}
			channels = append(channels, ch)
		}
		shuffledAt := map[string]func(time.Time) bool{}
		for _, ch := range channels {
			shuffledAt[ch.channelID] = ch.shuffledAt
		}
		limit := 1 + rng.IntN(5)
		picks := pickHighlights(channels, limit)
		all := pickHighlights(channels, 1000)

		if len(picks) > limit || (len(all) >= limit && len(picks) != limit) || (len(all) < limit && len(picks) != len(all)) {
			t.Fatalf("seed %d: %d picks from %d candidates at limit %d", seed, len(picks), len(all), limit)
		}
		candidateChannels := map[string]bool{}
		for _, p := range all {
			candidateChannels[p.channelID] = true
		}
		pickedChannels := map[string]bool{}
		pickedShows := map[string]bool{}
		for i, p := range picks {
			if show := p.channelID + "|" + string(p.airing.Key); pickedShows[show] {
				t.Fatalf("seed %d: a show picked twice on one channel: %s", seed, describePicks(picks))
			} else {
				pickedShows[show] = true
			}
			if i > 0 && p.airing.Start.Before(picks[i-1].airing.Start) {
				t.Fatalf("seed %d: not in airtime order: %s", seed, describePicks(picks))
			}
			if len(candidateChannels) >= limit && pickedChannels[p.channelID] {
				t.Fatalf("seed %d: a channel picked twice while others had highlights: %s", seed, describePicks(picks))
			}
			pickedChannels[p.channelID] = true
			if p.airing.Kind != schedule.SlotProgram || p.airing.SeriesTitle == "" {
				t.Fatalf("seed %d: non-episode highlight %+v", seed, p.airing)
			}
			if p.reason != highlightMarathon {
				if shuffled := shuffledAt[p.channelID]; shuffled != nil && shuffled(p.airing.Start) {
					t.Fatalf("seed %d: %s on a shuffle: %s", seed, p.reason, describePicks(picks))
				}
			}
			switch p.reason {
			case highlightSeriesPremiere:
				if p.airing.Season != 1 || p.airing.Episode != 1 {
					t.Fatalf("seed %d: series premiere on S%dE%d", seed, p.airing.Season, p.airing.Episode)
				}
			case highlightSeasonPremiere:
				if p.airing.Season < 2 || p.airing.Episode != 1 {
					t.Fatalf("seed %d: season premiere on S%dE%d", seed, p.airing.Season, p.airing.Episode)
				}
			case highlightMarathon:
				if p.episodes < marathonMinEpisodes {
					t.Fatalf("seed %d: marathon of %d", seed, p.episodes)
				}
			default:
				t.Fatalf("seed %d: unknown reason %q", seed, p.reason)
			}
			if p.episodes > 0 {
				// The run the pick claims: `episodes` programmes of this show from the pick, no other
				// programme between them, ending at `until`.
				var bs []playout.Broadcast
				for _, ch := range channels {
					if ch.channelID == p.channelID {
						bs = ch.broadcasts
					}
				}
				n, last := 0, time.Time{}
				for _, b := range bs {
					if b.Start.Before(p.airing.Start) || b.Kind != schedule.SlotProgram {
						continue
					}
					if b.Key != p.airing.Key {
						break
					}
					n++
					last = b.Stop
				}
				if n != p.episodes || !last.Equal(p.until) {
					t.Fatalf("seed %d: pick claims %d until %s, airings give %d until %s: %s", seed,
						p.episodes, p.until.Sub(hlT0), n, last.Sub(hlT0), describePicks(picks))
				}
			}
		}
	}
}
