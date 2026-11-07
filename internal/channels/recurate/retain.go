package recurate

import (
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// staleCap is the age a title is credited when nothing says how long it has waited: it never
// aired and its AddedAt is unknown (a manual add, or an entry that predates AddedAt). It is the
// airing history's retention window (store airingRetention, 8 days): history older than that is
// pruned down to one row, so "never aired inside the window" is the longest the store can
// actually vouch for. Such a title reads as long without airing, so it is among the first to go.
const staleCap = 8 * 24 * time.Hour

// recencyScale is the age at which recency has halved. It equals staleCap so a title at the
// unknown-age cap retains exactly half the score of one that just aired.
const recencyScale = staleCap

// recency is how strongly a title's recent history argues for keeping it, in (0, 1]: 1 for a
// title that aired just now, 1/2 at recencyScale, falling towards 0 the longer it has gone
// without airing. The longest-without-airing title therefore has the lowest recency, and the
// lowest retain score at equal fit.
func recency(age time.Duration) float64 {
	if age < 0 {
		age = 0
	}
	return 1 / (1 + float64(age)/float64(recencyScale))
}

// idleFor is how long a lineup entry has gone without airing as of now: since its last airing;
// else, if it never aired, since it was added; else staleCap when neither is known.
func idleFor(e schedule.LineupEntry, lastAired time.Time, now time.Time) time.Duration {
	switch {
	case !lastAired.IsZero():
		return now.Sub(lastAired)
	case !e.AddedAt.IsZero():
		return now.Sub(e.AddedAt)
	default:
		return staleCap
	}
}

// retainScore is what the turnstile ranks a retirable title on: its fit to the channel's intent
// times its recency. The lowest score is retired first, so a poorly fitting title that has been
// off the air a long time goes before a well fitting one that aired yesterday.
func retainScore(e schedule.LineupEntry, lastAired time.Time, now time.Time) float64 {
	return e.EffectiveFit() * recency(idleFor(e, lastAired, now))
}
