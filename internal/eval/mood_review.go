//go:build eval

package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
)

const (
	MoodReviewSchemaVersion        = 1
	MoodReviewContractVersion      = "query-mood-model-review-v1"
	MoodReviewRubricVersion        = "movie-mood-ordinal-v1"
	MoodReviewPromptVersion        = "query-mood-blind-review-v1"
	MoodReviewStatusModelAttested  = "model-attested-development"
	MoodReviewCompletenessComplete = "complete"
	MoodReviewCompletenessPartial  = "partial-uncertain"
)

var moodAxisNames = []string{"valence", "arousal", "threatFear", "comedicWarmth", "attentionalDemand"}

// MoodReviewPacket is the complete model-visible contract. It deliberately has
// no corpus keys, request text, expected polarity, ownership facts, or prior
// reviewer answers. DisplayTitle is disclosed because the cited evidence is
// title-specific; the authority records that limitation rather than calling the
// review anonymous.
type MoodReviewPacket struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ContractVersion string               `json:"contractVersion"`
	RubricVersion   string               `json:"rubricVersion"`
	PromptVersion   string               `json:"promptVersion"`
	PacketID        string               `json:"packetId"`
	PreparedAt      time.Time            `json:"preparedAt"`
	Evidence        []MoodReviewEvidence `json:"evidence"`
	Cases           []MoodReviewCase     `json:"cases"`
}

type MoodReviewEvidence struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Observed    string `json:"observed"`
	Summary     string `json:"summary"`
	Uncertainty string `json:"uncertainty"`
	SHA256      string `json:"sha256"`
}

type MoodReviewCase struct {
	Alias        string   `json:"alias"`
	DisplayTitle string   `json:"displayTitle"`
	EvidenceIDs  []string `json:"evidenceIds"`
}

// MoodReviewPrivateMap is withheld from reviewers. It is the only bridge from
// a packet alias to a corpus key and is applied only after terminal decisions.
type MoodReviewPrivateMap struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ContractVersion string               `json:"contractVersion"`
	PacketID        string               `json:"packetId"`
	PacketSHA256    string               `json:"packetSha256"`
	Entries         []MoodReviewMapEntry `json:"entries"`
}

type MoodReviewMapEntry struct {
	Alias string        `json:"alias"`
	Key   provision.Key `json:"key"`
}

type MoodReviewerIdentity struct {
	ID                     string `json:"id"`
	Provider               string `json:"provider"`
	Route                  string `json:"route"`
	RouteSlug              string `json:"routeSlug,omitempty"`
	Model                  string `json:"model"`
	ResolvedModel          string `json:"resolvedModel"`
	ModelFamily            string `json:"modelFamily"`
	IdentityKind           string `json:"identityKind"`
	IdentitySHA256         string `json:"identitySha256"`
	SnapshotSHA256         string `json:"snapshotSha256,omitempty"`
	ZeroDataRetention      bool   `json:"zeroDataRetention"`
	RetentionAuthorization string `json:"retentionAuthorization,omitempty"`
	PromptVersion          string `json:"promptVersion"`
}

type MoodReviewInference struct {
	CompletedAt      time.Time `json:"completedAt"`
	Attempts         int       `json:"attempts"`
	PromptTokens     int       `json:"promptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	LatencyMS        int64     `json:"latencyMs"`
	CostBasis        string    `json:"costBasis"`
	ChargeAmount     string    `json:"chargeAmount,omitempty"`
	ChargeCurrency   string    `json:"chargeCurrency,omitempty"`
	GenerationID     string    `json:"generationId,omitempty"`
}

type MoodReviewSubmission struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ContractVersion string               `json:"contractVersion"`
	PacketSHA256    string               `json:"packetSha256"`
	Reviewer        MoodReviewerIdentity `json:"reviewer"`
	Output          string               `json:"output"`
	OutputSHA256    string               `json:"outputSha256"`
	Inference       MoodReviewInference  `json:"inference"`
}

type MoodReviewOutput struct {
	Assessments []MoodReviewAssessment `json:"assessments"`
}

type MoodReviewAssessment struct {
	Alias         string         `json:"alias"`
	Scores        MoodAxisScores `json:"scores"`
	UncertainAxes []string       `json:"uncertainAxes,omitempty"`
	EvidenceIDs   []string       `json:"evidenceIds"`
	Rationale     string         `json:"rationale"`
}

type MoodAxisScores struct {
	Valence           int `json:"valence"`
	Arousal           int `json:"arousal"`
	ThreatFear        int `json:"threatFear"`
	ComedicWarmth     int `json:"comedicWarmth"`
	AttentionalDemand int `json:"attentionalDemand"`
}

type MoodReviewAuthority struct {
	SchemaVersion    int                  `json:"schemaVersion"`
	ContractVersion  string               `json:"contractVersion"`
	Status           string               `json:"status"`
	Completeness     string               `json:"completeness"`
	PacketSHA256     string               `json:"packetSha256"`
	PrivateMapSHA256 string               `json:"privateMapSha256"`
	SubmissionSHA256 []string             `json:"submissionSha256"`
	Decisions        []MoodReviewDecision `json:"decisions"`
	Limitations      []string             `json:"limitations"`
}

type MoodReviewDecision struct {
	Key           provision.Key  `json:"key"`
	Scores        MoodAxisScores `json:"scores"`
	UncertainAxes []string       `json:"uncertainAxes,omitempty"`
}

type MoodReviewRunConfig struct {
	ReviewerID               string
	ModelFamily              string
	IdentityKind             string
	IdentitySHA256           string
	ExpectedResolvedProvider string
	RouteSlug                string
	SnapshotSHA256           string
	ZeroDataRetention        bool
	RetentionAuthorization   string
	Now                      func() time.Time
}

const moodReviewSystemPrompt = `You are an independent evidence reviewer. Assess only the title-specific evidence in the supplied blinded packet. Do not infer the hidden user request, expected polarity, corpus identity, ownership, or another reviewer's answer.

Score every candidate on five ordinal axes from 0 through 3:
- valence: 0 strongly negative/dark, 1 somewhat negative, 2 somewhat positive, 3 strongly positive/reassuring
- arousal: 0 very calm, 1 low, 2 elevated, 3 intense
- threatFear: 0 none, 1 mild, 2 substantial, 3 severe/sustained
- comedicWarmth: 0 absent/cold, 1 slight, 2 clear, 3 central/strong
- attentionalDemand: 0 background-friendly, 1 low, 2 moderate, 3 close attention required

If the supplied evidence does not support an axis, put that exact axis name in uncertainAxes; the numeric placeholder will not be treated as evidence. Cite only evidenceIds attached to that candidate. Return only JSON with this shape: {"assessments":[{"alias":"...","scores":{"valence":0,"arousal":0,"threatFear":0,"comedicWarmth":0,"attentionalDemand":0},"uncertainAxes":[],"evidenceIds":["..."],"rationale":"..."}]}. Include exactly one assessment for every candidate.`

// RunMoodReview performs one provider-neutral, blinded JSON review turn and
// converts the adapter's observed attribution into a lockable submission.
func RunMoodReview(ctx context.Context, provider llm.Provider, packetBlob []byte, config MoodReviewRunConfig) (MoodReviewSubmission, error) {
	var packet MoodReviewPacket
	if err := decodeMoodReviewJSON(packetBlob, &packet); err != nil {
		return MoodReviewSubmission{}, fmt.Errorf("decode mood review packet: %w", err)
	}
	caseEvidence, aliases, err := validateMoodReviewPacket(packet)
	if err != nil {
		return MoodReviewSubmission{}, err
	}
	if provider == nil || strings.TrimSpace(config.ReviewerID) == "" || strings.TrimSpace(config.ModelFamily) == "" || !slices.Contains([]string{"ollama-model-digest", "openrouter-route-snapshot"}, config.IdentityKind) || !moodReviewSHA(config.IdentitySHA256) {
		return MoodReviewSubmission{}, fmt.Errorf("mood review run identity is incomplete")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	temperature := 0.0
	response, err := provider.Chat(ctx, []llm.Message{{Role: llm.System, Content: moodReviewSystemPrompt}, {Role: llm.User, Content: string(packetBlob)}}, llm.ChatOptions{JSONMode: true, Temperature: &temperature, MaxTokens: 2048})
	if err != nil {
		return MoodReviewSubmission{}, fmt.Errorf("run mood review: %w", err)
	}
	if response.WantsTools() || strings.TrimSpace(response.Content) == "" {
		return MoodReviewSubmission{}, fmt.Errorf("mood review returned tools or empty output")
	}
	attribution := response.Attribution
	if attribution.RequestedProvider == "" || attribution.RequestedModel == "" || attribution.ResolvedProvider == "" || attribution.ResolvedModel == "" {
		return MoodReviewSubmission{}, fmt.Errorf("mood review provider attribution is incomplete")
	}
	if config.ExpectedResolvedProvider != "" && attribution.ResolvedProvider != config.ExpectedResolvedProvider {
		return MoodReviewSubmission{}, fmt.Errorf("mood review resolved provider %q, want %q", attribution.ResolvedProvider, config.ExpectedResolvedProvider)
	}
	inference := MoodReviewInference{
		CompletedAt: now().UTC(), Attempts: attribution.Attempts,
		PromptTokens: attribution.Tokens.Prompt, CompletionTokens: attribution.Tokens.Completion,
		LatencyMS: max(int64(1), attribution.Latency.Milliseconds()), GenerationID: attribution.GenerationID,
	}
	route := attribution.ResolvedProvider
	if attribution.RequestedProvider == "ollama" {
		route, inference.CostBasis = "loopback", "local-unmetered"
	} else {
		inference.CostBasis = "provider-reported"
		if attribution.Charge != nil {
			inference.ChargeAmount, inference.ChargeCurrency = attribution.Charge.Amount, attribution.Charge.Currency
		}
	}
	submission := MoodReviewSubmission{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		PacketSHA256: moodReviewSHA256(packetBlob),
		Reviewer: MoodReviewerIdentity{
			ID: config.ReviewerID, Provider: attribution.RequestedProvider, Route: route,
			RouteSlug: config.RouteSlug, Model: attribution.RequestedModel, ResolvedModel: attribution.ResolvedModel,
			ModelFamily: config.ModelFamily, IdentityKind: config.IdentityKind,
			IdentitySHA256: config.IdentitySHA256, SnapshotSHA256: config.SnapshotSHA256,
			ZeroDataRetention: config.ZeroDataRetention, RetentionAuthorization: config.RetentionAuthorization,
			PromptVersion: MoodReviewPromptVersion,
		},
		Output: response.Content, OutputSHA256: moodReviewSHA256([]byte(response.Content)), Inference: inference,
	}
	if _, err := validateMoodReviewSubmission(submission, moodReviewSHA256(packetBlob), aliases, caseEvidence); err != nil {
		return MoodReviewSubmission{}, fmt.Errorf("validate mood review result: %w", err)
	}
	return submission, nil
}

// MoodReviewEvidenceSHA256 binds the evidence fields without recursively
// including the digest itself.
func MoodReviewEvidenceSHA256(evidence MoodReviewEvidence) string {
	evidence.SHA256 = ""
	blob, _ := json.Marshal(evidence)
	return moodReviewSHA256(blob)
}

// CompileMoodReviewAuthority validates and locks two independent submissions,
// plus an optional third-family adjudication. It does not expose private corpus
// keys until every input has been validated and every axis is terminal.
func CompileMoodReviewAuthority(packetBlob, mapBlob []byte, submissionBlobs ...[]byte) (MoodReviewAuthority, error) {
	if len(submissionBlobs) < 2 || len(submissionBlobs) > 3 {
		return MoodReviewAuthority{}, fmt.Errorf("mood review requires two submissions and at most one adjudicator")
	}
	var packet MoodReviewPacket
	if err := decodeMoodReviewJSON(packetBlob, &packet); err != nil {
		return MoodReviewAuthority{}, fmt.Errorf("decode mood review packet: %w", err)
	}
	packetSHA := moodReviewSHA256(packetBlob)
	caseEvidence, aliases, err := validateMoodReviewPacket(packet)
	if err != nil {
		return MoodReviewAuthority{}, err
	}
	var privateMap MoodReviewPrivateMap
	if err := decodeMoodReviewJSON(mapBlob, &privateMap); err != nil {
		return MoodReviewAuthority{}, fmt.Errorf("decode mood review private map: %w", err)
	}
	keys, err := validateMoodReviewMap(privateMap, packet, packetSHA, aliases)
	if err != nil {
		return MoodReviewAuthority{}, err
	}

	outputs := make([]map[string]MoodReviewAssessment, 0, len(submissionBlobs))
	submissionSHA := make([]string, 0, len(submissionBlobs))
	families := make(map[string]bool)
	reviewers := make(map[string]bool)
	usedRetainingRoute := false
	for index, blob := range submissionBlobs {
		var submission MoodReviewSubmission
		if err := decodeMoodReviewJSON(blob, &submission); err != nil {
			return MoodReviewAuthority{}, fmt.Errorf("decode mood review submission %d: %w", index+1, err)
		}
		output, err := validateMoodReviewSubmission(submission, packetSHA, aliases, caseEvidence)
		if err != nil {
			return MoodReviewAuthority{}, fmt.Errorf("mood review submission %d: %w", index+1, err)
		}
		if !registeredMoodReviewerFamily(submission.Reviewer) {
			return MoodReviewAuthority{}, fmt.Errorf("mood review submission %d uses an unregistered model family", index+1)
		}
		family := strings.ToLower(strings.TrimSpace(submission.Reviewer.ModelFamily))
		if families[family] || reviewers[submission.Reviewer.ID] {
			return MoodReviewAuthority{}, fmt.Errorf("mood review submissions must use distinct reviewer identities and registered model families")
		}
		families[family], reviewers[submission.Reviewer.ID] = true, true
		usedRetainingRoute = usedRetainingRoute || (submission.Reviewer.Provider == "openrouter" && !submission.Reviewer.ZeroDataRetention)
		outputs = append(outputs, output)
		submissionSHA = append(submissionSHA, moodReviewSHA256(blob))
	}

	authority := MoodReviewAuthority{
		SchemaVersion: MoodReviewSchemaVersion, ContractVersion: MoodReviewContractVersion,
		Status: MoodReviewStatusModelAttested, Completeness: MoodReviewCompletenessComplete, PacketSHA256: packetSHA,
		PrivateMapSHA256: moodReviewSHA256(mapBlob), SubmissionSHA256: submissionSHA,
		Limitations: []string{
			"reviewers saw display titles because the evidence is title-specific",
			"model attestation is exposed development evidence, not human review, certification, qualification, or a sealed holdout",
		},
	}
	if usedRetainingRoute {
		authority.Limitations = append(authority.Limitations, "an explicitly authorized OpenRouter reviewer used a non-ZDR route for this public-only evidence packet; provider data collection was denied but temporary retention may occur")
	}
	for _, alias := range aliases {
		decision := decideMoodReview(outputs, alias)
		decision.Key = keys[alias]
		if len(decision.UncertainAxes) != 0 {
			authority.Completeness = MoodReviewCompletenessPartial
		}
		authority.Decisions = append(authority.Decisions, decision)
	}
	return authority, nil
}

func registeredMoodReviewerFamily(identity MoodReviewerIdentity) bool {
	model := strings.ToLower(identity.Model)
	resolved := strings.ToLower(identity.ResolvedModel)
	switch strings.ToLower(identity.ModelFamily) {
	case "gemini":
		return identity.Provider == "openrouter" && strings.HasPrefix(model, "google/gemini-") && strings.HasPrefix(resolved, "google/gemini-")
	case "qwen3.5":
		return identity.Provider == "ollama" && strings.HasPrefix(model, "qwen3.5:") && strings.HasPrefix(resolved, "qwen3.5:")
	case "gemma4":
		return identity.Provider == "ollama" && strings.HasPrefix(model, "gemma4:") && strings.HasPrefix(resolved, "gemma4:")
	default:
		return false
	}
}

func validateMoodReviewPacket(packet MoodReviewPacket) (map[string]map[string]bool, []string, error) {
	if packet.SchemaVersion != MoodReviewSchemaVersion || packet.ContractVersion != MoodReviewContractVersion || packet.RubricVersion != MoodReviewRubricVersion || packet.PromptVersion != MoodReviewPromptVersion || strings.TrimSpace(packet.PacketID) == "" || packet.PreparedAt.IsZero() || len(packet.Evidence) == 0 || len(packet.Cases) == 0 {
		return nil, nil, fmt.Errorf("mood review packet identity is incomplete")
	}
	evidence := make(map[string]bool)
	for _, item := range packet.Evidence {
		parsed, err := url.Parse(item.URL)
		if item.ID == "" || evidence[item.ID] || err != nil || parsed.Scheme != "https" || parsed.Host == "" || item.Observed == "" || strings.TrimSpace(item.Summary) == "" || strings.TrimSpace(item.Uncertainty) == "" || item.SHA256 != MoodReviewEvidenceSHA256(item) {
			return nil, nil, fmt.Errorf("mood review packet has invalid evidence %q", item.ID)
		}
		evidence[item.ID] = true
	}
	caseEvidence := make(map[string]map[string]bool)
	aliases := make([]string, 0, len(packet.Cases))
	for _, item := range packet.Cases {
		if strings.TrimSpace(item.Alias) == "" || caseEvidence[item.Alias] != nil || strings.TrimSpace(item.DisplayTitle) == "" || len(item.EvidenceIDs) == 0 {
			return nil, nil, fmt.Errorf("mood review packet has invalid case %q", item.Alias)
		}
		allowed := make(map[string]bool)
		for _, evidenceID := range item.EvidenceIDs {
			if !evidence[evidenceID] || allowed[evidenceID] {
				return nil, nil, fmt.Errorf("mood review case %q has invalid evidence reference", item.Alias)
			}
			allowed[evidenceID] = true
		}
		caseEvidence[item.Alias] = allowed
		aliases = append(aliases, item.Alias)
	}
	return caseEvidence, aliases, nil
}

func validateMoodReviewMap(privateMap MoodReviewPrivateMap, packet MoodReviewPacket, packetSHA string, aliases []string) (map[string]provision.Key, error) {
	if privateMap.SchemaVersion != MoodReviewSchemaVersion || privateMap.ContractVersion != MoodReviewContractVersion || privateMap.PacketID != packet.PacketID || privateMap.PacketSHA256 != packetSHA || len(privateMap.Entries) != len(aliases) {
		return nil, fmt.Errorf("mood review private map does not bind the packet")
	}
	keys := make(map[string]provision.Key)
	seenKeys := make(map[provision.Key]bool)
	for _, entry := range privateMap.Entries {
		if !slices.Contains(aliases, entry.Alias) || keys[entry.Alias] != "" || seenKeys[entry.Key] {
			return nil, fmt.Errorf("mood review private map has an invalid or duplicate entry")
		}
		if _, _, _, ok := provision.ParseKey(entry.Key); !ok {
			return nil, fmt.Errorf("mood review private map has invalid key %q", entry.Key)
		}
		keys[entry.Alias], seenKeys[entry.Key] = entry.Key, true
	}
	return keys, nil
}

func validateMoodReviewSubmission(submission MoodReviewSubmission, packetSHA string, aliases []string, caseEvidence map[string]map[string]bool) (map[string]MoodReviewAssessment, error) {
	identity := submission.Reviewer
	if submission.SchemaVersion != MoodReviewSchemaVersion || submission.ContractVersion != MoodReviewContractVersion || submission.PacketSHA256 != packetSHA ||
		strings.TrimSpace(identity.ID) == "" || strings.TrimSpace(identity.Provider) == "" || strings.TrimSpace(identity.Route) == "" || strings.TrimSpace(identity.Model) == "" ||
		strings.TrimSpace(identity.ResolvedModel) == "" || strings.TrimSpace(identity.ModelFamily) == "" || !slices.Contains([]string{"ollama-model-digest", "openrouter-route-snapshot"}, identity.IdentityKind) || identity.PromptVersion != MoodReviewPromptVersion || !moodReviewSHA(identity.IdentitySHA256) || submission.OutputSHA256 != moodReviewSHA256([]byte(submission.Output)) {
		return nil, fmt.Errorf("identity, packet, prompt, model identity digest, or output digest is incomplete")
	}
	if identity.IdentityKind == "ollama-model-digest" {
		if identity.Provider != "ollama" || identity.Route != "loopback" || identity.RouteSlug != "" || identity.SnapshotSHA256 != "" || identity.ZeroDataRetention || identity.RetentionAuthorization != "" || submission.Inference.CostBasis != "local-unmetered" {
			return nil, fmt.Errorf("local mood reviewer identity does not match its route or accounting")
		}
	} else if identity.Provider != "openrouter" || identity.Route == "" || identity.Route == "loopback" || identity.RouteSlug == "" || !moodReviewSHA(identity.SnapshotSHA256) || submission.Inference.CostBasis != "provider-reported" {
		return nil, fmt.Errorf("hosted mood reviewer identity does not match its route snapshot or accounting")
	}
	if identity.IdentityKind == "openrouter-route-snapshot" && identity.ZeroDataRetention == (identity.RetentionAuthorization != "") {
		return nil, fmt.Errorf("hosted mood reviewer retention posture or authorization is invalid")
	}
	inference := submission.Inference
	if inference.CompletedAt.IsZero() || inference.Attempts < 1 || inference.PromptTokens < 1 || inference.CompletionTokens < 1 || inference.LatencyMS < 1 || (inference.CostBasis != "local-unmetered" && inference.CostBasis != "provider-reported") || (inference.CostBasis == "provider-reported" && (inference.ChargeAmount == "" || inference.ChargeCurrency == "" || inference.GenerationID == "")) {
		return nil, fmt.Errorf("inference accounting is incomplete")
	}
	var output MoodReviewOutput
	if err := decodeMoodReviewJSON([]byte(submission.Output), &output); err != nil {
		return nil, fmt.Errorf("decode bound output: %w", err)
	}
	if len(output.Assessments) != len(aliases) {
		return nil, fmt.Errorf("output does not cover every packet case")
	}
	result := make(map[string]MoodReviewAssessment)
	for _, assessment := range output.Assessments {
		allowed := caseEvidence[assessment.Alias]
		if allowed == nil || result[assessment.Alias].Alias != "" || !assessment.Scores.valid() || strings.TrimSpace(assessment.Rationale) == "" || len(assessment.EvidenceIDs) == 0 {
			return nil, fmt.Errorf("output has invalid assessment %q", assessment.Alias)
		}
		seenAxes := make(map[string]bool)
		for _, axis := range assessment.UncertainAxes {
			if !slices.Contains(moodAxisNames, axis) || seenAxes[axis] {
				return nil, fmt.Errorf("assessment %q has invalid uncertain axis", assessment.Alias)
			}
			seenAxes[axis] = true
		}
		seenEvidence := make(map[string]bool)
		for _, evidenceID := range assessment.EvidenceIDs {
			if !allowed[evidenceID] || seenEvidence[evidenceID] {
				return nil, fmt.Errorf("assessment %q cites evidence outside its packet case", assessment.Alias)
			}
			seenEvidence[evidenceID] = true
		}
		result[assessment.Alias] = assessment
	}
	return result, nil
}

func decideMoodReview(outputs []map[string]MoodReviewAssessment, alias string) MoodReviewDecision {
	decision := MoodReviewDecision{}
	chosen := make([]int, len(moodAxisNames))
	for index, axis := range moodAxisNames {
		first, firstOK := outputs[0][alias].axis(axis, index)
		second, secondOK := outputs[1][alias].axis(axis, index)
		if firstOK && secondOK && first == second {
			chosen[index] = first
			continue
		}
		if len(outputs) == 3 {
			third, thirdOK := outputs[2][alias].axis(axis, index)
			if thirdOK && ((firstOK && third == first) || (secondOK && third == second)) {
				chosen[index] = third
				continue
			}
		}
		decision.UncertainAxes = append(decision.UncertainAxes, axis)
	}
	decision.Scores = scoresFromAxes(chosen)
	return decision
}

func (assessment MoodReviewAssessment) axis(name string, index int) (int, bool) {
	if slices.Contains(assessment.UncertainAxes, name) {
		return 0, false
	}
	return assessment.Scores.axes()[index], true
}

func (scores MoodAxisScores) valid() bool {
	for _, value := range scores.axes() {
		if value < 0 || value > 3 {
			return false
		}
	}
	return true
}

func (scores MoodAxisScores) axes() []int {
	return []int{scores.Valence, scores.Arousal, scores.ThreatFear, scores.ComedicWarmth, scores.AttentionalDemand}
}

func scoresFromAxes(values []int) MoodAxisScores {
	return MoodAxisScores{Valence: values[0], Arousal: values[1], ThreatFear: values[2], ComedicWarmth: values[3], AttentionalDemand: values[4]}
}

func decodeMoodReviewJSON(blob []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing data")
	}
	return nil
}

func moodReviewSHA256(blob []byte) string {
	digest := sha256.Sum256(blob)
	return hex.EncodeToString(digest[:])
}

func moodReviewSHA(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
