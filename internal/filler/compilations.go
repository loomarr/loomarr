package filler

import (
	"fmt"
	"time"
)

// CompilationGate holds compilation reels back from acquisition until automatic splitting is
// certified (#1773, `filler.acquisition.compilations`). Every filler item waiting on a person was
// a reel whose cut points needed confirming, so the unattended paths stop fetching new ones and
// prefer single-spot items instead; nothing already downloaded is dropped.
//
// A reel is recognised the way probe recognises one after download (ProbeStage.looksComposite):
// a known runtime over `filler.autosplit.max_duration`, the longest a single clip is expected to
// run. Before download that is the only signal there is; the structure cues belong to the split
// stage, which needs the file. An item of unknown length is not held back, and probe marks it a
// compilation on arrival if it is one.
//
// The folder source is exempt: it is the operator's own media, dropped on purpose.
type CompilationGate struct {
	// Take reports whether compilations are acquired. nil takes them (the gate is off).
	Take func() bool
	// Over is the longest expected single clip. nil or zero holds nothing back.
	Over func() time.Duration
}

// Defers reports whether an item of this source kind and known runtime (0 = unknown) waits.
func (g CompilationGate) Defers(sourceKind string, durationMS int64) bool {
	if g.Take == nil || g.Take() || g.Over == nil || sourceKind == "folder" || durationMS <= 0 {
		return false
	}
	over := g.Over()
	return over > 0 && time.Duration(durationMS)*time.Millisecond > over
}

// Detail explains a deferral to the operator reading a pull plan's rejected candidates.
func (g CompilationGate) Detail(durationMS int64) string {
	return fmt.Sprintf("a compilation reel (%s, longer than the %s single-clip limit); compilations wait until automatic splitting is certified (filler.acquisition.compilations)",
		(time.Duration(durationMS) * time.Millisecond).Round(time.Second), g.Over())
}
