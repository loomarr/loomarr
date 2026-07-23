package suggest

import (
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
)

// groundPolicy is the extraction grounding gate (programming-design §8): the model
// proposes rule VALUES; we validate + clamp them. A valid policy survives; a
// hallucinated ceiling is DROPPED (not passed through to enforcement).
func TestGroundPolicy_ValidPolicySurvives(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-Y7"
	raw.Era.From, raw.Era.To = 1990, 1999
	raw.Genres.Include = []string{"Animation"}
	raw.Ordering = "syndication"
	raw.Seasonal.Mode = "auto"

	p := groundPolicy(raw, nil, nil)
	if p.Audience.Ceiling != "TV-Y7" {
		t.Errorf("ceiling = %q, want TV-Y7", p.Audience.Ceiling)
	}
	if p.Scope.Era == nil || p.Scope.Era.From != 1990 || p.Scope.Era.To != 1999 {
		t.Errorf("era not extracted: %+v", p.Scope.Era)
	}
	if p.Ordering != schedule.OrderSyndication {
		t.Errorf("ordering = %q, want syndication", p.Ordering)
	}
	if p.Seasonal.Mode != schedule.SeasonalAuto {
		t.Errorf("seasonal mode = %q, want auto", p.Seasonal.Mode)
	}
	// The grounded policy must itself validate (closed enums, on-ladder ceiling).
	if err := p.Validate(); err != nil {
		t.Errorf("grounded policy should validate: %v", err)
	}
}

// A hallucinated / off-ladder ceiling is DROPPED, never passed through — the model
// can't invent a rating the enforcer doesn't understand.
func TestGroundPolicy_OffLadderCeilingDropped(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-SUPERSAFE" // not on the ladder
	p := groundPolicy(raw, nil, nil)
	if p.Audience.Ceiling != "" {
		t.Errorf("off-ladder ceiling should be dropped, got %q", p.Audience.Ceiling)
	}
}

// Unknown enum values (ordering / seasonal mode) are dropped, not persisted.
func TestGroundPolicy_UnknownEnumsDropped(t *testing.T) {
	raw := &pickPolicy{}
	raw.Ordering = "sideways"
	raw.Seasonal.Mode = "eventually"
	p := groundPolicy(raw, nil, nil)
	if p.Ordering != schedule.OrderInherit {
		t.Errorf("unknown ordering should be dropped (inherit), got %q", p.Ordering)
	}
	if p.Seasonal.Mode != schedule.SeasonalDefault {
		t.Errorf("unknown seasonal mode should be dropped, got %q", p.Seasonal.Mode)
	}
}

// A nil policy (the model omitted it) yields an empty policy → built-in defaults.
func TestGroundPolicy_NilIsEmpty(t *testing.T) {
	p := groundPolicy(nil, nil, nil)
	if err := p.Validate(); err != nil {
		t.Errorf("empty policy must validate: %v", err)
	}
	if p.Audience.Ceiling != "" || p.Ordering != schedule.OrderInherit {
		t.Errorf("nil policy should be fully empty, got %+v", p)
	}
}

// THE empty-channel bug (live smoke): a small model set a TV-G ceiling but picked The
// Simpsons (TV-PG). The fail-closed audience gate would drop every episode → an empty
// channel. groundPolicy must RAISE the ceiling to admit the grounded picks.
func TestGroundPolicy_CeilingRaisedToAdmitPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-G" // rank 2 — stricter than the pick
	lineup := []ProposalItem{
		{Name: "The Simpsons", OfficialRating: "TV-PG"}, // rank 3
	}
	p := groundPolicy(raw, lineup, nil)
	if p.Audience.Ceiling != "TV-PG" {
		t.Errorf("ceiling = %q, want TV-PG (raised to admit the TV-PG pick)", p.Audience.Ceiling)
	}
}

// The guard NEVER lowers a ceiling: a kids channel (TV-Y7) whose picks are all at/below
// stays capped, so a stray adult title can't sneak in via a low-rated pick set.
func TestGroundPolicy_CeilingNotLoweredBelowPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-PG" // rank 3
	lineup := []ProposalItem{
		{Name: "Kids Show", OfficialRating: "TV-Y7"}, // rank 1 — below the ceiling
	}
	p := groundPolicy(raw, lineup, nil)
	if p.Audience.Ceiling != "TV-PG" {
		t.Errorf("ceiling = %q, want TV-PG (unchanged — picks are below it)", p.Audience.Ceiling)
	}
}

// An unrated pick doesn't raise the ceiling (it's governed by UnratedPolicy, not the
// ladder) — so an unrated title can't blow the roof off a kids ceiling.
func TestGroundPolicy_UnratedPickDoesNotRaiseCeiling(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-Y7"
	lineup := []ProposalItem{
		{Name: "Mystery", OfficialRating: ""},        // unrated → ignored by the guard
		{Name: "Also Mystery", OfficialRating: "??"}, // off-ladder → ignored
	}
	p := groundPolicy(raw, lineup, nil)
	if p.Audience.Ceiling != "TV-Y7" {
		t.Errorf("ceiling = %q, want TV-Y7 (unrated picks never raise it)", p.Audience.Ceiling)
	}
}
