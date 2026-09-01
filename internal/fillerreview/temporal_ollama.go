package fillerreview

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/httpx"
)

const OllamaTemporalPromptVersion = "filler-temporal-unit-role-ollama-v4"

type OllamaTemporalConfig struct {
	PackagePath    string
	BaseURL        string
	Model          string
	ModelFamily    string
	ModelDigest    string
	AssessorID     string
	ExpectedCases  int
	PerCaseTimeout time.Duration
	Client         *http.Client
	Now            func() time.Time
}

type temporalUnitWire struct {
	Kind              string   `json:"kind"`
	DecisiveSignalIDs []string `json:"decisiveSignalIds"`
	Reason            string   `json:"reason"`
}

type temporalRoleWire struct {
	Kind              string   `json:"kind"`
	DecisiveSignalIDs []string `json:"decisiveSignalIds"`
	Reason            string   `json:"reason"`
}

type temporalCallError struct {
	code      fillereval.TemporalFailureCode
	detail    string
	retryable bool
}

func (e *temporalCallError) Error() string { return e.detail }

// RunOllamaTemporalAssessment makes one serial unit request per case and a
// second role request only for standalone units. Case-level failures remain
// explicit assessments; they do not erase the rest of the diagnostic run.
func RunOllamaTemporalAssessment(ctx context.Context, config OllamaTemporalConfig) (fillereval.TemporalAssessmentSet, error) {
	baseURL, client, err := validateOllamaTemporalConfig(config)
	if err != nil {
		return fillereval.TemporalAssessmentSet{}, err
	}
	pack, cases, packageSHA256, err := LoadTemporalReviewPackage(config.PackagePath, config.ExpectedCases)
	if err != nil {
		return fillereval.TemporalAssessmentSet{}, err
	}
	if err := verifyReviewModel(ctx, client, baseURL, config.Model, config.ModelDigest); err != nil {
		return fillereval.TemporalAssessmentSet{}, err
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	set := fillereval.TemporalAssessmentSet{
		SchemaVersion: fillereval.TemporalAssessmentSchemaVersion, ContractVersion: fillereval.TemporalAssessmentContractVersion,
		BatchID: pack.BatchID, PackageSHA256: packageSHA256,
		Assessor: fillereval.TemporalAssessorIdentity{
			ID: config.AssessorID, Provider: "ollama", Model: config.Model, ModelFamily: config.ModelFamily,
			ModelDigest: config.ModelDigest, PromptVersion: OllamaTemporalPromptVersion,
		},
		Assessments: make([]fillereval.TemporalAssessment, 0, len(pack.Cases)),
	}
	root := filepath.Dir(config.PackagePath)
	for index, item := range pack.Cases {
		caseCtx, cancel := context.WithTimeout(ctx, config.PerCaseTimeout)
		assessment, calls, callErr := assessTemporalCase(caseCtx, client, baseURL, config.Model, root, item)
		cancel()
		assessment.Alias = item.Alias
		assessment = fillereval.NormalizeTemporalAssessment(assessment)
		assessment.Inference = temporalInferenceFromCalls(now().UTC(), calls)
		if callErr != nil {
			var failure *temporalCallError
			if !errors.As(callErr, &failure) {
				failure = &temporalCallError{code: fillereval.TemporalFailureProvider, detail: callErr.Error(), retryable: true}
			}
			assessment.Unit, assessment.Role = nil, nil
			assessment.OperationalFailure = &fillereval.TemporalOperationalFailure{Code: failure.code, Detail: boundedTemporalDetail(failure.detail), Retryable: failure.retryable}
		}
		one := set
		one.Assessments = []fillereval.TemporalAssessment{assessment}
		if err := fillereval.ValidateTemporalAssessmentSet(one, pack.BatchID, packageSHA256, cases[index:index+1]); err != nil {
			assessment.Unit, assessment.Role = nil, nil
			assessment.OperationalFailure = &fillereval.TemporalOperationalFailure{Code: fillereval.TemporalFailureInvalidResponse, Detail: boundedTemporalDetail(err.Error()), Retryable: false}
			last := len(assessment.Inference.Calls) - 1
			assessment.Inference.Calls[last].OperationalFailure = fillereval.TemporalFailureInvalidResponse
		}
		set.Assessments = append(set.Assessments, assessment)
	}
	if err := fillereval.ValidateTemporalAssessmentSet(set, pack.BatchID, packageSHA256, cases); err != nil {
		return fillereval.TemporalAssessmentSet{}, fmt.Errorf("validate temporal assessment run: %w", err)
	}
	return set, nil
}

func temporalInferenceFromCalls(assessedAt time.Time, calls []fillereval.TemporalInferenceCall) fillereval.TemporalInference {
	inference := fillereval.TemporalInference{AssessedAt: assessedAt, Attempts: len(calls), Calls: calls}
	for _, call := range calls {
		inference.LatencyMS += call.LatencyMS
		inference.PromptTokens += call.PromptTokens
		inference.CompletionTokens += call.CompletionTokens
	}
	return inference
}

func validateOllamaTemporalConfig(config OllamaTemporalConfig) (string, *http.Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || !reviewLoopback(parsed.Hostname()) {
		return "", nil, fmt.Errorf("ollama temporal assessment requires a loopback HTTP API base")
	}
	if strings.TrimSpace(config.PackagePath) == "" || strings.TrimSpace(config.Model) == "" || strings.Contains(strings.ToLower(config.Model), "latest") || strings.TrimSpace(config.ModelFamily) == "" || !reviewSHA256(config.ModelDigest) || strings.TrimSpace(config.AssessorID) == "" || config.ExpectedCases <= 0 || config.PerCaseTimeout <= 0 {
		return "", nil, fmt.Errorf("ollama temporal assessment requires package, concrete model/family/digest, assessor, expected cases, and timeout")
	}
	client := config.Client
	if client == nil {
		client = httpx.NewNamed("filler-temporal-ollama", httpx.TimeoutLLM)
	}
	copy := *client
	copy.Timeout = 0
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return baseURL, &copy, nil
}

func assessTemporalCase(ctx context.Context, client *http.Client, baseURL, model, root string, item TemporalReviewCase) (fillereval.TemporalAssessment, []fillereval.TemporalInferenceCall, error) {
	content, images, err := temporalReviewerContent(root, item)
	if err != nil {
		failure := &temporalCallError{code: fillereval.TemporalFailureEvidence, detail: err.Error()}
		return fillereval.TemporalAssessment{}, []fillereval.TemporalInferenceCall{{Axis: "unit", Attempt: 1, OperationalFailure: failure.code}}, failure
	}
	unitRaw, unitCall, err := callTemporalModel(ctx, client, baseURL, model, "unit", 1, temporalUnitSchema(item), temporalUnitSystemPrompt, content, images)
	calls := []fillereval.TemporalInferenceCall{unitCall}
	if err != nil {
		return fillereval.TemporalAssessment{}, calls, err
	}
	var unit temporalUnitWire
	if err := decodeStrictReviewJSON(unitRaw, &unit); err != nil {
		failure := &temporalCallError{code: fillereval.TemporalFailureInvalidResponse, detail: "unit assessment JSON is invalid: " + err.Error()}
		calls[0].OperationalFailure = failure.code
		return fillereval.TemporalAssessment{}, calls, failure
	}
	assessment := fillereval.TemporalAssessment{Unit: &fillereval.UnitAssessment{Kind: fillereval.UnitKind(unit.Kind), DecisiveSignalIDs: unit.DecisiveSignalIDs, Reason: strings.TrimSpace(unit.Reason)}}
	if assessment.Unit.Kind != fillereval.UnitStandalone {
		return assessment, calls, nil
	}
	roleContent := content + "\nThe independent unit pass classified this span as standalone. Classify only its semantic role."
	roleRaw, roleCall, err := callTemporalModel(ctx, client, baseURL, model, "role", 2, temporalRoleSchema(item), temporalRoleSystemPrompt, roleContent, images)
	calls = append(calls, roleCall)
	if err != nil {
		return fillereval.TemporalAssessment{}, calls, err
	}
	var role temporalRoleWire
	if err := decodeStrictReviewJSON(roleRaw, &role); err != nil {
		failure := &temporalCallError{code: fillereval.TemporalFailureInvalidResponse, detail: "role assessment JSON is invalid: " + err.Error()}
		calls[1].OperationalFailure = failure.code
		return fillereval.TemporalAssessment{}, calls, failure
	}
	assessment.Role = &fillereval.RoleAssessment{Kind: fillereval.TemporalRole(role.Kind), DecisiveSignalIDs: role.DecisiveSignalIDs, Reason: strings.TrimSpace(role.Reason)}
	return assessment, calls, nil
}

func callTemporalModel(ctx context.Context, client *http.Client, baseURL, model, axis string, attempt int, schema map[string]any, systemPrompt, content string, images []string) (_ []byte, call fillereval.TemporalInferenceCall, returnErr error) {
	call.Axis, call.Attempt = axis, attempt
	started := time.Now()
	defer func() {
		call.LatencyMS = max(int64(0), time.Since(started).Milliseconds())
		var failure *temporalCallError
		if errors.As(returnErr, &failure) {
			call.OperationalFailure = failure.code
		}
	}()
	body, err := json.Marshal(ollamaReviewRequest{
		Model: model, Stream: false, Think: false, Format: schema, KeepAlive: "10m",
		Options:  map[string]any{"temperature": 0, "num_ctx": 32768, "num_predict": 1024},
		Messages: []ollamaReviewMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: content, Images: images}},
	})
	if err != nil {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureEvidence, detail: err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureProvider, detail: err.Error(), retryable: true}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, call, &temporalCallError{code: fillereval.TemporalFailureTimeout, detail: "per-case inference deadline exceeded", retryable: true}
		}
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureProvider, detail: err.Error(), retryable: true}
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReviewResponseBytes+1))
	if err != nil || len(raw) > maxReviewResponseBytes {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureProvider, detail: "response exceeded its byte ceiling", retryable: true}
	}
	call.ResponseSHA256 = hashBytes(raw)
	if response.StatusCode != http.StatusOK {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureProvider, detail: fmt.Sprintf("provider returned status %d: %s", response.StatusCode, boundedReviewMessage(raw)), retryable: response.StatusCode >= 500}
	}
	var provider ollamaReviewResponse
	if err := decodeProviderReviewJSON(raw, &provider); err != nil {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureInvalidResponse, detail: "provider envelope is invalid"}
	}
	call.PromptTokens, call.CompletionTokens = provider.PromptEvalCount, provider.EvalCount
	if provider.Model != model || provider.DoneReason != "stop" || provider.Message.Thinking != "" || strings.TrimSpace(provider.Message.Content) == "" {
		return nil, call, &temporalCallError{code: fillereval.TemporalFailureInvalidResponse, detail: fmt.Sprintf("response is unbound, truncated, thinking, or empty (model=%q done=%q thinking=%t contentBytes=%d)", provider.Model, provider.DoneReason, provider.Message.Thinking != "", len(provider.Message.Content))}
	}
	return []byte(provider.Message.Content), call, nil
}

func temporalReviewerContent(root string, item TemporalReviewCase) (string, []string, error) {
	type visibleFrame struct {
		ImageIndex           int                            `json:"imageIndex"`
		ID                   string                         `json:"id"`
		AtMS                 int64                          `json:"atMs"`
		OCRSignalID          string                         `json:"ocrSignalId"`
		OCR                  []TemporalReviewOCRObservation `json:"ocr"`
		CardCandidate        bool                           `json:"cardCandidate"`
		CardCandidateReasons []string                       `json:"cardCandidateReasons"`
	}
	visibleFrames := make([]visibleFrame, 0, len(item.Frames))
	images := make([]string, 0, len(item.Frames))
	for index, frame := range item.Frames {
		path, err := resolveWithin(root, frame.Path)
		if err != nil {
			return "", nil, err
		}
		data, err := osReadFileBounded(path, frame.Bytes)
		if err != nil {
			return "", nil, err
		}
		images = append(images, base64.StdEncoding.EncodeToString(data))
		visibleFrames = append(visibleFrames, visibleFrame{ImageIndex: index + 1, ID: frame.ID, AtMS: frame.AtMS, OCRSignalID: frame.OCRSignalID, OCR: frame.OCR, CardCandidate: frame.CardCandidate, CardCandidateReasons: frame.CardCandidateReasons})
	}
	payload := struct {
		DurationMS         int64                      `json:"durationMs"`
		Frames             []visibleFrame             `json:"frames"`
		TranscriptSegments []TemporalReviewTranscript `json:"transcriptSegments"`
	}{DurationMS: item.DurationMS, Frames: visibleFrames, TranscriptSegments: item.TranscriptSegments}
	raw, err := json.Marshal(payload)
	return string(raw), images, err
}

func temporalUnitSchema(item TemporalReviewCase) map[string]any {
	return temporalClaimSchema(item, []string{"standalone", "compilation", "programme_excerpt", "unusable", "unclear"})
}

func temporalRoleSchema(item TemporalReviewCase) map[string]any {
	return temporalClaimSchema(item, []string{"commercial", "promo", "bumper", "psa", "station_id", "trailer", "interstitial", "unclear"})
}

func temporalClaimSchema(item TemporalReviewCase, kinds []string) map[string]any {
	signalIDs := make([]string, 0, len(item.Frames)*2+len(item.TranscriptSegments))
	for _, frame := range item.Frames {
		signalIDs = append(signalIDs, frame.ID)
		if frame.OCRSignalID != "" {
			signalIDs = append(signalIDs, frame.OCRSignalID)
		}
	}
	for _, segment := range item.TranscriptSegments {
		signalIDs = append(signalIDs, segment.ID)
	}
	references := map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": signalIDs}}
	reason := map[string]any{"type": "string", "minLength": 1, "maxLength": 500}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"kind", "decisiveSignalIds", "reason"},
		"properties": map[string]any{
			"kind":              map[string]any{"type": "string", "enum": kinds},
			"decisiveSignalIds": references,
			"reason":            reason,
		},
	}
}

func boundedTemporalDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 512 {
		return detail[:512]
	}
	if detail == "" {
		return "unspecified temporal assessment failure"
	}
	return detail
}

const temporalUnitSystemPrompt = `Assess the temporal structure of one identity-blind video span using only its ordered images, OCR, card hints, and transcript. Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"],"reason":"one sentence"} with no other keys.

standalone means exactly one bounded, self-contained item. compilation means multiple discrete items or boundaries inside the supplied span. programme_excerpt means an ordinary sustained programme scene without standalone framing. unusable means the evidence cannot be interpreted because the media itself is defective. unclear means the supplied evidence is insufficient to choose. Do not classify semantic purpose and do not use semantic purpose to force a standalone unit.

Cite one to four decisive supplied signal ids. A frame id cites what is visibly present; its OCR signal id cites only machine-read text from that frame; a transcript id cites only that segment. Keep the reason to one evidence-grounded sentence. Never infer source identity, prior labels, or facts absent from the package.`

const temporalRoleSystemPrompt = `Classify the semantic role of one video span already assessed as a standalone unit. Use only its ordered images, OCR, card hints, and transcript. Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"],"reason":"one sentence"} with no other keys.

commercial means a product, brand, service, or paid proposition. promo means a television programme, channel, episode, or scheduled broadcast. bumper means a product-free break transition. psa means a public-interest appeal. station_id means station or network identification. trailer means a theatrical or film release. interstitial means other standalone connective filler. unclear means the standalone role cannot be established.

Cite one to four decisive supplied signal ids. A frame id cites what is visibly present; its OCR signal id cites only machine-read text from that frame; a transcript id cites only that segment. Keep the reason to one evidence-grounded sentence. Never infer source identity, prior labels, or facts absent from the package.`
