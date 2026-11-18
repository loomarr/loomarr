package growth

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// Why a channel does not grow tonight; empty when it does.
const (
	StillKept     = "kept"       // a person switched growth off ("keep as it is")
	StillByHand   = "by_hand"    // made by hand: growth writes only through a request's approval for now
	StillFixed    = "fixed"      // one seed, one show, or nothing airable to draw a line from
	StillInactive = "not_active" // the channel is not live or building
)

// Status is what a person sees of a channel's growth (decision 25): the line in plain words, how
// far the channel is from its target, what joined and can be undone, and what was taken off and
// why. It is read only; changing any of it is an ordinary channel edit.
type Status struct {
	// Still is why the channel does not grow; empty when it does.
	Still   string
	Line    Line
	Words   string
	Carried Carried
	Target  Target
	// Short is what the channel lacks of its target, kind by kind; zero when it has enough.
	Short Target
	// Waiting is how many unclear fits wait for the model's verdict before they may join.
	Waiting int
	Joined  []Joined
	Off     []TakenOff
}

// Joined is a title growth added; taking it off is its undo.
type Joined struct {
	Key   provision.Key
	Name  string
	Batch string // the night it joined, e.g. "growth-2026-11-16"
}

// TakenOff is a title that was on through growth and came off. A person can add it back by hand.
type TakenOff struct {
	Key  provision.Key
	Name string
	// Reason is the model's reason when the model took it off; empty when a person did.
	Reason string
	At     time.Time // when the model judged it; zero when a person took it off
}

// StatusReader reads a channel's growth status from the store.
type StatusReader struct {
	store    FillStore
	hasModel func() bool
}

// NewStatusReader wires the reader. hasModel reports whether a model reviews unclear fits; with
// none, nothing waits.
func NewStatusReader(st FillStore, hasModel func() bool) *StatusReader {
	if hasModel == nil {
		hasModel = func() bool { return false }
	}
	return &StatusReader{store: st, hasModel: hasModel}
}

// Status reads ch's growth status.
func (r *StatusReader) Status(ctx context.Context, ch channelstore.Channel) (Status, error) {
	lib, err := loadLibrary(ctx, r.store)
	if err != nil {
		return Status{}, err
	}
	reviews, err := r.store.GrowthReviews(ctx, ch.ID)
	if err != nil {
		return Status{}, fmt.Errorf("growth: reviews of %s: %w", ch.ID, err)
	}
	v := view(ch, lib)
	s := Status{Line: v.line, Words: v.line.Words(), Carried: v.carried}
	switch {
	case !ch.Status.Reconcilable():
		s.Still = StillInactive
	case !ch.Policy.Grows():
		s.Still = StillKept
	case !ch.FromRequest:
		s.Still = StillByHand
	case v.line.Fixed:
		s.Still = StillFixed
	}
	name := func(k provision.Key) string {
		if i, ok := lib.byKey[k]; ok {
			return lib.titles[i].name
		}
		return string(k)
	}
	for _, e := range ch.Lineup {
		if e.Grown != "" {
			s.Joined = append(s.Joined, Joined{Key: e.Key, Name: e.Title, Batch: e.Grown})
		}
	}
	if ch.Policy.Growth != nil {
		for _, k := range ch.Policy.Growth.Declined {
			off := TakenOff{Key: k, Name: name(k)}
			if rv, ok := reviews[k]; ok && rv.Verdict == string(JudgedNotFit) {
				off.Reason, off.At = rv.Reason, rv.ReviewedAt
			}
			s.Off = append(s.Off, off)
		}
	}
	if v.line.Fixed {
		return s, nil
	}
	s.Target = TargetFor(v.need, MaxShowShare)
	s.Short = Target{
		Episodes: max(s.Target.Episodes-s.Carried.Episodes, 0),
		Shows:    max(s.Target.Shows-s.Carried.Shows, 0),
		Films:    max(s.Target.Films-s.Carried.Films, 0),
	}
	if s.Still == "" && r.hasModel() {
		judged := judgedAgainst(v.line, reviews)
		for _, p := range Plan(v.line, v.candidates, v.carried, s.Target, Bar, ch.Policy.Declined, judged, true) {
			if p.Fit == Unclear && judged(p.Key) == "" {
				s.Waiting++
			}
		}
	}
	return s, nil
}
