package fillerreview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestRunOllamaTemporalAssessmentUsesSeparateUnitAndRoleCalls(t *testing.T) {
	const model = "model:fixed"
	digest := strings.Repeat("e", 64)
	var mu sync.Mutex
	var axes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": model, "model": model, "digest": digest}}})
		case "/api/chat":
			var request ollamaReviewRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			axis := "unit"
			content := `{"kind":"standalone","decisiveSignalIds":["frame-01"],"reason":"One bounded item."}`
			if strings.Contains(request.Messages[0].Content, "semantic role") {
				axis = "role"
				content = `{"kind":"commercial","decisiveSignalIds":["transcript-01","transcript-01"],"reason":"A product offer is made."}`
			}
			mu.Lock()
			axes = append(axes, axis)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "message": map[string]string{"content": content, "thinking": ""}, "done_reason": "stop", "prompt_eval_count": 10, "eval_count": 5})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	set, err := RunOllamaTemporalAssessment(t.Context(), OllamaTemporalConfig{
		PackagePath: writeTemporalTestPackage(t), BaseURL: server.URL, Model: model, ModelFamily: "family-a", ModelDigest: digest,
		AssessorID: "assessor", ExpectedCases: 1, PerCaseTimeout: time.Second, Now: func() time.Time { return time.Unix(2, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(axes) != 2 || axes[0] != "unit" || axes[1] != "role" {
		t.Fatalf("axes = %v", axes)
	}
	assessment := set.Assessments[0]
	if assessment.Unit.Kind != fillereval.UnitStandalone || assessment.Role.Kind != fillereval.TemporalRoleCommercial || assessment.OperationalFailure != nil {
		t.Fatalf("assessment = %+v", assessment)
	}
	if len(assessment.Role.DecisiveSignalIDs) != 1 {
		t.Fatalf("normalized role signals = %v", assessment.Role.DecisiveSignalIDs)
	}
	if assessment.Inference.Attempts != 2 || len(assessment.Inference.Calls) != 2 || assessment.Inference.PromptTokens != 20 || assessment.Inference.CompletionTokens != 10 {
		t.Fatalf("inference = %+v", assessment.Inference)
	}
}
