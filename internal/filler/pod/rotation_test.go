package pod_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

func rotationCatalog() []pod.Clip {
	return []pod.Clip{
		{Hash: "a", Path: "a.mp4", Name: "A", Role: pod.Commercial, Era: 1994, Audience: pod.Kids, Category: "a", DurationMs: 30_000},
		{Hash: "b", Path: "b.mp4", Name: "B", Role: pod.Commercial, Era: 1994, Audience: pod.Kids, Category: "b", DurationMs: 30_000},
		{Hash: "c", Path: "c.mp4", Name: "C", Role: pod.Commercial, Era: 1994, Audience: pod.Kids, Category: "c", DurationMs: 30_000},
	}
}

func rotationWindow(at time.Time, max int) pod.Window {
	return pod.Window{
		ChannelID: "channel", Seed: 19, Era: pod.Year(1994), Audience: pod.Kids,
		GapMs: 180_000, PodMax: max, SnapshotAt: at,
	}
}

func commercialIDs(p pod.Pod) []string {
	var ids []string
	for _, e := range p.Entries {
		if e.Role == pod.Commercial {
			ids = append(ids, e.Hash)
		}
	}
	return ids
}

func TestRotation_NewThenLeastRecentlyPlayed(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	w := rotationWindow(now, 2)
	w.Exposures = map[string]pod.Exposure{
		"a": {PlayCount: 4, LastPlayedAt: now.Add(-2 * time.Hour)},
		"b": {PlayCount: 2, LastPlayedAt: now.Add(-3 * time.Hour)},
		// c is newly admitted and deliberately absent.
	}
	p := pod.Assemble(rotationCatalog(), w, pod.Policy{Cooldown: 30 * time.Minute}, nil)
	if got, want := commercialIDs(p), []string{"c", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rotation order = %v, want new then least-recent %v", got, want)
	}
	if p.CooldownRelaxed {
		t.Fatal("ready pool incorrectly reported cooldown pressure")
	}
}

func TestRotation_ConsecutiveBreakAvoidsPriorClip(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	cat := rotationCatalog()[:2]
	first := pod.Assemble(cat, rotationWindow(now, 1), pod.Policy{Cooldown: time.Hour}, nil)
	firstID := commercialIDs(first)[0]

	w := rotationWindow(now.Add(10*time.Minute), 1)
	w.Exposures = map[string]pod.Exposure{
		firstID: {PlayCount: 1, LastPlayedAt: now},
	}
	second := pod.Assemble(cat, w, pod.Policy{Cooldown: time.Hour}, nil)
	if got := commercialIDs(second)[0]; got == firstID {
		t.Fatalf("consecutive break repeated %q despite an unplayed alternative", got)
	}
}

func TestRotation_DepletedPoolRelaxesOldestWithoutDeadAir(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	w := rotationWindow(now, 1)
	w.Exposures = map[string]pod.Exposure{
		"a": {PlayCount: 3, LastPlayedAt: now.Add(-20 * time.Minute)},
		"b": {PlayCount: 3, LastPlayedAt: now.Add(-10 * time.Minute)},
		"c": {PlayCount: 3, LastPlayedAt: now.Add(-5 * time.Minute)},
	}
	policy := pod.Policy{Cooldown: time.Hour}
	p1 := pod.Assemble(rotationCatalog(), w, policy, nil)
	p2 := pod.Assemble(rotationCatalog(), w, policy, nil)
	if got := commercialIDs(p1); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("depleted rotation = %v, want oldest recent clip", got)
	}
	if !p1.CooldownRelaxed || !p1.Entries[0].RecentRepeat {
		t.Fatalf("depleted pod did not expose cooldown pressure: %+v", p1)
	}
	if !reflect.DeepEqual(p1, p2) {
		t.Fatal("same catalog, exposure snapshot, channel, and break was not deterministic")
	}
}

func TestRotation_PinIntentionallyOverridesCooldown(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	w := rotationWindow(now, 1)
	w.Pinned = []string{"b"}
	w.Exposures = map[string]pod.Exposure{
		"b": {PlayCount: 8, LastPlayedAt: now.Add(-time.Minute)},
	}
	p := pod.Assemble(rotationCatalog(), w, pod.Policy{Cooldown: time.Hour}, nil)
	if got := commercialIDs(p); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("pinned rotation = %v, want operator pin first", got)
	}
	if len(p.Entries) != 1 || !p.Entries[0].RecentRepeat || !p.Entries[0].RotationPinned {
		t.Fatalf("pin override metadata = %+v", p.Entries)
	}
	if p.CooldownRelaxed {
		t.Fatal("an intentional pin must not be reported as depleted-pool pressure")
	}
}

func TestRotation_ExactFitStillPlays(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	w := rotationWindow(now, 1)
	w.GapMs = 42_000 // 12s bumper reservation + one exact 30s commercial.
	w.Exposures = map[string]pod.Exposure{
		"a": {PlayCount: 1, LastPlayedAt: now.Add(-time.Minute)},
		"b": {PlayCount: 1, LastPlayedAt: now.Add(-2 * time.Minute)},
		"c": {PlayCount: 1, LastPlayedAt: now.Add(-3 * time.Minute)},
	}
	p := pod.Assemble(rotationCatalog(), w, pod.Policy{Cooldown: time.Hour}, nil)
	if got := commercialIDs(p); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("exact-fit depleted pool = %v, want oldest clip and no dead air", got)
	}
}
