package upkeep

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/quarantine"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

func TestReadyFetchedClipsReleaseTheirOriginalAndNothingElseDoes(t *testing.T) {
	f := newFixture(t)
	fetched := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	byHand := f.ready(t, hashOf("by hand"), "", masterRel("by-hand"))
	pending := f.ready(t, hashOf("awaiting re-encode"), "run-1", masterRel("pending"))
	cutA := f.ready(t, hashOf("cut a"), "run-1", masterRel("reel"))
	f.reject(t, hashOf("held cut"), "run-1", masterRel("reel"), t0) // still held, names the same original
	provenance := f.write(t, tools.SidecarPathFor(masterRel("fetched")))
	now := func() time.Time { return t0 }

	job := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).
		WithReencode(func(c clipstore.Clip) bool { return c.Hash == hashOf("awaiting re-encode") }, settled)
	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Originals != 2 {
		t.Fatalf("Run = %+v, want 2 originals released (the fetched clip and one cut of the reel)", res)
	}

	if m := sidecarMaster(t, fetched); !m.Released() || m.SHA256 == "" || m.Bytes == 0 {
		t.Fatalf("fetched clip's master = %+v, want released with its digest and size kept", m)
	}
	if exists(filepath.Join(f.dir, masterRel("fetched"))) {
		t.Error("the fetched clip's original is still in place")
	}
	held := filepath.Join(f.dir, quarantine.DirName, "1900000000", filepath.FromSlash(masterRel("fetched")))
	if !exists(held) {
		t.Errorf("the fetched clip's original is not in quarantine at %s", held)
	}
	if exists(provenance) {
		t.Error("the provenance sidecar beside the released original was left behind, named by nothing")
	}
	for name, full := range map[string]string{"hand-added": byHand, "awaiting re-encode": pending} {
		if sidecarMaster(t, full).Released() {
			t.Errorf("the %s clip's original was released", name)
		}
	}
	for _, kept := range []string{masterRel("by-hand"), masterRel("pending"), masterRel("reel")} {
		if !exists(filepath.Join(f.dir, kept)) {
			t.Errorf("%s was moved, want it kept", kept)
		}
	}
	if !sidecarMaster(t, cutA).Released() {
		t.Error("a Ready cut's sidecar still names an original another clip keeps")
	}
}

// The original goes to quarantine before the sidecar stops naming it. A crash between the two
// leaves a sidecar naming a file that is no longer there; the next pass finishes the release.
func TestAReleaseInterruptedAfterTheMoveIsFinishedByTheNextPass(t *testing.T) {
	f := newFixture(t)
	full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	if err := os.Remove(filepath.Join(f.dir, masterRel("fetched"))); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return t0 }
	res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Originals != 1 || !sidecarMaster(t, full).Released() {
		t.Fatalf("Run = %+v, want the sidecar marked released", res)
	}
}

func settled() bool { return true }

// Until encoder detection settles every host reads as software, where the library re-encode
// needs no original; a pass then releases nothing.
func TestNoOriginalIsReleasedBeforeTheEncoderIsKnown(t *testing.T) {
	f := newFixture(t)
	full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	now := func() time.Time { return t0 }
	known := false
	job := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).
		WithReencode(func(clipstore.Clip) bool { return false }, func() bool { return known })

	if res, err := job.Run(context.Background()); err != nil || res.Originals != 0 || sidecarMaster(t, full).Released() {
		t.Fatalf("Run = %+v, %v; want the original kept while the encoder is unknown", res, err)
	}
	known = true
	if res, err := job.Run(context.Background()); err != nil || res.Originals != 1 {
		t.Fatalf("Run = %+v, %v; want the original released once the encoder is known", res, err)
	}
}

// heldOnReread is a store whose clip a rewind holds after the pass listed it as Ready.
type heldOnReread struct{ *fakeStore }

func (s heldOnReread) GetClip(ctx context.Context, hash string) (clipstore.Clip, bool, error) {
	c, ok, err := s.fakeStore.GetClip(ctx, hash)
	c.Held = true
	return c, ok, err
}

func TestAClipHeldAfterTheListingKeepsItsOriginal(t *testing.T) {
	f := newFixture(t)
	full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	now := func() time.Time { return t0 }
	res, err := New(heldOnReread{f.store}, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Originals != 0 || sidecarMaster(t, full).Released() || !exists(filepath.Join(f.dir, masterRel("fetched"))) {
		t.Fatalf("Run = %+v, want a clip going back on the conveyor to keep its original", res)
	}
}

// The original is the only way back to a clip whose airing copy is damaged, so it goes only once
// that copy is the file its sidecar records and intake has finished with the download.
func TestAnOriginalStaysWhileItsAiringCopyOrAcquisitionIsNotSound(t *testing.T) {
	cases := map[string]func(t *testing.T, f *fixture, full string){
		"broken airing copy": func(t *testing.T, f *fixture, full string) {
			if err := os.WriteFile(full, []byte("damaged bytes, not the ones recorded"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"missing airing copy": func(t *testing.T, f *fixture, full string) {
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
		},
		"acquisition in repair": func(t *testing.T, f *fixture, _ string) {
			a := f.store.artifacts[hashOf("fetched")]
			a.State, a.RepairReason = acquisition.ArtifactRepair, acquisition.ArtifactMediaMissing
			f.store.artifacts[hashOf("fetched")] = a
		},
		"no acquisition record": func(t *testing.T, f *fixture, _ string) {
			delete(f.store.artifacts, hashOf("fetched"))
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
			damage(t, f, full)
			now := func() time.Time { return t0 }
			res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if res.Originals != 0 || !exists(filepath.Join(f.dir, masterRel("fetched"))) {
				t.Fatalf("Run = %+v, want the original kept", res)
			}
			if tags, ok := asset.ReadSidecarTags(full); ok && tags.MediaAssets.SourceMaster.Released() {
				t.Fatal("the sidecar records the original released")
			}
		})
	}
}

// A clip kept for a broken copy is looked at again: once the copy is sound, the original goes.
func TestAnOriginalKeptForABrokenCopyIsReleasedOnceTheCopyIsSound(t *testing.T) {
	f := newFixture(t)
	full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	sound, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return t0 }
	job := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil)
	if res, err := job.Run(context.Background()); err != nil || res.Originals != 0 {
		t.Fatalf("Run = %+v, %v; want the original kept while the copy is broken", res, err)
	}
	if err := os.WriteFile(full, sound, 0o644); err != nil {
		t.Fatal(err)
	}
	if res, err := job.Run(context.Background()); err != nil || res.Originals != 1 {
		t.Fatalf("Run = %+v, %v; want the original released once the copy is sound", res, err)
	}
}

func TestAReleaseWhoseMoveFailsKeepsTheSidecarNamingTheOriginal(t *testing.T) {
	f := newFixture(t)
	full := f.ready(t, hashOf("fetched"), "run-1", masterRel("fetched"))
	now := func() time.Time { return t0 }
	res, err := New(f.store, failingBin{quarantine.New(f.dir, now)}, nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Originals != 0 || sidecarMaster(t, full).Released() || !exists(filepath.Join(f.dir, masterRel("fetched"))) {
		t.Fatalf("Run = %+v, want the original and its sidecar entry kept when the move fails", res)
	}
}
