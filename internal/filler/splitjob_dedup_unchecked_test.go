package filler_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

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
		"a catalog clip that cannot be read leaves the others compared": {
			setup: func(st *splitMemStore) (splitStoreFor, *fakeTools) {
				existingClip(st, "known.mp4")
				existingClip(st, "unreadable.mp4")
				return st, &fakeTools{blacks: []tools.Interval{{StartMs: 29800, EndMs: 30200}}, spans: map[string]creative.Fingerprint{
					key3("clean.mp4", 0, 30000): spot(71), key3("clean.mp4", 30000, 60_000): spot(72),
					key3("clean.mp4", 0, 60_000): spot(73), key3("known.mp4", 0, 30_000): spot(71),
					// unreadable.mp4 has no entry: its fingerprint fails
				}}
			},
			wantFlags: []bool{false, false}, wantDup: []string{"old/known.mp4", ""}, wantWarn: true,
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
