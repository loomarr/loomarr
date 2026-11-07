package filler

import (
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

func structureMaterializationAuthorityIdentity(materialization *StructureMaterializationPolicy, artifact *structure.Artifact) string {
	if materialization == nil || artifact == nil {
		return ""
	}
	switch artifact.Decision.Input.Kind {
	case structure.AssessmentInputCompleteVideo:
		if materialization.Authority != nil && structure.ValidateAuthority(*materialization.Authority) == nil {
			return materialization.Authority.SHA256
		}
	case structure.AssessmentInputWindowMediaSet:
		if materialization.WindowAuthority != nil && window.ValidateMaterializationAuthority(*materialization.WindowAuthority) == nil {
			return materialization.WindowAuthority.SHA256
		}
	}
	return ""
}
