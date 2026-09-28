package fillerreview

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/httpx"
)

const (
	ReviewRunSchemaVersion    = 2
	OllamaReviewPromptVersion = "filler-blind-review-ollama-v7"
	maxReviewResponseBytes    = 256 << 10
)

type OllamaReviewConfig struct {
	PackageDir     string
	Transcripts    []fillerbakeoff.TranscriptArtifact
	BaseURL        string
	Model          string
	ModelDigest    string
	ReviewerID     string
	ExpectedCases  int
	PerCaseTimeout time.Duration
	Client         *http.Client
	Now            func() time.Time
}

type ReviewRun struct {
	SchemaVersion            int             `json:"schemaVersion"`
	BatchID                  string          `json:"batchId"`
	PackageManifestSHA256    string          `json:"packageManifestSha256"`
	ReviewerID               string          `json:"reviewerId"`
	Provider                 string          `json:"provider"`
	Model                    string          `json:"model"`
	ResolvedModel            string          `json:"resolvedModel"`
	ModelDigest              string          `json:"modelDigest"`
	UpstreamProvider         string          `json:"upstreamProvider,omitempty"`
	UpstreamProviderSlug     string          `json:"upstreamProviderSlug,omitempty"`
	PromptVersion            string          `json:"promptVersion"`
	PromptSHA256             string          `json:"promptSha256"`
	CapabilitySnapshotSHA256 string          `json:"capabilitySnapshotSha256,omitempty"`
	TranscriptSetSHA256      string          `json:"transcriptSetSha256,omitempty"`
	CompletedAt              time.Time       `json:"completedAt"`
	Cases                    int             `json:"cases"`
	Requests                 int             `json:"requests"`
	MaxRequests              int             `json:"maxRequests,omitempty"`
	PromptTokens             int64           `json:"promptTokens"`
	CompletionTokens         int64           `json:"completionTokens"`
	TotalLatencyMS           int64           `json:"totalLatencyMs"`
	ChargedNanoUSD           int64           `json:"chargedNanoUsd,omitempty"`
	MaxSpendNanoUSD          int64           `json:"maxSpendNanoUsd,omitempty"`
	MaxChargeNanoUSD         int64           `json:"maxChargeNanoUsd,omitempty"`
	Calls                    []ReviewCall    `json:"calls"`
	Attempts                 []ReviewAttempt `json:"attempts,omitempty"`
	SubmissionSHA256         string          `json:"submissionSha256"`
}

type ReviewCall struct {
	Alias            string    `json:"alias"`
	ReviewedAt       time.Time `json:"reviewedAt"`
	GenerationID     string    `json:"generationId,omitempty"`
	LatencyMS        int64     `json:"latencyMs"`
	PromptTokens     int64     `json:"promptTokens"`
	CompletionTokens int64     `json:"completionTokens"`
	ChargedAmountUSD string    `json:"chargedAmountUsd,omitempty"`
	ChargedNanoUSD   int64     `json:"chargedNanoUsd,omitempty"`
	RequestSHA256    string    `json:"requestSha256,omitempty"`
	Attempt          int       `json:"attempt,omitempty"`
}

// ReviewAttempt is the immutable paid-request ledger for a hosted review.
// Failed attempts remain visible so a resumed run cannot reset its ceilings.
type ReviewAttempt struct {
	Alias            string    `json:"alias"`
	Attempt          int       `json:"attempt"`
	RequestedAt      time.Time `json:"requestedAt"`
	RequestSHA256    string    `json:"requestSha256"`
	State            string    `json:"state"`
	GenerationID     string    `json:"generationId,omitempty"`
	LatencyMS        int64     `json:"latencyMs"`
	PromptTokens     int64     `json:"promptTokens,omitempty"`
	CompletionTokens int64     `json:"completionTokens,omitempty"`
	ChargedAmountUSD string    `json:"chargedAmountUsd,omitempty"`
	ChargedNanoUSD   int64     `json:"chargedNanoUsd,omitempty"`
	SubmissionSHA256 string    `json:"submissionSha256,omitempty"`
}

type ollamaReviewRequest struct {
	Model     string                `json:"model"`
	Stream    bool                  `json:"stream"`
	Think     bool                  `json:"think"`
	Format    map[string]any        `json:"format"`
	KeepAlive string                `json:"keep_alive"`
	Options   map[string]any        `json:"options"`
	Messages  []ollamaReviewMessage `json:"messages"`
}

type ollamaReviewMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaReviewResponse struct {
	Model   string `json:"model"`
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int64  `json:"prompt_eval_count"`
	EvalCount       int64  `json:"eval_count"`
}

type ollamaReviewTags struct {
	Models []struct {
		Name   string `json:"name"`
		Model  string `json:"model"`
		Digest string `json:"digest"`
	} `json:"models"`
}

func RunOllamaReview(ctx context.Context, config OllamaReviewConfig) (ReviewRun, []fillereval.LabelSubmission, error) {
	baseURL, client, err := validateOllamaReviewConfig(config)
	if err != nil {
		return ReviewRun{}, nil, err
	}
	manifestPath, err := resolveWithin(config.PackageDir, "manifest.json")
	if err != nil {
		return ReviewRun{}, nil, fmt.Errorf("resolve review manifest: %w", err)
	}
	manifest, err := readStrictJSON[Package](manifestPath)
	if err != nil {
		return ReviewRun{}, nil, fmt.Errorf("read review manifest: %w", err)
	}
	if err := validateReviewPackage(config.PackageDir, manifest, config.ExpectedCases); err != nil {
		return ReviewRun{}, nil, err
	}
	if err := verifyReviewModel(ctx, client, baseURL, config.Model, config.ModelDigest); err != nil {
		return ReviewRun{}, nil, err
	}
	transcripts, transcriptSetSHA256, err := indexReviewTranscripts(config.Transcripts, manifest)
	if err != nil {
		return ReviewRun{}, nil, err
	}
	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		return ReviewRun{}, nil, fmt.Errorf("hash review manifest: %w", err)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	run := ReviewRun{SchemaVersion: ReviewRunSchemaVersion, BatchID: manifest.BatchID, PackageManifestSHA256: manifestSHA256, ReviewerID: config.ReviewerID, Provider: "ollama", Model: config.Model, ResolvedModel: config.Model + "@sha256:" + config.ModelDigest, ModelDigest: config.ModelDigest, PromptVersion: OllamaReviewPromptVersion, PromptSHA256: hashBytes([]byte(reviewerSystemPrompt)), TranscriptSetSHA256: transcriptSetSHA256, Cases: len(manifest.Cases)}
	submissions := make([]fillereval.LabelSubmission, 0, len(manifest.Cases))
	for _, item := range manifest.Cases {
		caseCtx, cancel := context.WithTimeout(ctx, config.PerCaseTimeout)
		started := time.Now()
		labels, promptTokens, completionTokens, reviewErr := reviewOne(caseCtx, client, baseURL, config, manifest, item, transcripts)
		latency := time.Since(started)
		cancel()
		if reviewErr != nil {
			return ReviewRun{}, nil, fmt.Errorf("review alias %q: %w", item.Alias, reviewErr)
		}
		run.Requests++
		run.PromptTokens += promptTokens
		run.CompletionTokens += completionTokens
		latencyMS := max(int64(0), latency.Milliseconds())
		run.TotalLatencyMS += latencyMS
		reviewedAt := now().UTC()
		run.Calls = append(run.Calls, ReviewCall{Alias: item.Alias, ReviewedAt: reviewedAt, LatencyMS: latencyMS, PromptTokens: promptTokens, CompletionTokens: completionTokens})
		submissions = append(submissions, fillereval.LabelSubmission{Alias: item.Alias, ReviewerID: config.ReviewerID, BatchID: manifest.BatchID, ReviewedAt: reviewedAt, Labels: fillereval.NormalizeLabels(labels)})
	}
	run.CompletedAt = now().UTC()
	run.SubmissionSHA256 = submissionSHA256(submissions)
	return run, submissions, nil
}

func PublishReview(outputDir string, run ReviewRun, submissions []fillereval.LabelSubmission) error {
	labels, err := encodeReviewSubmissions(submissions)
	if err != nil {
		return err
	}
	if run.SchemaVersion != ReviewRunSchemaVersion || run.Cases != len(submissions) || run.Requests < len(submissions) || len(run.Calls) != len(submissions) || run.SubmissionSHA256 != hashBytes(labels) || strings.TrimSpace(run.BatchID) == "" || strings.TrimSpace(run.ReviewerID) == "" || strings.TrimSpace(run.ResolvedModel) == "" {
		return fmt.Errorf("review publication does not match its attestation")
	}
	if run.Provider == "openrouter" {
		if err := validateHostedReviewAttestation(run, submissions); err != nil {
			return err
		}
	} else if run.Requests != len(submissions) || len(run.Attempts) != 0 {
		return fmt.Errorf("local review publication has an unexpected attempt ledger")
	}
	for index, submission := range submissions {
		call := run.Calls[index]
		if submission.BatchID != run.BatchID || submission.ReviewerID != run.ReviewerID || submission.ReviewedAt.IsZero() || call.Alias != submission.Alias || !call.ReviewedAt.Equal(submission.ReviewedAt) || call.LatencyMS < 0 || call.PromptTokens < 0 || call.CompletionTokens < 0 || call.ChargedNanoUSD < 0 {
			return fmt.Errorf("review submission does not match its attestation")
		}
	}
	output, err := filepath.Abs(outputDir)
	if err != nil || strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("review output path is required")
	}
	if _, err := os.Lstat(output); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("review output already exists: %s", output)
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".filler-completed-review-*")
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.Chmod(temporary, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temporary, "labels.jsonl"), labels, 0o640); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "review-run.json"), raw, 0o640); err != nil {
		return err
	}
	if err := os.Rename(temporary, output); err != nil {
		return err
	}
	published = true
	return nil
}

func validateHostedReviewAttestation(run ReviewRun, submissions []fillereval.LabelSubmission) error {
	if !reviewSHA256(run.PackageManifestSHA256) || !reviewSHA256(run.CapabilitySnapshotSHA256) || !reviewSHA256(run.PromptSHA256) || run.Model == "" || run.ResolvedModel == "" || run.UpstreamProvider == "" || run.UpstreamProviderSlug == "" || run.PromptVersion == "" || run.CompletedAt.IsZero() || run.MaxRequests < run.Cases || run.MaxRequests > run.Cases+1 || run.Requests > run.MaxRequests || run.PromptTokens < 0 || run.CompletionTokens < 0 || run.TotalLatencyMS < 0 || run.MaxSpendNanoUSD <= 0 || run.ChargedNanoUSD < 0 || run.ChargedNanoUSD > run.MaxSpendNanoUSD || run.MaxChargeNanoUSD <= 0 || run.MaxChargeNanoUSD > run.MaxSpendNanoUSD || len(run.Attempts) != run.Requests {
		return fmt.Errorf("hosted review attempt ledger does not match its request count")
	}
	type acceptedAttempt struct {
		attempt ReviewAttempt
	}
	accepted := make(map[string]acceptedAttempt, len(submissions))
	nextAttempt := map[string]int{}
	var promptTokens, completionTokens, latencyMS, chargedNanoUSD int64
	for _, attempt := range run.Attempts {
		nextAttempt[attempt.Alias]++
		settledCharge, err := fillereval.USDToNanoCeil(attempt.ChargedAmountUSD)
		if attempt.Alias == "" || attempt.Attempt != nextAttempt[attempt.Alias] || attempt.RequestedAt.IsZero() || !reviewSHA256(attempt.RequestSHA256) || attempt.State == openRouterAttemptReserved || (attempt.State != openRouterAttemptFailed && attempt.State != openRouterAttemptAccepted) || err != nil || settledCharge != attempt.ChargedNanoUSD || attempt.ChargedNanoUSD < 0 || attempt.ChargedNanoUSD > run.MaxChargeNanoUSD || attempt.LatencyMS < 0 || attempt.PromptTokens < 0 || attempt.CompletionTokens < 0 {
			return fmt.Errorf("hosted review attempt ledger contains an invalid or unsettled attempt")
		}
		if promptTokens > run.PromptTokens-attempt.PromptTokens || completionTokens > run.CompletionTokens-attempt.CompletionTokens || latencyMS > run.TotalLatencyMS-attempt.LatencyMS || chargedNanoUSD > run.ChargedNanoUSD-attempt.ChargedNanoUSD {
			return fmt.Errorf("hosted review attempt accounting exceeds its attestation")
		}
		promptTokens += attempt.PromptTokens
		completionTokens += attempt.CompletionTokens
		latencyMS += attempt.LatencyMS
		chargedNanoUSD += attempt.ChargedNanoUSD
		if attempt.State == openRouterAttemptAccepted {
			if _, duplicate := accepted[attempt.Alias]; duplicate || attempt.GenerationID == "" || !reviewSHA256(attempt.SubmissionSHA256) {
				return fmt.Errorf("hosted review attempt ledger contains a duplicate or unbound accepted attempt")
			}
			accepted[attempt.Alias] = acceptedAttempt{attempt: attempt}
		} else if attempt.SubmissionSHA256 != "" {
			return fmt.Errorf("hosted review failed attempt binds a submission")
		}
	}
	if err := validateHostedReviewAttemptOrder(run.Attempts, submissions); err != nil {
		return err
	}
	if promptTokens != run.PromptTokens || completionTokens != run.CompletionTokens || latencyMS != run.TotalLatencyMS || chargedNanoUSD != run.ChargedNanoUSD || len(accepted) != len(submissions) {
		return fmt.Errorf("hosted review attempt accounting does not match its attestation")
	}
	seen := make(map[string]struct{}, len(submissions))
	for index, submission := range submissions {
		if _, duplicate := seen[submission.Alias]; duplicate {
			return fmt.Errorf("hosted review contains duplicate accepted aliases")
		}
		seen[submission.Alias] = struct{}{}
		bound, ok := accepted[submission.Alias]
		call := run.Calls[index]
		attempt := bound.attempt
		if !ok || attempt.SubmissionSHA256 != submissionSHA256([]fillereval.LabelSubmission{submission}) || call.Alias != submission.Alias || call.Attempt != attempt.Attempt || call.RequestSHA256 != attempt.RequestSHA256 || call.GenerationID != attempt.GenerationID || call.LatencyMS != attempt.LatencyMS || call.PromptTokens != attempt.PromptTokens || call.CompletionTokens != attempt.CompletionTokens || call.ChargedAmountUSD != attempt.ChargedAmountUSD || call.ChargedNanoUSD != attempt.ChargedNanoUSD {
			return fmt.Errorf("hosted review accepted attempt does not bind its exact submission and call")
		}
	}
	return nil
}

func validateHostedReviewAttemptOrder(attempts []ReviewAttempt, submissions []fillereval.LabelSubmission) error {
	attemptIndex := 0
	for _, submission := range submissions {
		if attemptIndex >= len(attempts) || attempts[attemptIndex].Alias != submission.Alias {
			return fmt.Errorf("hosted review attempts do not follow serial submission order")
		}
		accepted := 0
		for attemptIndex < len(attempts) && attempts[attemptIndex].Alias == submission.Alias {
			attempt := attempts[attemptIndex]
			if attempt.State == openRouterAttemptAccepted {
				accepted++
				if attemptIndex+1 < len(attempts) && attempts[attemptIndex+1].Alias == submission.Alias {
					return fmt.Errorf("hosted review failed attempts are not contiguous immediately before the final acceptance in serial order")
				}
			} else if accepted != 0 {
				return fmt.Errorf("hosted review failed attempts follow an accepted attempt in serial order")
			}
			attemptIndex++
		}
		if accepted != 1 {
			return fmt.Errorf("hosted review serial order lacks one final accepted attempt")
		}
	}
	if attemptIndex != len(attempts) {
		return fmt.Errorf("hosted review attempt ledger continues beyond serial submission order")
	}
	return nil
}

func encodeReviewSubmissions(submissions []fillereval.LabelSubmission) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, submission := range submissions {
		if err := encoder.Encode(submission); err != nil {
			return nil, fmt.Errorf("encode review submission: %w", err)
		}
	}
	return output.Bytes(), nil
}

func submissionSHA256(submissions []fillereval.LabelSubmission) string {
	raw, err := encodeReviewSubmissions(submissions)
	if err != nil {
		return ""
	}
	return hashBytes(raw)
}

func validateOllamaReviewConfig(config OllamaReviewConfig) (string, *http.Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || !reviewLoopback(parsed.Hostname()) {
		return "", nil, fmt.Errorf("ollama blind review requires a loopback HTTP API base")
	}
	if strings.TrimSpace(config.PackageDir) == "" || strings.TrimSpace(config.Model) == "" || strings.Contains(strings.ToLower(config.Model), "latest") || !reviewSHA256(config.ModelDigest) || strings.TrimSpace(config.ReviewerID) == "" || config.ExpectedCases <= 0 || config.PerCaseTimeout <= 0 {
		return "", nil, fmt.Errorf("ollama blind review requires package, concrete model/digest, reviewer, expected cases, and timeout")
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-review-ollama", httpx.TimeoutLLM)
	}
	copy := *client
	copy.Timeout = 0
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return baseURL, &copy, nil
}

func validateReviewPackage(root string, manifest Package, expectedCases int) error {
	artifacts, err := validateReviewPackageStructure(manifest, expectedCases)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		path, resolveErr := resolveWithin(root, artifact.Path)
		digest, hashErr := hashFile(path)
		if resolveErr != nil || hashErr != nil || digest != artifact.SHA256 {
			return fmt.Errorf("%s", artifact.Failure)
		}
	}
	return nil
}

type reviewPackageArtifact struct {
	Path    string
	SHA256  string
	Failure string
}

// validateReviewPackageStructure is the single package-shape authority used by
// both live review and descriptor-rooted offline inspection. Its returned file
// set is also the offline inspector's exact package-tree allowlist.
func validateReviewPackageStructure(manifest Package, expectedCases int) ([]reviewPackageArtifact, error) {
	if manifest.SchemaVersion != SchemaVersion || strings.TrimSpace(manifest.BatchID) == "" || !reviewSHA256(manifest.DraftSHA256) || !reviewSHA256(manifest.ReviewPacketSHA256) || manifest.EvidenceVersion == "" || len(manifest.Cases) != expectedCases {
		return nil, fmt.Errorf("review package must use the current schema and contain exactly %d cases", expectedCases)
	}
	artifacts := make([]reviewPackageArtifact, 0, 2+expectedCases*5)
	for _, document := range []struct{ path, digest string }{{manifest.InstructionsPath, manifest.InstructionsSHA256}, {manifest.LabelTemplatePath, manifest.LabelTemplateSHA256}} {
		if _, err := reviewPackageRelativePath(document.path); err != nil || !reviewSHA256(document.digest) {
			return nil, fmt.Errorf("review package document path or digest is invalid")
		}
		artifacts = append(artifacts, reviewPackageArtifact{Path: document.path, SHA256: document.digest, Failure: "review package document failed SHA-256 verification"})
	}
	aliases := map[string]struct{}{}
	for _, item := range manifest.Cases {
		if _, duplicate := aliases[item.Alias]; duplicate || strings.TrimSpace(item.Alias) == "" || !reviewSHA256(item.ContentSHA256) || !reviewSHA256(item.EvidenceSHA256) || item.SegmentDurationMS <= 0 {
			return nil, fmt.Errorf("review package contains an invalid or duplicate alias")
		}
		aliases[item.Alias] = struct{}{}
		frames := 0
		signalIDs := map[string]struct{}{}
		for _, signal := range item.Signals {
			if _, duplicate := signalIDs[signal.ID]; duplicate || signal.ID == "" || signal.Bytes <= 0 || !reviewSHA256(signal.SHA256) {
				return nil, fmt.Errorf("review alias %q contains an invalid or duplicate signal", item.Alias)
			}
			signalIDs[signal.ID] = struct{}{}
			if _, err := reviewPackageRelativePath(signal.Path); err != nil {
				return nil, fmt.Errorf("review alias %q signal %q failed SHA-256 verification", item.Alias, signal.ID)
			}
			artifacts = append(artifacts, reviewPackageArtifact{
				Path: signal.Path, SHA256: signal.SHA256,
				Failure: fmt.Sprintf("review alias %q signal %q failed SHA-256 verification", item.Alias, signal.ID),
			})
			if signal.Kind == "frame" {
				frames++
			}
		}
		if frames != 4 && (frames != 0 || len(item.Signals) != 0 || !decoderProvesUnusable(item.DecoderFacts)) {
			return nil, fmt.Errorf("review alias %q requires exactly four frames", item.Alias)
		}
	}
	return artifacts, nil
}

func reviewPackageRelativePath(path string) (string, error) {
	clean := filepath.Clean(path)
	if path == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != path {
		return "", fmt.Errorf("review package path is not canonical and relative")
	}
	return clean, nil
}

func verifyReviewModel(ctx context.Context, client *http.Client, baseURL, model, digest string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("ollama reviewer model preflight: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	var tags ollamaReviewTags
	if response.StatusCode != http.StatusOK || decodeBoundedReviewJSON(response.Body, &tags) != nil {
		return fmt.Errorf("ollama reviewer model preflight failed")
	}
	for _, installed := range tags.Models {
		if (installed.Name == model || installed.Model == model) && installed.Digest == digest {
			return nil
		}
	}
	return fmt.Errorf("ollama reviewer model %q is not installed at digest %s", model, digest)
}

func indexReviewTranscripts(artifacts []fillerbakeoff.TranscriptArtifact, manifest Package) (map[string]fillerbakeoff.TranscriptArtifact, string, error) {
	indexed := make(map[string]fillerbakeoff.TranscriptArtifact, len(artifacts))
	for _, artifact := range artifacts {
		if !validReviewTranscript(artifact) {
			return nil, "", fmt.Errorf("review transcript has invalid audio, text, or time binding")
		}
		if _, duplicate := indexed[artifact.AudioSHA256]; duplicate {
			return nil, "", fmt.Errorf("duplicate review transcript audio hash")
		}
		indexed[artifact.AudioSHA256] = artifact
	}
	audioSignals := 0
	for _, item := range manifest.Cases {
		for _, signal := range item.Signals {
			if signal.Kind != "audio" {
				continue
			}
			audioSignals++
			artifact, ok := indexed[signal.SHA256]
			if !ok || artifact.AudioBytes != signal.Bytes || artifact.AudioDurationMS != signal.DurationMS {
				return nil, "", fmt.Errorf("review alias %q has no content-bound transcript", item.Alias)
			}
		}
	}
	if len(indexed) != audioSignals {
		return nil, "", fmt.Errorf("review transcript set must cover exactly the package audio signals")
	}
	if len(artifacts) == 0 {
		return indexed, "", nil
	}
	return indexed, fillerbakeoff.TranscriptSetSHA256(artifacts), nil
}

func validReviewTranscript(artifact fillerbakeoff.TranscriptArtifact) bool {
	identity := artifact.Engine
	if artifact.SchemaVersion != fillerbakeoff.TranscriptSchemaVersion || !reviewSHA256(artifact.PacketSHA256) || artifact.EvidenceVersion == "" || artifact.AudioSignalID == "" || !reviewSHA256(artifact.AudioSHA256) || artifact.AudioBytes <= 0 || artifact.AudioDurationMS <= 0 || artifact.GeneratedAt.IsZero() || artifact.LatencyMS < 0 || identity.Provider == "" || identity.ImplementationVersion == "" || identity.Model == "" || !reviewSHA256(identity.BinarySHA256) || !reviewSHA256(identity.ModelSHA256) || artifact.TextSHA256 != hashBytes([]byte(artifact.Text)) {
		return false
	}
	parts := make([]string, 0, len(artifact.Segments))
	var previousEnd int64
	for _, segment := range artifact.Segments {
		if segment.Text == "" || strings.TrimSpace(segment.Text) != segment.Text || segment.StartMS < previousEnd || segment.StartMS < 0 || segment.EndMS <= segment.StartMS || segment.EndMS > artifact.AudioDurationMS {
			return false
		}
		previousEnd = segment.EndMS
		parts = append(parts, segment.Text)
	}
	return strings.Join(parts, "\n") == artifact.Text
}

func reviewOne(ctx context.Context, client *http.Client, baseURL string, config OllamaReviewConfig, manifest Package, item Case, transcripts map[string]fillerbakeoff.TranscriptArtifact) (fillereval.Labels, int64, int64, error) {
	content, images, err := reviewerContent(config.PackageDir, manifest, item, transcripts)
	if err != nil {
		return fillereval.Labels{}, 0, 0, err
	}
	body, err := json.Marshal(ollamaReviewRequest{Model: config.Model, Stream: false, Think: false, Format: reviewLabelsSchema(item), KeepAlive: "10m", Options: map[string]any{"temperature": 0, "num_ctx": 16384, "num_predict": 4096}, Messages: []ollamaReviewMessage{{Role: "system", Content: reviewerSystemPrompt}, {Role: "user", Content: content, Images: images}}})
	if err != nil {
		return fillereval.Labels{}, 0, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fillereval.Labels{}, 0, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fillereval.Labels{}, 0, 0, fmt.Errorf("ollama reviewer request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReviewResponseBytes+1))
	if err != nil || len(raw) > maxReviewResponseBytes {
		return fillereval.Labels{}, 0, 0, fmt.Errorf("ollama reviewer response exceeded its byte ceiling")
	}
	if response.StatusCode != http.StatusOK {
		return fillereval.Labels{}, 0, 0, fmt.Errorf("ollama reviewer returned status %d: %s", response.StatusCode, boundedReviewMessage(raw))
	}
	var wire ollamaReviewResponse
	if decodeProviderReviewJSON(raw, &wire) != nil {
		return fillereval.Labels{}, 0, 0, fmt.Errorf("ollama reviewer returned an invalid response")
	}
	if wire.Model != config.Model || wire.DoneReason != "stop" || wire.Message.Thinking != "" || strings.TrimSpace(wire.Message.Content) == "" {
		return fillereval.Labels{}, wire.PromptEvalCount, wire.EvalCount, fmt.Errorf("ollama reviewer response is unbound, truncated, thinking, or empty (model=%q done=%q thinking=%t contentBytes=%d)", wire.Model, wire.DoneReason, wire.Message.Thinking != "", len(wire.Message.Content))
	}
	labels, err := decodeReviewLabels([]byte(wire.Message.Content))
	if err != nil {
		return fillereval.Labels{}, wire.PromptEvalCount, wire.EvalCount, fmt.Errorf("decode review labels: %w", err)
	}
	if failures := fillereval.ValidateLabels(labels); len(failures) > 0 {
		return fillereval.Labels{}, wire.PromptEvalCount, wire.EvalCount, fmt.Errorf("invalid review labels: %s", strings.Join(failures, "; "))
	}
	if err := validateReviewEvidence(item, labels.Evidence, transcripts); err != nil {
		return fillereval.Labels{}, wire.PromptEvalCount, wire.EvalCount, err
	}
	return labels, wire.PromptEvalCount, wire.EvalCount, nil
}

func reviewerContent(root string, manifest Package, item Case, transcripts map[string]fillerbakeoff.TranscriptArtifact) (string, []string, error) {
	type visibleSignal struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Path       string `json:"path"`
		AtMS       int64  `json:"atMs"`
		DurationMS int64  `json:"durationMs"`
	}
	type visibleDecoderFact struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Claim      string `json:"claim"`
		Value      string `json:"value"`
		Location   string `json:"location"`
		Provenance string `json:"provenance"`
	}
	type visibleTranscript struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Text       string `json:"text"`
		Provenance string `json:"provenance"`
	}
	visible := make([]visibleSignal, 0, len(item.Signals))
	decoderFacts := make([]visibleDecoderFact, 0, len(item.DecoderFacts))
	for index, fact := range item.DecoderFacts {
		decoderFacts = append(decoderFacts, visibleDecoderFact{ID: fmt.Sprintf("decoder-%02d", index+1), Kind: fact.Kind, Claim: fact.Claim, Value: fact.Value, Location: fact.Location, Provenance: fmt.Sprintf("manifest.json#cases/%s/decoderFacts/%d", item.Alias, index)})
	}
	images := make([]string, 0, 4)
	var transcript *visibleTranscript
	for _, signal := range item.Signals {
		if signal.Kind == "frame" {
			visible = append(visible, visibleSignal{ID: signal.ID, Kind: signal.Kind, Path: signal.Path, AtMS: signal.AtMS, DurationMS: signal.DurationMS})
			path, err := resolveWithin(root, signal.Path)
			if err != nil {
				return "", nil, err
			}
			data, err := osReadFileBounded(path, signal.Bytes)
			if err != nil {
				return "", nil, err
			}
			images = append(images, base64.StdEncoding.EncodeToString(data))
		}
		if signal.Kind == "audio" {
			transcript = &visibleTranscript{ID: "transcript-01", Kind: "transcript", Text: transcripts[signal.SHA256].Text, Provenance: signal.Path + "#transcript"}
		}
	}
	payload := struct {
		BatchID      string               `json:"batchId"`
		Alias        string               `json:"alias"`
		DurationMS   int64                `json:"durationMs"`
		DecoderFacts []visibleDecoderFact `json:"decoderFacts"`
		Signals      []visibleSignal      `json:"signals"`
		Transcript   *visibleTranscript   `json:"transcript,omitempty"`
	}{manifest.BatchID, item.Alias, item.SegmentDurationMS, decoderFacts, visible, transcript}
	raw, err := json.Marshal(payload)
	return string(raw), images, err
}

func validateReviewEvidence(item Case, evidence []fillereval.Evidence, transcripts map[string]fillerbakeoff.TranscriptArtifact) error {
	signals := make(map[string]Signal, len(item.Signals))
	for _, signal := range item.Signals {
		signals[signal.ID] = signal
	}
	for _, fact := range evidence {
		if fact.ID == "transcript-01" {
			matchedTranscript := false
			for _, signal := range item.Signals {
				_, ok := transcripts[signal.SHA256]
				if signal.Kind == "audio" && ok && fact.Kind == "transcript" && fact.Provenance == signal.Path+"#transcript" && fact.AtMS == 0 {
					matchedTranscript = true
					break
				}
			}
			if !matchedTranscript {
				return fmt.Errorf("review transcript evidence does not bind its package audio (kind=%q provenance=%q atMs=%d)", fact.Kind, fact.Provenance, fact.AtMS)
			}
			continue
		}
		signal, ok := signals[fact.ID]
		if !ok {
			matchedDecoder := false
			for index, decoder := range item.DecoderFacts {
				if fact.ID == fmt.Sprintf("decoder-%02d", index+1) && fact.Kind == decoder.Kind && fact.Provenance == fmt.Sprintf("manifest.json#cases/%s/decoderFacts/%d", item.Alias, index) && fact.AtMS == 0 {
					matchedDecoder = true
					break
				}
			}
			if !matchedDecoder {
				return fmt.Errorf("review evidence references unknown package evidence %q", fact.ID)
			}
			continue
		}
		if fact.Kind != signal.Kind || fact.Provenance != signal.Path || (signal.Kind == "frame" && fact.AtMS != signal.AtMS) || (signal.DurationMS > 0 && (fact.AtMS < 0 || fact.AtMS > signal.DurationMS)) {
			return fmt.Errorf("review evidence %q does not bind its package signal (kind=%q provenance=%q atMs=%d; expected kind=%q provenance=%q atMs=%d)", fact.ID, fact.Kind, fact.Provenance, fact.AtMS, signal.Kind, signal.Path, signal.AtMS)
		}
	}
	return nil
}

func decoderProvesUnusable(facts []DecoderFact) bool {
	return slices.ContainsFunc(facts, func(fact DecoderFact) bool {
		return fact.Kind == "decoder" && fact.Claim == "media_usability" && fact.Value == "unusable"
	})
}

func reviewLabelsSchema(item Case) map[string]any {
	evidenceChoices := make([]any, 0, len(item.Signals)+len(item.DecoderFacts)+1)
	addEvidence := func(id, kind, provenance string, atMS int64) {
		evidenceChoices = append(evidenceChoices, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "kind", "claim", "value", "provenance", "atMs"}, "properties": map[string]any{
			"id": map[string]any{"const": id}, "kind": map[string]any{"const": kind}, "claim": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "provenance": map[string]any{"const": provenance}, "atMs": map[string]any{"const": atMS},
		}})
	}
	for _, signal := range item.Signals {
		switch signal.Kind {
		case "frame":
			addEvidence(signal.ID, signal.Kind, signal.Path, signal.AtMS)
		case "audio":
			addEvidence("transcript-01", "transcript", signal.Path+"#transcript", 0)
		}
	}
	for index := range item.DecoderFacts {
		addEvidence(fmt.Sprintf("decoder-%02d", index+1), item.DecoderFacts[index].Kind, fmt.Sprintf("manifest.json#cases/%s/decoderFacts/%d", item.Alias, index), 0)
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"disposition", "contentRole", "taxonomy", "policyFlags", "slices", "evidence", "reviewQuestion"}, "properties": map[string]any{
		"disposition":    map[string]any{"type": "string", "enum": []string{"eligible", "invalid_deterministic", "invalid_semantic", "ambiguous"}},
		"contentRole":    map[string]any{"type": "string", "enum": []string{"", "commercial", "promo", "bumper", "psa", "station_id", "trailer", "interstitial", "programme_excerpt", "compilation"}},
		"taxonomy":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		"policyFlags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"slices":         map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
		"evidence":       map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"oneOf": evidenceChoices}},
		"reviewQuestion": map[string]any{"type": "string"},
	}}
}

type reviewLabelsWire struct {
	Disposition    string                `json:"disposition"`
	ContentRole    string                `json:"contentRole"`
	Taxonomy       map[string][]string   `json:"taxonomy"`
	PolicyFlags    []string              `json:"policyFlags"`
	Slices         []string              `json:"slices"`
	Evidence       []fillereval.Evidence `json:"evidence"`
	ReviewQuestion string                `json:"reviewQuestion"`
}

func decodeReviewLabels(raw []byte) (fillereval.Labels, error) {
	var wire reviewLabelsWire
	if err := decodeStrictReviewJSON(raw, &wire); err != nil {
		return fillereval.Labels{}, err
	}
	labels := fillereval.Labels{ContentRole: wire.ContentRole, Taxonomy: wire.Taxonomy, PolicyFlags: wire.PolicyFlags, Slices: wire.Slices, Evidence: wire.Evidence, ReviewQuestion: wire.ReviewQuestion}
	switch wire.Disposition {
	case "eligible":
		labels.Truth = fillereval.TruthEligible
	case "invalid_deterministic":
		labels.Truth, labels.RejectClass = fillereval.TruthInvalid, fillereval.RejectDeterministic
	case "invalid_semantic":
		labels.Truth, labels.RejectClass = fillereval.TruthInvalid, fillereval.RejectSemantic
	case "ambiguous":
		labels.Truth = fillereval.TruthAmbiguous
	default:
		return fillereval.Labels{}, fmt.Errorf("invalid review disposition %q", wire.Disposition)
	}
	return labels, nil
}

const reviewerSystemPrompt = `Review one identity-blind filler clip from only the supplied decoder facts, transcript, and ordered frames when present. Return strict JSON. Follow the closed role definitions in the package instructions. Set disposition to eligible for a standalone filler unit, invalid_deterministic for unusable media, invalid_semantic for programme excerpts or unresolved compilations, and ambiguous only with one concrete answerable reviewQuestion. Set reviewQuestion to an empty string unless disposition is ambiguous. Eligible and invalid_semantic cases require a closed contentRole; invalid_deterministic or ambiguous cases may use an empty contentRole only when the evidence cannot establish one. Taxonomy keys and values and policyFlags must be literal observations, never guesses. For each evidence entry, choose one supplied evidence alternative; its id, kind, provenance, and atMs are fixed by the schema, while claim and value state the bounded conclusion it supports. Do not identify the source or infer facts not present in the evidence.`

func decodeBoundedReviewJSON(reader io.Reader, output any) error {
	raw, err := io.ReadAll(io.LimitReader(reader, maxReviewResponseBytes+1))
	if err != nil || len(raw) > maxReviewResponseBytes {
		return fmt.Errorf("response exceeds byte ceiling")
	}
	return decodeProviderReviewJSON(raw, output)
}

func decodeProviderReviewJSON(raw []byte, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing provider JSON value")
	}
	return nil
}

func boundedReviewMessage(raw []byte) string {
	message := strings.TrimSpace(string(raw))
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

func decodeStrictReviewJSON(raw []byte, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func osReadFileBounded(path string, expected int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, expected+1))
	if err != nil || int64(len(data)) != expected {
		return nil, fmt.Errorf("signal size changed")
	}
	return data, nil
}

func reviewLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func reviewSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return (r < '0' || r > '9') && (r < 'a' || r > 'f') }) == -1
}
