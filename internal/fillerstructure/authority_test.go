package fillerstructure

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestVerifyAuthorityRequiresExactCertifiedProfilesAndSlices(t *testing.T) {
	artifact, err := NewArtifact(fixtureRequest(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureAuthority(artifact)
	if err := VerifyAuthority(artifact, authority); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Authority)
	}{
		{name: "permission", mutate: func(a *Authority) { a.AutomaticMaterializationAllowed = false }},
		{name: "model", mutate: func(a *Authority) { a.Assessors[0].Model = "another-model" }},
		{name: "unit slice", mutate: func(a *Authority) { a.AllowedUnits = []Unit{UnitProgrammeSpots} }},
		{name: "role slice", mutate: func(a *Authority) { a.AllowedRoles = []Role{RoleCommercial} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := authority
			candidate.Assessors = slices.Clone(authority.Assessors)
			candidate.AllowedUnits = slices.Clone(authority.AllowedUnits)
			candidate.AllowedRoles = slices.Clone(authority.AllowedRoles)
			test.mutate(&candidate)
			candidate.SHA256 = AuthoritySHA256(candidate)
			if err := VerifyAuthority(artifact, candidate); err == nil {
				t.Fatal("decision escaped changed authority")
			}
		})
	}
}

func TestVerifyAuthorityRejectsHeldDecision(t *testing.T) {
	passing, err := NewArtifact(fixtureRequest(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := fixtureRequest()
	request.Candidates[1].Segments[0].Role = RolePSA
	held, err := NewArtifact(request, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyAuthority(held, fixtureAuthority(passing)); err == nil {
		t.Fatal("held decision was authorized")
	}
}

func fixtureAuthority(artifact Artifact) Authority {
	authority := Authority{
		SchemaVersion: AuthoritySchemaVersion, ContractVersion: AuthorityContractVersion,
		CertificateSHA256: strings.Repeat("f", 64), ReducerVersion: ReducerContractVersion,
		BoundaryToleranceMS: artifact.BoundaryToleranceMS, AllowedUnits: []Unit{UnitCompilation},
		AllowedRoles: []Role{RoleCommercial, RolePromo}, AutomaticMaterializationAllowed: true,
	}
	for _, candidate := range artifact.Decision.Candidates {
		authority.Assessors = append(authority.Assessors, Profile(candidate.Assessor))
	}
	authority.SHA256 = AuthoritySHA256(authority)
	return authority
}
