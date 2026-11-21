package filler_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// splitStoreFor is the store a case hands the splitter: the memory store, or a wrapper of it.
type splitStoreFor = filler.SplitStore

// catalogListFails is a split store whose catalog cannot be listed once armed (the fixture builder
// lists it while the splitter is being set up).
type catalogListFails struct {
	*splitMemStore
	armed *bool
}

func (s catalogListFails) ListClips(ctx context.Context) ([]clipstore.Clip, error) {
	if *s.armed {
		return nil, errors.New("catalog store unavailable")
	}
	return s.splitMemStore.ListClips(ctx)
}

// A segment whose duplicate check could not be made is not a segment that was checked and found
// new. The review must be able to tell the two apart, so every way the check can fail marks the
// segment, and every way it succeeds (a copy, a clear result, nothing to compare against) does not.
func TestPropose_DedupMarksSegmentsItCouldNotCheck(t *testing.T) {
	existingClip := func(st *splitMemStore, name string) clipstore.Clip {
		c := clipstore.Clip{}
		c.Hash, c.Path, c.DurationMs = "hash-of-"+name, "old/"+name, 30_000
		st.clips[c.Hash] = c
		return c
	}
	for name, tc := range map[string]struct {
		setup     func(st *splitMemStore) (splitStoreFor, *fakeTools)
		wantFlags []bool // DedupUnchecked per segment
		wantDup   []string
		wantWarn  bool
	}{
		"every segment reads and none is a copy": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(11), key3("clean.mp4", 30000, 60_000): spot(12),
					key3("clean.mp4", 0, 60_000): spot(13), key3("known.mp4", 0, 30_000): spot(14),
				}}
			},
			wantFlags: []bool{false, false}, wantDup: []string{"", ""},
		},
		"a copy is flagged as a copy, not as unchecked": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(21), key3("clean.mp4", 30000, 60_000): spot(22),
					key3("clean.mp4", 0, 60_000): spot(23), key3("known.mp4", 0, 30_000): spot(21),
				}}
			},
			wantFlags: []bool{false, false}, wantDup: []string{"old/known.mp4", ""},
		},
		"one segment cannot be fingerprinted": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000):  spot(31), // the second segment has no entry: it fails
					key3("clean.mp4", 0, 60_000): spot(33), key3("known.mp4", 0, 30_000): spot(34),
				}}
			},
			wantFlags: []bool{false, true}, wantDup: []string{"", ""}, wantWarn: true,
		},
		"every segment cannot be fingerprinted": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 60_000): spot(43), key3("known.mp4", 0, 30_000): spot(44),
				}}
			},
			wantFlags: []bool{true, true}, wantDup: []string{"", ""}, wantWarn: true,
		},
		"the catalog cannot be listed": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				return catalogListFails{st, new(bool)}, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(51), key3("clean.mp4", 30000, 60_000): spot(52),
					key3("clean.mp4", 0, 60_000): spot(53), key3("known.mp4", 0, 30_000): spot(51),
				}}
			},
			wantFlags: []bool{true, true}, wantDup: []string{"", ""}, wantWarn: true,
		},
		"every catalog clip is unreadable, so there is nothing to compare against": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				existingClip(st, "unreadable.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(81), key3("clean.mp4", 30000, 60_000): spot(82), key3("clean.mp4", 0, 60_000): spot(83),
					// neither catalog clip has an entry: their fingerprints fail
				}}
			},
			wantFlags: []bool{true, true}, wantDup: []string{"", ""}, wantWarn: true,
		},
		"there is nothing in the catalog to compare against": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(61), key3("clean.mp4", 30000, 60_000): spot(62), key3("clean.mp4", 0, 60_000): spot(63),
				}}
			},
			wantFlags: []bool{false, false}, wantDup: []string{"", ""},
		},
		"a catalog clip that cannot be read leaves every segment without a finding unchecked": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				existingClip(st, "unreadable.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(71), key3("clean.mp4", 30000, 60_000): spot(72),
					key3("clean.mp4", 0, 60_000): spot(73), key3("known.mp4", 0, 30_000): spot(71),
					// unreadable.mp4 has no entry: its fingerprint fails
				}}
			},
			// The copy found against the readable clip stands. The other segment was compared with
			// only part of the catalog, so it is not "checked and new".
			wantFlags: []bool{false, true}, wantDup: []string{"old/known.mp4", ""}, wantWarn: true,
		},
		"a catalog clip that cannot be read and no copy found marks every segment": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				existingClip(st, "unreadable.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(91), key3("clean.mp4", 30000, 60_000): spot(92),
					key3("clean.mp4", 0, 60_000): spot(93), key3("known.mp4", 0, 30_000): spot(94),
				}}
			},
			wantFlags: []bool{true, true}, wantDup: []string{"", ""}, wantWarn: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSplitMemStore()
			hash := seedCompilation(st, "comps/clean.mp4", 60_000)
			store, fake := tc.setup(st)
			var logs strings.Builder
			log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			splitter := newSplitterLogging(store, fake, nil, t.TempDir(), log)
			if failing, ok := store.(catalogListFails); ok {
				*failing.armed = true
			}
			p, err := splitter.Propose(context.Background(), hash)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Segments) != len(tc.wantFlags) {
				t.Fatalf("segments = %+v, want %d", p.Segments, len(tc.wantFlags))
			}
			for i, seg := range p.Segments {
				if seg.DedupUnchecked != tc.wantFlags[i] || seg.DupOf != tc.wantDup[i] {
					t.Errorf("segment %d: unchecked=%v dupOf=%q, want unchecked=%v dupOf=%q",
						i, seg.DedupUnchecked, seg.DupOf, tc.wantFlags[i], tc.wantDup[i])
				}
				if seg.DedupUnchecked && seg.DupOf != "" {
					t.Errorf("segment %d is both a copy and unchecked", i)
				}
			}
			if warned := strings.Contains(logs.String(), "level=WARN"); warned != tc.wantWarn {
				t.Errorf("WARN logged = %v, want %v; log:\n%s", warned, tc.wantWarn, logs.String())
			}
		})
	}
}

// uncheckedProposal proposes a two-segment reel whose second segment cannot be fingerprinted, with
// one readable clip in the catalog, and returns what a later pass needs to re-check it.
func uncheckedProposal(t *testing.T) (*splitMemStore, *fakeTools, *filler.Splitter, *filler.SplitProposal) {
	t.Helper()
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/clean.mp4", 60_000)
	known := clipstore.Clip{}
	known.Hash, known.Path, known.DurationMs = "hash-of-known.mp4", "old/known.mp4", 30_000
	st.clips[known.Hash] = known
	fake := &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
		key3("clean.mp4", 0, 30000): spot(11), key3("known.mp4", 0, 30_000): spot(14),
		// the second segment has no entry yet: its check cannot run
	}}
	splitter := newSplitter(st, fake, nil, t.TempDir())
	p, err := splitter.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 2 || p.Segments[0].DedupUnchecked || !p.Segments[1].DedupUnchecked {
		t.Fatalf("segments = %+v, want only the second unchecked", p.Segments)
	}
	return st, fake, splitter, p
}

// gateReady makes the proposal's segments pass every other part of the gate, so only the duplicate
// check decides.
func gateReady(p filler.SplitProposal) filler.SplitProposal {
	segs := append([]split.Segment(nil), p.Segments...)
	for i := range segs {
		segs[i].Era, segs[i].Audience, segs[i].Category = 1993, "kids", "toys"
		segs[i].BoundaryConfidence = 90
	}
	p.Segments = segs
	return p
}

// Auto-confirm holds an unchecked segment back and releases it on a later pass once the check works.
func TestRecheckDedup_ReleasesAHeldSegmentOnceTheCheckWorks(t *testing.T) {
	st, fake, splitter, p := uncheckedProposal(t)
	pol := policy(true, 85, 120*time.Second)

	first := filler.AutoConfirmable(gateReady(*p), pol, 0)
	if len(first.Confirm) != 1 || len(first.Hold) != 1 || first.Hold[0].HoldReason != string(filler.RejectDedupUnchecked) {
		t.Fatalf("first pass = %+v, want one confirmed and the unchecked one held", first)
	}

	// Tooling still down: the segment stays unchecked and the proposal is untouched.
	remaining, err := splitter.RecheckDedup(context.Background(), p)
	if err != nil || remaining != 1 || !p.Segments[1].DedupUnchecked {
		t.Fatalf("recheck while down = %d, %v, segments %+v", remaining, err, p.Segments)
	}

	// Tooling back: the segment is checked, the stored proposal agrees, and it now confirms.
	fake.spans[key3("clean.mp4", 30000, 60_000)] = spot(12)
	remaining, err = splitter.RecheckDedup(context.Background(), p)
	if err != nil || remaining != 0 {
		t.Fatalf("recheck once up = %d, %v", remaining, err)
	}
	stored, err := st.GetSplitProposal(context.Background(), p.ID)
	if err != nil || stored.Segments[1].DedupUnchecked {
		t.Fatalf("stored = %+v, %v; the check result was not persisted", stored.Segments, err)
	}
	second := filler.AutoConfirmable(gateReady(stored), pol, 0)
	if len(second.Confirm) != 2 || len(second.Hold) != 0 {
		t.Fatalf("second pass = %+v, want both confirmed", second)
	}
}

// A catalog clip that cannot be fingerprinted holds the segments it could not be compared with, and
// the re-check releases them once that clip reads. Until then a re-check changes nothing.
func TestRecheckDedup_ReleasesSegmentsHeldByAnUnreadableCatalogClip(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/clean.mp4", 60_000)
	for _, name := range []string{"known.mp4", "unreadable.mp4"} {
		c := clipstore.Clip{}
		c.Hash, c.Path, c.DurationMs = "hash-of-"+name, "old/"+name, 30_000
		st.clips[c.Hash] = c
	}
	fake := &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
		key3("clean.mp4", 0, 30000): spot(11), key3("clean.mp4", 30000, 60_000): spot(12),
		key3("clean.mp4", 0, 60_000): spot(13), key3("known.mp4", 0, 30_000): spot(14),
		// unreadable.mp4 has no entry yet
	}}
	splitter := newSplitter(st, fake, nil, t.TempDir())
	p, err := splitter.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	pol := policy(true, 85, 120*time.Second)
	first := filler.AutoConfirmable(gateReady(*p), pol, 0)
	if len(first.Confirm) != 0 || len(first.Hold) != 2 || first.Hold[0].HoldReason != string(filler.RejectDedupUnchecked) {
		t.Fatalf("first pass = %+v, want both segments held as unchecked", first)
	}

	remaining, err := splitter.RecheckDedup(context.Background(), p)
	if err != nil || remaining != 2 {
		t.Fatalf("recheck while the clip is unreadable = %d, %v; want both still unchecked", remaining, err)
	}

	fake.spans[key3("unreadable.mp4", 0, 30_000)] = spot(15)
	remaining, err = splitter.RecheckDedup(context.Background(), p)
	if err != nil || remaining != 0 {
		t.Fatalf("recheck once the clip reads = %d, %v", remaining, err)
	}
	stored, err := st.GetSplitProposal(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second := filler.AutoConfirmable(gateReady(stored), pol, 0); len(second.Confirm) != 2 || len(second.Hold) != 0 {
		t.Fatalf("second pass = %+v, want both confirmed", second)
	}
}

// A re-check that finds a copy records it as a copy, which the stage then discards.
func TestRecheckDedup_AFoundCopyIsRecordedAsACopy(t *testing.T) {
	_, fake, splitter, p := uncheckedProposal(t)
	fake.spans[key3("clean.mp4", 30000, 60_000)] = spot(14) // the catalog clip's advert

	if remaining, err := splitter.RecheckDedup(context.Background(), p); err != nil || remaining != 0 {
		t.Fatalf("recheck = %d, %v", remaining, err)
	}
	if got := p.Segments[1]; got.DedupUnchecked || got.DupOf != "old/known.mp4" {
		t.Fatalf("segment = %+v, want a recorded copy", got)
	}
}

// A person can still confirm a segment the check could not clear: the hold is for unattended
// confirmation only.
func TestConfirm_ManualConfirmAcceptsAnUncheckedSegment(t *testing.T) {
	st, fake, splitter, p := uncheckedProposal(t)
	stageParentForSplitReview(st, p.ClipHash)
	if _, err := splitter.Confirm(context.Background(), p.ID, p.Segments); err != nil {
		t.Fatalf("manual confirm of an unchecked segment: %v", err)
	}
	if len(fake.cutCalls) != 2 {
		t.Fatalf("cuts = %v, want both segments cut", fake.cutCalls)
	}
	if _, err := st.GetSplitProposal(context.Background(), p.ID); err == nil {
		t.Error("proposal survived a manual confirm")
	}
}

// The mark rides the proposal through its JSON store and the wire: present when set, absent when
// not, and a proposal stored before the field existed reads as checked, not as unchecked.
func TestSegmentDedupUncheckedRoundTrips(t *testing.T) {
	marked, _ := json.Marshal(split.Segment{StartMs: 0, EndMs: 30_000, DedupUnchecked: true})
	if !strings.Contains(string(marked), `"dedupUnchecked":true`) {
		t.Fatalf("marked segment = %s, want dedupUnchecked on the wire", marked)
	}
	plain, _ := json.Marshal(split.Segment{StartMs: 0, EndMs: 30_000})
	if strings.Contains(string(plain), "dedupUnchecked") {
		t.Fatalf("a checked segment carries the field: %s", plain)
	}
	var legacy split.Segment
	if err := json.Unmarshal([]byte(`{"startMs":0,"endMs":30000,"dupOf":"old/clip.mp4"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.DedupUnchecked || legacy.DupOf != "old/clip.mp4" {
		t.Fatalf("a segment stored before the field existed = %+v, want it read as checked", legacy)
	}
	var back split.Segment
	if err := json.Unmarshal(marked, &back); err != nil || !back.DedupUnchecked {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
}
