package filler

import (
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/split"
)

type structureDecisionIntervalAuthority struct {
	sha256 string
	role   split.Role
}

// structureDecisionAuthorityForInterval returns provenance only when the proposal is an exact
// projection of one confirmed artifact and this child is one of that decision's keep spans.
// Manual boundary edits and detector-only cuts deliberately return no model-decision identity.
//
// It returns the exact role as well as the artifact identity: materialization must not preserve
// the decision digest while throwing away the decision's answer.
func structureDecisionAuthorityForInterval(proposal SplitProposal, segment split.Segment) (structureDecisionIntervalAuthority, bool) {
	if proposal.Structure == nil || proposal.StructureDecision == nil ||
		proposal.Structure.Source != proposal.Source ||
		split.ValidateDecisionProjection(*proposal.Structure, *proposal.StructureDecision) != nil {
		return structureDecisionIntervalAuthority{}, false
	}
	for _, planned := range proposal.Structure.Plan {
		if planned.Disposition == split.DispositionKeep && split.CertifiedFillerRole(planned.Role) && planned.StartMs == segment.StartMs && planned.EndMs == segment.EndMs {
			return structureDecisionIntervalAuthority{sha256: proposal.StructureDecision.SHA256, role: planned.Role}, true
		}
	}
	return structureDecisionIntervalAuthority{}, false
}

func catalogRoleForStructureRole(role split.Role) (clips.Role, bool) {
	switch role {
	case split.RoleCommercial:
		return clips.Commercial, true
	case split.RolePromo, split.RoleInterstitial:
		return clips.Interstitial, true
	case split.RoleBumper:
		return clips.Bumper, true
	case split.RoleStationID:
		return clips.StationID, true
	case split.RolePSA:
		return clips.PSA, true
	case split.RoleTrailer:
		return clips.Trailer, true
	default:
		return "", false
	}
}
