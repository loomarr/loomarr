package upkeep

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/quarantine"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// budgetGovernor is the real governor over the clip folder with a library allowance of budget
// bytes, as an install whose operator set filler.storage.library_budget_gb.
func budgetGovernor(dir string, budget int64) *storagegovernor.Governor {
	return storagegovernor.NewFilesystemLive(
		func() []storagegovernor.ManagedRoot {
			return []storagegovernor.ManagedRoot{{Domain: storagegovernor.DomainFiller, Path: dir}}
		},
		func(storagegovernor.Domain) storagegovernor.Policy {
			return storagegovernor.Policy{SoftBudgetBytes: budget}
		})
}

// fill rewrites a clip's playback file to n bytes.
func (f *fixture) fill(t *testing.T, playback string, n int) string {
	t.Helper()
	f.writeContent(t, strings.TrimPrefix(strings.TrimPrefix(playback, f.dir), "/"), strings.Repeat("x", n))
	return playback
}

// A full library budget stops every fetch with library_limit. A fetched clip rejected two days ago
// is still on disk, and filler must get its room back from it without waiting out its week.
func TestAFullBudgetDoesNotStopFiller(t *testing.T) {
	const budget, clipBytes, ask = 10_000, 4_000, 3_000
	f := newFixture(t)
	airing := f.fill(t, f.ready(t, hashOf("airing"), "run-1", ""), clipBytes)
	f.fill(t, f.reject(t, hashOf("two days"), "run-2", "", t0.Add(-48*time.Hour)), clipBytes)
	f.fill(t, f.reject(t, hashOf("yesterday"), "run-3", "", t0.Add(-24*time.Hour)), clipBytes)
	ctx := context.Background()

	if lease, d := budgetGovernor(f.dir, budget).Reserve(ctx, storagegovernor.Request{
		Path: f.dir, EstimatedBytes: ask}); lease != nil || d.Snapshot.Reason != storagegovernor.ReasonLibraryLimit {
		t.Fatalf("setup: reserve = %v %+v, want a library_limit refusal", lease, d)
	}

	// A day of hourly passes: the first sets the rejects aside, the quarantine empties a day later.
	// Each pass reads the budget as the next fetch would, which asks again and measures the folder.
	clock := t0
	now := func() time.Time { return clock }
	job := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).WithRoom(Room{
		Shortfall: func(ctx context.Context) int64 {
			g := budgetGovernor(f.dir, budget)
			if lease, _ := g.Reserve(ctx, storagegovernor.Request{Path: f.dir, EstimatedBytes: ask}); lease != nil {
				lease.Release()
				return 0
			}
			return g.LibraryShortfall(ctx, f.dir, storagegovernor.DomainFiller)
		},
		Standing: func(context.Context) (Standing, error) { return Standing{}, nil },
		InBreak:  func(context.Context) (map[string]bool, error) { return nil, nil },
	})
	for range 26 {
		if _, err := job.Run(ctx); err != nil {
			t.Fatal(err)
		}
		clock = clock.Add(time.Hour)
	}

	// A fetch that comes after: the governor measures the folder afresh.
	if lease, d := budgetGovernor(f.dir, budget).Reserve(ctx, storagegovernor.Request{
		Path: f.dir, EstimatedBytes: ask}); lease == nil {
		t.Fatalf("a day after the refusal filler is still stopped: %+v", d.Snapshot)
	}
	if !exists(airing) {
		t.Fatal("room was made by evicting the clip on air, which no channel was done with")
	}
}

// roomFixture is a library with something to evict at every rung of the order. Every clip holds
// 1000 bytes of media.
type roomFixture struct {
	*fixture
	files    map[string]string // name -> playback file
	clipSize int64             // a clip's media and sidecar together
	idle     []string          // Standing.Idle
	surplus  []string          // Standing.Surplus
	inBreak  map[string]bool
}

func newRoomFixture(t *testing.T) *roomFixture {
	t.Helper()
	r := &roomFixture{fixture: newFixture(t), files: map[string]string{}, inBreak: map[string]bool{}}
	add := func(name string, full string) { r.files[name] = r.fill(t, full, 1000) }
	// Fetched rejects, not yet a week old, and one added by hand that is older than both.
	add("reject, older", r.reject(t, hashOf("reject, older"), "run-1", "", t0.Add(-3*24*time.Hour)))
	add("reject, newer", r.reject(t, hashOf("reject, newer"), "run-1", "", t0.Add(-24*time.Hour)))
	add("reject by hand", r.reject(t, hashOf("reject by hand"), "", "", t0.Add(-5*24*time.Hour)))
	// Fetched clips on air that no channel can use, most aired first.
	add("idle, most aired", r.ready(t, hashOf("idle, most aired"), "run-2", ""))
	add("idle, less aired", r.ready(t, hashOf("idle, less aired"), "run-2", ""))
	// Fetched clips on air that no channel needs for its evening, most aired first.
	add("surplus, most aired", r.ready(t, hashOf("surplus, most aired"), "run-3", ""))
	add("surplus, less aired", r.ready(t, hashOf("surplus, less aired"), "run-3", ""))
	// Clips that look evictable and must never be.
	add("hand-added, idle", r.ready(t, hashOf("hand-added, idle"), "", ""))
	add("pinned, surplus", r.ready(t, hashOf("pinned, surplus"), "run-4", ""))
	add("in a break, surplus", r.ready(t, hashOf("in a break, surplus"), "run-4", ""))
	add("needed", r.ready(t, hashOf("needed"), "run-5", ""))
	r.store.pinned[hashOf("pinned, surplus")] = true
	r.inBreak[hashOf("in a break, surplus")] = true
	r.idle = []string{hashOf("hand-added, idle"), hashOf("idle, most aired"), hashOf("idle, less aired")}
	r.surplus = []string{hashOf("pinned, surplus"), hashOf("in a break, surplus"), hashOf("surplus, most aired"), hashOf("surplus, less aired")}
	media, err := os.Stat(r.files["needed"])
	if err != nil {
		t.Fatal(err)
	}
	side, err := os.Stat(tools.SidecarPathFor(r.files["needed"]))
	if err != nil {
		t.Fatal(err)
	}
	r.clipSize = media.Size() + side.Size()
	return r
}

// job is a pass that finds the budget short by short bytes.
func (r *roomFixture) job(short int64, now func() time.Time) *Job {
	return New(r.store, quarantine.New(r.dir, now), nil, r.dir, now, nil).WithRoom(Room{
		Shortfall: func(context.Context) int64 { return short },
		Standing:  func(context.Context) (Standing, error) { return Standing{Idle: r.idle, Surplus: r.surplus}, nil },
		InBreak:   func(context.Context) (map[string]bool, error) { return r.inBreak, nil },
	})
}

// gone is the clips whose playback file has left the clip folder.
func (r *roomFixture) gone() map[string]bool {
	out := map[string]bool{}
	for name, file := range r.files {
		if !exists(file) {
			out[name] = true
		}
	}
	return out
}

// A refused fetch frees space from leftovers, then rejects, then clips that fit no channel, then
// surplus beyond the evening target, most aired first; each rung only when the one before is not
// enough, and never a clip a channel needs, one in a coming break, a hand-added or a pinned clip.
func TestRoomIsMadeInOrderAndStopsWhenItIsEnough(t *testing.T) {
	order := []string{"reject, older", "reject, newer", "idle, most aired", "idle, less aired", "surplus, most aired", "surplus, less aired"}
	now := func() time.Time { return t0 }
	for n := 1; n <= len(order); n++ {
		t.Run(order[n-1], func(t *testing.T) {
			r := newRoomFixture(t)
			// One byte past what n-1 clips free, so exactly n clips are needed.
			res, err := r.job(int64(n-1)*r.clipSize+1, now).Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{}
			for _, name := range order[:n] {
				want[name] = true
			}
			got := r.gone()
			for name := range want {
				if !got[name] {
					t.Errorf("%q is still in place, want it evicted", name)
				}
			}
			for name := range got {
				if !want[name] {
					t.Errorf("%q was evicted, want it kept", name)
				}
			}
			if res.Evicted != n || res.EvictedBytes != int64(n)*r.clipSize {
				t.Errorf("Run = %+v, want %d clips and %d bytes evicted", res, n, int64(n)*r.clipSize)
			}
		})
	}
}

// Nothing on the list is permitted to go, however large the shortfall.
func TestNothingProtectedIsEvictedWhateverTheShortfall(t *testing.T) {
	r := newRoomFixture(t)
	now := func() time.Time { return t0 }
	if _, err := r.job(1<<40, now).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reject by hand", "hand-added, idle", "pinned, surplus", "in a break, surplus", "needed"} {
		if r.gone()[name] {
			t.Errorf("%q was evicted", name)
		}
	}
	if got := len(r.gone()); got != 6 {
		t.Errorf("%d clips evicted, want all six that may be", got)
	}
}

// Files let go wait a day in quarantine and still count against the budget, so a refusal that
// outlasts the pass that answered it must not evict a second time for the same room.
func TestRoomAlreadyOnItsWayIsNotAskedForTwice(t *testing.T) {
	r := newRoomFixture(t)
	clock := t0
	now := func() time.Time { return clock }
	short := r.clipSize // one clip's worth, still refused on every pass until the bytes are gone
	job := r.job(short, now)
	ctx := context.Background()

	if res, err := job.Run(ctx); err != nil || res.Evicted != 1 {
		t.Fatalf("first pass = %+v, %v; want one clip evicted", res, err)
	}
	clock = clock.Add(time.Hour)
	if res, err := job.Run(ctx); err != nil || res.Evicted != 0 {
		t.Fatalf("an hour later = %+v, %v; want the clip in quarantine to count as room on its way", res, err)
	}
	// The quarantine empties; the governor still counts the bytes until it measures again, so the
	// pass that deleted them has not seen the room yet either.
	clock = t0.Add(quarantine.Hold + time.Second)
	if res, err := job.Run(ctx); err != nil || res.Emptied != 1 || res.Evicted != 0 {
		t.Fatalf("the pass that emptied the quarantine = %+v, %v; want the bytes it deleted counted as room", res, err)
	}
	// Still refused after the governor measured: the room was not enough, and the next one goes.
	clock = clock.Add(time.Hour)
	if res, err := job.Run(ctx); err != nil || res.Evicted != 1 {
		t.Fatalf("a pass still refused with nothing on its way = %+v, %v; want one more clip evicted", res, err)
	}
}

// Without a refusal there is nothing to make room for: a library with surplus is left alone.
func TestNothingIsEvictedWithoutARefusal(t *testing.T) {
	r := newRoomFixture(t)
	now := func() time.Time { return t0 }
	if res, err := r.job(0, now).Run(context.Background()); err != nil || res.Evicted != 0 || len(r.gone()) != 0 {
		t.Fatalf("Run = %+v, %v, gone %v; want nothing evicted", res, err, r.gone())
	}
}

// A pass that cannot say how the channels stand, or which clips are in coming breaks, evicts no clip
// on air: it cannot tell the ones it may take from the ones it may not. Rejects go regardless.
func TestAClipOnAirIsOnlyEvictedWhenTheChannelsAreKnown(t *testing.T) {
	now := func() time.Time { return t0 }
	for name, tc := range map[string]func(*Room){
		"how the channels stand cannot be read": func(room *Room) {
			room.Standing = func(context.Context) (Standing, error) { return Standing{}, errors.New("channels unavailable") }
		},
		"the coming breaks cannot be read": func(room *Room) {
			room.InBreak = func(context.Context) (map[string]bool, error) { return nil, errors.New("timeline unavailable") }
		},
		"no timeline is wired": func(room *Room) { room.InBreak = nil },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRoomFixture(t)
			job := r.job(1<<40, now)
			tc(&job.room)
			_, err := job.Run(context.Background())
			if name != "no timeline is wired" && err == nil {
				t.Error("Run reported no error for a read that failed")
			}
			if got := r.gone(); len(got) != 2 || !got["reject, older"] || !got["reject, newer"] {
				t.Fatalf("evicted %v, want the two rejects only", got)
			}
		})
	}
}

// Crash injection at each step of an eviction. The retire commits before any file moves, so a crash
// leaves bytes on disk that the next pass moves, on every pass whether or not a fetch is refused.
func TestAnEvictionInterruptedAtAnyStepIsFinishedByTheNextPass(t *testing.T) {
	ctx := context.Background()
	now := func() time.Time { return t0 }
	master := masterRel("evicted")
	for name, tc := range map[string]struct {
		bin func(quarantine.Bin) Bin
		// stays is what the interrupted pass leaves in place.
		stays func(playback string) []string
	}{
		"before any file moves": {
			bin: func(b quarantine.Bin) Bin { return failingBin{b} },
			stays: func(playback string) []string {
				return []string{playback, tools.SidecarPathFor(playback)}
			},
		},
		"between the media and its sidecar": {
			bin:   func(b quarantine.Bin) Bin { return sidecarStuckBin{b} },
			stays: func(playback string) []string { return []string{tools.SidecarPathFor(playback)} },
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			playback := f.ready(t, hashOf("evicted"), "run-1", master)
			standing := Standing{Idle: []string{hashOf("evicted")}}
			room := Room{
				Shortfall: func(context.Context) int64 { return 1 },
				Standing:  func(context.Context) (Standing, error) { return standing, nil },
				InBreak:   func(context.Context) (map[string]bool, error) { return nil, nil },
			}
			bin := quarantine.New(f.dir, now)

			// The library re-encode still needs the original, so only the eviction moves it.
			keepOriginal := func(clipstore.Clip) bool { return true }
			encoderSettled := func() bool { return true }
			interrupted := New(f.store, tc.bin(bin), nil, f.dir, now, nil).WithReencode(keepOriginal, encoderSettled).WithRoom(room)
			if _, err := interrupted.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if f.store.retires != 1 {
				t.Fatalf("retires = %d after the interrupted pass, want the clip retired once", f.store.retires)
			}
			for _, file := range tc.stays(playback) {
				if !exists(file) {
					t.Fatalf("%s moved in the interrupted pass, want it left for the next", file)
				}
			}

			// A new process, and a fetch that is still refused: the retired clip is not retired again,
			// and the files it left behind are moved.
			next := New(f.store, bin, nil, f.dir, now, nil).WithReencode(keepOriginal, encoderSettled).WithRoom(room)
			res, err := next.Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if f.store.retires != 1 || res.Quarantined != 1 {
				t.Fatalf("next pass = %+v, retires = %d; want the files moved without retiring again", res, f.store.retires)
			}
			for _, file := range []string{playback, tools.SidecarPathFor(playback), filepath.Join(f.dir, master)} {
				if exists(file) {
					t.Errorf("%s is still in place after the next pass", file)
				}
			}
		})
	}
}

// The same crash, found by a process that has no refusal to answer: the files of a clip let go are
// moved on every pass, not only on one that needs room.
func TestAnEvictedClipsFilesAreMovedWithoutARefusal(t *testing.T) {
	f := newFixture(t)
	now := func() time.Time { return t0 }
	playback := f.ready(t, hashOf("evicted"), "run-1", "")
	row := f.store.rows[hashOf("evicted")]
	row.FilesDeletedAt = t0
	f.store.rows[hashOf("evicted")] = row

	res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Quarantined != 1 || exists(playback) || exists(tools.SidecarPathFor(playback)) {
		t.Fatalf("Run = %+v; want the let-go clip's files moved to quarantine", res)
	}
}
