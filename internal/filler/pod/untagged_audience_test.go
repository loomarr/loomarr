package pod

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

// A clip whose audience is unknown is in no channel's ceiling (#3838 decisions 6 and 7): doubt
// resolves to the stricter reading, so "we could not tell who this is for" never airs, on any
// channel, at any era.
func TestLadder_AnEntirelyUntaggedCatalogFallsToTheCard(t *testing.T) {
	cat := []Clip{
		{Hash: "u1", Path: "u1", Role: Commercial, Era: 1992, DurationMs: 30_000},
		{Hash: "u2", Path: "u2", Role: Commercial, Era: 1992, DurationMs: 30_000},
	}
	for _, aud := range []Audience{"", Kids, Family, General, LateNight} {
		brk := Assemble(cat, Window{Era: Year(1992), Audience: aud, GapMs: 120_000, PodMax: 4}, Policy{}, nil)
		if brk.MatchLevel != MatchBumperCard || len(brk.Entries) != 1 || !brk.Entries[0].IsFallbackCard {
			t.Errorf("%q channel: level=%s entries=%+v, want only the bumper card", aud, brk.MatchLevel, brk.Entries)
		}
	}
}

// ⚠ THE #1449 SAFETY INVARIANT, as a property over random catalogs: on any channel a clip outside
// its audience ceiling NEVER airs, whatever else is in the catalog. With nothing eligible the pod
// is the bumper card, never an unsuitable ad.
func TestAssemble_AClipOutsideTheCeilingNeverAirs(t *testing.T) {
	rng := rand.New(rand.NewSource(1449))
	audiences := []Audience{"", Kids, Family, General, LateNight}
	for trial := 0; trial < 300; trial++ {
		var cat []Clip
		for i := 0; i < 1+rng.Intn(12); i++ {
			cat = append(cat, Clip{
				Hash: fmt.Sprintf("c%d-%d", trial, i), Path: fmt.Sprintf("c%d-%d", trial, i), Role: Commercial,
				Era: []int{0, 1985, 1992, 2005}[rng.Intn(4)], Audience: audiences[rng.Intn(len(audiences))],
				DurationMs: 30_000,
			})
		}
		byPath := map[string]Clip{}
		for _, c := range cat {
			byPath[c.Path] = c
		}
		for _, aud := range []Audience{Kids, Family, General, LateNight} {
			w := Window{Era: Year(1992), Audience: aud, GapMs: 120_000, PodMax: 4, Seed: int64(trial)}
			brk := Assemble(cat, w, Policy{}, nil)
			for _, e := range brk.Entries {
				if c, ok := byPath[e.Path]; ok && !audienceFits(aud, c.Audience) {
					t.Fatalf("trial %d: %s channel aired a %q-audience clip (rung %s)", trial, aud, c.Audience, brk.MatchLevel)
				}
			}
		}
	}
}

// An era channel airs its own era and the decade either side, in-window clips first; a clip
// further away never tops up the pod (#3838 decision 21).
func TestAssemble_InWindowClipsLeadAndDistantOnesNeverAir(t *testing.T) {
	cat := []Clip{commercial("in", 1992, General), commercial("near", 2005, General), commercial("far", 2015, General)}
	w := Window{Era: EraRange{From: 1989, To: 1999}, Audience: General, GapMs: 120_000, PodMax: 4}
	brk := Assemble(cat, w, Policy{}, nil)
	if len(brk.Entries) != 2 || brk.Entries[0].Path != "in" || brk.Entries[1].Path != "near" {
		t.Fatalf("entries=%+v, want the in-window clip first, then the neighbouring decade, never 2015", brk.Entries)
	}
	if brk.MatchLevel != MatchWidened {
		t.Fatalf("level=%s, want widened", brk.MatchLevel)
	}
	w.PodMax = 1
	exact := Assemble(cat, w, Policy{}, nil)
	if exact.MatchLevel != MatchExact || len(exact.Entries) != 1 || exact.Entries[0].Path != "in" {
		t.Fatalf("level=%s entries=%+v, want only the in-window clip on the exact rung", exact.MatchLevel, exact.Entries)
	}
}

// audienceFits is the property's oracle, written independently of the ladder.
func audienceFits(channel, clip Audience) bool {
	admits := map[Audience][]Audience{
		Kids:      {Kids, Family},
		Family:    {Kids, Family, General},
		General:   {Kids, Family, General},
		LateNight: {Kids, Family, General, LateNight},
	}
	return slices.Contains(admits[channel], clip)
}

func TestAssemble_KidsChannelNeverAirsGeneralClips(t *testing.T) {
	cat := []Clip{commercial("general", 1992, General), commercial("kids", 1992, Kids), commercial("family", 1992, Family)}
	brk := Assemble(cat, Window{Era: Year(1992), Audience: Kids, GapMs: 120_000, PodMax: 4}, Policy{}, nil)
	for _, e := range brk.Entries {
		if e.Path == "general" {
			t.Fatal("a general-audience clip aired on a kids channel")
		}
	}
	only := Assemble(cat[:1], Window{Era: Year(1992), Audience: Kids, GapMs: 120_000, PodMax: 4}, Policy{}, nil)
	if only.MatchLevel != MatchBumperCard {
		t.Fatalf("level=%s, want the bumper card when only general clips exist", only.MatchLevel)
	}
}
