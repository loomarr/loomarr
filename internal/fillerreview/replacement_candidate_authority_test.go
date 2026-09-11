package fillerreview

import "testing"

func TestOpenReplacementCandidateReviewAuthorityReproducesExistingLocks(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	authority, err := OpenReplacementCandidateReviewAuthority(ReplacementCandidateReviewConfig{
		SelectionPath: fixture.selection, EvidenceManifestPath: fixture.manifest, EvidencePrivateMapPath: fixture.privateMap,
		HumanAssessmentPath: fixture.humanAssessment, HumanAttestationPath: fixture.humanAttestation,
		MediaQualityPath: fixture.quality, SuitabilityPath: fixture.suitability,
		ReferenceAuditPath: fixture.referenceAudit, ReferenceDownloadLedgerPath: fixture.referenceDownloadLedger,
		FamilyAuditPath: fixture.family, TransitionAuthorityPath: fixture.transition,
		SourceRoot: fixture.root, OpenedAt: fixture.plannedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(authority.Cases) != 48 || len(authority.Inputs) != 11 {
		t.Fatalf("authority cases=%d inputs=%d", len(authority.Cases), len(authority.Inputs))
	}
	for _, item := range authority.Cases {
		if item.CaseID == "" || item.EvidenceAlias == "" || item.InspectedSourceFile == "" || item.InspectedSourceSHA == "" || item.ReviewedMediaPath == "" || item.ReviewedMediaSHA == "" || item.FamilyID == "" || item.Transition.CaseID != item.CaseID {
			t.Fatalf("incomplete normalized case: %+v", item)
		}
	}
}
