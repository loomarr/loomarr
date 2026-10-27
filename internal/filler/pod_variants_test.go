package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// variantCatalog is a spot and its 15 s cut-down, linked as versions of one ad (#2033), beside two
// unrelated ads. Everything matches the kids window so only identity decides.
func variantCatalog() []filler.Clip {
	ad := func(hash, category string, durationMs int64, variants ...string) filler.Clip {
		return filler.Clip{Hash: hash, Path: hash, Name: hash, Kind: filler.Commercial, Era: 1992,
			Audience: filler.Kids, Category: category, DurationMs: durationMs, Variants: variants}
	}
	return []filler.Clip{
		ad("spot", "cereal", 30_000, "spot-cut"),
		ad("spot-cut", "cereal", 15_000, "spot"),
		ad("toy", "toys", 30_000),
		ad("tech", "tech", 30_000),
	}
}

// A break with room for every clip still never carries two versions of one ad, whichever seed
// shuffles the pool.
func TestAssemble_NeverPlacesTwoVersionsOfOneAdInABreak(t *testing.T) {
	cat := variantCatalog()
	for seed := range int64(64) {
		pod := filler.Assemble(cat, kidsWindow(seed), filler.Policy{}, nil)
		var spot, cut bool
		for _, e := range pod.Entries {
			spot = spot || e.Hash == "spot"
			cut = cut || e.Hash == "spot-cut"
		}
		if spot && cut {
			t.Fatalf("seed %d: pod %v carries the spot and its cut-down", seed, commercialIDs(pod))
		}
		if !spot && !cut {
			t.Fatalf("seed %d: pod %v dropped the ad entirely; one version should still air", seed, commercialIDs(pod))
		}
	}
}

// An airing of the spot rests its cut-down: with the spot seen ten minutes ago and the toy ad
// twenty, the cut-down is not "never aired" — the longer-rested toy ad goes first.
func TestAssemble_AVariantRestsWithTheAdItIsAVersionOf(t *testing.T) {
	at := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	cat := variantCatalog()[:3]
	w := kidsWindow(7)
	w.PodMax = 1
	w.SnapshotAt = at
	w.Exposures = map[string]filler.Exposure{
		"spot": {PlayCount: 1, LastPlayedAt: at.Add(-10 * time.Minute)},
		"toy":  {PlayCount: 1, LastPlayedAt: at.Add(-20 * time.Minute)},
	}
	pod := filler.Assemble(cat, w, filler.Policy{Cooldown: time.Hour}, nil)
	if got := commercialIDs(pod); len(got) != 1 || got[0] != "toy" {
		t.Fatalf("pod = %v, want the toy ad: the cut-down rests with the spot aired ten minutes ago", got)
	}
}
