// Package plannerreference binds a planner scorecard to the exact local model,
// runtime, host, and cold/warm protocol used to produce it.
package plannerreference

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	manifestSchemaVersion = 1
	contractVersion       = "planner-reference-host-v1"
	maxScorecardBytes     = 16 << 20
	maxCaptureBytes       = 1 << 20
	maxEvidenceBytes      = 1 << 20
	maxEvidenceTotalBytes = 8 << 20
)

var (
	referenceToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*$`)
	slugToken      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	repositoryID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	quantizationID = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]{1,31}$`)
	licenseID      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,79}$`)
)

var requiredEvidenceKinds = []string{
	"ollama-list.json",
	"ollama-ps-after.json",
	"ollama-ps-cold-before.json",
	"ollama-ps-warm-before.json",
	"ollama-show.json",
	"ollama-version.txt",
	"sw-vers.txt",
	"system-profiler.json",
	"uname.txt",
}

// RawInputs is the complete evidence presented at the module interface. The
// evidence map is keyed by the fixed capture filename, never by a local path.
type RawInputs struct {
	Scorecard   []byte
	Capture     []byte
	Evidence    map[string][]byte
	GeneratedAt time.Time
}

// Artifact is the canonical publication and its identity.
type Artifact struct {
	JSON   []byte
	SHA256 string
}

type capture struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Contract        string              `json:"contract"`
	RunID           string              `json:"runId"`
	StartedAt       time.Time           `json:"startedAt"`
	CompletedAt     time.Time           `json:"completedAt"`
	ScorecardSHA256 string              `json:"scorecardSha256"`
	ScorecardBytes  int64               `json:"scorecardBytes"`
	Model           modelCapture        `json:"model"`
	Runtime         runtimeCapture      `json:"runtime"`
	Protocol        protocolCapture     `json:"protocol"`
	Residency       residencyCapture    `json:"residency"`
	Evidence        []evidenceReference `json:"evidence"`
}

type modelCapture struct {
	Tag              string `json:"tag"`
	OllamaDigest     string `json:"ollamaDigest"`
	SourceRepository string `json:"sourceRepository"`
	SourceRevision   string `json:"sourceRevision"`
	GGUFFile         string `json:"ggufFile"`
	GGUFSHA256       string `json:"ggufSha256"`
	Quantization     string `json:"quantization"`
	ContextLength    int    `json:"contextLength"`
	TemplateSHA256   string `json:"templateSha256"`
	ModelfileSHA256  string `json:"modelfileSha256"`
	LicenseID        string `json:"licenseId"`
	LicenseSHA256    string `json:"licenseSha256"`
}

type runtimeCapture struct {
	OllamaVersion              string `json:"ollamaVersion"`
	MacOSVersion               string `json:"macosVersion"`
	MacOSBuild                 string `json:"macosBuild"`
	Architecture               string `json:"architecture"`
	HardwareModel              string `json:"hardwareModel"`
	Chip                       string `json:"chip"`
	PhysicalUnifiedMemoryBytes int64  `json:"physicalUnifiedMemoryBytes"`
}

type protocolCapture struct {
	Profile            string  `json:"profile"`
	ContextLength      int     `json:"contextLength"`
	MaxOutputTokens    int     `json:"maxOutputTokens"`
	Temperature        float64 `json:"temperature"`
	Seed               int64   `json:"seed"`
	ColdRuns           int     `json:"coldRuns"`
	UnreportedWarmups  int     `json:"unreportedWarmups"`
	MeasuredWarmTrials int     `json:"measuredWarmTrials"`
}

type residencyCapture struct {
	ColdBefore selectedResidency `json:"coldBefore"`
	WarmBefore selectedResidency `json:"warmBefore"`
	After      selectedResidency `json:"after"`
}

type selectedResidency struct {
	SelectedModelResident bool   `json:"selectedModelResident"`
	Model                 string `json:"model,omitempty"`
	OllamaDigest          string `json:"ollamaDigest,omitempty"`
	RAMBytes              int64  `json:"ramBytes,omitempty"`
	VRAMBytes             int64  `json:"vramBytes,omitempty"`
}

type evidenceReference struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type scorecardEnvelope struct {
	SchemaVersion int                  `json:"schemaVersion"`
	CorpusVersion string               `json:"corpusVersion"`
	Profile       string               `json:"profile"`
	Generator     scorecardIdentity    `json:"generator"`
	Contract      scorecardContract    `json:"contract"`
	Assessment    *scorecardAssessment `json:"assessment"`
	Cases         []scorecardCase      `json:"cases"`
}

type scorecardIdentity struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type scorecardContract struct {
	CorpusVersion        string `json:"corpusVersion"`
	CatalogFixtureSHA256 string `json:"catalogFixtureSha256"`
	PromptVersion        string `json:"promptVersion"`
	ToolSchemaVersion    string `json:"toolSchemaVersion"`
	ScorerVersion        string `json:"scorerVersion"`
}

type scorecardAssessment struct {
	Performance scorecardPerformance `json:"performance"`
}

type scorecardPerformance struct {
	ResourceStatus string `json:"resourceStatus"`
	ResourceSource string `json:"resourceSource"`
	PeakRAMBytes   int64  `json:"peakRamBytes"`
	PeakVRAMBytes  int64  `json:"peakVramBytes"`
}

type scorecardCase struct {
	Trials int `json:"trials"`
}

type manifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Contract      string              `json:"contract"`
	GeneratedAt   time.Time           `json:"generatedAt"`
	RunID         string              `json:"runId"`
	StartedAt     time.Time           `json:"startedAt"`
	CompletedAt   time.Time           `json:"completedAt"`
	Scorecard     scorecardReference  `json:"scorecard"`
	Model         modelCapture        `json:"model"`
	Runtime       runtimeCapture      `json:"runtime"`
	Protocol      protocolCapture     `json:"protocol"`
	Residency     residencyCapture    `json:"residency"`
	Evidence      []evidenceReference `json:"evidence"`
}

type scorecardReference struct {
	SHA256                string `json:"sha256"`
	Bytes                 int64  `json:"bytes"`
	SchemaVersion         int    `json:"schemaVersion"`
	CorpusVersion         string `json:"corpusVersion"`
	Profile               string `json:"profile"`
	GeneratorProvider     string `json:"generatorProvider"`
	GeneratorModel        string `json:"generatorModel"`
	ContractCorpusVersion string `json:"contractCorpusVersion"`
	CatalogFixtureSHA256  string `json:"catalogFixtureSha256"`
	PromptVersion         string `json:"promptVersion"`
	ToolSchemaVersion     string `json:"toolSchemaVersion"`
	ScorerVersion         string `json:"scorerVersion"`
}

// BuildManifest validates and cross-binds all reference-host evidence, then
// returns one canonical publication. It performs no host command, model call,
// download, or filesystem write.
func BuildManifest(in RawInputs) (Artifact, error) {
	if err := bounded("scorecard", in.Scorecard, maxScorecardBytes); err != nil {
		return Artifact{}, err
	}
	if err := bounded("capture", in.Capture, maxCaptureBytes); err != nil {
		return Artifact{}, err
	}
	if in.GeneratedAt.IsZero() {
		return Artifact{}, errors.New("generatedAt is required")
	}

	var captured capture
	if err := decodeStrict("capture", in.Capture, &captured); err != nil {
		return Artifact{}, err
	}
	var card scorecardEnvelope
	if err := decodeScorecard(in.Scorecard, &card); err != nil {
		return Artifact{}, err
	}
	if err := validateCapture(captured, in.GeneratedAt); err != nil {
		return Artifact{}, err
	}
	scorecardSum := sha256.Sum256(in.Scorecard)
	if captured.ScorecardSHA256 != hex.EncodeToString(scorecardSum[:]) || captured.ScorecardBytes != int64(len(in.Scorecard)) {
		return Artifact{}, errors.New("capture scorecard digest or byte count does not match raw scorecard")
	}
	if err := validateScorecard(card, captured); err != nil {
		return Artifact{}, err
	}
	evidence, err := validateEvidence(captured.Evidence, in.Evidence)
	if err != nil {
		return Artifact{}, err
	}

	out := manifest{
		SchemaVersion: manifestSchemaVersion,
		Contract:      contractVersion,
		GeneratedAt:   in.GeneratedAt.UTC(),
		RunID:         captured.RunID,
		StartedAt:     captured.StartedAt.UTC(),
		CompletedAt:   captured.CompletedAt.UTC(),
		Scorecard: scorecardReference{
			SHA256: hex.EncodeToString(scorecardSum[:]), Bytes: int64(len(in.Scorecard)),
			SchemaVersion: card.SchemaVersion, CorpusVersion: card.CorpusVersion,
			Profile: card.Profile, GeneratorProvider: card.Generator.Provider,
			GeneratorModel: card.Generator.Model, ContractCorpusVersion: card.Contract.CorpusVersion,
			CatalogFixtureSHA256: card.Contract.CatalogFixtureSHA256,
			PromptVersion:        card.Contract.PromptVersion, ToolSchemaVersion: card.Contract.ToolSchemaVersion,
			ScorerVersion: card.Contract.ScorerVersion,
		},
		Model: captured.Model, Runtime: captured.Runtime, Protocol: captured.Protocol,
		Residency: captured.Residency, Evidence: evidence,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("marshal manifest: %w", err)
	}
	raw = append(raw, '\n')
	sum := sha256.Sum256(raw)
	return Artifact{JSON: raw, SHA256: hex.EncodeToString(sum[:])}, nil
}

func validateCapture(c capture, generatedAt time.Time) error {
	if c.SchemaVersion != 1 || c.Contract != contractVersion {
		return fmt.Errorf("capture schema/contract = %d/%q, want 1/%q", c.SchemaVersion, c.Contract, contractVersion)
	}
	if err := validText("runId", c.RunID, 128, slugToken); err != nil {
		return err
	}
	if c.StartedAt.IsZero() || c.CompletedAt.IsZero() || c.CompletedAt.Before(c.StartedAt) {
		return errors.New("benchmark start/end times are missing or reversed")
	}
	if c.CompletedAt.Sub(c.StartedAt) > 24*time.Hour || generatedAt.Before(c.CompletedAt) {
		return errors.New("benchmark duration exceeds 24h or generatedAt precedes completion")
	}
	if err := validateModel(c.Model); err != nil {
		return err
	}
	if err := validateRuntime(c.Runtime); err != nil {
		return err
	}
	if err := validateProtocol(c.Protocol, c.Model); err != nil {
		return err
	}
	return validateResidency(c.Residency, c.Model)
}

func validateModel(m modelCapture) error {
	if err := validText("model.tag", m.Tag, 256, referenceToken); err != nil {
		return err
	}
	if !strings.Contains(m.Tag, ":") {
		return errors.New("model.tag must include an explicit immutable tag")
	}
	if !isSHA256(m.OllamaDigest) || !isSHA256(m.GGUFSHA256) || !isSHA256(m.TemplateSHA256) ||
		!isSHA256(m.ModelfileSHA256) || !isSHA256(m.LicenseSHA256) {
		return errors.New("model artifact, GGUF, template, Modelfile, and license identities must be lowercase SHA-256")
	}
	if !repositoryID.MatchString(m.SourceRepository) || strings.Contains(m.SourceRepository, "..") {
		return errors.New("model.sourceRepository must be an owner/repository id")
	}
	if len(m.SourceRevision) < 40 || len(m.SourceRevision) > 64 || !isLowerHex(m.SourceRevision) {
		return errors.New("model.sourceRevision must be a 40..64 character lowercase hexadecimal revision")
	}
	if filepath.Base(m.GGUFFile) != m.GGUFFile || !strings.HasSuffix(strings.ToLower(m.GGUFFile), ".gguf") || len(m.GGUFFile) > 240 {
		return errors.New("model.ggufFile must be a bounded basename ending in .gguf")
	}
	if !quantizationID.MatchString(m.Quantization) {
		return errors.New("model.quantization is invalid")
	}
	if m.ContextLength != 8192 && m.ContextLength != 16384 {
		return errors.New("model.contextLength must be 8192 or 16384")
	}
	if !licenseID.MatchString(m.LicenseID) {
		return errors.New("model.licenseId is invalid")
	}
	return nil
}

func validateRuntime(r runtimeCapture) error {
	values := []struct{ name, value string }{
		{"runtime.ollamaVersion", r.OllamaVersion}, {"runtime.macosVersion", r.MacOSVersion},
		{"runtime.macosBuild", r.MacOSBuild}, {"runtime.hardwareModel", r.HardwareModel},
		{"runtime.chip", r.Chip},
	}
	for _, item := range values {
		if err := validText(item.name, item.value, 160, nil); err != nil {
			return err
		}
		if strings.ContainsAny(item.value, `/\\`) || strings.Contains(item.value, "..") {
			return fmt.Errorf("%s contains path-like text", item.name)
		}
	}
	if r.Architecture != "arm64" {
		return errors.New("runtime.architecture must be arm64 for the reference host")
	}
	if r.PhysicalUnifiedMemoryBytes < 64<<30 || r.PhysicalUnifiedMemoryBytes > 512<<30 {
		return errors.New("runtime.physicalUnifiedMemoryBytes is outside the 64..512 GiB reference-host range")
	}
	return nil
}

func validateProtocol(p protocolCapture, m modelCapture) error {
	if err := validText("protocol.profile", p.Profile, 128, slugToken); err != nil {
		return err
	}
	if p.ContextLength != m.ContextLength || p.MaxOutputTokens < 1 || p.MaxOutputTokens > p.ContextLength {
		return errors.New("protocol context/output limits are invalid or inconsistent with the model")
	}
	if math.IsNaN(p.Temperature) || math.IsInf(p.Temperature, 0) || p.Temperature < 0 || p.Temperature > 2 {
		return errors.New("protocol.temperature must be finite in 0..2")
	}
	if p.Seed < 0 || p.ColdRuns != 1 || p.UnreportedWarmups < 1 || p.UnreportedWarmups > 5 ||
		p.MeasuredWarmTrials < 1 || p.MeasuredWarmTrials > 10 {
		return errors.New("protocol seed or cold/warm run counts are invalid")
	}
	return nil
}

func validateResidency(r residencyCapture, m modelCapture) error {
	if r.ColdBefore.SelectedModelResident || r.ColdBefore.Model != "" || r.ColdBefore.OllamaDigest != "" ||
		r.ColdBefore.RAMBytes != 0 || r.ColdBefore.VRAMBytes != 0 {
		return errors.New("residency.coldBefore must prove the selected model was absent")
	}
	for _, item := range []struct {
		name   string
		sample selectedResidency
	}{{"warmBefore", r.WarmBefore}, {"after", r.After}} {
		name, sample := item.name, item.sample
		if !sample.SelectedModelResident || sample.Model != m.Tag || sample.OllamaDigest != m.OllamaDigest ||
			sample.RAMBytes < 0 || sample.VRAMBytes <= 0 || sample.RAMBytes > math.MaxInt64-sample.VRAMBytes {
			return fmt.Errorf("residency.%s does not bind the selected resident model and positive bounded memory", name)
		}
	}
	if r.WarmBefore.RAMBytes != r.After.RAMBytes || r.WarmBefore.VRAMBytes != r.After.VRAMBytes {
		return errors.New("warm-before and after residency disagree")
	}
	return nil
}

func validateScorecard(card scorecardEnvelope, c capture) error {
	if card.SchemaVersion != 10 || card.CorpusVersion == "" || card.Contract.CorpusVersion != card.CorpusVersion {
		return errors.New("scorecard schema/corpus identity is invalid")
	}
	if card.Profile != c.Protocol.Profile || card.Generator.Provider != "ollama" || card.Generator.Model != c.Model.Tag {
		return errors.New("scorecard profile or generator does not match the captured local model")
	}
	if !isSHA256(card.Contract.CatalogFixtureSHA256) {
		return errors.New("scorecard catalog fixture digest is invalid")
	}
	for _, item := range []struct{ name, value string }{
		{"promptVersion", card.Contract.PromptVersion},
		{"toolSchemaVersion", card.Contract.ToolSchemaVersion},
		{"scorerVersion", card.Contract.ScorerVersion},
	} {
		if err := validText("scorecard."+item.name, item.value, 160, referenceToken); err != nil {
			return err
		}
	}
	if card.Assessment == nil || card.Assessment.Performance.ResourceStatus != "measured" ||
		card.Assessment.Performance.ResourceSource != "ollama:/api/ps" {
		return errors.New("scorecard lacks measured Ollama residency")
	}
	performance := card.Assessment.Performance
	if performance.PeakRAMBytes != c.Residency.After.RAMBytes || performance.PeakVRAMBytes != c.Residency.After.VRAMBytes {
		return errors.New("scorecard peak residency does not match the captured after sample")
	}
	if len(card.Cases) == 0 {
		return errors.New("scorecard contains no case summaries")
	}
	for _, item := range card.Cases {
		if item.Trials != c.Protocol.MeasuredWarmTrials {
			return errors.New("scorecard trials do not match the measured warm protocol")
		}
	}
	return nil
}

func validateEvidence(declared []evidenceReference, raw map[string][]byte) ([]evidenceReference, error) {
	if len(declared) != len(requiredEvidenceKinds) || len(raw) != len(requiredEvidenceKinds) {
		return nil, errors.New("evidence set is incomplete or contains extra files")
	}
	byKind := make(map[string]evidenceReference, len(declared))
	for _, item := range declared {
		if _, exists := byKind[item.Kind]; exists {
			return nil, fmt.Errorf("duplicate evidence kind %q", item.Kind)
		}
		byKind[item.Kind] = item
	}
	var total int64
	out := make([]evidenceReference, 0, len(requiredEvidenceKinds))
	for _, kind := range requiredEvidenceKinds {
		decl, ok := byKind[kind]
		value, rawOK := raw[kind]
		if !ok || !rawOK {
			return nil, fmt.Errorf("missing evidence %q", kind)
		}
		if err := bounded("evidence "+kind, value, maxEvidenceBytes); err != nil {
			return nil, err
		}
		total += int64(len(value))
		if total > maxEvidenceTotalBytes {
			return nil, errors.New("evidence exceeds aggregate byte ceiling")
		}
		sum := sha256.Sum256(value)
		if decl.Kind != kind || decl.Bytes != int64(len(value)) || decl.SHA256 != hex.EncodeToString(sum[:]) {
			return nil, fmt.Errorf("evidence %q digest or byte count does not match raw capture", kind)
		}
		out = append(out, decl)
	}
	for kind := range raw {
		if !slices.Contains(requiredEvidenceKinds, kind) {
			return nil, fmt.Errorf("unexpected evidence %q", kind)
		}
	}
	return out, nil
}

func decodeStrict(name string, raw []byte, out any) error {
	if err := rejectDuplicateKeys(raw); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if err := requireEOF(decoder); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func decodeScorecard(raw []byte, out *scorecardEnvelope) error {
	if err := rejectDuplicateKeys(raw); err != nil {
		return fmt.Errorf("decode scorecard: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode scorecard: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return fmt.Errorf("decode scorecard: %w", err)
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return keyErr
			}
			key, keyOK := keyToken.(string)
			if !keyOK {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func bounded(name string, raw []byte, limit int) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s is empty", name)
	}
	if len(raw) > limit {
		return fmt.Errorf("%s exceeds %d-byte ceiling", name, limit)
	}
	return nil
}

func validText(name, value string, maxRunes int, pattern *regexp.Regexp) error {
	if value == "" || strings.TrimSpace(value) != value || len([]rune(value)) > maxRunes {
		return fmt.Errorf("%s is empty, untrimmed, or over-bound", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	if pattern != nil && !pattern.MatchString(value) {
		return fmt.Errorf("%s has an invalid format", name)
	}
	return nil
}

func isSHA256(value string) bool { return len(value) == 64 && isLowerHex(value) }

func isLowerHex(value string) bool {
	if value == "" || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
