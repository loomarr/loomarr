package channels_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// The accepted window does not move when library facts change (#4140 step 1). On the install a
// series being rewritten through the day (remux upgrades adding a second item for an episode,
// runtimes re-read a few milliseconds apart) re-dealt the window on air at every reconcile: the
// programme on air changed under the viewer, and each reconcile recorded the new arrangement from
// the anchor to now, so the ledger marked most of the pool as aired. Inside a window a reconcile
// only heals an item that has vanished; a fact that leaves every programme airable moves nothing.
func TestReconcile_LibraryFactsDoNotMoveTheWindowOnAir(t *testing.T) {
	const show = provision.Key("series:tvdb:90000601")
	for _, tc := range []struct {
		name string
		edit func(avail runtimeAvail, onAir []schedule.Slot)
	}{
		{"an episode's runtime is read 25 ms longer", func(avail runtimeAvail, window []schedule.Slot) {
			later := programIDs(window)[len(programIDs(window))-1]
			for i, p := range avail.series[show] {
				if p.LibraryItemID == later {
					avail.series[show][i].DurationMs += 25
				}
			}
		}},
		{"a remux upgrade adds a second item for an episode", func(avail runtimeAvail, _ []schedule.Slot) {
			dup := avail.series[show][9]
			dup.LibraryItemID += "-remux"
			avail.series[show] = append(avail.series[show], dup)
		}},
		{"an episode joins the show", func(avail runtimeAvail, _ []schedule.Slot) {
			avail.series[show] = append(avail.series[show], schedule.ResolvedProgram{
				LibraryItemID: "show1-e61", DurationMs: (30 * time.Minute).Milliseconds(), Season: 1, Episode: 61,
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, avail := twoRotatingShows(show)
			st := newStore(t)
			now := time.Unix(1_800_000_000, 0).UTC() // 08:00 UTC, mid-window
			e := recencyEngine(st, avail, &now)
			seedShuffleChannel(t, st, "wh", entries)

			before := reconcileDesired(t, e, st, "wh")
			tc.edit(avail, before.Desired)
			now = now.Add(2 * time.Hour) // a later sweep, same window
			after := reconcileDesired(t, e, st, "wh")

			if !after.PlayoutAnchor.Equal(before.PlayoutAnchor) {
				t.Fatalf("fixture: the window turned (%v -> %v)", before.PlayoutAnchor, after.PlayoutAnchor)
			}
			end := before.PlayoutAnchor.Add(24 * time.Hour)
			was := programmes(before.Desired, before.PlayoutAnchor, end)
			is := programmes(after.Desired, after.PlayoutAnchor, end)
			if fmt.Sprint(was) != fmt.Sprint(is) {
				t.Errorf("the window on air moved:\n before %v\n after  %v", was, is)
			}

			// The ledger holds one row per programme that started on the arrangement on air, and
			// nothing the replaced arrangement would have aired. The sweep after the edit is the
			// one that records it, walking whatever cycle it finds accepted from the anchor.
			now = now.Add(15 * time.Minute)
			reconcileDesired(t, e, st, "wh")
			ledger, err := st.LastScheduledByChannel(context.Background(), "wh", now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			started := programmes(before.Desired, before.PlayoutAnchor, now.Add(time.Nanosecond))
			for _, b := range started {
				if at, ok := ledger[b.id]; !ok || !at.Equal(b.start) {
					t.Errorf("%s started at %v; the ledger has %v (present %v)", b.id, b.start, at, ok)
				}
			}
			if len(ledger) != len(started) {
				t.Errorf("the ledger holds %d programmes, want the %d that started: %v", len(ledger), len(started), ledger)
			}
		})
	}
}

// An operator's edit takes effect at once, from the programme after the one on air: what has aired
// and what is on air stay put (so the ledger keeps one row per programme started and the stream
// is not restarted), and the rest of the window is the new deal, without a programme that already
// aired today.
func TestReconcile_AnEditReDealsAfterTheProgrammeOnAir(t *testing.T) {
	entries, avail := films(20, 2*time.Hour) // a 40 h pool on a 24 h window
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC().Add(30 * time.Minute) // 08:30 UTC, mid-window
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "ed", entries)
	before := reconcileDesired(t, e, st, "ed")

	now = now.Add(5 * time.Hour)
	end := before.PlayoutAnchor.Add(24 * time.Hour)
	upcoming := programmes(before.Desired, before.PlayoutAnchor, end)
	var removed string
	for _, b := range upcoming {
		if b.start.After(now.Add(4 * time.Hour)) {
			removed = b.id
			break
		}
	}
	if removed == "" {
		t.Fatal("fixture: nothing airs late in the window")
	}
	ch, err := st.GetChannel(context.Background(), "ed")
	if err != nil {
		t.Fatal(err)
	}
	var kept []schedule.LineupEntry
	for _, entry := range ch.Lineup {
		if avail.movies[entry.Key].LibraryItemID != removed {
			kept = append(kept, entry)
		}
	}
	ch.Lineup = kept
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	after := reconcileDesired(t, e, st, "ed")

	for at := now; at.Before(now.Add(timeline.OnAirLookahead)); at = at.Add(time.Second) {
		if got, want := timeline.AiringAt(after.Desired, after.PlayoutAnchor, at), timeline.AiringAt(before.Desired, before.PlayoutAnchor, at); got != want {
			t.Fatalf("at +%v the edit changed what is on air: %+v, was %+v", at.Sub(now), got, want)
		}
	}
	aired := map[string]bool{}
	for _, b := range programmes(before.Desired, before.PlayoutAnchor, now.Add(time.Nanosecond)) {
		aired[b.id] = true
	}
	afterOnAir := programmes(after.Desired, after.PlayoutAnchor, end)
	for _, b := range afterOnAir {
		if b.id == removed {
			t.Errorf("%s was taken out of the lineup but still airs at %v", removed, b.start)
		}
		if aired[b.id] && b.start.After(now) {
			t.Errorf("%s aired earlier today and airs again at %v", b.id, b.start)
		}
	}

	now = now.Add(15 * time.Minute)
	reconcileDesired(t, e, st, "ed")
	ledger, err := st.LastScheduledByChannel(context.Background(), "ed", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != len(aired) {
		t.Errorf("the ledger holds %d programmes, want the %d that started: %v", len(ledger), len(aired), ledger)
	}
}

// A programme whose file goes away is healed where it stands: a re-identified film takes its new
// item, and one that is gone becomes a placeholder at its runtime. Nothing else in the window moves.
func TestReconcile_AVanishedProgrammeIsHealedInPlace(t *testing.T) {
	entries, avail := films(20, 2*time.Hour)
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "hl", entries)
	before := reconcileDesired(t, e, st, "hl")

	keyOf := map[string]provision.Key{}
	for k, p := range avail.movies {
		keyOf[p.LibraryItemID] = k
	}
	later := programIDs(before.Desired)
	reidentified, gone := later[len(later)-1], later[len(later)-2]
	p := avail.movies[keyOf[reidentified]]
	p.LibraryItemID = reidentified + "-new"
	avail.movies[keyOf[reidentified]] = p
	delete(avail.movies, keyOf[gone])

	now = now.Add(time.Hour)
	after := reconcileDesired(t, e, st, "hl")
	if len(after.Desired) != len(before.Desired) || !after.PlayoutAnchor.Equal(before.PlayoutAnchor) {
		t.Fatalf("the window was re-dealt: %d slots from %v, was %d from %v",
			len(after.Desired), after.PlayoutAnchor, len(before.Desired), before.PlayoutAnchor)
	}
	for i, s := range after.Desired {
		was := before.Desired[i]
		switch {
		case was.LibraryItemID == reidentified:
			if s.LibraryItemID != reidentified+"-new" || s.DurationMs != was.DurationMs {
				t.Errorf("re-identified film healed to %+v", s)
			}
		case was.LibraryItemID == gone:
			if s.Kind != schedule.SlotFiller || s.Key != was.Key || s.DurationMs != was.DurationMs {
				t.Errorf("a film that is gone healed to %+v, want a placeholder at its runtime", s)
			}
		case s != was:
			t.Errorf("slot %d moved: %+v, was %+v", i, s, was)
		}
	}

	// The film comes back (a mount that dropped out for one sweep): its slot airs it again.
	avail.movies[keyOf[gone]] = schedule.ResolvedProgram{LibraryItemID: gone, DurationMs: (2 * time.Hour).Milliseconds()}
	now = now.Add(15 * time.Minute)
	back := reconcileDesired(t, e, st, "hl")
	for i, s := range back.Desired {
		if was := before.Desired[i]; was.LibraryItemID == gone && s != was {
			t.Errorf("a film that came back airs as %+v, want its slot back: %+v", s, was)
		}
	}
}

// twoRotatingShows is two shows of 60 half-hour episodes: 60 h on a 24 h window, so the window is a
// rotating slice and a change in the pool can change which slice is dealt.
func twoRotatingShows(first provision.Key) ([]schedule.LineupEntry, runtimeAvail) {
	avail := runtimeAvail{series: map[provision.Key][]schedule.ResolvedProgram{}}
	var entries []schedule.LineupEntry
	for s, key := range []provision.Key{first, "series:tvdb:90000602"} {
		for ep := 1; ep <= 60; ep++ {
			avail.series[key] = append(avail.series[key], schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("show%d-e%d", s+1, ep), DurationMs: (30 * time.Minute).Milliseconds(),
				Season: 1, Episode: ep,
			})
		}
		entries = append(entries, schedule.LineupEntry{Key: key, Title: fmt.Sprintf("Show %d", s+1)})
	}
	return entries, avail
}

type started struct {
	id    string
	start time.Time
}

func (s started) String() string { return s.id + "@" + s.start.Format("15:04:05.000") }

// programmes is every programme the cycle, walked from its anchor, starts before `until`.
func programmes(slots []schedule.Slot, anchor, until time.Time) []started {
	var out []started
	for _, b := range timeline.BroadcastsBetween(slots, anchor, anchor, until) {
		if b.Kind == schedule.SlotProgram && b.LibraryItemID != "" && b.Start.Before(until) {
			out = append(out, started{b.LibraryItemID, b.Start})
		}
	}
	return out
}
