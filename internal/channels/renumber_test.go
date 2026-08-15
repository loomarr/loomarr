package channels_test

import (
	"context"
	"sync"
	"testing"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// ⚠ **Tunarr refuses a duplicate channel number, and it says so as `500` with an EMPTY BODY**
// (§9 V54). Found on a real install: an approved channel was given number 1 because
// `nextFreeChannelNumber` consulted only Loomarr's own store, Tunarr already had a channel there
// from an earlier install, and every reconcile retried a create that could never succeed. The
// operator's only symptom was a channel stuck on "Creating".
//
// Loomarr moves ITSELF rather than touching the occupant: §9's "channels Loomarr didn't create are
// never touched" is the rule this is shaped around — after a database reset Loomarr genuinely
// cannot tell its own orphan from a stranger's channel, so it must assume stranger.
func TestReconcile_MovesAroundANumberTunarrAlreadyUses(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	// A channel Loomarr did NOT create, sitting on number 1 — the state a reset database or an
	// earlier install leaves behind.
	tun.SeedForeignChannel(1, "Someone else's channel")

	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	e := newEngine(st, tun, avail, nil)

	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:1"), Title: "A", DurationMs: 3600000},
	}}
	ch.ID = "c1"
	ch.Name = "Springfield Classics"
	ch.Number = 1 // collides
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ctx := context.Background()
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	if err := e.Reconcile(ctx, "c1"); err != nil {
		t.Fatalf("reconcile: %v — a taken number must not fail the channel, it must move it", err)
	}

	got, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	// It went live...
	if got.TunarrID == "" {
		t.Fatal("no Tunarr id — the channel never got created")
	}
	// ...on a DIFFERENT number...
	if got.Number == 1 {
		t.Error("still on number 1, which Tunarr already uses — the create cannot have succeeded")
	}
	// ...and Loomarr's row agrees with Tunarr about which number that is. A renumber that only
	// happened in the push would leave the guide and the XMLTV pointing at the wrong channel.
	actual, ok, err := tun.GetChannel(ctx, got.TunarrID)
	if err != nil || !ok {
		t.Fatalf("Tunarr channel: ok=%v err=%v", ok, err)
	}
	if actual.Number != got.Number {
		t.Errorf("Tunarr says number %d, Loomarr's row says %d — the two disagree about where the channel is",
			actual.Number, got.Number)
	}

	// ⚠ And the occupant is untouched — same id, same number, same name. This is the assertion
	// that keeps the fix on the right side of "channels Loomarr didn't create are never touched".
	all, err := tun.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var foreign int
	for _, c := range all {
		if c.Name == "Someone else's channel" {
			foreign++
			if c.Number != 1 {
				t.Errorf("the foreign channel moved to %d — Loomarr must never renumber a channel it did not create", c.Number)
			}
		}
	}
	if foreign != 1 {
		t.Errorf("found %d foreign channels, want exactly 1 (untouched)", foreign)
	}
}

func TestReconcile_AutoRenumberAlsoAvoidsLocalOnlyChannels(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	tun.SeedForeignChannel(1, "Remote occupant")
	seedChannel(t, st, "local-only", 2)
	seedChannel(t, st, "target", 1, entry("movie:tmdb:1", "A"))
	e := newEngine(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil)

	if err := e.Reconcile(context.Background(), "target"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetChannel(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 3 {
		t.Fatalf("auto-renumber chose %d, want 3 (Tunarr owns 1 and local-only owns 2)", got.Number)
	}
	if tun.Creates != 1 || tun.Deletes != 0 {
		t.Fatalf("remote creates/deletes = %d/%d, want 1/0", tun.Creates, tun.Deletes)
	}
}

func TestReconcile_CleansUpWhenLocalNumberIsClaimedAfterRemotePlan(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	tun.SeedForeignChannel(1, "Remote occupant")
	seedChannel(t, st, "target", 1, entry("movie:tmdb:1", "A"))
	e := newEngine(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil)

	var once sync.Once
	tun.BeforeEnsureChannel = func(spec programmer.ChannelSpec) {
		if spec.TunarrID == "" && spec.Number == 2 {
			once.Do(func() { seedChannel(t, st, "late-local", 2) })
		}
	}
	if err := e.Reconcile(context.Background(), "target"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetChannel(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 3 {
		t.Fatalf("retry chose %d, want 3 after local channel claimed 2", got.Number)
	}
	if tun.Creates != 2 || tun.Deletes != 1 {
		t.Fatalf("remote creates/deletes = %d/%d, want 2/1 cleanup", tun.Creates, tun.Deletes)
	}
}

// The ordinary case must not pay for the collision handling: a free number is used as-is.
func TestReconcile_KeepsItsNumberWhenTunarrHasNothingThere(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	tun.SeedForeignChannel(7, "Unrelated")

	e := newEngine(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil)
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:1"), Title: "A", DurationMs: 3600000},
	}}
	ch.ID = "c1"
	ch.Name = "Mine"
	ch.Number = 3
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ctx := context.Background()
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	if err := e.Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetChannel(ctx, "c1")
	if got.Number != 3 {
		t.Errorf("number = %d, want 3 — an uncontested number must be left alone", got.Number)
	}
}
