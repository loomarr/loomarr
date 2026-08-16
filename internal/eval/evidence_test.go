//go:build eval

package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificationEvidenceIsStableAndContainsNoCredentials(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("LLM_MODEL", "openai/gpt-4o-mini")
	t.Setenv("LLM_API_KEY", "llm-secret-123")
	t.Setenv("LIBRARY_FLAVOR", "emby")
	t.Setenv("LIBRARY_URL", "https://library-user:library-password@media.internal/emby?api_key=url-secret")
	t.Setenv("LIBRARY_TOKEN", "library-token-456")
	t.Setenv("TMDB_API_KEY", "tmdb-secret-789")

	certifiedAt := time.Date(2026, time.August, 15, 18, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	evidence := certificationEvidenceFromEnv(certifiedAt)
	if evidence.Provider != "openrouter" || evidence.Model != "openai/gpt-4o-mini" {
		t.Fatalf("selection evidence = %+v", evidence)
	}
	if evidence.Catalog.Identity != "live-emby-library+tmdb" {
		t.Errorf("catalog identity = %q", evidence.Catalog.Identity)
	}
	if !strings.HasPrefix(evidence.Catalog.Fingerprint, "sha256:") || len(evidence.Catalog.Fingerprint) != len("sha256:")+64 {
		t.Errorf("catalog fingerprint = %q, want sha256:<64 hex>", evidence.Catalog.Fingerprint)
	}
	if !evidence.Timestamp.Equal(certifiedAt.UTC()) || evidence.Timestamp.Location() != time.UTC {
		t.Errorf("timestamp = %v, want UTC %v", evidence.Timestamp, certifiedAt.UTC())
	}

	encoded, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"llm-secret-123", "library-token-456", "tmdb-secret-789", "library-user", "library-password", "url-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("evidence leaked %q: %s", secret, encoded)
		}
	}

	// Credentials do not identify a catalog. Rotating every secret must preserve
	// the fingerprint, while changing the sanitized catalog target must change it.
	t.Setenv("LLM_API_KEY", "rotated-llm")
	t.Setenv("LIBRARY_TOKEN", "rotated-library")
	t.Setenv("TMDB_API_KEY", "rotated-tmdb")
	t.Setenv("LIBRARY_URL", "https://other-user:other-password@media.internal/emby?api_key=other")
	rotated := certificationEvidenceFromEnv(certifiedAt)
	if rotated.Catalog.Fingerprint != evidence.Catalog.Fingerprint {
		t.Error("credential rotation changed the catalog fingerprint")
	}
	t.Setenv("LIBRARY_URL", "https://media.internal/a-different-library")
	changed := certificationEvidenceFromEnv(certifiedAt)
	if changed.Catalog.Fingerprint == evidence.Catalog.Fingerprint {
		t.Error("a different sanitized catalog target kept the same fingerprint")
	}
}

func TestWriteScorecardPersistsVersionedRunEvidence(t *testing.T) {
	out := filepath.Join(t.TempDir(), "template-certification.json")
	t.Setenv("LOOMARR_EVAL_OUT", out)
	t.Setenv("LLM_API_KEY", "artifact-llm-secret")
	t.Setenv("LIBRARY_TOKEN", "artifact-library-secret")
	t.Setenv("TMDB_API_KEY", "artifact-tmdb-secret")
	t.Setenv("LIBRARY_URL", "https://artifact-user:artifact-password@media.internal/emby?token=artifact-query-secret")
	evidence := CertificationEvidence{
		Provider: "ollama", Model: "qwen3:8b",
		Catalog:   CatalogEvidence{Identity: "live-jellyfin-library+tmdb", Fingerprint: "sha256:abc"},
		Timestamp: time.Date(2026, time.August, 15, 22, 0, 0, 0, time.UTC),
	}
	writeScorecard(t, evidence, []Result{{
		Case: "template_saturday_cartoons",
		JudgeNote: "provider artifact-llm-secret library artifact-library-secret tmdb artifact-tmdb-secret " +
			"url artifact-user artifact-password artifact-query-secret",
	}})

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read scorecard: %v", err)
	}
	if !strings.Contains(string(raw), `"case": "template_saturday_cartoons"`) || strings.Contains(string(raw), `"Case"`) {
		t.Fatalf("scorecard result fields are not stable lower-camel JSON: %s", raw)
	}
	for _, secret := range []string{
		"artifact-llm-secret", "artifact-library-secret", "artifact-tmdb-secret",
		"artifact-user", "artifact-password", "artifact-query-secret",
	} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("scorecard artifact leaked %q: %s", secret, raw)
		}
	}
	var scorecard Scorecard
	if err := json.Unmarshal(raw, &scorecard); err != nil {
		t.Fatalf("decode scorecard: %v", err)
	}
	if scorecard.SchemaVersion != 1 || scorecard.Evidence != evidence {
		t.Errorf("scorecard evidence = %+v, want version 1 and %+v", scorecard, evidence)
	}
	if scorecard.Summary.Total != 1 || scorecard.Summary.Passed != 1 || scorecard.Summary.Failed != 0 {
		t.Errorf("scorecard summary = %+v", scorecard.Summary)
	}
	if len(scorecard.Results) != 1 || scorecard.Results[0].Case != "template_saturday_cartoons" {
		t.Errorf("scorecard results = %+v", scorecard.Results)
	}
}
