//go:build eval

package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CatalogEvidence identifies the live grounding corpus without copying connection
// details or credentials into a durable artifact.
type CatalogEvidence struct {
	Identity    string `json:"identity"`
	Fingerprint string `json:"fingerprint"`
}

// CertificationEvidence records the non-secret identity of one semantic run.
// It is evidence about what ran, not application configuration or runtime state.
type CertificationEvidence struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Catalog   CatalogEvidence `json:"catalog"`
	Timestamp time.Time       `json:"timestamp"`
}

// Scorecard is the durable, versioned semantic-evaluation artifact.
type Scorecard struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Evidence      CertificationEvidence `json:"evidence"`
	Summary       ScorecardSummary      `json:"summary"`
	Results       []Result              `json:"results"`
}

type ScorecardSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

func certificationEvidenceFromEnv(now time.Time) CertificationEvidence {
	provider := strings.TrimSpace(os.Getenv("LLM_PROVIDER"))
	if provider == "" {
		provider = "ollama"
	}
	flavor := "emby"
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LIBRARY_FLAVOR")), "jellyfin") {
		flavor = "jellyfin"
	}
	identity := "live-" + flavor + "-library+tmdb"
	canonical := strings.Join([]string{
		"catalog=library+tmdb-v3",
		"flavor=" + flavor,
		"library=" + sanitizedCatalogTarget(os.Getenv("LIBRARY_URL")),
	}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	return CertificationEvidence{
		Provider: provider,
		Model:    os.Getenv("LLM_MODEL"),
		Catalog: CatalogEvidence{
			Identity: identity, Fingerprint: "sha256:" + hex.EncodeToString(digest[:]),
		},
		Timestamp: now.UTC(),
	}
}

func sanitizedCatalogTarget(raw string) string {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Scheme == "" || target.Host == "" {
		return "unparseable-library-target"
	}
	target.User = nil
	target.RawQuery = ""
	target.ForceQuery = false
	target.Fragment = ""
	target.Scheme = strings.ToLower(target.Scheme)
	target.Host = strings.ToLower(target.Host)
	target.Path = strings.TrimRight(target.Path, "/")
	return target.String()
}

func buildScorecard(evidence CertificationEvidence, results []Result) Scorecard {
	scorecard := Scorecard{
		SchemaVersion: 1,
		Evidence:      evidence,
		Results:       results,
		Summary:       ScorecardSummary{Total: len(results)},
	}
	for _, result := range results {
		if result.Passed() {
			scorecard.Summary.Passed++
		} else {
			scorecard.Summary.Failed++
		}
	}
	return scorecard
}

func writeScorecardArtifact(path string, scorecard Scorecard) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(redactScorecard(scorecard), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".eval-scorecard-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, absolute); err != nil {
		return fmt.Errorf("replace scorecard: %w", err)
	}
	ok = true
	return nil
}

func redactScorecard(scorecard Scorecard) Scorecard {
	secrets := credentialValuesFromEnv()
	results := make([]Result, len(scorecard.Results))
	copy(results, scorecard.Results)
	for i := range results {
		results[i].Failures = append([]string(nil), results[i].Failures...)
		for j := range results[i].Failures {
			results[i].Failures[j] = redactCredentialValues(results[i].Failures[j], secrets)
		}
		results[i].JudgeNote = redactCredentialValues(results[i].JudgeNote, secrets)
	}
	scorecard.Results = results
	return scorecard
}

func credentialValuesFromEnv() []string {
	seen := make(map[string]struct{})
	add := func(value string) {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, name := range []string{"LLM_API_KEY", "LIBRARY_TOKEN", "TMDB_API_KEY"} {
		add(os.Getenv(name))
	}
	for _, name := range []string{"LLM_URL", "LIBRARY_URL"} {
		raw := strings.TrimSpace(os.Getenv(name))
		add(raw)
		target, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if target.User != nil {
			add(target.User.Username())
			if password, ok := target.User.Password(); ok {
				add(password)
			}
		}
		for _, values := range target.Query() {
			for _, value := range values {
				add(value)
			}
		}
	}
	secrets := make([]string, 0, len(seen))
	for secret := range seen {
		secrets = append(secrets, secret)
	}
	// Replace longer values first so an overlapping short token cannot leave a
	// credential suffix behind.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func redactCredentialValues(text string, secrets []string) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	return text
}
