package filler

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// PlanAcquisition applies hard constraints first, then selects a diverse stable prefix.
func PlanAcquisition(intent AcquisitionIntent, candidates []AcquisitionCandidate, existing map[string]ExistingRemoteState) (AcquisitionPlan, error) {
	return PlanAcquisitionFor(intent, candidates, existing, CoverageGaps{})
}

// PlanAcquisitionFor is PlanAcquisition steered toward channel coverage gaps (#749): a candidate
// that fills a gap (its observed year in a gap era, or its observed role a missing role) ranks
// ahead of every candidate that does not.
//
// ⚠ A PREFERENCE, never a constraint, and deliberately not part of the persisted intent. A gap
// ranks what a bounded pass takes first; it never rejects a candidate, so a source whose items
// carry no year still supplies the catalog, and stored pulls keep decoding under the same intent
// version. Hard constraints still run first and are unchanged.
func PlanAcquisitionFor(intent AcquisitionIntent, candidates []AcquisitionCandidate, existing map[string]ExistingRemoteState, gaps CoverageGaps) (AcquisitionPlan, error) {
	intent = intent.Normalize()
	if err := intent.Validate(); err != nil {
		return AcquisitionPlan{}, fmt.Errorf("%w: %v", ErrInvalidAcquisitionIntent, err)
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
			return candidateBetter(eligible[a].Candidate, eligible[b].Candidate, usedSources, usedYears, gaps)
		})
		decision := eligible[0]
		eligible = eligible[1:]
		decision.Disposition = CandidateSelected
		decision.Detail = "selected by deterministic quality, diversity, and identity ranking"
		if gap, ok := gapFilled(decision.Candidate, gaps); ok {
			decision.Gap = gap
			decision.Detail = "selected first: it fills a channel coverage gap (" + gap + ")"
			if strings.HasPrefix(gap, "era:") {
				decision.Detail = fmt.Sprintf("selected first: its year %d falls in a channel coverage gap", decision.Candidate.ObservedYear)
			}
		}
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
		Count: 12, CatalogReason: reason,
	}
}

func rejectByIntent(intent AcquisitionIntent, c AcquisitionCandidate) (CandidateDisposition, string) {
	if len(intent.SourceAllowlist) > 0 && !containsFold(intent.SourceAllowlist, c.Identity.SourceID) {
		return CandidateSourceNotAllowed, "registered source is outside the intent allow-list"
	}
	if intent.Geography.Country != "" && !SourceGeographicallyEligible(c.Geography, intent.Geography) {
		return CandidateGeographyMismatch, "candidate geography does not cover the target"
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

func candidateBetter(a, b AcquisitionCandidate, usedSources map[string]bool, usedYears map[int]bool, gaps CoverageGaps) bool {
	var av, bv bool
	// Relevance to a channel that is short of material outranks representation quality: a sharp
	// modern spot does nothing for a channel whose breaks cannot fill from its own era.
	_, av = gapFilled(a, gaps)
	_, bv = gapFilled(b, gaps)
	if av != bv {
		return av
	}
	if a.Height != b.Height {
		return a.Height > b.Height
	}
	av, bv = !usedSources[a.Identity.SourceID], !usedSources[b.Identity.SourceID]
	if av != bv {
		return av
	}
	av, bv = a.ObservedYear > 0 && !usedYears[a.ObservedYear], b.ObservedYear > 0 && !usedYears[b.ObservedYear]
	if av != bv {
		return av
	}
	return a.Identity.Key() < b.Identity.Key()
}

// CoverageGaps are what the live channels' breaks are short of (#749), read from the same pool
// report the pool strip and readiness show.
type CoverageGaps struct {
	// Eras are the era windows of channels whose breaks cannot fill from their own era.
	Eras []EraRange
	// Roles are break roles the catalog lacks entirely: bookends (bumpers and station IDs) when
	// no live channel's pod can open or close on one.
	Roles []Kind
}

// gapFilled returns the gap key a candidate fills, era first. Only OBSERVED facts count: a
// candidate with no observed year fills no era gap, and one with no role token fills no role gap,
// because missing metadata never satisfies a target (see AcquisitionIntent).
func gapFilled(c AcquisitionCandidate, gaps CoverageGaps) (string, bool) {
	if c.ObservedYear > 0 {
		for _, r := range gaps.Eras {
			if !r.Any() && r.Contains(c.ObservedYear) {
				return EraGapKey(r), true
			}
		}
	}
	for _, role := range observedRoles(c) {
		if slices.Contains(gaps.Roles, role) {
			return RoleGapKey(role), true
		}
	}
	return "", false
}

// observedRoles are the provider's observed roles or, failing those, the explicit role token in
// the item's title: the same KindFromName rule intake applies to the downloaded file's name, so an
// item steered here as an ident is classified as one on arrival. Ranking evidence only.
func observedRoles(c AcquisitionCandidate) []Kind {
	if len(c.ObservedRoles) > 0 {
		return c.ObservedRoles
	}
	if kind := KindFromName(c.Title); kind != Unclassified {
		return []Kind{kind}
	}
	return nil
}

// RoleGapKey is the stable record of a role gap an acquisition was for: "role:station_id".
func RoleGapKey(role Kind) string { return "role:" + string(role) }

// EraGapKey is the stable record of an era gap an acquisition was for (#749): "era:1990-1999",
// "era:2005-" or "era:-1979"; "" for any era, which is never a gap. The "era:" prefix leaves room
// for the other gap kinds a download can be for, without a schema change.
func EraGapKey(r EraRange) string {
	if r.Any() {
		return ""
	}
	key := "era:"
	if r.From > 0 {
		key += fmt.Sprint(r.From)
	}
	key += "-"
	if r.To > 0 {
		key += fmt.Sprint(r.To)
	}
	return key
}

// CoverageGapsFrom derives the gaps from the per-channel coverage the pool strip and readiness
// show, so acquisition steers by the answer operators see rather than a second opinion (#749).
//
// Era gaps are the era windows of live channels whose breaks cannot fill from their own era
// (Coverage.Level below exact, down to the bumper card); channels with no era target are skipped
// (any era is their exact rung) and overlapping windows merge. The role gap is bookends: live
// channels and not one bumper or station ID in the catalog. Audience is deliberately not a gap
// kind: no provider observes a remote item's audience before download, so steering by it would
// be a guess.
func CoverageGapsFrom(pool PoolReport) CoverageGaps {
	var eras []EraRange
	for _, ch := range pool.Channels {
		if ch.Report.Level == MatchExact {
			continue
		}
		for _, r := range ch.Report.EraWindows {
			if !r.Any() {
				eras = append(eras, r)
			}
		}
	}
	gaps := CoverageGaps{Eras: NormalizeEraWindows(eras)}
	if len(pool.Channels) > 0 && pool.Bookends == 0 {
		gaps.Roles = []Kind{Bumper, StationID}
	}
	return gaps
}

// formatEraWindows renders gap windows for a reason line: "1990-1999, 2005 onwards".
func formatEraWindows(windows []EraRange) string {
	parts := make([]string, 0, len(windows))
	for _, r := range windows {
		switch {
		case r.From > 0 && r.To > 0 && r.From == r.To:
			parts = append(parts, fmt.Sprint(r.From))
		case r.From > 0 && r.To > 0:
			parts = append(parts, fmt.Sprintf("%d-%d", r.From, r.To))
		case r.From > 0:
			parts = append(parts, fmt.Sprintf("%d onwards", r.From))
		case r.To > 0:
			parts = append(parts, fmt.Sprintf("up to %d", r.To))
		}
	}
	return strings.Join(parts, ", ")
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
