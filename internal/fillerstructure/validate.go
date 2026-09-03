package fillerstructure

import (
	"encoding/hex"
	"slices"
	"strings"
)

func invalidCandidates(request Request) bool {
	if request.Source.DurationMS <= 0 || !digest(request.Source.SHA256) || request.BoundaryToleranceMS < 0 || len(request.Candidates) < 2 {
		return true
	}
	assessors := make(map[string]struct{}, len(request.Candidates))
	families := make(map[string]struct{}, len(request.Candidates))
	for _, candidate := range request.Candidates {
		identity := candidate.Assessor
		family := strings.ToLower(strings.TrimSpace(identity.ModelFamily))
		if candidate.Source != request.Source || !canonicalIdentity(identity.ID) || family == "" || !canonicalIdentity(identity.Provider) || !canonicalIdentity(identity.Model) || !canonicalIdentity(identity.PromptVersion) || !canonicalIdentity(identity.EvidenceContract) || !digest(identity.ModelDigest) || !digest(identity.CapabilitySHA256) || !digest(identity.AssessmentSHA256) {
			return true
		}
		if _, duplicate := assessors[identity.ID]; duplicate {
			return true
		}
		assessors[identity.ID] = struct{}{}
		families[family] = struct{}{}
		if candidate.Failure != "" {
			if strings.TrimSpace(candidate.Failure) != candidate.Failure || len(candidate.Failure) > 64 || candidate.Unit != "" || candidate.Role != "" || len(candidate.Segments) != 0 {
				return true
			}
			continue
		}
		if !validUnit(candidate.Unit) || !validCandidateRole(candidate) || !completeTimeline(candidate.Segments, request.Source.DurationMS) {
			return true
		}
	}
	return len(families) < 2
}

func canonicalIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 256
}

func validCandidateRole(candidate Candidate) bool {
	if candidate.Unit == UnitStandalone {
		return fillerRole(candidate.Role) && len(candidate.Segments) == 1 && candidate.Segments[0].Role == candidate.Role
	}
	return candidate.Role == ""
}

func completeTimeline(segments []Segment, durationMS int64) bool {
	if len(segments) == 0 {
		return false
	}
	next := int64(0)
	for _, segment := range segments {
		if segment.StartMS != next || segment.EndMS <= segment.StartMS || segment.EndMS > durationMS || !validRole(segment.Role) {
			return false
		}
		next = segment.EndMS
	}
	return next == durationMS
}

func validUnit(unit Unit) bool {
	return slices.Contains([]Unit{UnitStandalone, UnitCompilation, UnitProgrammeExcerpt, UnitProgrammeSpots, UnitUnusable, UnitUnclear}, unit)
}

func validRole(role Role) bool {
	return fillerRole(role) || role == RoleProgrammeFragment || role == RoleNonFiller || role == RoleAmbiguous || role == RoleUnusable
}

func fillerRole(role Role) bool {
	return slices.Contains([]Role{RoleCommercial, RolePromo, RoleBumper, RolePSA, RoleStationID, RoleTrailer, RoleInterstitial}, role)
}

func digest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
