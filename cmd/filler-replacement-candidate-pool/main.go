// Command filler-replacement-candidate-pool joins qualified corpus and review
// authorities into the sole candidate input accepted by replacement planning.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/fillercandidatepool/build"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type repeatedPaths []string

func (paths *repeatedPaths) String() string { return fmt.Sprint([]string(*paths)) }
func (paths *repeatedPaths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("prior adjudication path cannot be empty")
	}
	*paths = append(*paths, value)
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-replacement-candidate-pool", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventory := flags.String("inventory", "", "current schema-v5 corpus inventory")
	rights := flags.String("rights-decisions", "", "locked development or certification rights decisions JSONL")
	profile := flags.String("rights-profile", "", "development or certification")
	materialization := flags.String("materialization-ledger", "", "exact development or certification materialization ledger")
	quarantineLedger := flags.String("quarantine-download-ledger", "", "exact quarantine download ledger for non-local cases")
	quarantineInspection := flags.String("quarantine-inspection", "", "exact quarantine inspection for non-local cases")
	selection := flags.String("selection", "", "frozen temporal selection JSON")
	evidence := flags.String("evidence", "", "public evidence manifest JSON")
	evidenceMap := flags.String("evidence-map", "", "private evidence map JSON")
	human := flags.String("human", "", "locked human assessment JSON")
	humanAttestation := flags.String("human-attestation", "", "locked human attestation JSON")
	quality := flags.String("media-quality", "", "full-decode media-quality report JSON")
	suitability := flags.String("suitability", "", "two-family suitability comparison JSON")
	referenceAudit := flags.String("reference-audit", "", "reference audit JSON")
	referenceLedger := flags.String("reference-download-ledger", "", "reference audit download ledger JSON")
	families := flags.String("families", "", "duplicate-family audit JSON")
	transitions := flags.String("transitions", "", "transition-edge authority JSON")
	sourceRoot := flags.String("source-root", "", "common root containing inspected and reviewed media")
	generatedText := flags.String("generated-at", "", "fixed RFC3339 generation time")
	output := flags.String("out", "", "new replacement candidate pool JSON")
	var prior repeatedPaths
	flags.Var(&prior, "prior-adjudication", "immutable burned-challenge adjudication; repeat for cumulative lineage")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedText)
	validProfile := *profile == fillercorpus.RightsProfileDevelopment || *profile == fillercorpus.RightsProfileCertification
	required := []*string{inventory, rights, selection, evidence, evidenceMap, human, humanAttestation, quality, suitability, referenceAudit, referenceLedger, families, transitions, sourceRoot, output}
	missing := false
	for _, value := range required {
		missing = missing || *value == ""
	}
	if err != nil || missing || !validProfile || len(prior) == 0 {
		_, _ = fmt.Fprintln(stderr, "filler-replacement-candidate-pool: inventory, rights decisions/profile, temporal selection, evidence/map, human assessment/attestation, media quality, suitability, reference audit/download ledger, families, transitions, source root, prior adjudication, fixed generation time, and output are required; non-local inventories also require materialization and quarantine ledgers plus inspection")
		return 2
	}
	result, err := build.Publish(build.Config{
		InventoryPath: *inventory, RightsDecisionsPath: *rights, RightsProfile: *profile,
		MaterializationLedgerPath: *materialization, QuarantineDownloadLedgerPath: *quarantineLedger,
		QuarantineInspectionPath: *quarantineInspection, SourceRoot: *sourceRoot,
		Review: fillerreview.ReplacementCandidateReviewConfig{
			SelectionPath: *selection, EvidenceManifestPath: *evidence, EvidencePrivateMapPath: *evidenceMap,
			HumanAssessmentPath: *human, HumanAttestationPath: *humanAttestation, MediaQualityPath: *quality,
			SuitabilityPath: *suitability, ReferenceAuditPath: *referenceAudit,
			ReferenceDownloadLedgerPath: *referenceLedger, FamilyAuditPath: *families,
			TransitionAuthorityPath: *transitions,
		},
		PriorAdjudicationPaths: prior, GeneratedAt: generatedAt.UTC(), OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-replacement-candidate-pool:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-replacement-candidate-pool: %d candidates; %d eligible; %d held; sha256 %s; training false; scheduling false; production admission false\n", result.Candidates, result.Eligible, result.Held, result.SHA256)
	return 0
}
