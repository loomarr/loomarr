package filler

import (
	"strings"
	"testing"
)

func TestSidecarRejectsMalformedSegmentScreeningReference(t *testing.T) {
	for _, raw := range []string{
		`{"loomarr":{"segmentScreening":null}}`,
		`{"loomarr":{"segmentScreening":{}}}`,
		`{"loomarr":{"segmentScreening":{"schemaVersion":1,"subjectSha256":"bad","evidenceSha256":"bad"}}}`,
	} {
		if _, state, _ := decodeSidecarTags([]byte(raw)); state != SidecarInvalid {
			t.Fatalf("malformed reference state=%v, want invalid: %s", state, raw)
		}
	}
}

func TestSidecarRequiresStructureDecisionAndRoleTogether(t *testing.T) {
	lineage := `"childHash":"` + strings.Repeat("1", 64) + `","parentHash":"` + strings.Repeat("2", 64) + `","intendedStartMs":0,"intendedEndMs":30000,`
	for _, raw := range []string{
		`{"loomarr":{"conditioningLineage":{` + lineage + `"structureDecisionSha256":"` + strings.Repeat("3", 64) + `"}}}`,
		`{"loomarr":{"conditioningLineage":{` + lineage + `"structureRole":"commercial"}}}`,
	} {
		if _, state, _ := decodeSidecarTags([]byte(raw)); state != SidecarInvalid {
			t.Fatalf("unpaired structure authority state=%v, want invalid: %s", state, raw)
		}
	}
}
