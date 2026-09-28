package fillerstore

import (
	"context"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/store/storetest"
)

// NewStoreFunc builds a fresh, migrated, empty extended store for one test. Like the core suite's,
// it hides the backend, so every assertion below runs identically on SQLite and Postgres.
type NewStoreFunc func(t *testing.T) Store

// RunConformance is the filler half of the store conformance suite (AGENTS.md: one suite, two
// backends). It runs against the extended store the app holds, so clip-catalog assertions whose
// methods still live in the core store run here too, beside the filler state they share tables
// with (#1747).
func RunConformance(t *testing.T, newStore NewStoreFunc) {
	t.Run("Filler", func(t *testing.T) {
		t.Run("ClipRoundTripAndFilters", func(t *testing.T) { testClipFilters(t, newStore) })
		t.Run("ClipTagsAndPrune", func(t *testing.T) { testClipTagsAndPrune(t, newStore) })
		t.Run("ClipNameSearch", func(t *testing.T) { testClipNameSearch(t, newStore) })
		t.Run("ClipPlayCounters", func(t *testing.T) { testClipPlayCounters(t, newStore) })
		t.Run("ClipExposureRotation", func(t *testing.T) { testClipExposureRotation(t, newStore) })
		t.Run("ClipKeyIsHashNotPath", func(t *testing.T) { testClipKeyIsHashNotPath(t, newStore) })
		t.Run("ClipIdentityReplacement", func(t *testing.T) { testClipIdentityReplacement(t, newStore) })
		t.Run("ConditioningPublicationCommit", func(t *testing.T) { testConditioningPublicationCommit(t, newStore) })
		t.Run("ConditioningPublicationFailsClosed", func(t *testing.T) {
			testConditioningPublicationFailsClosed(t, newStore)
		})
		t.Run("ClipIdentityReplacementSameHash", func(t *testing.T) { testClipIdentityReplacementSameHash(t, newStore) })
		t.Run("ClipFingerprintCache", func(t *testing.T) { testClipFingerprintCache(t, newStore) })
		t.Run("ClipCounts", func(t *testing.T) { testClipCounts(t, newStore) })
		t.Run("ClipLicense", func(t *testing.T) { testClipLicense(t, newStore) })
		t.Run("ClipHeldLifecycle", func(t *testing.T) { testClipHeld(t, newStore) })
		t.Run("ClipPaging", func(t *testing.T) { testClipPaging(t, newStore) })
		t.Run("ClipSearchWidened", func(t *testing.T) { testClipSearchWidened(t, newStore) })
		t.Run("ClipTopLevelOnly", func(t *testing.T) { testClipTopLevelOnly(t, newStore) })
		t.Run("ClipCreatedAt", func(t *testing.T) { testClipCreatedAt(t, newStore) })
		t.Run("FillerProgressiveEnrichment", func(t *testing.T) { testFillerProgressiveEnrichment(t, newStore) })
		t.Run("FillerContextResearch", func(t *testing.T) { testFillerContextResearch(t, newStore) })
		t.Run("FillerResearchWebUsage", func(t *testing.T) { testFillerResearchWebUsage(t, newStore) })
		t.Run("CompositeLineage", func(t *testing.T) { testCompositeLineage(t, newStore) })
		t.Run("SplitConfirmationAtomic", func(t *testing.T) { testSplitConfirmationAtomic(t, newStore) })
		t.Run("SplitConfirmationRequiresReviewParent", func(t *testing.T) {
			testSplitConfirmationRequiresReviewParent(t, newStore)
		})
		t.Run("SplitProposalClaimFencesConfirmers", func(t *testing.T) {
			testSplitProposalClaimFencesConfirmers(t, newStore)
		})
		t.Run("FillerSourceRegistry", func(t *testing.T) { testFillerSources(t, newStore) })
		t.Run("FillerProviderPolicy", func(t *testing.T) { testFillerProviderPolicy(t, newStore) })
		t.Run("SeededDefaultSources", func(t *testing.T) { testSeededDefaultSources(t, newStore) })
		t.Run("FillerPulls", func(t *testing.T) { testFillerPulls(t, newStore) })
		t.Run("FillerAcquisitionRuns", func(t *testing.T) { testFillerAcquisitionRuns(t, newStore) })
		t.Run("FillerAcquisitionArtifacts", func(t *testing.T) { testFillerAcquisitionArtifacts(t, newStore) })
		t.Run("FillerPullCommit", func(t *testing.T) { testFillerPullCommit(t, newStore) })
		t.Run("FillerAcquisitionRepairSummary", func(t *testing.T) { testFillerAcquisitionRepairSummary(t, newStore) })
		t.Run("InteractiveOperations", func(t *testing.T) { testInteractiveOperations(t, newStore) })
		t.Run("FillerInferenceAccountingAndBudgets", func(t *testing.T) { testFillerInferenceAccountingAndBudgets(t, newStore) })
		t.Run("FillerStructureAssessmentLedger", func(t *testing.T) { testFillerStructureAssessmentLedger(t, newStore) })
		t.Run("FillerStructureWindowCallLedger", func(t *testing.T) { testFillerStructureWindowCallLedger(t, newStore) })
		t.Run("FillerAdmissionDecisionAudit", func(t *testing.T) { testFillerAdmissionDecisionAudit(t, newStore) })
		t.Run("FillerDiagnosticRecoveryActions", func(t *testing.T) { testFillerDiagnosticRecoveryActions(t, newStore) })
		t.Run("FillerAppliedAdmissionTransaction", func(t *testing.T) { testFillerAppliedAdmissionTransaction(t, newStore) })
		t.Run("FillerTerminalReadyTransaction", func(t *testing.T) { testFillerTerminalReadyTransaction(t, newStore) })
		t.Run("FillerSplitShadowDecisions", func(t *testing.T) { testFillerSplitShadowDecisions(t, newStore) })
		t.Run("FillerSpokenSafetyLedger", func(t *testing.T) { testFillerSpokenSafetyLedger(t, newStore) })
		t.Run("FillerSpokenSafetyExecutionPort", func(t *testing.T) { testFillerSpokenSafetyExecutionPort(t, newStore) })
		t.Run("SplitProposals", func(t *testing.T) { testSplitProposals(t, newStore) })
		t.Run("ClipPipelineState", func(t *testing.T) { testClipPipeline(t, newStore) })
		t.Run("ClipPipelineOverview", func(t *testing.T) { testClipPipelineOverview(t, newStore) })
		t.Run("ClipPipelineRetry", func(t *testing.T) { testClipPipelineRetry(t, newStore) })
		t.Run("IncomingConveyorCount", func(t *testing.T) { testIncomingConveyorCount(t, newStore) })
		t.Run("Taxonomy", func(t *testing.T) { testTaxonomy(t, newStore) })
		t.Run("CoreTransactionSpansFillerTables", func(t *testing.T) { testCoreTransactionSpansFillerTables(t, newStore) })
	})
}

// reopenURLs remembers the DATABASE_URL each factory store was opened from, so the restart tests
// can open a second pool on the same database without reaching into the core adapter.
var reopenURLs sync.Map // Store -> string

// openExtended is the production open path (fillerstore.Open), recording the URL it used.
func openExtended(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	s, err := Open(ctx, databaseURL, autoMigrate)
	if err == nil {
		reopenURLs.Store(s, databaseURL)
	}
	return s, err
}

func reopenURL(t *testing.T, s Store) string {
	t.Helper()
	url, ok := reopenURLs.Load(s)
	if !ok {
		t.Fatalf("store %T was not opened by the conformance factory", s)
	}
	return url.(string)
}

func checkpointSQLite(s Store) error {
	_, err := store.HandleOf(s).ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// TestSQLiteConformance runs the filler suite against SQLite; postgres_test.go runs the same
// suite against Postgres under the integration tag.
func TestSQLiteConformance(t *testing.T) {
	t.Parallel()
	RunConformance(t, storetest.SQLiteClones(t, openExtended, checkpointSQLite))
}
