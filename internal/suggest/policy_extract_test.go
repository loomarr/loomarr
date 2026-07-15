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
