package channels_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

// A lineup title that cannot air is recorded on the channel with why, so the channel page can say
// so (#4140): on the install four lineup titles never aired in six days and nothing said why. The
// record follows the library: once the title arrives it is gone, and its arrival is a library fact,
// so the window on air does not move.
func TestReconcile_RecordsTheTitlesThatCannotAir(t *testing.T) {
	const show = provision.Key("series:tvdb:90000601")
	const missing = provision.Key("movie:tmdb:90000699")
	entries, avail := twoRotatingShows(show)
	entries = append(entries, schedule.LineupEntry{Key: missing, Title: "Missing Film"})
	avail.movies = map[provision.Key]schedule.ResolvedProgram{}
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "ua", entries)

	before := reconcileDesired(t, e, st, "ua")
	want := []schedule.UnairableTitle{{Key: missing, Title: "Missing Film", Reason: schedule.UnairableNotInLibrary}}
	if !slices.Equal(before.Policy.Unairable, want) {
		t.Fatalf("stored unairable %+v, want %+v", before.Policy.Unairable, want)
	}

	avail.movies[missing] = schedule.ResolvedProgram{LibraryItemID: "film-99", DurationMs: (95 * time.Minute).Milliseconds()}
	now = now.Add(2 * time.Hour)
	after := reconcileDesired(t, e, st, "ua")
	if len(after.Policy.Unairable) != 0 {
		t.Fatalf("the film arrived, still recorded as unairable: %+v", after.Policy.Unairable)
	}
	end := before.PlayoutAnchor.Add(24 * time.Hour)
	if was, is := programmes(before.Desired, before.PlayoutAnchor, end), programmes(after.Desired, after.PlayoutAnchor, end); fmt.Sprint(was) != fmt.Sprint(is) {
		t.Errorf("the film's arrival moved the window on air:\n before %v\n after  %v", was, is)
	}
}
