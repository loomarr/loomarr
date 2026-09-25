package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/testkit"
)

type lifetimeMeter struct{}

func (lifetimeMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return storagegovernor.Measurement{ID: "lifetime", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 100 * storagegovernor.GiB}, nil
}

func (lifetimeMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	return 0, nil
}

type lifetimePackager struct {
	started chan struct{}
	finish  chan struct{}
}

func (p lifetimePackager) Package(
	ctx context.Context, workspace string, _ prepared.Input, _ int, _ prepared.RenditionContract,
) (prepared.Output, error) {
	p.started <- struct{}{}
	select {
	case <-ctx.Done():
		return prepared.Output{}, ctx.Err()
	case <-p.finish:
	}
	if err := os.WriteFile(filepath.Join(workspace, "segment-000000.m4s"), []byte("media"), 0o600); err != nil {
		return prepared.Output{}, err
	}
	return prepared.Output{Files: []string{"segment-000000.m4s"}}, nil
}

// lifetimeStack is the production shape: the real Preparer (library, storage governor, packager)
// under the real Planner and the pool exactly as buildplayout wires it, with the planner's workers
// owned by a lifecycle that outlives every scheduler pass.
func lifetimeStack(t *testing.T) (planner *prepared.Planner, library *prepared.Library, root string,
	governor *storagegovernor.Governor, request prepared.Request, packager lifetimePackager,
) {
	t.Helper()
	root = t.TempDir()
	var err error
	library, err = prepared.NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	packager = lifetimePackager{started: make(chan struct{}, 4), finish: make(chan struct{})}
	governor = storagegovernor.New(lifetimeMeter{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB * 50}
	})
	preparer := prepared.NewPreparer(prepared.PreparerDependencies{
		Library: library, Packager: packager, Storage: governor,
		Access: &testkit.PreparedSourceAccess{Input: prepared.LocalInput("/media/movie.mkv")},
	})
	request = prepared.Request{
		Source: prepared.Source{ItemID: "item", SourceID: "source", Revision: "r1"},
		Rendition: prepared.RenditionContract{
			VideoCodec: "h264", AudioCodec: "aac", Width: 1280, Height: 720, FrameRate: 25,
			VideoBitrateKbps: 5000, AudioBitrateKbps: 160, SegmentDurationMS: 2000,
			PackagingVersion: prepared.CurrentPackagingVersion,
		},
		DurationMS: int64(time.Hour / time.Millisecond),
	}
	pool := newPreparedEncodePool(
		func() playout.Encoder { return playout.EncoderNVENC },
		func() int { return 12 },
		func(measured int) int { return playout.EffectiveCapacity(measured, 4, 0) },
	)
	planner = prepared.NewPlanner(prepared.PlannerDependencies{
		Resolver: wiringResolver{plan: prepared.ReadinessPlan{
			Candidates: []prepared.Candidate{{Class: prepared.CandidateCurrent, Request: request}},
			Summary:    prepared.ReadinessSummary{ScheduledBindings: 1, MissingBindings: 1, QueuedPublications: 1},
		}},
		Preparation: preparer, Pool: pool, Lifecycle: t.Context(),
		Now: func() time.Time { return time.Unix(1_000, 0) },
	})
	return planner, library, root, governor, request, packager
}

// #1469: the scheduler's 30-minute LongJobTimeout killed every in-flight encode and discarded its
// staging, so a programme needing longer than that on a cold store could never publish.
func TestPreparedPublicationLongerThanTheJobTimeoutStillPublishes(t *testing.T) {
	planner, library, _, _, request, packager := lifetimeStack(t)

	passCtx, cancelPass := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancelPass()
	if err := planner.Run(passCtx); err != nil {
		t.Fatalf("pass: %v", err)
	}
	<-packager.started
	<-passCtx.Done() // the job timeout fires while the encode is still running

	time.Sleep(100 * time.Millisecond)
	close(packager.finish) // the encode finishes long after the job ceiling
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelWait()
	if err := planner.Wait(waitCtx); err != nil {
		t.Fatalf("workers did not drain: %v", err)
	}
	spec, ok, err := prepared.NewPreparer(prepared.PreparerDependencies{Library: library}).Lookup(request)
	if err != nil || !ok {
		t.Fatalf("publication longer than the job timeout was not published (ready=%v err=%v spec=%+v)", ok, err, spec)
	}
}

func TestPreparedWorkerKeepsItsStorageReservationAfterThePassReturns(t *testing.T) {
	planner, _, root, governor, _, packager := lifetimeStack(t)
	if err := planner.Run(t.Context()); err != nil {
		t.Fatalf("pass: %v", err)
	}
	<-packager.started
	held := governor.Snapshot(t.Context(), root).Snapshot.FilesystemReservedBytes
	if held == 0 {
		t.Fatal("a running worker's storage reservation was released when its pass returned")
	}
	close(packager.finish)
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelWait()
	if err := planner.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if left := governor.Snapshot(t.Context(), root).Snapshot.FilesystemReservedBytes; left != 0 {
		t.Fatalf("reservation not released when the worker finished: %d bytes", left)
	}
}
