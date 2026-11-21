package pod_test

import (
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// airedHashes is every clip a channel airs across many seeded breaks.
func airedHashes(catalog []pod.Clip, w pod.Window) []string {
	seen := map[string]bool{}
	for seed := int64(1); seed <= 80; seed++ {
		w.Seed = seed
		for _, e := range pod.Assemble(catalog, w, pod.Policy{}, nil).Entries {
			if !e.IsFallbackCard {
				seen[e.Hash] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func ceilingCatalog() []pod.Clip {
	var out []pod.Clip
	for _, aud := range []pod.Audience{pod.Kids, pod.Family, pod.General, pod.LateNight} {
		out = append(out,
			pod.Clip{Hash: "ad-" + string(aud), Path: "ad-" + string(aud), Role: pod.Commercial, Era: 1992, Audience: aud, DurationMs: 20_000},
			pod.Clip{Hash: "bumper-" + string(aud), Path: "bumper-" + string(aud), Role: pod.Bumper, Era: 1992, Audience: aud, DurationMs: 5_000},
			pod.Clip{Hash: "ident-" + string(aud), Path: "ident-" + string(aud), Role: pod.StationID, Era: 1992, Audience: aud, DurationMs: 5_000},
		)
	}
	return out
}

// The audience is a ceiling (#3838 decision 8): a kids channel airs kids and family; family and
// general channels air kids, family and general; a late-night channel airs everything. Ads and
// bookends follow the same ceiling.
func TestAudienceIsACeiling(t *testing.T) {
	for _, tc := range []struct {
		channel pod.Audience
		admits  []pod.Audience
	}{
		{pod.Kids, []pod.Audience{pod.Kids, pod.Family}},
		{pod.Family, []pod.Audience{pod.Kids, pod.Family, pod.General}},
		{pod.General, []pod.Audience{pod.Kids, pod.Family, pod.General}},
		{pod.LateNight, []pod.Audience{pod.Kids, pod.Family, pod.General, pod.LateNight}},
	} {
		t.Run(string(tc.channel), func(t *testing.T) {
			w := pod.Window{ChannelID: "c", Era: pod.Year(1992), Audience: tc.channel, GapMs: 240_000, PodMax: 4}
			got := airedHashes(ceilingCatalog(), w)
			for _, kind := range []string{"ad-", "bumper-", "ident-"} {
				for _, aud := range []pod.Audience{pod.Kids, pod.Family, pod.General, pod.LateNight} {
					id := kind + string(aud)
					if want := slices.Contains(tc.admits, aud); slices.Contains(got, id) != want {
						t.Errorf("%s channel: %s aired = %v, want %v (aired %v)", tc.channel, id, !want, want, got)
					}
				}
			}
		})
	}
}

// A clip missing a fact never airs (#3838 decision 6): an unclassified role, an unknown era or an
// unknown audience fits no channel, whatever the channel's audience.
func TestClipsMissingAFactNeverAir(t *testing.T) {
	good := pod.Clip{Hash: "good", Path: "good", Role: pod.Commercial, Era: 1992, Audience: pod.General, DurationMs: 20_000}
	catalog := []pod.Clip{
		good,
		{Hash: "no-era", Path: "no-era", Role: pod.Commercial, Audience: pod.General, DurationMs: 20_000},
		{Hash: "no-audience", Path: "no-audience", Role: pod.Commercial, Era: 1992, DurationMs: 20_000},
		{Hash: "no-role", Path: "no-role", Role: pod.Unclassified, Era: 1992, Audience: pod.General, DurationMs: 20_000},
		{Hash: "no-era-bumper", Path: "no-era-bumper", Role: pod.Bumper, Audience: pod.General, DurationMs: 5_000},
		{Hash: "no-audience-ident", Path: "no-audience-ident", Role: pod.StationID, Era: 1992, DurationMs: 5_000},
	}
	for _, channel := range []pod.Audience{"", pod.Kids, pod.General, pod.LateNight} {
		for _, era := range []pod.EraRange{pod.Year(1992), {}} {
			w := pod.Window{ChannelID: "c", Era: era, Audience: channel, GapMs: 240_000, PodMax: 8}
			got := airedHashes(catalog, w)
			for _, h := range got {
				if h != "good" {
					t.Errorf("channel %q era %+v aired %s, a clip missing a fact", channel, era, h)
				}
			}
			if channel != pod.Kids && !slices.Contains(got, "good") {
				t.Errorf("channel %q era %+v aired %v, want the clip with all its facts", channel, era, got)
			}
		}
	}
}

// Era falls back to the channel's own decade, then a decade either side, and nothing further
// (#3838 decision 21).
func TestEraNeverFallsFurtherThanANeighbouringDecade(t *testing.T) {
	ad := func(era int) pod.Clip {
		id := fmt.Sprintf("ad-%d", era)
		return pod.Clip{Hash: id, Path: id, Role: pod.Commercial, Era: era, Audience: pod.General, DurationMs: 20_000}
	}
	catalog := []pod.Clip{ad(1975), ad(1985), ad(1993), ad(2003), ad(2015)}
	w := pod.Window{ChannelID: "c", Era: pod.EraRange{From: 1990, To: 1999}, Audience: pod.General, GapMs: 240_000, PodMax: 4}
	got := airedHashes(catalog, w)
	want := []string{"ad-1985", "ad-1993", "ad-2003"}
	if !slices.Equal(got, want) {
		t.Errorf("a 1990s channel aired %v, want %v", got, want)
	}
	// Nothing in reach: the break is station material or the card, never a distant decade.
	far := pod.Window{ChannelID: "c", Era: pod.EraRange{From: 1990, To: 1999}, Audience: pod.General, GapMs: 240_000, PodMax: 4}
	if got := airedHashes([]pod.Clip{ad(1975), ad(2015)}, far); len(got) != 0 {
		t.Errorf("a 1990s channel with only distant ads aired %v, want none", got)
	}
}
