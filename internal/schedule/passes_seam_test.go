package schedule_test

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// #1965: with holiday episodes off, the shuffle dealer alone must keep its repeat spacing at every
// seam, whatever day the channel counts its passes from. A pool one to six windows wide is dealt by
// a chained walk (passes.go), and the walk's anchor used to be repaired against the pass before it
// as dealt, not as aired: on an 80-episode deck in a 24-hour window, 63-66 of 365 origins aired an
// episode back to back. A seeded sample of 120 origins of a year is walked over the four days around
// an ordinary March day for every deck shape the seam tests use; that crosses an anchor at some
// origin of every shape (all 365, measured: 70 short on 80 episodes a day, 21 on 9 an hour).
// Under -race the first 40 of the sample are walked; -args -holiday-sweep walks all 365 (#2007).
func TestDealer_RepeatSpacingHoldsForEveryDealOrigin(t *testing.T) {
	year := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	seam := day(time.March, 10, 0)
	origins := rand.New(rand.NewPCG(1965, 1939)).Perm(365)
	switch {
	case *holidaySweep:
	case sampleSweeps():
		origins = origins[:40]
	default:
		origins = origins[:120]
	}
	for _, pool := range []seamPool{{9, time.Hour}, {80, 6 * time.Hour}, {80, 12 * time.Hour}, {80, 24 * time.Hour}, {400, 24 * time.Hour}} {
		for _, order := range []schedule.OrderingMode{schedule.OrderShuffle, schedule.OrderSyndication} {
			t.Run(fmt.Sprintf("%d episodes/%s window/%s", pool.ordinary, pool.window, order), func(t *testing.T) {
				t.Parallel() // origins share one chain, so a case walks them in turn
				avail := seriesLike(pool.ordinary)
				policy := seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
				policy.Ordering = order
				spacing := repeatSpacingProgrammes(pool.ordinary, pool.window)
				short, worst, first := 0, spacing, ""
				for _, o := range origins {
					ch := channelWithWindow(pool.window, nil)
					ch.DealOrigin = year.AddDate(0, 0, o)
					if gap, at := closestRepeat(t, ch, avail, policy, seam); gap < spacing {
						short++
						if gap < worst {
							worst = gap
						}
						if first == "" {
							first = fmt.Sprintf("origin %s: %s", ch.DealOrigin.Format(time.DateOnly), at)
						}
					}
				}
				if short > 0 {
					t.Errorf("%d of %d origins air an episode sooner than %d programmes apart, worst %d (first: %s)", short, len(origins), spacing, worst, first)
				}
			})
		}
	}
}

// closestRepeat walks the windows of the two days either side of seam as playout airs them and
// returns the fewest programmes between two airings of one episode, with where it happened.
func closestRepeat(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, seam time.Time) (int, string) {
	t.Helper()
	at := map[string]int{}
	closest, where := int(^uint(0)>>1), ""
	n := 0
	for open := seam.Add(-48 * time.Hour); open.Before(seam.Add(48 * time.Hour)); open = open.Add(ch.DefaultWindow) {
		for _, a := range airedLooped(t, ch, avail, policy, open) {
			n++
			if k, ok := at[a.id]; ok && n-k < closest {
				closest, where = n-k, fmt.Sprintf("%s again at %s", a.id, a.start.Format(time.Stamp))
			}
			at[a.id] = n
		}
	}
	return closest, where
}
