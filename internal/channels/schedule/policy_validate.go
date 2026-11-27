package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
)

// ValidateWithHolidays is Validate plus the one check that needs the household's holiday
// vocabulary: a rule's holiday id must be one it defines, exactly as written (the engine compares
// ids as they are), because an unknown id is a rule that never fires. Writers that can reach the
// vocabulary use this; Validate alone leaves holiday ids to the vocabulary-free callers.
func (p ChannelPolicy) ValidateWithHolidays(v holidayvocab.Vocabulary) error {
	return p.validateWith(&v, nil)
}

// ValidateEditOf is ValidateWithHolidays for an edit of a policy that is already stored. A rule the
// edit leaves exactly as stored (same id and content) is not checked: the stored policy may hold
// a rule an older writer allowed (a one-hour window written as 5 to 5, a pattern no hour of the week
// satisfies), and refusing an unrelated edit for it would blame a change the operator never made.
// Such a rule never fires, so leaving it costs nothing; a rule the edit adds or changes is held to
// the same checks as a fresh write, and its error names it (RuleError). Every other field is
// checked as usual.
func (p ChannelPolicy) ValidateEditOf(stored ChannelPolicy, v holidayvocab.Vocabulary) error {
	return p.validateWith(&v, stored.Rules)
}

// ValidateKeeping is Validate for a policy a writer built from the one already stored (an approved
// proposal merged onto its channel): rules left exactly as stored are not re-checked, for the reason
// ValidateEditOf gives, and a rule the writer added or changed is. It takes no holiday vocabulary,
// so the holiday id is left to the writers that have one.
func (p ChannelPolicy) ValidateKeeping(stored ChannelPolicy) error {
	return p.validateWith(nil, stored.Rules)
}

// WithoutUnsatisfiableLLMRules drops the suggester's rules that could never fire (WhenPredicate.validate
// refuses them): a stored proposal from before the lowering refused contradictory composites still
// carries them. A rule that never fires changes nothing the channel airs, and an LLM rule is the
// suggester's to replace, so removing one loses nothing; operator rules are never touched.
func (p ChannelPolicy) WithoutUnsatisfiableLLMRules() ChannelPolicy {
	kept := make([]SchedulingRule, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if rule.Source.isLLM() && rule.When.validate() != nil {
			continue
		}
		kept = append(kept, rule)
	}
	if len(kept) == len(p.Rules) {
		return p
	}
	if len(kept) == 0 {
		kept = nil // the zero value stays nil, as mergeRules keeps it (round-trip stability)
	}
	p.Rules = kept
	return p
}

// RuleError is a policy validation failure in one rule, carrying which, so a caller can tell the
// operator the rule rather than a generic policy complaint. Its text keeps the form
// "channel policy: rule N: <reason>".
type RuleError struct {
	Index int // 1-based position in the policy's rules
	ID    string
	Label string
	Err   error
}

func (e *RuleError) Error() string { return fmt.Sprintf("channel policy: rule %d: %v", e.Index, e.Err) }

func (e *RuleError) Unwrap() error { return e.Err }

// Describe is the failure in the operator's words: which rule, and why it cannot be saved.
func (e *RuleError) Describe() string {
	name := fmt.Sprintf("Rule %d", e.Index)
	if label := strings.TrimSpace(e.Label); label != "" {
		name += fmt.Sprintf(" (%s)", label)
	}
	return fmt.Sprintf("%s can never air: %v. Fix or remove it, then try again.", name, e.Err)
}

// sameStoredRule reports whether rule is, id and content, one of the stored rules (a rule saved
// before ids existed has none, and matches a stored one that has none either). The comparison is on
// the stored (JSON) form, not the Go value, so a time that decoded with a different location pointer
// or a nil slice against an empty one is still the same rule.
func sameStoredRule(rule SchedulingRule, stored []SchedulingRule) bool {
	want, err := json.Marshal(rule)
	if err != nil {
		return false
	}
	for _, s := range stored {
		if s.ID != rule.ID {
			continue
		}
		if got, err := json.Marshal(s); err == nil && string(got) == string(want) {
			return true
		}
	}
	return false
}

// Validate rejects a policy with an unknown non-empty enum value or an unmappable
// audience ceiling (grounding: the suggester must never emit a ceiling outside the
// closed ladder), and a rule whose weekday or hour values name nothing or can never be
// satisfied (WhenPredicate.validate).
// Empty/omitted fields are always valid (they resolve to defaults).
func (p ChannelPolicy) Validate() error { return p.validateWith(nil, nil) }

// validateWith is Validate, plus the holiday check when v is given, with the rules in stored (an
// edit's baseline) exempt from the per-rule checks.
func (p ChannelPolicy) validateWith(v *holidayvocab.Vocabulary, stored []SchedulingRule) error {
	if err := validateDates("channel policy: scope.dates", p.Scope.Dates); err != nil {
		return err
	}
	for i, rule := range p.Rules {
		if sameStoredRule(rule, stored) {
			continue
		}
		if err := rule.When.validate(); err != nil {
			return &RuleError{Index: i + 1, ID: rule.ID, Label: rule.Label, Err: err}
		}
		if rule.What != nil {
			if err := validateDates("channel policy: rule.what.dates", rule.What.Dates); err != nil {
				return err
			}
		}
	}
	if !p.Ordering.valid() {
		return fmt.Errorf("channel policy: unknown ordering %q", p.Ordering)
	}
	if !p.Audience.Unrated.valid() {
		return fmt.Errorf("channel policy: unknown unrated policy %q", p.Audience.Unrated)
	}
	if p.Audience.Ceiling != "" && !p.Audience.Ceiling.Mapped() {
		return fmt.Errorf("channel policy: ceiling %q is not on the rating ladder", p.Audience.Ceiling)
	}
	if !p.Seasonal.Mode.valid() {
		return fmt.Errorf("channel policy: unknown seasonal mode %q", p.Seasonal.Mode)
	}
	if !p.Seasonal.OffSeason.valid() {
		return fmt.Errorf("channel policy: unknown offSeason %q", p.Seasonal.OffSeason)
	}
	if !p.Seasonal.Strength.valid() {
		return fmt.Errorf("channel policy: unknown holiday strength %q", p.Seasonal.Strength)
	}
	if !p.Seasonal.Placement.valid() {
		return fmt.Errorf("channel policy: unknown holiday placement %q", p.Seasonal.Placement)
	}
	if err := p.Filler.validate(); err != nil {
		return fmt.Errorf("channel policy: %w", err)
	}
	if err := p.Watermark.validate(); err != nil {
		return fmt.Errorf("channel policy: %w", err)
	}
	if p.BreakDuration != nil && p.BreakDuration.Std() < 30*time.Second {
		return fmt.Errorf("channel policy: breakDuration %s must be at least 30s", p.BreakDuration.Std())
	}
	if p.CommercialSpacing != nil && p.CommercialSpacing.Std() < 0 {
		return fmt.Errorf("channel policy: commercialSpacing %s must not be negative", p.CommercialSpacing.Std())
	}
	if p.AutoCurate != nil {
		if p.AutoCurate.MinScorePct < 0 || p.AutoCurate.MinScorePct > 100 {
			return fmt.Errorf("channel policy: autoCurate.minScorePct %d out of range (0–100)", p.AutoCurate.MinScorePct)
		}
		if p.AutoCurate.MaxTitles < 0 {
			return fmt.Errorf("channel policy: autoCurate.maxTitles %d must not be negative", p.AutoCurate.MaxTitles)
		}
	}
	if v != nil {
		for i, rule := range p.Rules {
			if id := rule.When.Holiday; id != "" && !sameStoredRule(rule, stored) && !knownHoliday(id, *v) {
				return &RuleError{Index: i + 1, ID: rule.ID, Label: rule.Label,
					Err: fmt.Errorf("holiday %q is not a holiday this household's calendar defines", id)}
			}
		}
	}
	return nil
}
