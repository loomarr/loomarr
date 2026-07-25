package schedule

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A flat policy_json blob exactly as pre-P3 code wrote it (no operatorSet, no rule source).
// The embedded sub-structs must read it byte-for-byte the same — wire compatibility, zero
// migration — and re-marshal to a still-FLAT object (no proposalPolicy/operatorPolicy keys).
const legacyPolicyJSON = `{"scope":{"era":{"from":1990,"to":1999}},"audience":{"ceiling":"TV-Y7"},` +
	`"separation":{"episodeNoRepeat":"168h"},"ordering":"syndication","seasonal":{"mode":"auto"},` +
	`"applied":[{"kind":"episodeNoRepeat","from":"168h","to":"84h"}],"filler":{"audience":"kids"},` +
	`"rules":[{"id":"r1","label":"Weekend","priority":10}],"window":"24h","autoCurate":{"minScorePct":70}}`

func TestChannelPolicy_LegacyWireRoundTrips(t *testing.T) {
	var p ChannelPolicy
	if err := json.Unmarshal([]byte(legacyPolicyJSON), &p); err != nil {
		t.Fatalf("legacy blob must unmarshal into the embedded struct: %v", err)
	}
	// Promoted fields must have landed (proof the embeds flatten on read).
	if p.Scope.Era == nil || p.Scope.Era.From != 1990 {
		t.Errorf("scope.era did not land: %+v", p.Scope.Era)
	}
	if p.Audience.Ceiling != "TV-Y7" {
		t.Errorf("audience.ceiling did not land: %q", p.Audience.Ceiling)
	}
	if p.Filler == nil || p.Filler.Audience != "kids" {
		t.Errorf("filler did not land: %+v", p.Filler)
	}
	if p.Window != Duration(24*time.Hour) {
		t.Errorf("window did not land: %v", p.Window)
	}
	if p.AutoCurate == nil || p.AutoCurate.MinScorePct != 70 {
		t.Errorf("autoCurate did not land: %+v", p.AutoCurate)
	}
	if len(p.Rules) != 1 || len(p.Applied) != 1 {
		t.Errorf("rules/applied did not land: rules=%d applied=%d", len(p.Rules), len(p.Applied))
	}
	// Re-marshal: the wire must stay FLAT (no nested owner objects) so old and new agree.
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "proposalPolicy") || strings.Contains(string(out), "operatorPolicy") ||
		strings.Contains(string(out), "ProposalPolicy") {
		t.Fatalf("policy_json must stay flat, got nested owner keys: %s", out)
	}
	// And the flat keys survive.
	for _, key := range []string{`"scope"`, `"audience"`, `"filler"`, `"window"`, `"autoCurate"`, `"applied"`} {
		if !strings.Contains(string(out), key) {
			t.Errorf("re-marshaled blob dropped %s: %s", key, out)
		}
	}
}

// An operator's edit is STICKY: once a proposal-owned field is pinned (OperatorSet), a later
// refine cannot revert it, while UNPINNED fields still refresh from the proposal (§8.2).
func TestMergeFromProposal_OperatorSetIsSticky(t *testing.T) {
	current := ChannelPolicy{
		ProposalPolicy: ProposalPolicy{
			Scope:    ScopePolicy{Era: &Range{From: 1990, To: 1999}}, // operator pinned this
			Ordering: OrderSequential,                                // NOT pinned
		},
		OperatorSet: []string{pathScope},
	}
	incoming := ChannelPolicy{ProposalPolicy: ProposalPolicy{
		Scope:    ScopePolicy{Era: &Range{From: 2000, To: 2009}}, // a refine tries to change the era
		Ordering: OrderSyndication,                               // and the ordering
	}}

	got := current.MergeFromProposal(incoming)
	if got.Scope.Era == nil || got.Scope.Era.From != 1990 {
		t.Errorf("pinned scope was reverted by a refine: %+v (the exact data-loss bug P3 fixes)", got.Scope.Era)
	}
	if got.Ordering != OrderSyndication {
		t.Errorf("unpinned ordering should refresh from the proposal, got %q", got.Ordering)
	}
}

// Operator-owned fields (filler/window/autoCurate) survive a refine that carries none of them.
func TestMergeFromProposal_PreservesOperatorOwned(t *testing.T) {
	current := ChannelPolicy{OperatorPolicy: OperatorPolicy{
		Filler:     &FillerSelection{Audience: "kids"},
		Window:     Duration(12 * time.Hour),
		AutoCurate: &AutoCurate{MinScorePct: 80},
	}}
	got := current.MergeFromProposal(ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderSyndication}})
	if got.Filler == nil || got.Filler.Audience != "kids" {
		t.Error("filler was clobbered by a refine")
	}
	if got.Window != Duration(12*time.Hour) {
		t.Error("window was clobbered by a refine")
	}
	if got.AutoCurate == nil { // the "channel silently turns itself off" bug
		t.Error("AutoCurate opt-in was clobbered by a refine — the channel would stop self-updating")
	}
}

// Rules merge by provenance: a refine REPLACES the llm rules and PRESERVES operator rules.
func TestMergeFromProposal_RuleProvenance(t *testing.T) {
	current := ChannelPolicy{ProposalPolicy: ProposalPolicy{Rules: []SchedulingRule{
		{ID: "op1", Source: RuleSourceOperator, Label: "hand-authored marathon"},
		{ID: "llm-old", Source: RuleSourceLLM, Label: "stale llm rule"},
		{ID: "legacy", Label: "pre-provenance rule (source empty → preserved)"},
	}}}
	incoming := ChannelPolicy{ProposalPolicy: ProposalPolicy{Rules: []SchedulingRule{
		{ID: "llm-new", Source: RuleSourceLLM, Label: "fresh llm rule"},
	}}}

	got := current.MergeFromProposal(incoming).Rules
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if !ids["op1"] {
		t.Error("operator-authored rule was dropped by a refine")
	}
	if !ids["legacy"] {
		t.Error("unknown-provenance rule should be preserved (fail-safe)")
	}
	if ids["llm-old"] {
		t.Error("stale llm rule should have been replaced")
	}
	if !ids["llm-new"] {
		t.Error("fresh llm rule should have been added")
	}
}

// Safety asymmetry (§4): even a PINNED audience may be TIGHTENED by a stricter proposal
// ceiling, but never LOOSENED.
func TestMergeFromProposal_CeilingNeverRelaxed(t *testing.T) {
	pinnedKids := ChannelPolicy{
		ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: "TV-Y7"}},
		OperatorSet:    []string{pathAudience},
	}
	// A refine tries to LOOSEN to TV-MA — must be rejected (stays TV-Y7).
	if got := pinnedKids.MergeFromProposal(ChannelPolicy{ProposalPolicy: ProposalPolicy{
		Audience: AudiencePolicy{Ceiling: "TV-MA"}}}); got.Audience.Ceiling != "TV-Y7" {
		t.Errorf("a pinned kids ceiling must never be loosened, got %q", got.Audience.Ceiling)
	}
	// A refine that TIGHTENS to TV-Y is allowed even against the pin (safety outranks stickiness).
	if got := pinnedKids.MergeFromProposal(ChannelPolicy{ProposalPolicy: ProposalPolicy{
		Audience: AudiencePolicy{Ceiling: "TV-Y"}}}); got.Audience.Ceiling != "TV-Y" {
		t.Errorf("a stricter proposal ceiling should tighten even a pinned one, got %q", got.Audience.Ceiling)
	}
}

// MergeFromOperator records which proposal-owned fields the operator CHANGED (so a later
// refine leaves them alone) and force-preserves the reconcile-owned Applied.
func TestMergeFromOperator_RecordsDirtyAndPreservesApplied(t *testing.T) {
	current := ChannelPolicy{
		ProposalPolicy: ProposalPolicy{Ordering: OrderSequential},
		Applied:        []AppliedRelaxation{{Kind: "episodeNoRepeat"}},
	}
	// The operator changes ordering (and a client tries to sneak in an Applied value).
	incoming := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderSyndication}}
	incoming.Applied = []AppliedRelaxation{{Kind: "client-forged"}}

	got := current.MergeFromOperator(incoming)
	if got.Ordering != OrderSyndication {
		t.Errorf("operator ordering edit should win, got %q", got.Ordering)
	}
	if !pathSet(got.OperatorSet)[pathOrdering] {
		t.Errorf("ordering should be recorded as operator-set, got %v", got.OperatorSet)
	}
	if pathSet(got.OperatorSet)[pathScope] {
		t.Errorf("scope was unchanged and must NOT be marked operator-set, got %v", got.OperatorSet)
	}
	if len(got.Applied) != 1 || got.Applied[0].Kind != "episodeNoRepeat" {
		t.Errorf("Applied is reconcile-owned and must be force-preserved, got %+v", got.Applied)
	}
}

// A refine must never move a channel between playout backends (§9.1). Backend choice is
// an operator decision about infrastructure; the LLM is proposing CONTENT. This is the
// same class as the era/audience data loss that MergeFromProposal was rewritten to fix —
// asserted when the field is added rather than after someone loses a setting to it.
//
// It lives on OperatorPolicy — the embedded, operator-edited half that
// MergeFromProposal preserves wholesale via `out := current` — so this is enforced by
// structure rather than by a guard someone could forget to write.
func TestMergeFromProposal_NeverTouchesPlayoutBackend(t *testing.T) {
	current := ChannelPolicy{
		OperatorPolicy: OperatorPolicy{Playout: &PlayoutPolicy{Backend: "tunarr"}},
	}
	// An incoming proposal that says nothing about playout (the normal case) …
	got := current.MergeFromProposal(ChannelPolicy{})
	if got.Playout == nil || got.Playout.Backend != "tunarr" {
		t.Fatalf("refine dropped the channel's playout backend: %+v", got.Playout)
	}
	// … and one that tries to set it anyway must still not win.
	got = current.MergeFromProposal(ChannelPolicy{
		OperatorPolicy: OperatorPolicy{Playout: &PlayoutPolicy{Backend: "internal"}},
	})
	if got.Playout == nil || got.Playout.Backend != "tunarr" {
		t.Fatalf("a proposal overwrote the operator's playout backend: %+v", got.Playout)
	}
}
