//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestCompileMoodReviewAuthorityPreservesFrozenV1Evidence(t *testing.T) {
	t.Parallel()

	read := func(path string) []byte {
		t.Helper()
		blob, err := queryPilotFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return blob
	}
	compiled, err := CompileMoodReviewAuthority(
		read("testdata/query-mood-review-packet-v1.json"),
		read("testdata/query-mood-review-map-v1.json"),
		read("testdata/query-mood-review-submission-qwen-v1.json"),
		read("testdata/query-mood-review-submission-gemini-v1.json"),
		read("testdata/query-mood-review-submission-gemma-v1.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var frozen MoodReviewAuthority
	if err := json.Unmarshal(read("testdata/query-mood-review-authority-v1.json"), &frozen); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compiled, frozen) {
		t.Fatalf("compiled v1 authority changed:\ncompiled=%+v\nfrozen=%+v", compiled, frozen)
	}
}

func TestCompileMoodReviewAuthorityAcceptsIndependentAgreement(t *testing.T) {
	t.Parallel()

	packetBlob := testMoodPacket(t)
	packetSHA := testSHA256(packetBlob)
	mapBlob := testMoodMap(t, packetSHA)
	first := testMoodSubmission(t, packetSHA, "reviewer-a", "gemma4", "gemma4:12b", "a", testMoodScores(3, 1, 0, 3, 1))
	second := testMoodSubmission(t, packetSHA, "reviewer-b", "qwen3.5", "qwen3.5:9b", "b", testMoodScores(3, 1, 0, 3, 1))

	authority, err := CompileMoodReviewAuthority(packetBlob, mapBlob, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Status != MoodReviewStatusModelAttested || authority.Completeness != MoodReviewCompletenessComplete || authority.PacketSHA256 != packetSHA || authority.PrivateMapSHA256 != testSHA256(mapBlob) {
		t.Fatalf("unexpected authority identity: %+v", authority)
	}
	if len(authority.Decisions) != 1 || authority.Decisions[0].Key != provision.Key("movie:tmdb:346648") || !maps.Equal(authority.Decisions[0].Scores, testMoodScores(3, 1, 0, 3, 1)) || len(authority.Decisions[0].UncertainAxes) != 0 {
		t.Fatalf("unexpected authority decision: %+v", authority.Decisions)
	}
	if len(authority.SubmissionSHA256) != 2 || authority.SubmissionSHA256[0] != testSHA256(first) || authority.SubmissionSHA256[1] != testSHA256(second) {
		t.Fatalf("authority did not bind exact submissions: %+v", authority.SubmissionSHA256)
	}
}

func TestRunMoodReviewBindsProviderAttributionAndKeepsCorpusAnswersBlind(t *testing.T) {
	t.Parallel()

	packet := testMoodPacket(t)
	output := testJSON(t, MoodReviewOutput{Assessments: []MoodReviewAssessment{{
		Alias: "candidate-a", Scores: testMoodScores(3, 1, 0, 3, 1), EvidenceIDs: []string{"evidence-1"},
		Rationale: "The evidence supports positive warmth with little threat.",
	}}})
	provider := testkit.NewLLM(llm.Response{
		Content: string(output),
		Attribution: llm.Attribution{
			RequestedProvider: "openrouter", RequestedModel: "google/gemini-3.7-flash",
			ResolvedProvider: "Google AI Studio", ResolvedModel: "google/gemini-3.7-flash-20260901",
			Tokens: llm.TokenUsage{Prompt: 321, Completion: 87}, Charge: &llm.Money{Amount: "0.00042", Currency: "USD"},
			Latency: 1500 * time.Millisecond, Attempts: 1, GenerationID: "generation-1",
		},
	})
	now := time.Date(2026, 9, 16, 13, 0, 0, 0, time.UTC)
	submission, err := RunMoodReview(context.Background(), provider, packet, MoodReviewRunConfig{
		ReviewerID: "gemini-reviewer", ModelFamily: "gemini", IdentityKind: "openrouter-route-snapshot",
		IdentitySHA256: testDigest("route-identity"), ExpectedResolvedProvider: "Google AI Studio",
		RouteSlug: "google-ai-studio", SnapshotSHA256: testDigest("route-snapshot"), ZeroDataRetention: true,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if submission.Reviewer.Provider != "openrouter" || submission.Reviewer.Route != "Google AI Studio" || submission.Reviewer.ResolvedModel != "google/gemini-3.7-flash-20260901" {
		t.Fatalf("provider attribution was not retained: %+v", submission.Reviewer)
	}
	if submission.Inference.CostBasis != "provider-reported" || submission.Inference.ChargeAmount != "0.00042" || submission.Inference.GenerationID != "generation-1" {
		t.Fatalf("provider accounting was not retained: %+v", submission.Inference)
	}
	if provider.Calls != 1 || !provider.LastOpts.JSONMode || provider.LastOpts.Temperature == nil || *provider.LastOpts.Temperature != 0 {
		t.Fatalf("review call was not one deterministic JSON turn: calls=%d opts=%+v", provider.Calls, provider.LastOpts)
	}
	prompt := provider.Prompt()
	for _, hidden := range []string{"movie:tmdb:346648", "positiveKeys", "negativeKeys", "Comforting, low-threat family movies"} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("review prompt leaked corpus answer %q: %s", hidden, prompt)
		}
	}
}

func TestCompileMoodReviewAuthorityFailsClosed(t *testing.T) {
	t.Parallel()

	packet := testMoodPacket(t)
	packetSHA := testSHA256(packet)
	privateMap := testMoodMap(t, packetSHA)
	first := testMoodSubmission(t, packetSHA, "reviewer-a", "gemma4", "gemma4:12b", "a", testMoodScores(3, 1, 0, 3, 1))
	second := testMoodSubmission(t, packetSHA, "reviewer-b", "qwen3.5", "qwen3.5:9b", "b", testMoodScores(3, 1, 0, 3, 1))

	tests := []struct {
		name        string
		packet      []byte
		privateMap  []byte
		submissions [][]byte
		want        string
	}{
		{name: "same family under another model", packet: packet, privateMap: privateMap, submissions: [][]byte{first, testMoodSubmission(t, packetSHA, "reviewer-c", "gemma4", "gemma4:8b-q4_k_m", "c", testMoodScores(3, 1, 0, 3, 1))}, want: "distinct reviewer identities and registered model families"},
		{name: "unregistered family", packet: packet, privateMap: privateMap, submissions: [][]byte{first, testMoodSubmission(t, packetSHA, "reviewer-c", "mystery", "mystery:1", "c", testMoodScores(3, 1, 0, 3, 1))}, want: "unregistered model family"},
		{name: "tampered packet", packet: append(append([]byte(nil), packet...), ' '), privateMap: privateMap, submissions: [][]byte{first, second}, want: "private map does not bind"},
		{name: "tampered output", packet: packet, privateMap: privateMap, submissions: [][]byte{first, testMutateMoodSubmission(t, second, func(value *MoodReviewSubmission) {
			value.Output = strings.Replace(value.Output, "positive warmth", "warmth", 1)
		})}, want: "output digest"},
		{name: "missing accounting", packet: packet, privateMap: privateMap, submissions: [][]byte{first, testMutateMoodSubmission(t, second, func(value *MoodReviewSubmission) { value.Inference.PromptTokens = 0 })}, want: "accounting is incomplete"},
		{name: "third reviewer shares a family", packet: packet, privateMap: privateMap, submissions: [][]byte{first, testMoodSubmission(t, packetSHA, "reviewer-b", "qwen3.5", "qwen3.5:9b", "b", testMoodScores(2, 1, 0, 3, 1)), testMoodSubmission(t, packetSHA, "reviewer-c", "gemma4", "gemma4:8b-q4_k_m", "c", testMoodScores(3, 1, 0, 3, 1))}, want: "distinct reviewer identities and registered model families"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			authority, err := CompileMoodReviewAuthority(tc.packet, tc.privateMap, tc.submissions...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			if authority.Status != "" || len(authority.Decisions) != 0 {
				t.Fatalf("failed compilation exposed private decisions: %+v", authority)
			}
		})
	}
}

func TestCompileMoodReviewAuthorityUsesThirdFamilyOnlyToResolveDisagreement(t *testing.T) {
	t.Parallel()

	packet := testMoodPacket(t)
	packetSHA := testSHA256(packet)
	privateMap := testMoodMap(t, packetSHA)
	first := testMoodSubmission(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", testMoodScores(3, 1, 0, 3, 1))
	second := testMoodSubmission(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", testMoodScores(2, 1, 0, 3, 1))

	uncertain, err := CompileMoodReviewAuthority(packet, privateMap, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if uncertain.Status != MoodReviewStatusModelAttested || uncertain.Completeness != MoodReviewCompletenessPartial || len(uncertain.Decisions) != 1 || !slicesEqual(uncertain.Decisions[0].UncertainAxes, []string{"valence"}) {
		t.Fatalf("unresolved disagreement was not preserved: %+v", uncertain)
	}

	third := testHostedMoodSubmission(t, packetSHA, "reviewer-c", testMoodScores(3, 1, 0, 3, 1))
	resolved, err := CompileMoodReviewAuthority(packet, privateMap, first, second, third)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != MoodReviewStatusModelAttested || resolved.Completeness != MoodReviewCompletenessComplete || resolved.Decisions[0].Scores["valence"] != 3 || len(resolved.Decisions[0].UncertainAxes) != 0 {
		t.Fatalf("third-family adjudication did not resolve the disagreement: %+v", resolved)
	}

	third = testHostedMoodSubmission(t, packetSHA, "reviewer-c", testMoodScores(1, 1, 0, 3, 1))
	stillUncertain, err := CompileMoodReviewAuthority(packet, privateMap, first, second, third)
	if err != nil {
		t.Fatal(err)
	}
	if stillUncertain.Status != MoodReviewStatusModelAttested || stillUncertain.Completeness != MoodReviewCompletenessPartial || !slicesEqual(stillUncertain.Decisions[0].UncertainAxes, []string{"valence"}) {
		t.Fatalf("three-way disagreement was not preserved: %+v", stillUncertain)
	}
}

func TestCompileMoodReviewAuthorityQuarantinesOnlyTheDisputedOperationalAxis(t *testing.T) {
	t.Parallel()

	packet := testOperationalMoodPacket(t)
	packetSHA := testSHA256(packet)
	privateMap := testOperationalMoodMap(t, packetSHA)
	first := testOperationalMoodSubmission(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", 2)
	second := testOperationalMoodSubmission(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", 3)

	authority, err := CompileMoodReviewAuthority(packet, privateMap, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Completeness != MoodReviewCompletenessPartial || len(authority.Decisions) != 1 {
		t.Fatalf("operational authority = %+v", authority)
	}
	decision := authority.Decisions[0]
	if !slicesEqual(decision.UncertainAxes, []string{"narrativeContinuityDependence"}) {
		t.Fatalf("uncertain axes = %v, want only narrative continuity", decision.UncertainAxes)
	}
	scores := testMoodScoreMap(t, decision.Scores)
	if scores["valence"] != 3 || scores["arousal"] != 1 || scores["threatFear"] != 0 || scores["comedicWarmth"] != 3 {
		t.Fatalf("resolved mood scores = %v", scores)
	}
	if _, present := scores["attentionalDemand"]; present {
		t.Fatalf("successor rubric retained the composite attention score: %v", scores)
	}
}

func TestCompileMoodReviewAuthorityQuarantinesUnsupportedContinuityConsensus(t *testing.T) {
	t.Parallel()

	packet := testOperationalMoodPacket(t)
	packetSHA := testSHA256(packet)
	privateMap := testOperationalMoodMap(t, packetSHA)
	first := testOperationalMoodSubmission(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", 2)
	second := testOperationalMoodSubmission(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", 2)

	authority, err := CompileMoodReviewAuthority(packet, privateMap, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Completeness != MoodReviewCompletenessPartial || len(authority.Decisions) != 1 || !slicesEqual(authority.Decisions[0].UncertainAxes, []string{"narrativeContinuityDependence"}) {
		t.Fatalf("unsupported model consensus escaped the evidence gate: %+v", authority)
	}
}

func TestCompileMoodReviewAuthorityRejectsCrossVersionPrivateMap(t *testing.T) {
	t.Parallel()

	packet := testOperationalMoodPacket(t)
	packetSHA := testSHA256(packet)
	legacyMap := testMoodMap(t, packetSHA)
	first := testOperationalMoodSubmission(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", 2)
	second := testOperationalMoodSubmission(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", 2)

	authority, err := CompileMoodReviewAuthority(packet, legacyMap, first, second)
	if err == nil || !strings.Contains(err.Error(), "private map does not bind") {
		t.Fatalf("cross-version map error = %v", err)
	}
	if authority.Status != "" || len(authority.Decisions) != 0 {
		t.Fatalf("cross-version map exposed authority: %+v", authority)
	}
}

func TestCompileMoodReviewAuthorityRequiresEachReviewerToCiteContinuityEvidence(t *testing.T) {
	t.Parallel()

	packet := testOperationalMoodPacketWithContinuityEvidence(t)
	packetSHA := testSHA256(packet)
	privateMap := testOperationalMoodMap(t, packetSHA)
	first := testOperationalMoodSubmission(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", 2)
	second := testOperationalMoodSubmission(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", 2)

	insufficient, err := CompileMoodReviewAuthority(packet, privateMap, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if insufficient.Completeness != MoodReviewCompletenessPartial || !slicesEqual(insufficient.Decisions[0].UncertainAxes, []string{"narrativeContinuityDependence"}) {
		t.Fatalf("uncited continuity evidence escaped quarantine: %+v", insufficient)
	}

	first = testOperationalMoodSubmissionWithEvidence(t, packetSHA, "reviewer-a", "qwen3.5", "qwen3.5:9b", "a", 2, []string{"evidence-1", "evidence-2"})
	second = testOperationalMoodSubmissionWithEvidence(t, packetSHA, "reviewer-b", "gemma4", "gemma4:12b", "b", 2, []string{"evidence-1", "evidence-2"})
	supported, err := CompileMoodReviewAuthority(packet, privateMap, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if supported.Completeness != MoodReviewCompletenessComplete || len(supported.Decisions[0].UncertainAxes) != 0 || supported.Decisions[0].Scores["narrativeContinuityDependence"] != 2 {
		t.Fatalf("fully cited continuity consensus was not resolved: %+v", supported)
	}
}

func testMoodPacket(t *testing.T) []byte {
	t.Helper()
	evidence := MoodReviewEvidence{
		ID: "evidence-1", URL: "https://example.test/review", Observed: "2026-09-16",
		Summary:     "The source describes kind family fun with mild threat and a reassuring resolution.",
		Uncertainty: "Editorial language is evidence for ordinal review, not an objective label.",
	}
	evidence.SHA256 = MoodReviewEvidenceSHA256(evidence)
	return testJSON(t, MoodReviewPacket{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		RubricVersion: MoodReviewRubricVersion, PromptVersion: MoodReviewPromptVersion,
		PacketID: "movie-mood-review-test", PreparedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Evidence: []MoodReviewEvidence{evidence},
		Cases:    []MoodReviewCase{{Alias: "candidate-a", DisplayTitle: "Paddington 2", EvidenceIDs: []string{"evidence-1"}}},
	})
}

func testOperationalMoodPacket(t *testing.T) []byte {
	t.Helper()
	var packet MoodReviewPacket
	if err := json.Unmarshal(testMoodPacket(t), &packet); err != nil {
		t.Fatal(err)
	}
	packet.ContractVersion = "query-mood-model-review-v2"
	packet.RubricVersion = "movie-mood-ordinal-v2"
	packet.PromptVersion = "query-mood-blind-review-v2"
	return testJSON(t, packet)
}

func testOperationalMoodPacketWithContinuityEvidence(t *testing.T) []byte {
	t.Helper()
	var packet MoodReviewPacket
	if err := json.Unmarshal(testOperationalMoodPacket(t), &packet); err != nil {
		t.Fatal(err)
	}
	packet.Evidence[0].SourceAuthority = "example-studio"
	packet.Evidence[0].SourceRole = "direct-work"
	packet.Evidence[0].SupportsAxes = []string{"narrativeContinuityDependence"}
	packet.Evidence[0].SHA256 = MoodReviewEvidenceSHA256(packet.Evidence[0])
	second := MoodReviewEvidence{
		ID: "evidence-2", URL: "https://archive.example.test/structure", Observed: "2026-09-16",
		Summary:         "The independent structural source describes the continuing causal thread and state changes.",
		Uncertainty:     "The source supports structure only, not a universal viewer response.",
		SourceAuthority: "example-archive", SourceRole: "independent-structural",
		SupportsAxes: []string{"narrativeContinuityDependence"},
	}
	second.SHA256 = MoodReviewEvidenceSHA256(second)
	packet.Evidence = append(packet.Evidence, second)
	packet.Cases[0].EvidenceIDs = append(packet.Cases[0].EvidenceIDs, second.ID)
	return testJSON(t, packet)
}

func testOperationalMoodSubmission(t *testing.T, packetSHA, reviewerID, family, model, digestSeed string, continuity int) []byte {
	return testOperationalMoodSubmissionWithEvidence(t, packetSHA, reviewerID, family, model, digestSeed, continuity, []string{"evidence-1"})
}

func testOperationalMoodSubmissionWithEvidence(t *testing.T, packetSHA, reviewerID, family, model, digestSeed string, continuity int, evidenceIDs []string) []byte {
	t.Helper()
	output := testJSON(t, map[string]any{"assessments": []any{map[string]any{
		"alias": "candidate-a",
		"scores": map[string]int{
			"valence": 3, "arousal": 1, "threatFear": 0, "comedicWarmth": 3,
			"narrativeContinuityDependence": continuity,
		},
		"evidenceIds": evidenceIDs,
		"rationale":   "The cited evidence supports the resolved mood dimensions.",
	}}})
	return testJSON(t, MoodReviewSubmission{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: "query-mood-model-review-v2",
		PacketSHA256: packetSHA,
		Reviewer: MoodReviewerIdentity{
			ID: reviewerID, Provider: "ollama", Route: "loopback", Model: model,
			ResolvedModel: model, ModelFamily: family, IdentityKind: "ollama-model-digest",
			IdentitySHA256: testDigest(digestSeed), PromptVersion: "query-mood-blind-review-v2",
		},
		Output: string(output), OutputSHA256: testSHA256(output),
		Inference: MoodReviewInference{
			CompletedAt: time.Date(2026, 9, 16, 12, 5, 0, 0, time.UTC), Attempts: 1,
			PromptTokens: 100, CompletionTokens: 50, LatencyMS: 1200, CostBasis: "local-unmetered",
		},
	})
}

func testMoodScoreMap(t *testing.T, scores MoodAxisScores) map[string]int {
	t.Helper()
	blob, err := json.Marshal(scores)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]int)
	if err := json.Unmarshal(blob, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func testMoodMap(t *testing.T, packetSHA string) []byte {
	t.Helper()
	return testJSON(t, MoodReviewPrivateMap{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		PacketID: "movie-mood-review-test", PacketSHA256: packetSHA,
		Entries: []MoodReviewMapEntry{{Alias: "candidate-a", Key: provision.Key("movie:tmdb:346648")}},
	})
}

func testOperationalMoodMap(t *testing.T, packetSHA string) []byte {
	t.Helper()
	var privateMap MoodReviewPrivateMap
	if err := json.Unmarshal(testMoodMap(t, packetSHA), &privateMap); err != nil {
		t.Fatal(err)
	}
	privateMap.ContractVersion = MoodReviewContractVersionV2
	return testJSON(t, privateMap)
}

func testMoodSubmission(t *testing.T, packetSHA, reviewerID, family, model, digestSeed string, scores MoodAxisScores) []byte {
	t.Helper()
	output := testJSON(t, MoodReviewOutput{Assessments: []MoodReviewAssessment{{
		Alias: "candidate-a", Scores: scores, EvidenceIDs: []string{"evidence-1"},
		Rationale: "The cited evidence supports positive warmth and low threat.",
	}}})
	return testJSON(t, MoodReviewSubmission{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		PacketSHA256: packetSHA,
		Reviewer: MoodReviewerIdentity{
			ID: reviewerID, Provider: "ollama", Route: "loopback", Model: model,
			ResolvedModel: model, ModelFamily: family, IdentityKind: "ollama-model-digest", IdentitySHA256: testDigest(digestSeed), PromptVersion: MoodReviewPromptVersion,
		},
		Output: string(output), OutputSHA256: testSHA256(output),
		Inference: MoodReviewInference{
			CompletedAt: time.Date(2026, 9, 16, 12, 5, 0, 0, time.UTC), Attempts: 1,
			PromptTokens: 100, CompletionTokens: 50, LatencyMS: 1200, CostBasis: "local-unmetered",
		},
	})
}

func testMutateMoodSubmission(t *testing.T, blob []byte, mutate func(*MoodReviewSubmission)) []byte {
	t.Helper()
	var submission MoodReviewSubmission
	if err := json.Unmarshal(blob, &submission); err != nil {
		t.Fatal(err)
	}
	mutate(&submission)
	return testJSON(t, submission)
}

func testHostedMoodSubmission(t *testing.T, packetSHA, reviewerID string, scores MoodAxisScores) []byte {
	t.Helper()
	return testMutateMoodSubmission(t, testMoodSubmission(t, packetSHA, reviewerID, "gemini", "google/gemini-versioned", "hosted", scores), func(submission *MoodReviewSubmission) {
		submission.Reviewer.Provider = "openrouter"
		submission.Reviewer.Route = "Google AI Studio"
		submission.Reviewer.RouteSlug = "google-ai-studio"
		submission.Reviewer.IdentityKind = "openrouter-route-snapshot"
		submission.Reviewer.SnapshotSHA256 = testDigest("snapshot")
		submission.Reviewer.ZeroDataRetention = true
		submission.Inference.CostBasis = "provider-reported"
		submission.Inference.ChargeAmount = "0.001"
		submission.Inference.ChargeCurrency = "USD"
		submission.Inference.GenerationID = "generation-test"
	})
}

func slicesEqual[T comparable](left, right []T) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func testMoodScores(valence, arousal, threatFear, comedicWarmth, attentionalDemand int) MoodAxisScores {
	return MoodAxisScores{
		"valence": valence, "arousal": arousal, "threatFear": threatFear,
		"comedicWarmth": comedicWarmth, "attentionalDemand": attentionalDemand,
	}
}

func testJSON(t *testing.T, value any) []byte {
	t.Helper()
	blob, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(blob, '\n')
}

func testSHA256(blob []byte) string {
	digest := sha256.Sum256(blob)
	return hex.EncodeToString(digest[:])
}

func testDigest(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(digest[:])
}
