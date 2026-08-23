package channels_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/channels"
)

// THE POINT OF THE PER-BREAK SEED: consecutive breaks must not replay the same adverts. The
// channel-only seed produced one pod for the whole channel, so every break on a channel aired
// an identical sequence — which real television does not do, and which `filler.Window` has
// always documented against ("the seed derives from channel + window start").
func TestPodSeedAt_DiffersPerBreak(t *testing.T) {
	const ch = "ch_springfield"
	seeds := map[int64]int64{}
	// Six half-hourly breaks across an evening.
	for i := int64(0); i < 6; i++ {
		start := 1785000000000 + i*30*60*1000
		s := channels.PodSeedAt(ch, start)
		if prev, dup := seeds[s]; dup {
			t.Errorf("breaks at %d and %d share seed %d — the same clips would air in both",
				prev, start, s)
		}
		seeds[s] = start
	}
}

// DETERMINISM IS THE OTHER HALF (§10/§19). The same break must rebuild identically, or two
// viewers tuning into one break mid-way would see different commercials — and the guide's
// hover card would promise clips that are not the ones playing.
func TestPodSeedAt_IsStableForTheSameBreak(t *testing.T) {
	const ch, start = "ch_springfield", int64(1785000000000)
	first := channels.PodSeedAt(ch, start)
	for i := 0; i < 50; i++ {
		if got := channels.PodSeedAt(ch, start); got != first {
			t.Fatalf("same break seeded %d then %d — pod assembly would not be reproducible",
				first, got)
		}
	}
}

// Two channels breaking at the SAME instant must still differ, or every channel would cut to
// the identical advert simultaneously.
func TestPodSeedAt_DiffersPerChannelAtTheSameInstant(t *testing.T) {
	const start = int64(1785000000000)
	a := channels.PodSeedAt("ch_springfield", start)
	b := channels.PodSeedAt("ch_startrek", start)
	if a == b {
		t.Error("two channels breaking at the same moment share a seed — every channel would " +
			"cut to the same advert at once")
	}
}

// The channel-only seed stays available and unchanged: BuildFillerList attaches a POOL to
// Tunarr (there is no break to seed from at attach time) and HasPool only asks whether
// anything exists at all.
func TestPodSeed_StillStableForTheChannelWidePool(t *testing.T) {
	first := channels.PodSeed("ch_springfield")
	if got := channels.PodSeed("ch_springfield"); got != first {
		t.Fatal("the channel-wide seed is no longer stable; the Tunarr filler-list attach " +
			"would stop being idempotent across reconciles")
	}
	if channels.PodSeed("ch_springfield") == channels.PodSeed("ch_startrek") {
		t.Error("two channels share the channel-wide pool seed")
	}
}
