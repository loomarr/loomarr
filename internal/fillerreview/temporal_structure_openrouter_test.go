package fillerreview

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const temporalStructureStandaloneResponse = `{"unit":"standalone","unitDecisiveAtMs":[100],"unitReason":"independent opening and close","role":"commercial","roleDecisiveAtMs":[200],"roleReason":"product offer framing","segments":[{"startMs":0,"endMs":10000,"role":"commercial","decisiveAtMs":[200],"reason":"one complete product offer"}]}`

func TestRunOpenRouterTemporalStructureBindsOneAtomicVideoAssessment(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	fixture := newTemporalStructureFixture(t)
	root, challenge := fixture.build(t, "direct-video")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	alias := manifest.Cases[0].Alias
	var request openRouterStructuredRequest
	server := newTemporalSuitabilityServer(t, &request, temporalStructureStandaloneResponse)
	defer server.Close()
	checkpointDir := filepath.Join(t.TempDir(), "private")
	config := temporalStructureOpenRouterTestConfig(manifestPath, checkpointDir, []string{alias}, server, now)
	result, err := RunOpenRouterTemporalStructure(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublicManifestSHA256 != challenge.PublicManifestSHA256 || result.ReasoningMode != TemporalStructureOpenRouterReasoningDisabled || result.Requests != 1 || len(result.Assessments) != 1 || result.ProductionAdmissionAllowed || result.Assessments[0].Unit.Kind != fillereval.UnitStandalone || result.Assessments[0].Role.Kind != fillereval.TemporalRoleCommercial || len(result.Assessments[0].Inference.Calls) != 1 || result.Assessments[0].Inference.Calls[0].Axis != "structure" {
		t.Fatalf("result = %+v", result)
	}
	parts := request.Messages[1].Content
	if len(parts) != 2 || parts[1].Type != "video_url" || parts[1].VideoURL == nil || !strings.HasPrefix(parts[1].VideoURL.URL, "data:video/mp4;base64,") {
		t.Fatalf("structure request parts = %+v", parts)
	}
	attempt := result.Attempts[0]
	rawPath := filepath.Join(checkpointDir, filepath.FromSlash(attempt.RawResponsePath))
	info, err := os.Stat(rawPath)
	if err != nil || info.Mode().Perm() != 0o600 || attempt.State != temporalOpenRouterAttemptAccepted || attempt.ResponseSHA256 != result.Assessments[0].Inference.Calls[0].ResponseSHA256 {
		t.Fatalf("raw mode=%v attempt=%+v error=%v", info.Mode(), attempt, err)
	}
}

func TestValidateTemporalStructureOpenRouterResultRejectsEvidenceAccountingDrift(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	fixture := newTemporalStructureFixture(t)
	root, _ := fixture.build(t, "result-drift")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	server := newTemporalSuitabilityServer(t, nil, temporalStructureStandaloneResponse)
	defer server.Close()
	result, err := RunOpenRouterTemporalStructure(t.Context(), temporalStructureOpenRouterTestConfig(manifestPath, filepath.Join(t.TempDir(), "private"), []string{manifest.Cases[0].Alias}, server, now))
	if err != nil {
		t.Fatal(err)
	}
	selected := manifest.Cases[:1]
	tests := []struct {
		name   string
		mutate func(*TemporalStructureOpenRouterResult)
		want   string
	}{
		{name: "reasoning", mutate: func(candidate *TemporalStructureOpenRouterResult) { candidate.ReasoningMode = "" }, want: "identity"},
		{name: "prompt", mutate: func(candidate *TemporalStructureOpenRouterResult) { candidate.PromptSHA256 = strings.Repeat("f", 64) }, want: "identity"},
		{name: "model digest", mutate: func(candidate *TemporalStructureOpenRouterResult) {
			candidate.Assessor.ModelDigest = strings.Repeat("f", 64)
		}, want: "identity"},
		{name: "aggregate spend", mutate: func(candidate *TemporalStructureOpenRouterResult) { candidate.ConsumedNanoUSD++ }, want: "aggregate spend"},
		{name: "attempt reservation", mutate: func(candidate *TemporalStructureOpenRouterResult) { candidate.Attempts[0].ReservedNanoUSD++ }, want: "attempt 0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneTemporalStructureOpenRouterResult(t, result)
			test.mutate(&candidate)
			if err := validateTemporalStructureOpenRouterResult(candidate, manifest, selected); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunOpenRouterTemporalStructureTurnsInvalidConditionalRoleIntoFailure(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	fixture := newTemporalStructureFixture(t)
	root, _ := fixture.build(t, "invalid-role")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	server := newTemporalSuitabilityServer(t, nil, `{"unit":"compilation","unitDecisiveAtMs":[100],"unitReason":"join","role":"commercial","roleDecisiveAtMs":[200],"roleReason":"invalid"}`)
	defer server.Close()
	result, err := RunOpenRouterTemporalStructure(t.Context(), temporalStructureOpenRouterTestConfig(manifestPath, filepath.Join(t.TempDir(), "private"), []string{manifest.Cases[0].Alias}, server, now))
	if err != nil {
		t.Fatal(err)
	}
	assessment := result.Assessments[0]
	if assessment.OperationalFailure == nil || assessment.OperationalFailure.Code != fillereval.TemporalFailureInvalidResponse || result.Attempts[0].State != temporalOpenRouterAttemptFailed {
		t.Fatalf("assessment=%+v attempt=%+v", assessment, result.Attempts[0])
	}
}

func TestRunOpenRouterTemporalStructureRequiresSnapshotVideoModality(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	fixture := newTemporalStructureFixture(t)
	root, _ := fixture.build(t, "snapshot-modality")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	server := newTemporalSuitabilityServer(t, nil, temporalStructureStandaloneResponse)
	defer server.Close()
	config := temporalStructureOpenRouterTestConfig(manifestPath, filepath.Join(t.TempDir(), "private"), []string{manifest.Cases[0].Alias}, server, now)
	config.Snapshot.Models[0].InputModalities = []string{"image", "text"}
	if _, err := RunOpenRouterTemporalStructure(t.Context(), config); err == nil || !strings.Contains(err.Error(), "text/video") {
		t.Fatalf("error = %v", err)
	}
}

func TestTemporalStructureOpenRouterWireEnforcesClosedConditionalShape(t *testing.T) {
	tests := []struct {
		name string
		wire temporalStructureOpenRouterWire
		want string
	}{
		{name: "valid compilation", wire: temporalStructureOpenRouterWire{Unit: "compilation", UnitDecisiveAtMS: []int64{100}, UnitReason: "join", Role: "none", Segments: []temporalStructureOpenRouterSegmentWire{{StartMS: 0, EndMS: 100, Role: "commercial", DecisiveAtMS: []int64{50}, Reason: "offer"}, {StartMS: 100, EndMS: 1_000, Role: "promo", DecisiveAtMS: []int64{200}, Reason: "programme promotion"}}}},
		{name: "valid programme with spots", wire: temporalStructureOpenRouterWire{Unit: "programme_with_spots", UnitDecisiveAtMS: []int64{300, 700}, UnitReason: "inserted spot", Role: "none", Segments: []temporalStructureOpenRouterSegmentWire{{StartMS: 0, EndMS: 300, Role: "programme_fragment", DecisiveAtMS: []int64{100}, Reason: "programme scene"}, {StartMS: 300, EndMS: 700, Role: "commercial", DecisiveAtMS: []int64{500}, Reason: "product offer"}, {StartMS: 700, EndMS: 1_000, Role: "programme_fragment", DecisiveAtMS: []int64{900}, Reason: "programme resumes"}}}},
		{name: "programme spots without surrounding programme", wire: temporalStructureOpenRouterWire{Unit: "programme_with_spots", UnitDecisiveAtMS: []int64{500}, UnitReason: "claimed insert", Role: "none", Segments: []temporalStructureOpenRouterSegmentWire{{StartMS: 0, EndMS: 500, Role: "programme_fragment", DecisiveAtMS: []int64{100}, Reason: "programme"}, {StartMS: 500, EndMS: 1_000, Role: "commercial", DecisiveAtMS: []int64{700}, Reason: "offer"}}}, want: "programme with spots"},
		{name: "valid unicode character limit", wire: temporalStructureOpenRouterWire{Unit: "compilation", UnitDecisiveAtMS: []int64{100}, UnitReason: strings.Repeat("a", 511) + "–", Role: "none", Segments: []temporalStructureOpenRouterSegmentWire{{StartMS: 0, EndMS: 100, Role: "commercial", DecisiveAtMS: []int64{50}, Reason: "offer"}, {StartMS: 100, EndMS: 1_000, Role: "promo", DecisiveAtMS: []int64{200}, Reason: "programme promotion"}}}},
		{name: "missing standalone role", wire: temporalStructureOpenRouterWire{Unit: "standalone", UnitDecisiveAtMS: []int64{100}, UnitReason: "bounded", Role: "none"}, want: "standalone role"},
		{name: "non standalone role", wire: temporalStructureOpenRouterWire{Unit: "programme_excerpt", UnitDecisiveAtMS: []int64{0}, UnitReason: "cut", Role: "promo", RoleDecisiveAtMS: []int64{1}, RoleReason: "wrong"}, want: "carries role"},
		{name: "duplicate times", wire: temporalStructureOpenRouterWire{Unit: "compilation", UnitDecisiveAtMS: []int64{100, 100}, UnitReason: "join", Role: "none"}, want: "unit claim"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTemporalStructureOpenRouterWire(test.wire, 1_000)
			if test.want == "" && err != nil || test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestTemporalStructureOpenRouterSchemaRequiresCompleteSegmentPlan(t *testing.T) {
	schema := temporalStructureOpenRouterSchema(1_000)
	required, ok := schema["required"].([]string)
	if !ok || !slices.Contains(required, "segments") {
		t.Fatalf("top-level required fields = %#v", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v", schema["properties"])
	}
	segments, ok := properties["segments"].(map[string]any)
	if !ok || segments["minItems"] != 1 || segments["maxItems"] != 128 {
		t.Fatalf("segments schema = %#v", properties["segments"])
	}
}

func TestNormalizeTemporalStructureOpenRouterWireSortsMechanicalTimestamps(t *testing.T) {
	wire := temporalStructureOpenRouterWire{
		Unit: "standalone", UnitDecisiveAtMS: []int64{900, 100}, UnitReason: "bounded",
		Role: "promo", RoleDecisiveAtMS: []int64{800, 200}, RoleReason: "programme promotion",
		Segments: []temporalStructureOpenRouterSegmentWire{{StartMS: 0, EndMS: 1_000, Role: "promo", DecisiveAtMS: []int64{800, 200}, Reason: "programme promotion"}},
	}
	normalizeTemporalStructureOpenRouterWire(&wire)
	if err := validateTemporalStructureOpenRouterWire(wire, 1_000); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(wire.UnitDecisiveAtMS, []int64{100, 900}) || !slices.Equal(wire.RoleDecisiveAtMS, []int64{200, 800}) {
		t.Fatalf("normalized wire = %+v", wire)
	}
	if !slices.Equal(wire.Segments[0].DecisiveAtMS, []int64{200, 800}) {
		t.Fatalf("segment timestamps were not normalized: %+v", wire.Segments)
	}
}

func temporalStructureOpenRouterTestConfig(manifestPath, checkpointDir string, aliases []string, server *httptest.Server, now time.Time) TemporalStructureOpenRouterConfig {
	snapshot := openRouterReviewSnapshot(server.URL, now)
	snapshot.Models[0].InputModalities = append(snapshot.Models[0].InputModalities, "video")
	return TemporalStructureOpenRouterConfig{
		PublicManifestPath: manifestPath, CaseAliases: aliases, CheckpointDir: checkpointDir,
		BaseURL: server.URL, APIKey: "test-key", Snapshot: snapshot, Model: "review/vendor-model", ModelFamily: "video-family",
		UpstreamProvider: "Provider Route", UpstreamProviderSlug: "provider/route", AssessorID: "structure-assessor",
		ReasoningMode: TemporalStructureOpenRouterReasoningDisabled,
		ExpectedCases: len(aliases), PerCaseTimeout: time.Second, MaxRequests: len(aliases),
		MaxSpendNanoUSD: int64(len(aliases)) * 2_000_000, MaxChargeNanoUSD: 2_000_000,
		AllowInsecureTestURL: true, Client: server.Client(), Now: func() time.Time { return now },
	}
}

func writeTemporalStructureResult(t *testing.T, path string, result TemporalStructureOpenRouterResult) {
	t.Helper()
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cloneTemporalStructureOpenRouterResult(t *testing.T, result TemporalStructureOpenRouterResult) TemporalStructureOpenRouterResult {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var clone TemporalStructureOpenRouterResult
	if err := decodeStrictReviewJSON(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
