package suggest

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// Phase names a stage of grounded generation (§8), surfaced to the UI as SSE
// `suggestion` progress frames. A dropped frame is a latency bug, never a
// correctness bug — GET /v1/proposals/{id} is the source of truth on reconnect.
type Phase string

const (
	// PhaseSearching: a catalog tool call is RUNNING (the model asked for titles and
	// we're fetching them). Short — it's a local/TMDB query, not a model turn.
	PhaseSearching Phase = "searching"
	// PhaseReasoning: the model is THINKING — the turn is in flight and we're waiting
	// on it. This is where a slow run actually spends its time (a cold local model can
	// take ~9s just to load before it emits a token), which is precisely why it must be
	// reported while it happens rather than after.
	PhaseReasoning Phase = "reasoning"
	// PhaseScoring: deterministic post-scoring + proposal assembly. Outside the loop.
	PhaseScoring Phase = "scoring"
	// PhaseDone / PhaseFailed are emitted by the worker around Suggest, not from
	// inside it — the pipeline itself only knows the in-flight phases.
	PhaseDone   Phase = "done"
	PhaseFailed Phase = "failed"
)

// ProgressFunc receives each phase transition during Suggest. It must not block:
// the worker's publisher is non-blocking and drops on a full subscriber buffer.
//
// round is the 1-based tool-loop iteration the phase belongs to, or 0 for a phase
// outside the loop (scoring, and the worker's done/failed). Phases REPEAT: the loop
// alternates reasoning → searching → reasoning as the model calls the catalog and
// reads the results, so a receiver must treat these as "what is happening now",
// not as a monotonic sequence of distinct steps.
type ProgressFunc func(Phase, int)

type progressKey struct{}

// WithProgress returns a context carrying fn so Suggest can report phase
// transitions without a signature change across its many call sites. The worker
// injects this per job so the callback can tag frames with the job id; a bare
// context (unit tests calling Suggest directly) makes reporting a no-op.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, fn)
}

// reportProgress fires the context's ProgressFunc if one is set, tagging the frame
// with the tool-loop round (0 outside the loop).
func reportProgress(ctx context.Context, p Phase, round int) {
	trackerFrom(ctx).setPhase(p)
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && fn != nil {
		fn(p, round)
	}
}

// Stage is the user-facing step of a generation run, derived from real events only:
// a phase transition, a catalog_search call's own arguments, or a pick that resolved.
type Stage string

const (
	// StageReading: the request has been handed to the model; nothing searched yet.
	StageReading Stage = "reading"
	// StageSearching: a catalog search is running; Terms carries what it was asked for.
	StageSearching Stage = "searching"
	// StageChoosing: the model is selecting from what the search surfaced; Picks fill in.
	StageChoosing Stage = "choosing"
	// StageBuilding: deterministic scoring and proposal assembly.
	StageBuilding Stage = "building"
)

// LineupPickCap is the most picks the prompt asks the model for ("Select at most 8 picks
// total"), and so the honest denominator for "4 of about 8 picked". A test pins the prompt to
// this constant so the two cannot drift apart silently.
const LineupPickCap = 8

// maxProgressTerms bounds the search terms kept on a snapshot: the UI shows a phrase, and a
// model can emit a long genres list.
const (
	maxProgressTerms   = 3
	maxProgressTermLen = 60
)

// ProgressPick is one resolved pick as the UI renders a title row. Every field comes from
// the surfaced catalog Candidate, never from the model's own claim about the title.
type ProgressPick struct {
	Key           string
	MediaType     string
	Name          string
	Year          int
	TMDBID        int
	InLibrary     bool
	LibraryItemID string
}

// ProgressSnapshot is the latest known state of a running generation.
type ProgressSnapshot struct {
	Stage     Stage
	Terms     []string
	Picks     []ProgressPick
	Target    int
	StartedAt time.Time
}

// ProgressTracker accumulates a run's snapshot and tells the wiring when it changed. It is
// safe for concurrent use: the model's stream goroutine, the tool loop and an HTTP reader all
// touch it. A nil tracker is a valid no-op, so the pipeline never branches on it.
type ProgressTracker struct {
	mu       sync.Mutex
	snap     ProgressSnapshot
	onChange func()
}

// NewProgressTracker starts a run in StageReading. onChange (optional) must not block; it is
// called after every change, outside the lock.
func NewProgressTracker(now time.Time, onChange func()) *ProgressTracker {
	return &ProgressTracker{
		snap:     ProgressSnapshot{Stage: StageReading, Target: LineupPickCap, StartedAt: now},
		onChange: onChange,
	}
}

// Snapshot returns a copy safe to retain.
func (t *ProgressTracker) Snapshot() ProgressSnapshot {
	if t == nil {
		return ProgressSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.snap
	out.Terms = append([]string(nil), t.snap.Terms...)
	out.Picks = append([]ProgressPick(nil), t.snap.Picks...)
	return out
}

func (t *ProgressTracker) update(fn func(*ProgressSnapshot) bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	changed := fn(&t.snap)
	t.mu.Unlock()
	if changed && t.onChange != nil {
		t.onChange()
	}
}

// setPhase maps a pipeline phase onto a Stage. reasoning before any search is the model
// reading the request; reasoning after one is it choosing from the results.
func (t *ProgressTracker) setPhase(p Phase) {
	t.update(func(s *ProgressSnapshot) bool {
		next := s.Stage
		switch p {
		case PhaseSearching:
			next = StageSearching
		case PhaseReasoning:
			next = StageReading
			if len(s.Terms) > 0 {
				next = StageChoosing
			}
		case PhaseScoring:
			next = StageBuilding
		}
		if next == s.Stage {
			return false
		}
		s.Stage = next
		return true
	})
}

// addTerms records what a catalog_search call actually asked for.
func (t *ProgressTracker) addTerms(terms []string) {
	t.update(func(s *ProgressSnapshot) bool {
		changed := false
		for _, term := range terms {
			term = strings.TrimSpace(term)
			if term == "" || len(s.Terms) >= maxProgressTerms || slices.Contains(s.Terms, term) {
				continue
			}
			if len(term) > maxProgressTermLen {
				term = term[:maxProgressTermLen]
			}
			s.Terms = append(s.Terms, term)
			changed = true
		}
		return changed
	})
}

// addPick appends a resolved pick once; a repeated key (a repair turn re-listing it) is ignored.
func (t *ProgressTracker) addPick(p ProgressPick) {
	t.update(func(s *ProgressSnapshot) bool {
		for _, have := range s.Picks {
			if have.Key == p.Key {
				return false
			}
		}
		s.Picks = append(s.Picks, p)
		return true
	})
}

type progressTrackerKey struct{}

// WithProgressTracker threads a tracker through ctx, like WithProgress.
func WithProgressTracker(ctx context.Context, t *ProgressTracker) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, progressTrackerKey{}, t)
}

func trackerFrom(ctx context.Context) *ProgressTracker {
	t, _ := ctx.Value(progressTrackerKey{}).(*ProgressTracker)
	return t
}

// searchTerms reads what a catalog_search call asked for from its own arguments: the free-text
// query, or a discovery call's genres. Real event data — it is never guessed or paraphrased.
func searchTerms(args map[string]any) []string {
	var terms []string
	if q := strings.TrimSpace(stringArg(args["query"])); q != "" {
		terms = append(terms, q)
	}
	if genres, ok := args["genres"].([]any); ok {
		for _, g := range genres {
			if name := strings.TrimSpace(stringArg(g)); name != "" {
				terms = append(terms, name)
			}
		}
	}
	return terms
}

// streamPicks returns the OnContentDelta hook that turns a streaming final JSON into
// resolved progress picks. Each complete pick goes through resolvePick — the identical
// grounding gate final validation uses — and through the same acquisition cap, so only a
// pick the proposal will keep as an item is ever shown as chosen. Unresolvable picks are
// silently skipped: the final parse is still the authority and reports them.
//
// Cheap by construction (a byte scan plus a map lookup per finished pick), because it runs on
// the stream-reading goroutine. surfaced is read only from that goroutine while Chat blocks
// the tool loop that mutates it.
func (s *Suggester) streamPicks(t *ProgressTracker, intent Intent, surfaced map[provision.Key]catalog.Candidate) func(string) {
	if t == nil {
		return nil
	}
	maxAcq := s.maxAcq
	if intent.MaxAcquire > 0 && intent.MaxAcquire < maxAcq {
		maxAcq = intent.MaxAcquire
	}
	stream := newPickStream(func(p pick) {
		cand, rejection := resolvePick(intent, p, surfaced)
		if rejection != pickAccepted {
			return
		}
		if !cand.InLibrary {
			acquisitions := 0
			for _, have := range t.Snapshot().Picks {
				if !have.InLibrary {
					acquisitions++
				}
			}
			if acquisitions >= maxAcq {
				return // buildProposal turns an over-cap acquisition into an alternate
			}
		}
		t.addPick(ProgressPick{
			Key: p.Key, MediaType: string(cand.MediaType), Name: cand.Name, Year: cand.Year,
			TMDBID: cand.TMDBID, InLibrary: cand.InLibrary, LibraryItemID: cand.LibraryItemID,
		})
	})
	return stream.Write
}

// stagePhase is the SSE phase a stage change is announced under. The frame's only job is to
// tell a client "refetch the journey" (§8: SSE is the latency path, the GET is truth), so the
// closest existing phase is enough — the detail rides the GET's progress snapshot.
func stagePhase(s Stage) Phase {
	switch s {
	case StageSearching:
		return PhaseSearching
	case StageBuilding:
		return PhaseScoring
	default:
		return PhaseReasoning
	}
}

// trackJob starts a run's tracker, threads it through ctx and registers it so the API can read
// the live snapshot. Every change announces itself on the SSE bus. The returned func drops the
// tracker: once a run ends, the Proposal or the failure is the authoritative record, and a
// snapshot of a run that no longer exists would only mislead. The snapshot is in memory on
// purpose — a run does not survive a restart (its attempt is marked interrupted), so persisting
// it would only leave stale progress on a job that can never resume.
func (s *Service) trackJob(ctx context.Context, jobID string) (context.Context, func()) {
	t := NewProgressTracker(s.now(), nil)
	t.onChange = func() { s.emitPhase(jobID, stagePhase(t.Snapshot().Stage), 0) }
	s.tracked.Store(jobID, t)
	return WithProgressTracker(ctx, t), func() { s.tracked.Delete(jobID) }
}

// Progress returns the live snapshot of a running generation, or false when the job is not
// running on this process (queued, finished, or failed).
func (s *Service) Progress(jobID string) (ProgressSnapshot, bool) {
	v, ok := s.tracked.Load(jobID)
	if !ok {
		return ProgressSnapshot{}, false
	}
	return v.(*ProgressTracker).Snapshot(), true
}
