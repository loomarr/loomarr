package filler

import (
	"fmt"
	"sort"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func structureCertificationAuthorityIdentities(certification *StructureCertificationPolicy) (string, string) {
	if certification == nil {
		return "", ""
	}
	structureSHA := ""
	if certification.Authority != nil && fillerstructure.ValidateAuthority(*certification.Authority) == nil {
		structureSHA = certification.Authority.SHA256
	}
	screeningSHA := ""
	if certification.Screening != nil {
		screeningSHA = certification.Screening.AuthoritySHA256()
	}
	return structureSHA, screeningSHA
}

func structureScreeningIdentities(proposal SplitProposal) ([]string, error) {
	seen := make(map[string]struct{})
	identities := make([]string, 0, len(proposal.StructureScreenings)+len(proposal.Segments))
	add := func(evidence SegmentScreeningEvidence) error {
		if err := ValidateSegmentScreeningEvidence(evidence); err != nil {
			return err
		}
		if _, duplicate := seen[evidence.SHA256]; !duplicate {
			seen[evidence.SHA256] = struct{}{}
			identities = append(identities, evidence.SHA256)
		}
		return nil
	}
	for _, evidence := range proposal.StructureScreenings {
		if err := add(evidence); err != nil {
			return nil, fmt.Errorf("structure split shadow screening is invalid: %w", err)
		}
	}
	for _, segment := range proposal.Segments {
		if segment.Screening != nil {
			if err := add(*segment.Screening); err != nil {
				return nil, fmt.Errorf("structure split shadow segment screening is invalid: %w", err)
			}
		}
	}
	sort.Strings(identities)
	return identities, nil
}
