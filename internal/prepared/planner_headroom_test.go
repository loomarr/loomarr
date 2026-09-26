package prepared

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// headroomMeter reports the store's real logical bytes as managed, the way the production meter
// does, so the governor refuses exactly when the library sits within one reservation of budget.
type headroomMeter struct{ root string }

func (headroomMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return storagegovernor.Measurement{
		ID: "prepared", TotalBytes: 1024 * storagegovernor.GiB, FreeBytes: 1000 * storagegovernor.GiB,
	}, nil
}

func (m headroomMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	var total int64
	err := filepath.WalkDir(m.root, func(_ string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // evicted mid-walk, as the production meter tolerates
		}
		if err != nil || entry.IsDir() {
			return err
		}
		info, infoErr := entry.Info()
		if errors.Is(infoErr, fs.ErrNotExist) {
			return nil
		}
		if infoErr == nil {
			total += info.Size()
		}
		return infoErr
	})
	return total, err
}

type smallPackager struct{}

func (smallPackager) Package(
	_ context.Context, workspace string, _ Input, _ int, _ RenditionContract,
) (Output, error) {
	files := map[string]string{
		"media.m3u8":  "#EXTM3U\n#EXTINF:2,\nsegment.m4s\n#EXT-X-ENDLIST\n",
		"segment.m4s": "media",
	}
	var names []string
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(body), 0o600); err != nil {
			return Output{}, err
		}
		names = append(names, name)
	}
	return Output{Files: names}, nil
}

type localAccess struct{}

func (localAccess) OpenInput(context.Context, Source) (Input, error) {
	return LocalInput("/media/source.mkv"), nil
}

// completingPreparation drops a candidate from the resolver once its publication is ready.
type completingPreparation struct {
	preparer *Preparer
	resolver *pendingCandidates
}

func (c completingPreparation) Prepare(ctx context.Context, request Request) (Publication, error) {
	publication, err := c.preparer.Prepare(ctx, request)
	if err == nil {
		c.resolver.complete(request)
	}
	return publication, err
}

type headroomStore struct {
	t         *testing.T
	root      string
	clock     *atomic.Int64
	library   *Library
	planner   *Planner
	resolver  *pendingCandidates
	budget    int64
	cold      []Publication
	coldSpecs []Specification
}

func (s *headroomStore) advance(d time.Duration) { s.clock.Add(int64(d)) }

func (s *headroomStore) pass() {
	s.t.Helper()
	if err := s.planner.Wait(s.t.Context()); err != nil {
		s.t.Fatal(err)
	}
	// Errors are the expected refusals while wedged; the readiness outcome is asserted instead.
	if err := s.planner.Run(s.t.Context()); err != nil {
		s.t.Log(err)
	}
	if err := s.planner.Wait(s.t.Context()); err != nil {
		s.t.Fatal(err)
	}
}

// newHeadroomStore builds the production shape from #1509: coldCount cold, unscheduled
// publications filling the store to just under its soft budget, plus `pending` scheduled bindings
// whose reservations no longer fit. The startup grace has already elapsed.
func newHeadroomStore(t *testing.T, coldCount, pending int) *headroomStore {
	t.Helper()
	clock := &atomic.Int64{}
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock.Store(start.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	root := t.TempDir()
	library, err := newLibrary(root, now)
	if err != nil {
		t.Fatal(err)
	}
	rendition := baselineRendition()
	reservation, ok := storagegovernor.EstimatePrepared(
		int64(time.Second/time.Millisecond), rendition.VideoBitrateKbps, rendition.AudioBitrateKbps,
	)
	if !ok {
		t.Fatal("reservation estimate unavailable")
	}
	// One cold publication is a reservation wide; the budget leaves less than one reservation free.
	coldSize := reservation
	budget := int64(coldCount)*coldSize + reservation/2
	store := &headroomStore{t: t, root: root, clock: clock, library: library, budget: budget}
	for i := range coldCount {
		pub := publishSparseSized(t, library, "cold-"+string(rune('a'+i)), coldSize)
		setPublicationTime(t, pub, now().Add(time.Duration(i)*time.Second))
		store.cold = append(store.cold, pub)
		store.coldSpecs = append(store.coldSpecs, baselineSpec("cold-"+string(rune('a'+i))))
	}
	store.advance(time.Hour) // past the startup grace and every recent-use grace

	governor := storagegovernor.New(headroomMeter{root: root}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: budget}
	})
	preparer := NewPreparer(PreparerDependencies{
		Library: library, Packager: smallPackager{}, Access: localAccess{}, Storage: governor,
	})
	items := make([]Candidate, pending)
	for i := range items {
		request := Request{
			Source:     testSource("scheduled-" + string(rune('a'+i))),
			Rendition:  rendition,
			DurationMS: int64(time.Second / time.Millisecond),
		}
		items[i] = Candidate{NeededAt: now().Add(time.Duration(i) * time.Minute), Request: request}
	}
	store.resolver = &pendingCandidates{items: items}
	store.planner = NewPlanner(PlannerDependencies{
		Resolver:    store.resolver,
		Preparation: completingPreparation{preparer: preparer, resolver: store.resolver},
		Pool:        media.NewEncodePool(func() int { return 2 }),
		Retainer:    library,
		BudgetBytes: func() int64 { return budget },
		Now:         now,
	})
	return store
}

func TestPlannerConvergesWhenColdPublicationsFillTheStoreToItsBudget(t *testing.T) {
	store := newHeadroomStore(t, 12, 4)

	for range 12 {
		store.pass()
		store.advance(time.Minute)
	}

	if remaining := store.resolver.remaining(); remaining != 0 {
		t.Fatalf("%d of 4 scheduled bindings still unpublished after 12 passes: store wedged at budget", remaining)
	}
}

// protectingResolver reports a fixed protected set alongside the pending candidates.
type protectingResolver struct {
	*pendingCandidates
	protected []Specification
}

func (r protectingResolver) Plan(ctx context.Context, from, to time.Time) (ReadinessPlan, error) {
	plan, err := r.pendingCandidates.Plan(ctx, from, to)
	plan.Protected = r.protected
	return plan, err
}

func (s *headroomStore) present(pub Publication) bool {
	_, err := os.Stat(pub.Directory)
	return err == nil
}

func (s *headroomStore) evicted() int {
	count := 0
	for _, pub := range s.cold {
		if !s.present(pub) {
			count++
		}
	}
	return count
}

func TestMakeRoomEvictionNeverTakesScheduledOrRecentlyUsedPublications(t *testing.T) {
	store := newHeadroomStore(t, 12, 4)
	scheduled, recent := store.cold[0], store.cold[1] // the two least recently used
	store.planner.resolver = protectingResolver{
		pendingCandidates: store.resolver, protected: []Specification{store.coldSpecs[0]},
	}
	if _, ok, err := store.library.Lookup(store.coldSpecs[1]); err != nil || !ok {
		t.Fatalf("touch recent publication = (_, %v, %v)", ok, err)
	}

	for range 12 {
		store.pass()
		store.advance(time.Second) // stays inside the 15-minute recent-use grace
	}

	if remaining := store.resolver.remaining(); remaining != 0 {
		t.Fatalf("%d scheduled bindings unpublished: protected media must not block convergence", remaining)
	}
	if !store.present(scheduled) || !store.present(recent) {
		t.Fatalf("make-room eviction removed protected media: scheduled=%v recent=%v",
			store.present(scheduled), store.present(recent))
	}
	if store.evicted() == 0 {
		t.Fatal("nothing was evicted, so the test did not exercise make-room eviction")
	}
}

func TestRetentionEvictsNothingWhenNoPublicationWasRefused(t *testing.T) {
	// Under budget with nothing pending: cold media stays put for future reuse.
	store := newHeadroomStore(t, 12, 0)
	for range 3 {
		store.pass()
	}
	if got := store.evicted(); got != 0 {
		t.Fatalf("%d cold publications evicted with no refused work", got)
	}
}

func TestMakeRoomEvictsOnlyAsMuchAsTheRefusedReservationsNeed(t *testing.T) {
	store := newHeadroomStore(t, 12, 1)
	for range 3 {
		store.pass()
	}
	if remaining := store.resolver.remaining(); remaining != 0 {
		t.Fatalf("%d bindings unpublished", remaining)
	}
	// Half a reservation was free; one reservation must be freed, not the whole cold set.
	if got := store.evicted(); got != 1 {
		t.Fatalf("%d cold publications evicted for one reservation, want exactly 1", got)
	}
}

func TestMakeRoomSkipsAReservationThatCanNeverFit(t *testing.T) {
	store := newHeadroomStore(t, 12, 1)
	store.resolver.items[0].Request.DurationMS = int64(2000 * time.Second / time.Millisecond)
	for range 3 {
		store.pass()
	}
	if got := store.evicted(); got != 0 {
		t.Fatalf("%d publications evicted for a reservation larger than the whole budget", got)
	}
}
