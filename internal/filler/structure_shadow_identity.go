package filler

import (
	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func structureMaterializationAuthorityIdentity(materialization *StructureMaterializationPolicy) string {
	if materialization == nil {
		return ""
	}
	if materialization.Authority != nil && fillerstructure.ValidateAuthority(*materialization.Authority) == nil {
		return materialization.Authority.SHA256
	}
	return ""
}
