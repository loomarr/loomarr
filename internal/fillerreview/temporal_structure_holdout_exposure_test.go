package fillerreview

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercandidatepool"
)

func TestLoadTemporalStructureHoldoutPriorAcceptsPublishedAdjudication(t *testing.T) {
	fixture := newTemporalStructureAnchorAdjudicationFixture(t)
	path := filepath.Join(t.TempDir(), "authority.json")
	if _, err := PublishTemporalStructureAnchorAdjudication(fixture.config(path)); err != nil {
		t.Fatal(err)
	}
	prior, err := loadTemporalStructureHoldoutPrior(TemporalStructureHoldoutConfig{
		PriorAdjudicationPaths: []string{path}, PlannedAt: fixture.adjudicatedAt.Add(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The repaired fixture's complete burned lineage contains 78 source hashes, 12 family IDs,
	// and 6 programme parents; keep these cardinalities explicit rather than merely non-empty.
	if prior.planKind != TemporalStructureHoldoutPlanReplacement || len(prior.inputs) != 1 || len(prior.exposure.SourceSHA256) != 78 || len(prior.exposure.FamilyIDs) != 12 || len(prior.exposure.ProgrammeProvenance) != 6 {
		t.Fatalf("loaded published prior = %+v", prior)
	}
}

func TestBuildTemporalStructureReplacementHoldoutCarriesCumulativeExposure(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	prior := emptyTemporalStructureHoldoutExposure()
	prior.SourceSHA256 = []string{strings.Repeat("e", 64)}
	prior.FamilyIDs = []string{"prior-family"}
	prior.ProgrammeProvenance = []TemporalStructureHoldoutProgrammeProvenance{{Authority: "prior-authority", Reference: "prior-reference"}}
	priorPath := writeTemporalStructurePriorAdjudicationFixture(t, fixture, prior)
	config := fixture.config(filepath.Join(t.TempDir(), "replacement"))
	config.Genesis = false
	config.PriorAdjudicationPaths = []string{priorPath}
	config.CandidatePoolPath = writeTemporalStructureCandidatePoolFixture(t, fixture, prior)
	clearTemporalStructureGenesisInputs(&config)
	if _, err := BuildTemporalStructureHoldoutPlan(config); err != nil {
		t.Fatal(err)
	}
	receipt := readStrictTestJSON[TemporalStructureHoldoutReceipt](t, filepath.Join(config.OutputDir, "receipt.json"))
	if receipt.PlanKind != TemporalStructureHoldoutPlanReplacement || !equalTemporalStructureHoldoutExposure(receipt.PriorExposure, prior) {
		t.Fatalf("replacement lineage = kind %q prior %+v", receipt.PlanKind, receipt.PriorExposure)
	}
	if len(receipt.FutureTrainingExclusion.SourceSHA256) != 19 || len(receipt.FutureTrainingExclusion.FamilyIDs) != 13 || len(receipt.FutureTrainingExclusion.ProgrammeProvenance) != 7 {
		t.Fatalf("cumulative exposure = %+v", receipt.FutureTrainingExclusion)
	}
	foundPriorInput := false
	for _, input := range receipt.Inputs {
		foundPriorInput = foundPriorInput || strings.HasPrefix(input.Name, "prior_adjudication:")
	}
	if !foundPriorInput {
		t.Fatal("replacement receipt omitted its prior adjudication digest")
	}
}

func TestBuildTemporalStructureReplacementHoldoutRejectsPriorRequestLeakage(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	genesisRoot := filepath.Join(t.TempDir(), "genesis")
	if _, err := BuildTemporalStructureHoldoutPlan(fixture.config(genesisRoot)); err != nil {
		t.Fatal(err)
	}
	receipt := readStrictTestJSON[TemporalStructureHoldoutReceipt](t, filepath.Join(genesisRoot, "receipt.json"))
	authoring := readStrictTestJSON[TemporalStructureChallengeAuthoring](t, filepath.Join(genesisRoot, "authoring.json"))
	sources := make(map[string]TemporalStructureChallengeSource, len(authoring.Sources))
	for _, source := range authoring.Sources {
		sources[source.ID] = source
	}
	anchor := receipt.SelectedAnchors[0]
	programme := receipt.FutureTrainingExclusion.ProgrammeProvenance[0]
	tests := []struct {
		name     string
		exposure TemporalStructureHoldoutTrainingExclusion
		want     string
	}{
		{
			name: "rendered or source bytes",
			exposure: TemporalStructureHoldoutTrainingExclusion{
				Split: "holdout", SourceSHA256: []string{sources[anchor.SourceID].SHA256}, FamilyIDs: []string{"prior-family"},
				ProgrammeProvenance: []TemporalStructureHoldoutProgrammeProvenance{},
			},
			want: "insufficient eligible",
		},
		{
			name: "duplicate family",
			exposure: TemporalStructureHoldoutTrainingExclusion{
				Split: "holdout", SourceSHA256: []string{strings.Repeat("e", 64)}, FamilyIDs: []string{anchor.FamilyID},
				ProgrammeProvenance: []TemporalStructureHoldoutProgrammeProvenance{},
			},
			want: "insufficient eligible",
		},
		{
			name: "programme provenance",
			exposure: TemporalStructureHoldoutTrainingExclusion{
				Split: "holdout", SourceSHA256: []string{strings.Repeat("e", 64)}, FamilyIDs: []string{"prior-family"},
				ProgrammeProvenance: []TemporalStructureHoldoutProgrammeProvenance{programme},
			},
			want: "needs six eligible programme parents",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			priorPath := writeTemporalStructurePriorAdjudicationFixture(t, fixture, test.exposure)
			config := fixture.config(filepath.Join(t.TempDir(), "replacement"))
			config.Genesis = false
			config.PriorAdjudicationPaths = []string{priorPath}
			config.CandidatePoolPath = writeTemporalStructureCandidatePoolFixture(t, fixture, test.exposure)
			clearTemporalStructureGenesisInputs(&config)
			_, err := BuildTemporalStructureHoldoutPlan(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func clearTemporalStructureGenesisInputs(config *TemporalStructureHoldoutConfig) {
	config.SelectionPath = ""
	config.EvidenceManifestPath = ""
	config.EvidencePrivateMapPath = ""
	config.HumanAssessmentPath = ""
	config.HumanAttestationPath = ""
	config.MediaQualityPath = ""
	config.SuitabilityPath = ""
	config.ReferenceAuditPath = ""
	config.ReferenceDownloadLedgerPath = ""
	config.FamilyAuditPath = ""
	config.TransitionAuthorityPath = ""
	config.ProgrammeInventoryPath = ""
}

func writeTemporalStructureCandidatePoolFixture(t *testing.T, fixture temporalStructureHoldoutFixture, prior TemporalStructureHoldoutTrainingExclusion) string {
	t.Helper()
	genesisRoot := filepath.Join(t.TempDir(), "pool-genesis")
	if _, err := BuildTemporalStructureHoldoutPlan(fixture.config(genesisRoot)); err != nil {
		t.Fatal(err)
	}
	authoring := readStrictTestJSON[TemporalStructureChallengeAuthoring](t, filepath.Join(genesisRoot, "authoring.json"))
	receipt := readStrictTestJSON[TemporalStructureHoldoutReceipt](t, filepath.Join(genesisRoot, "receipt.json"))
	transition := readStrictTestJSON[TemporalTransitionAuthority](t, fixture.transition)
	transitionByAlias := make(map[string]TemporalTransitionAuthorityCase, len(transition.Cases))
	for _, item := range transition.Cases {
		transitionByAlias[item.EvidenceAlias] = item
	}
	sourceByID := make(map[string]TemporalStructureChallengeSource, len(authoring.Sources))
	for _, source := range authoring.Sources {
		sourceByID[source.ID] = source
	}
	pool := fillercandidatepool.Pool{
		SchemaVersion: fillercandidatepool.SchemaVersion, ContractVersion: fillercandidatepool.ContractVersion,
		GeneratedAt:   fixture.plannedAt.Add(-time.Minute),
		Inputs:        []fillercandidatepool.Input{{Name: "fixture_authority_bundle", SHA256: strings.Repeat("f", 64)}},
		PriorExposure: candidatePoolExposure(prior),
	}
	for _, anchor := range receipt.SelectedAnchors {
		source := sourceByID[anchor.SourceID]
		measured := transitionByAlias[anchor.EvidenceAlias]
		candidate := fillercandidatepool.Candidate{
			CaseID: "fixture.example/" + source.ID, Kind: fillercandidatepool.KindStandaloneAnchor,
			Disposition: fillercandidatepool.DispositionEligible, HoldReasons: []string{}, FamilyID: anchor.FamilyID,
			Role: string(anchor.Role), Transition: candidatePoolTransitionFixture(measured),
			Source: candidatePoolSourceFixture(t, fixture.root, source, "fixture.example", source.ID),
		}
		if containsString(prior.SourceSHA256, source.SHA256) {
			candidate.Disposition, candidate.HoldReasons = fillercandidatepool.DispositionHeld, []string{fillercandidatepool.HoldPriorSource}
			candidate.Role, candidate.Transition = "", nil
		} else if containsString(prior.FamilyIDs, anchor.FamilyID) {
			candidate.Disposition, candidate.HoldReasons = fillercandidatepool.DispositionHeld, []string{fillercandidatepool.HoldPriorFamily}
			candidate.Role, candidate.Transition = "", nil
		}
		pool.Candidates = append(pool.Candidates, candidate)
	}
	for _, source := range authoring.Sources {
		if source.Provenance.Kind != TemporalStructureSourceProgrammeParent {
			continue
		}
		candidate := fillercandidatepool.Candidate{
			CaseID: source.Provenance.Authority + "/" + source.Provenance.ItemID,
			Kind:   fillercandidatepool.KindProgrammeParent, Disposition: fillercandidatepool.DispositionEligible,
			HoldReasons: []string{}, FamilyID: "programme-family-" + source.SHA256[:24],
			Source: candidatePoolSourceFixture(t, fixture.root, source, source.Provenance.Authority, source.Provenance.ItemID),
		}
		provenance := TemporalStructureHoldoutProgrammeProvenance{Authority: source.Provenance.Authority, Reference: source.Provenance.Reference}
		if containsString(prior.SourceSHA256, source.SHA256) {
			candidate.Disposition, candidate.HoldReasons = fillercandidatepool.DispositionHeld, []string{fillercandidatepool.HoldPriorSource}
		} else if containsProgrammeProvenance(prior.ProgrammeProvenance, provenance) {
			candidate.Disposition, candidate.HoldReasons = fillercandidatepool.DispositionHeld, []string{fillercandidatepool.HoldPriorProgrammeProvenance}
		}
		pool.Candidates = append(pool.Candidates, candidate)
	}
	sort.Slice(pool.Candidates, func(i, j int) bool { return pool.Candidates[i].CaseID < pool.Candidates[j].CaseID })
	return writeTemporalHumanJSON(t, t.TempDir(), "candidate-pool.json", pool)
}

func candidatePoolSourceFixture(t *testing.T, root string, source TemporalStructureChallengeSource, authority, itemID string) fillercandidatepool.Source {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(source.Path)))
	if err != nil {
		t.Fatal(err)
	}
	itemURL := source.Provenance.Reference
	if !strings.HasPrefix(itemURL, "https://") {
		itemURL = "https://fixture.example/items/" + itemID
	}
	return fillercandidatepool.Source{
		ID: source.ID, Path: source.Path, SHA256: source.SHA256, Bytes: info.Size(), DurationMS: source.DurationMS,
		Transport: fillercandidatepool.TransportLocal, Authority: authority, ItemID: itemID,
		ItemURL: itemURL, MetadataSHA256: source.Provenance.MetadataSHA256,
		MetadataRetrievedAt: source.Provenance.RetrievedAt, SoundtrackStatus: "present_expected",
		SoundtrackEvidence: "reviewed_source_manifest", SoundtrackEvidenceSHA: source.Provenance.MetadataSHA256,
	}
}

func candidatePoolTransitionFixture(value TemporalTransitionAuthorityCase) *fillercandidatepool.Transition {
	return &fillercandidatepool.Transition{
		EvidenceAlias: value.EvidenceAlias, Head: candidatePoolEdgeFixture(value.Head), Tail: candidatePoolEdgeFixture(value.Tail),
	}
}

func candidatePoolEdgeFixture(value TemporalTransitionEdge) fillercandidatepool.Edge {
	result := fillercandidatepool.Edge{StartMS: value.StartMS, EndMS: value.EndMS, RMSMilliDBFS: value.RMSMilliDBFS, PeakMilliDBFS: value.PeakMilliDBFS}
	for _, interval := range value.Black {
		result.Black = append(result.Black, fillercandidatepool.Interval{StartMS: interval.StartMs, EndMS: interval.EndMs})
	}
	for _, interval := range value.Silence {
		result.Silence = append(result.Silence, fillercandidatepool.Interval{StartMS: interval.StartMs, EndMS: interval.EndMs})
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsProgrammeProvenance(values []TemporalStructureHoldoutProgrammeProvenance, want TemporalStructureHoldoutProgrammeProvenance) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeTemporalStructurePriorAdjudicationFixture(t *testing.T, fixture temporalStructureHoldoutFixture, exposure TemporalStructureHoldoutTrainingExclusion) string {
	t.Helper()
	inputs := []TemporalStructureHoldoutInput{
		{Name: "comparison", SHA256: strings.Repeat("1", 64)},
		{Name: "plan_authoring", SHA256: strings.Repeat("2", 64)},
		{Name: "plan_receipt", SHA256: strings.Repeat("3", 64)},
		{Name: "private_authority", SHA256: strings.Repeat("4", 64)},
		{Name: "public_manifest", SHA256: strings.Repeat("5", 64)},
		{Name: "submission", SHA256: strings.Repeat("6", 64)},
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	authority := TemporalStructureAnchorAdjudicationAuthority{
		SchemaVersion:   TemporalStructureAnchorAdjudicationSchemaVersion,
		ContractVersion: TemporalStructureAnchorAdjudicationAuthorityContract,
		ChallengeID:     "prior-challenge", AdjudicatedAt: fixture.plannedAt.Add(-1), ReviewerID: "prior-reviewer",
		Inputs: inputs, EvidenceManifestSHA256: strings.Repeat("7", 64), HumanAssessmentSHA256: strings.Repeat("8", 64),
		PlanReceiptSHA256: strings.Repeat("9", 64), ComparisonSHA256: strings.Repeat("a", 64), PriorExposure: exposure,
		Cases: []TemporalStructureAnchorAdjudicationCase{{
			Alias: "case-prior", EvidenceAlias: "evidence-prior", CaseID: "source-case-prior", SourceID: "source-prior",
			SourceSHA256: exposure.SourceSHA256[0], FamilyID: exposure.FamilyIDs[0], DurationMS: 1_000,
			Coverage: TemporalStructureAnchorReviewComplete,
			Observations: TemporalStructureAnchorObservations{
				Opening: "The opening was reviewed.", InternalJoins: []TemporalStructureAnchorJoinObservation{}, Closing: "The closing was reviewed.",
			},
			Disposition:  TemporalStructureAnchorConfirmed,
			Original:     TemporalStructureTruthLabel{Unit: "standalone", Role: "commercial"},
			Adjudicated:  TemporalStructureTruthLabel{Unit: "standalone", Role: "commercial"},
			DecisiveAtMS: []int64{100}, Rationale: "Complete audiovisual fixture review confirms the original bounded item.",
		}},
		ChallengeDisposition: TemporalStructureBurnedDiagnosticOnly,
	}
	return writeTemporalHumanJSON(t, t.TempDir(), "prior-adjudication.json", authority)
}
