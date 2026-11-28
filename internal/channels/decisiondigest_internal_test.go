package channels

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

func digestInputs() scheduleInputs {
	in := scheduleInputs{
		Lineup:   []schedule.LineupEntry{{Key: provision.Key("movie:tmdb:90000001"), Title: "A", DurationMs: 3_600_000}},
		Settings: scheduleSettings{BreakLength: time.Minute, Window: 4 * time.Hour},
	}
	in.Channel.Strategy = schedule.Sequential
	in.Channel.BreaksPerHour = 2
	return in
}

// What reconcile heals is not a decision, so it does not move the digest: otherwise every library
// change would read as an edit and re-deal the window on air (#4140). What reconcile finds lives in
// ReconcileFindings, which the digest's inputs do not carry.
func TestDecisionDigestIgnoresWhatReconcileHeals(t *testing.T) {
	want := decisionDigest(digestInputs())

	healed := digestInputs()
	healed.Lineup[0].OfficialRating = "PG"
	healed.Lineup[0].CollectionID = 90000010
	if got := decisionDigest(healed); got != want {
		t.Errorf("a healed rating and collection moved the digest: %s, want %s", got, want)
	}

	edited := digestInputs()
	edited.Channel.Strategy = schedule.Shuffle
	if decisionDigest(edited) == want {
		t.Error("a changed ordering strategy left the digest unchanged")
	}
}
