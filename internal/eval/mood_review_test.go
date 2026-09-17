//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

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
	if len(authority.Decisions) != 1 || authority.Decisions[0].Key != provision.Key("movie:tmdb:346648") || authority.Decisions[0].Scores != testMoodScores(3, 1, 0, 3, 1) || len(authority.Decisions[0].UncertainAxes) != 0 {
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
	if resolved.Status != MoodReviewStatusModelAttested || resolved.Completeness != MoodReviewCompletenessComplete || resolved.Decisions[0].Scores.Valence != 3 || len(resolved.Decisions[0].UncertainAxes) != 0 {
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

func testMoodMap(t *testing.T, packetSHA string) []byte {
	t.Helper()
	return testJSON(t, MoodReviewPrivateMap{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		PacketID: "movie-mood-review-test", PacketSHA256: packetSHA,
		Entries: []MoodReviewMapEntry{{Alias: "candidate-a", Key: provision.Key("movie:tmdb:346648")}},
	})
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
	return MoodAxisScores{Valence: valence, Arousal: arousal, ThreatFear: threatFear, ComedicWarmth: comedicWarmth, AttentionalDemand: attentionalDemand}
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
