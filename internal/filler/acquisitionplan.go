package filler

import (
	"fmt"
	"sort"
	"strings"
)

// AcquisitionIntentVersion is persisted with every pull so a later ranking change never
// rewrites what an operator was shown.
const AcquisitionIntentVersion = "filler-acquisition-intent/v1"

// RightsPreference says how declared remote rights metadata affects acquisition selection.
// A declaration is still evidence supplied by the source, not a legal conclusion.
type RightsPreference string

const (
	RightsAny             RightsPreference = "any"
	RightsPreferDeclared  RightsPreference = "prefer_declared"
	RightsRequireDeclared RightsPreference = "require_declared"
)

// AcquisitionIntent is the closed, inspectable request a deterministic planner satisfies.
// Zero values mean the operator did not request that constraint; they never mean that missing
// candidate metadata satisfies it.
type AcquisitionIntent struct {
	Version         string           `json:"version"`
	Roles           []Kind           `json:"roles,omitempty"`
	EraStart        int              `json:"eraStart,omitempty"`
	EraEnd          int              `json:"eraEnd,omitempty"`
	Audiences       []Audience       `json:"audiences,omitempty"`
	Geography       Geography        `json:"geography,omitempty"`
	MaxDurationMS   int              `json:"maxDurationMs,omitempty"`
	TaxonomyGaps    []string         `json:"taxonomyGaps,omitempty"`
	Rights          RightsPreference `json:"rights"`
	SourceAllowlist []string         `json:"sourceAllowlist,omitempty"`
	MinHeight       int              `json:"minHeight,omitempty"`
	Count           int              `json:"count"`
	CatalogReason   string           `json:"catalogReason"`
}

// Normalize supplies version/defaults and canonicalises set-like fields. It does not relax a
// requested constraint.
func (i AcquisitionIntent) Normalize() AcquisitionIntent {
	if i.Version == "" {
		i.Version = AcquisitionIntentVersion
	}
	if i.Count < 1 {
		i.Count = 12
	}
	if i.Count > 50 {
		i.Count = 50
	}
	if i.Rights == "" {
		i.Rights = RightsPreferDeclared
	}
	i.Geography = i.Geography.Normalize()
	i.CatalogReason = strings.TrimSpace(i.CatalogReason)
	i.Roles = uniqueKinds(i.Roles)
	i.Audiences = uniqueAudiences(i.Audiences)
	i.TaxonomyGaps = uniqueStrings(i.TaxonomyGaps)
	i.SourceAllowlist = uniqueStrings(i.SourceAllowlist)
	return i
}

func (i AcquisitionIntent) Validate() error {
	i = i.Normalize()
	if i.Version != AcquisitionIntentVersion {
		return fmt.Errorf("unsupported acquisition intent version %q", i.Version)
	}
	if i.EraStart < 0 || i.EraEnd < 0 || (i.EraStart > 0 && i.EraEnd > 0 && i.EraStart > i.EraEnd) {
		return fmt.Errorf("invalid era observation range %d-%d", i.EraStart, i.EraEnd)
	}
	if i.MaxDurationMS < 0 || i.MinHeight < 0 {
		return fmt.Errorf("duration and quality floors cannot be negative")
	}
	if err := i.Geography.Validate(); err != nil {
		return fmt.Errorf("acquisition geography: %w", err)
	}
	switch i.Rights {
	case RightsAny, RightsPreferDeclared, RightsRequireDeclared:
	default:
		return fmt.Errorf("unknown rights preference %q", i.Rights)
	}
	for _, role := range i.Roles {
		if !validKind(role) {
			return fmt.Errorf("unknown content role %q", role)
		}
	}
	for _, audience := range i.Audiences {
		if !validAudience(audience) {
			return fmt.Errorf("unknown audience %q", audience)
		}
	}
	return nil
}

// RemoteIdentity is the stable identity of one provider item inside one registered source.
// URL is deliberately absent: it is mutable transport payload, not identity.
type RemoteIdentity struct {
	Provider string `json:"provider"`
	SourceID string `json:"sourceId"`
	RemoteID string `json:"remoteId"`
}

func (i RemoteIdentity) Key() string {
	return strings.ToLower(strings.TrimSpace(i.Provider)) + "\x00" +
		strings.TrimSpace(i.SourceID) + "\x00" + strings.ToLower(strings.TrimSpace(i.RemoteID))
}

func (i RemoteIdentity) Validate() error {
	if strings.TrimSpace(i.Provider) == "" || strings.TrimSpace(i.SourceID) == "" || strings.TrimSpace(i.RemoteID) == "" {
		return fmt.Errorf("remote identity requires provider, source, and item ids")
	}
	return nil
}

// AcquisitionCandidate contains metadata-only observations. ObservedYear and PublishedAt are
// weak remote hints and must never be copied into an admitted clip as grounded era evidence.
type AcquisitionCandidate struct {
	Identity      RemoteIdentity `json:"identity"`
	URL           string         `json:"url"`
	Title         string         `json:"title,omitempty"`
	License       string         `json:"license,omitempty"`
	ObservedYear  int            `json:"observedYear,omitempty"`
	PublishedAt   string         `json:"publishedAt,omitempty"`
	DurationMS    int            `json:"durationMs,omitempty"`
	Height        int            `json:"height,omitempty"`
	Geography     Geography      `json:"geography,omitempty"`
	ObservedRoles []Kind         `json:"observedRoles,omitempty"`
	Audiences     []Audience     `json:"audiences,omitempty"`
	Taxonomy      []string       `json:"taxonomy,omitempty"`
}

// CandidateDisposition is a stable explanation for a selected or excluded remote item.
type CandidateDisposition string

const (
	CandidateSelected           CandidateDisposition = "selected"
	CandidateAlreadyCatalogued  CandidateDisposition = "already_catalogued"
	CandidateAlreadyQueued      CandidateDisposition = "already_queued"
	CandidatePreviouslyDeclined CandidateDisposition = "previously_declined"
	CandidateDuplicateRemote    CandidateDisposition = "duplicate_remote"
	CandidateSourceDisabled     CandidateDisposition = "source_disabled"
	CandidateSourceNotAllowed   CandidateDisposition = "source_not_allowed"
	CandidateGeographyMismatch  CandidateDisposition = "geography_mismatch"
	CandidateRightsUnknown      CandidateDisposition = "rights_unknown"
	CandidateRightsMismatch     CandidateDisposition = "rights_mismatch"
	CandidateEraUnknown         CandidateDisposition = "era_unknown"
	CandidateEraMismatch        CandidateDisposition = "era_mismatch"
	CandidateDurationUnknown    CandidateDisposition = "duration_unknown"
	CandidateDurationExceeded   CandidateDisposition = "duration_exceeded"
	CandidateQualityUnknown     CandidateDisposition = "quality_unknown"
	CandidateQualityBelowFloor  CandidateDisposition = "quality_below_floor"
	CandidateRoleUnknown        CandidateDisposition = "role_unknown"
	CandidateRoleMismatch       CandidateDisposition = "role_mismatch"
	CandidateAudienceUnknown    CandidateDisposition = "audience_unknown"
	CandidateAudienceMismatch   CandidateDisposition = "audience_mismatch"
	CandidateTaxonomyUnknown    CandidateDisposition = "taxonomy_unknown"
	CandidateTaxonomyMismatch   CandidateDisposition = "taxonomy_mismatch"
	CandidateRankedBelowLimit   CandidateDisposition = "ranked_below_limit"
)

// ExistingRemoteState prevents a planner from repeatedly proposing work already decided or in
// progress. Catalogued outranks queued, which outranks declined when inputs contain duplicates.
type ExistingRemoteState string

const (
	RemoteCatalogued ExistingRemoteState = "catalogued"
	RemoteQueued     ExistingRemoteState = "queued"
	RemoteDeclined   ExistingRemoteState = "declined"
)

// AcquisitionDecision is one completely explained candidate evaluation.
type AcquisitionDecision struct {
	Candidate   AcquisitionCandidate `json:"candidate"`
	Disposition CandidateDisposition `json:"disposition"`
	Detail      string               `json:"detail"`
}

// AcquisitionPlan is deterministic for a fixed intent, candidate set, and existing-state map.
type AcquisitionPlan struct {
	Intent   AcquisitionIntent     `json:"intent"`
	Selected []AcquisitionDecision `json:"selected"`
	Rejected []AcquisitionDecision `json:"rejected"`
}

// PlanAcquisition applies hard constraints first, then selects a diverse stable prefix.
func PlanAcquisition(intent AcquisitionIntent, candidates []AcquisitionCandidate, existing map[string]ExistingRemoteState) (AcquisitionPlan, error) {
	intent = intent.Normalize()
	if err := intent.Validate(); err != nil {
		return AcquisitionPlan{}, err
	}
	plan := AcquisitionPlan{Intent: intent, Selected: []AcquisitionDecision{}, Rejected: []AcquisitionDecision{}}
	eligible := make([]AcquisitionDecision, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		decision := AcquisitionDecision{Candidate: normalizeCandidate(candidate)}
		key := decision.Candidate.Identity.Key()
		if err := decision.Candidate.Identity.Validate(); err != nil || strings.TrimSpace(decision.Candidate.URL) == "" {
			decision.Disposition, decision.Detail = CandidateDuplicateRemote, "candidate has no actionable registered identity or URL"
		} else if seen[key] {
			decision.Disposition, decision.Detail = CandidateDuplicateRemote, "the same provider item appeared more than once"
		} else if state := existing[key]; state != "" {
			decision.Disposition, decision.Detail = dispositionForExisting(state), "the exact registered provider item was already "+string(state)
		} else if disposition, detail := rejectByIntent(intent, decision.Candidate); disposition != "" {
			decision.Disposition, decision.Detail = disposition, detail
		} else {
			eligible = append(eligible, decision)
		}
		seen[key] = true
		if decision.Disposition != "" {
			plan.Rejected = append(plan.Rejected, decision)
		}
	}

	usedSources := map[string]bool{}
	usedYears := map[int]bool{}
	for len(eligible) > 0 && len(plan.Selected) < intent.Count {
		sort.SliceStable(eligible, func(a, b int) bool {
			return candidateBetter(intent, eligible[a].Candidate, eligible[b].Candidate, usedSources, usedYears)
		})
		decision := eligible[0]
		eligible = eligible[1:]
		decision.Disposition = CandidateSelected
		decision.Detail = "selected by deterministic rights, diversity, quality, and identity ranking"
		plan.Selected = append(plan.Selected, decision)
		usedSources[decision.Candidate.Identity.SourceID] = true
		if decision.Candidate.ObservedYear > 0 {
			usedYears[decision.Candidate.ObservedYear] = true
		}
	}
	for _, decision := range eligible {
		decision.Disposition = CandidateRankedBelowLimit
		decision.Detail = fmt.Sprintf("ranked outside the requested %d items", intent.Count)
		plan.Rejected = append(plan.Rejected, decision)
	}
	sort.SliceStable(plan.Rejected, func(a, b int) bool {
		return plan.Rejected[a].Candidate.Identity.Key() < plan.Rejected[b].Candidate.Identity.Key()
	})
	return plan, nil
}

// DefaultAcquisitionIntent derives its rationale from the same pool projection the UI reads.
func DefaultAcquisitionIntent(pool PoolReport, geography Geography) AcquisitionIntent {
	reason := "Increase the eligible filler catalog."
	if weakest := pool.Weakest(); weakest != nil {
		reason = fmt.Sprintf("Improve filler coverage for %s; its current match level is %s.", weakest.Name, weakest.Report.Level)
	}
	return AcquisitionIntent{
		Version: AcquisitionIntentVersion, Geography: geography.Normalize(),
		Rights: RightsPreferDeclared, Count: 12, CatalogReason: reason,
	}
}

func rejectByIntent(intent AcquisitionIntent, c AcquisitionCandidate) (CandidateDisposition, string) {
	if len(intent.SourceAllowlist) > 0 && !containsFold(intent.SourceAllowlist, c.Identity.SourceID) {
		return CandidateSourceNotAllowed, "registered source is outside the intent allow-list"
	}
	if intent.Geography.Country != "" && !SourceGeographicallyEligible(c.Geography, intent.Geography) {
		return CandidateGeographyMismatch, "candidate geography does not cover the target"
	}
	if intent.Rights == RightsRequireDeclared && strings.TrimSpace(c.License) == "" {
		return CandidateRightsUnknown, "the provider supplied no rights declaration"
	}
	if intent.EraStart > 0 || intent.EraEnd > 0 {
		if c.ObservedYear == 0 {
			return CandidateEraUnknown, "the provider supplied no year observation"
		}
		if (intent.EraStart > 0 && c.ObservedYear < intent.EraStart) || (intent.EraEnd > 0 && c.ObservedYear > intent.EraEnd) {
			return CandidateEraMismatch, fmt.Sprintf("observed year %d is outside the requested range", c.ObservedYear)
		}
	}
	if intent.MaxDurationMS > 0 {
		if c.DurationMS == 0 {
			return CandidateDurationUnknown, "the provider supplied no duration"
		}
		if c.DurationMS > intent.MaxDurationMS {
			return CandidateDurationExceeded, fmt.Sprintf("remote duration %dms exceeds the ceiling", c.DurationMS)
		}
	}
	if intent.MinHeight > 0 {
		if c.Height == 0 {
			return CandidateQualityUnknown, "the provider supplied no video height"
		}
		if c.Height < intent.MinHeight {
			return CandidateQualityBelowFloor, fmt.Sprintf("remote height %dp is below the floor", c.Height)
		}
	}
	if len(intent.Roles) > 0 {
		if len(c.ObservedRoles) == 0 {
			return CandidateRoleUnknown, "the provider supplied no content-role observation"
		}
		if !kindOverlap(intent.Roles, c.ObservedRoles) {
			return CandidateRoleMismatch, "observed content role does not match the intent"
		}
	}
	if len(intent.Audiences) > 0 {
		if len(c.Audiences) == 0 {
			return CandidateAudienceUnknown, "the provider supplied no audience observation"
		}
		if !audienceOverlap(intent.Audiences, c.Audiences) {
			return CandidateAudienceMismatch, "observed audience does not match the intent"
		}
	}
	if len(intent.TaxonomyGaps) > 0 {
		if len(c.Taxonomy) == 0 {
			return CandidateTaxonomyUnknown, "the provider supplied no taxonomy observation"
		}
		if !stringOverlap(intent.TaxonomyGaps, c.Taxonomy) {
			return CandidateTaxonomyMismatch, "observed taxonomy does not address the requested gap"
		}
	}
	return "", ""
}

func candidateBetter(intent AcquisitionIntent, a, b AcquisitionCandidate, usedSources map[string]bool, usedYears map[int]bool) bool {
	av, bv := !usedSources[a.Identity.SourceID], !usedSources[b.Identity.SourceID]
	if av != bv {
		return av
	}
	av, bv = a.ObservedYear > 0 && !usedYears[a.ObservedYear], b.ObservedYear > 0 && !usedYears[b.ObservedYear]
	if av != bv {
		return av
	}
	if intent.Rights == RightsPreferDeclared {
		av, bv = strings.TrimSpace(a.License) != "", strings.TrimSpace(b.License) != ""
		if av != bv {
			return av
		}
	}
	if a.Height != b.Height {
		return a.Height > b.Height
	}
	return a.Identity.Key() < b.Identity.Key()
}

func dispositionForExisting(state ExistingRemoteState) CandidateDisposition {
	switch state {
	case RemoteCatalogued:
		return CandidateAlreadyCatalogued
	case RemoteQueued:
		return CandidateAlreadyQueued
	case RemoteDeclined:
		return CandidatePreviouslyDeclined
	default:
		return CandidateDuplicateRemote
	}
}

func normalizeCandidate(c AcquisitionCandidate) AcquisitionCandidate {
	c.Identity.Provider = strings.ToLower(strings.TrimSpace(c.Identity.Provider))
	c.Identity.SourceID = strings.TrimSpace(c.Identity.SourceID)
	c.Identity.RemoteID = strings.TrimSpace(c.Identity.RemoteID)
	c.URL = strings.TrimSpace(c.URL)
	c.Title = strings.TrimSpace(c.Title)
	c.License = strings.TrimSpace(c.License)
	c.Geography = c.Geography.Normalize()
	c.ObservedRoles = uniqueKinds(c.ObservedRoles)
	c.Audiences = uniqueAudiences(c.Audiences)
	c.Taxonomy = uniqueStrings(c.Taxonomy)
	return c
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a]) < strings.ToLower(out[b]) })
	return out
}

func uniqueKinds(in []Kind) []Kind {
	out := make([]Kind, 0, len(in))
	seen := map[Kind]bool{}
	for _, value := range in {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func uniqueAudiences(in []Audience) []Audience {
	out := make([]Audience, 0, len(in))
	seen := map[Audience]bool{}
	for _, value := range in {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func validKind(value Kind) bool {
	switch value {
	case Commercial, Bumper, StationID, PSA, Trailer, Interstitial:
		return true
	default:
		return false
	}
}

func validAudience(value Audience) bool {
	switch value {
	case Kids, Family, General, LateNight:
		return true
	default:
		return false
	}
}

func containsFold(haystack []string, needle string) bool {
	for _, value := range haystack {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func kindOverlap(a, b []Kind) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}

func audienceOverlap(a, b []Audience) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}

func stringOverlap(a, b []string) bool {
	for _, left := range a {
		if containsFold(b, left) {
			return true
		}
	}
	return false
}
