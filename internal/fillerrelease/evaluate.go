// Package fillerrelease evaluates one immutable filler release evidence bundle.
// It has no runtime admission or publication authority.
package fillerrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"
)

const manifestSchemaVersion = 1

var (
	requiredAuthorities = []string{
		"enrichment",
		"media_playback",
		"role",
		"spoken",
		"structure",
		"suitability",
		"terminal_admission",
		"visual",
		"written",
	}
	requiredJourneys         = []string{"shield", "web"}
	requiredReleaseArtifacts = []string{
		"deployment",
		"notices",
		"provenance",
		"rollback",
		"sbom",
		"signature",
	}
)

// Verdict is the only release decision emitted by this package.
type Verdict string

const (
	VerdictGo   Verdict = "GO"
	VerdictHold Verdict = "HOLD"
)

// Candidate identifies every independently shipped surface in the release.
type Candidate struct {
	GitCommit                  string `json:"git_commit"`
	Tag                        string `json:"tag"`
	ServerImageDigest          string `json:"server_image_digest"`
	WebBuildIdentity           string `json:"web_build_identity"`
	ShieldArtifactSHA256       string `json:"shield_artifact_sha256"`
	ShieldVersion              string `json:"shield_version"`
	ConfigurationProfileSHA256 string `json:"configuration_profile_sha256"`
}

// Hold is one stable, public-safe reason a candidate cannot be released.
type Hold struct {
	Code     string `json:"code"`
	Subject  string `json:"subject,omitempty"`
	Expected string `json:"expected,omitempty"`
	Observed string `json:"observed,omitempty"`
}

// Check records the bounded result retained in the public report. Artifact
// paths and contents deliberately stay out of it.
type Check struct {
	Kind           string `json:"kind"`
	Subject        string `json:"subject"`
	Status         string `json:"status"`
	Passed         int    `json:"passed,omitempty"`
	Total          int    `json:"total,omitempty"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
}

// Certificate records every declared outcome so a zero-hold result cannot be
// inferred from a rounded pass percentage.
type Certificate struct {
	Subject              string `json:"subject"`
	Status               string `json:"status"`
	Passed               int    `json:"passed"`
	Total                int    `json:"total"`
	Abstentions          int    `json:"abstentions"`
	Holds                int    `json:"holds"`
	ProhibitedAdmissions int    `json:"prohibited_admissions"`
	ArtifactSHA256       string `json:"artifact_sha256"`
}

// Report is the canonical, privacy-safe release decision.
type Report struct {
	SchemaVersion          int           `json:"schema_version"`
	ManifestSchemaVersion  int           `json:"manifest_schema_version,omitempty"`
	ManifestSHA256         string        `json:"manifest_sha256,omitempty"`
	Release                string        `json:"release,omitempty"`
	GeneratedAt            time.Time     `json:"generated_at"`
	Verdict                Verdict       `json:"verdict"`
	Candidate              Candidate     `json:"candidate"`
	CandidateSHA256        string        `json:"candidate_sha256,omitempty"`
	Cohort                 CohortSummary `json:"cohort"`
	Certificates           []Certificate `json:"certificates"`
	Journeys               []Check       `json:"journeys"`
	ReleaseArtifacts       []Check       `json:"release_artifacts"`
	ResidualHumanDecisions int           `json:"residual_human_decisions"`
	OperationalFailures    int           `json:"operational_failures"`
	Holds                  []Hold        `json:"holds"`
	ReportSHA256           string        `json:"report_sha256"`
}

// CohortSummary exposes counts only; source identities and clip-level evidence
// remain confined to the digest-bound manifest.
type CohortSummary struct {
	SHA256                  string `json:"sha256,omitempty"`
	SourceCount             int    `json:"source_count"`
	ClipCount               int    `json:"clip_count"`
	LineageCount            int    `json:"lineage_count"`
	PlaybackDerivativeCount int    `json:"playback_derivative_count"`
}

type manifest struct {
	SchemaVersion          int               `json:"schema_version"`
	Release                string            `json:"release"`
	AssembledAt            time.Time         `json:"assembled_at"`
	ValidUntil             time.Time         `json:"valid_until"`
	Candidate              Candidate         `json:"candidate"`
	Cohort                 cohort            `json:"cohort"`
	Authorities            []authority       `json:"authorities"`
	Journeys               []journey         `json:"journeys"`
	ReleaseArtifacts       []releaseArtifact `json:"release_artifacts"`
	ResidualHumanDecisions int               `json:"residual_human_decisions"`
	OperationalFailures    int               `json:"operational_failures"`
}

type cohort struct {
	SourceIdentities []string `json:"source_identities"`
	Clips            []clip   `json:"clips"`
}

type clip struct {
	ContentSHA256            string `json:"content_sha256"`
	LineageSHA256            string `json:"lineage_sha256"`
	PlaybackDerivativeSHA256 string `json:"playback_derivative_sha256"`
	SourceIdentity           string `json:"source_identity"`
}

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type authority struct {
	Kind                 string   `json:"kind"`
	CandidateSHA256      string   `json:"candidate_sha256"`
	Artifact             artifact `json:"artifact"`
	SchemaIdentity       string   `json:"schema_identity"`
	PolicyIdentity       string   `json:"policy_identity"`
	ModelIdentity        string   `json:"model_identity"`
	ProfileIdentity      string   `json:"profile_identity"`
	BuildIdentity        string   `json:"build_identity"`
	Passed               int      `json:"passed"`
	Total                int      `json:"total"`
	Abstentions          int      `json:"abstentions"`
	Holds                int      `json:"holds"`
	ProhibitedAdmissions int      `json:"prohibited_admissions"`
}

type journey struct {
	Platform                 string   `json:"platform"`
	CandidateSHA256          string   `json:"candidate_sha256"`
	Artifact                 artifact `json:"artifact"`
	BuildIdentity            string   `json:"build_identity"`
	DeploymentTargetIdentity string   `json:"deployment_target_identity"`
	Installed                bool     `json:"installed"`
	ChannelSelected          bool     `json:"channel_selected"`
	PodSelected              bool     `json:"pod_selected"`
	RangePlayback            bool     `json:"range_playback"`
	Passed                   bool     `json:"passed"`
}

type releaseArtifact struct {
	Kind            string   `json:"kind"`
	CandidateSHA256 string   `json:"candidate_sha256"`
	Artifact        artifact `json:"artifact"`
	Verified        bool     `json:"verified"`
}

type evaluation struct {
	root          fs.FS
	report        Report
	holds         map[string]Hold
	artifactPaths map[string]string
}

// Evaluate validates manifestBytes and every referenced artifact against root.
// Readable but malformed manifests still produce a canonical HOLD report.
func Evaluate(root fs.FS, manifestBytes []byte, generatedAt time.Time) Report {
	e := evaluation{
		root: root,
		report: Report{
			SchemaVersion:    1,
			GeneratedAt:      generatedAt.UTC(),
			Verdict:          VerdictHold,
			Certificates:     []Certificate{},
			Journeys:         []Check{},
			ReleaseArtifacts: []Check{},
			Holds:            []Hold{},
		},
		holds:         make(map[string]Hold),
		artifactPaths: make(map[string]string),
	}

	var input manifest
	if err := rejectDuplicateJSONKeys(manifestBytes); err != nil {
		e.addHold("manifest_malformed", "manifest")
		return e.finish()
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		e.addHold("manifest_malformed", "manifest")
		return e.finish()
	}
	if err := requireEOF(decoder); err != nil {
		e.addHold("manifest_malformed", "manifest")
		return e.finish()
	}

	e.report.ManifestSchemaVersion = input.SchemaVersion
	manifestSum := sha256.Sum256(manifestBytes)
	e.report.ManifestSHA256 = hex.EncodeToString(manifestSum[:])
	e.report.Release = input.Release
	e.report.Candidate = input.Candidate
	e.report.Cohort = CohortSummary{
		SHA256:      digestJSON(input.Cohort),
		SourceCount: len(input.Cohort.SourceIdentities),
		ClipCount:   len(input.Cohort.Clips),
	}
	e.report.ResidualHumanDecisions = input.ResidualHumanDecisions
	e.report.OperationalFailures = input.OperationalFailures
	e.report.CandidateSHA256 = candidateDigest(input.Candidate)
	e.validateManifest(input)
	return e.finish()
}

func (e *evaluation) validateManifest(input manifest) {
	if input.SchemaVersion != manifestSchemaVersion {
		e.addHold("manifest_schema_unsupported", "manifest")
	}
	if strings.TrimSpace(input.Release) == "" {
		e.addHold("release_missing", "release")
	}
	e.validateFreshness(input.AssembledAt, input.ValidUntil)
	e.validateCandidate(input.Candidate, input.Release)
	e.validateCohort(input.Cohort)
	e.validateAuthorities(input.Authorities)
	e.validateJourneys(input.Journeys)
	e.validateReleaseArtifacts(input.ReleaseArtifacts)
	if input.ResidualHumanDecisions != 0 {
		e.addHold("residual_human_decisions", "candidate")
	}
	if input.OperationalFailures != 0 {
		e.addHold("operational_failures", "candidate")
	}
}

func (e *evaluation) validateFreshness(assembledAt, validUntil time.Time) {
	if assembledAt.IsZero() || validUntil.IsZero() || !validUntil.After(assembledAt) {
		e.addHold("evidence_window_invalid", "manifest")
		return
	}
	if e.report.GeneratedAt.Before(assembledAt) {
		e.addHold("report_time_before_evidence", "manifest")
	}
	if e.report.GeneratedAt.After(validUntil) {
		e.addHold("evidence_stale", "manifest")
	}
}

func (e *evaluation) validateCandidate(candidate Candidate, release string) {
	identities := []struct {
		subject string
		valid   bool
	}{
		{"git_commit", validIdentity(candidate.GitCommit)},
		{"tag", strings.TrimSpace(candidate.Tag) != ""},
		{"server_image_digest", validImageDigest(candidate.ServerImageDigest)},
		{"web_build_identity", validIdentity(candidate.WebBuildIdentity)},
		{"shield_artifact_sha256", validDigest(candidate.ShieldArtifactSHA256)},
		{"shield_version", strings.TrimSpace(candidate.ShieldVersion) != ""},
		{"configuration_profile_sha256", validDigest(candidate.ConfigurationProfileSHA256)},
	}
	for _, identity := range identities {
		if !identity.valid {
			e.addHold("candidate_identity_invalid", identity.subject)
		}
	}
	if candidate.Tag != release {
		e.addHoldDetail("candidate_tag_mismatch", "tag", release, candidate.Tag)
	}
}

func (e *evaluation) validateCohort(input cohort) {
	if len(input.Clips) != 32 {
		e.addHoldDetail("clip_count_invalid", "review_set", "32", fmt.Sprint(len(input.Clips)))
	}
	declaredSources := make(map[string]struct{}, len(input.SourceIdentities))
	for _, identity := range input.SourceIdentities {
		if strings.TrimSpace(identity) == "" {
			e.addHold("source_identity_invalid", "review_set")
			continue
		}
		if _, exists := declaredSources[identity]; exists {
			e.addHold("source_identity_duplicate", "review_set")
		}
		declaredSources[identity] = struct{}{}
	}
	if len(declaredSources) == 0 {
		e.addHold("source_identity_missing", "review_set")
	}

	contentHashes := make(map[string]struct{}, len(input.Clips))
	lineageHashes := make(map[string]struct{}, len(input.Clips))
	derivativeHashes := make(map[string]struct{}, len(input.Clips))
	for _, item := range input.Clips {
		validateCohortDigest(e, item.ContentSHA256, contentHashes, "clip_content")
		validateCohortDigest(e, item.LineageSHA256, lineageHashes, "clip_lineage")
		validateCohortDigest(e, item.PlaybackDerivativeSHA256, derivativeHashes, "playback_derivative")
		if _, exists := declaredSources[item.SourceIdentity]; !exists {
			e.addHold("clip_source_unbound", "review_set")
		}
	}
	e.report.Cohort.LineageCount = len(lineageHashes)
	e.report.Cohort.PlaybackDerivativeCount = len(derivativeHashes)
}

func validateCohortDigest(e *evaluation, digest string, seen map[string]struct{}, subject string) {
	if !validDigest(digest) {
		e.addHold("cohort_digest_invalid", subject)
		return
	}
	if _, exists := seen[digest]; exists {
		e.addHold("cohort_digest_duplicate", subject)
	}
	seen[digest] = struct{}{}
}

func (e *evaluation) validateAuthorities(authorities []authority) {
	seen := make(map[string]struct{}, len(authorities))
	for _, result := range authorities {
		subject := "authority:" + result.Kind
		if !slices.Contains(requiredAuthorities, result.Kind) {
			e.addHold("authority_unknown", subject)
			continue
		}
		if _, exists := seen[result.Kind]; exists {
			e.addHold("authority_duplicate", subject)
			continue
		}
		seen[result.Kind] = struct{}{}
		passing := e.validateCandidateBinding(result.CandidateSHA256, subject)
		passing = e.validateArtifact(result.Artifact, subject) && passing
		for identityName, identity := range map[string]string{
			"build": result.BuildIdentity, "model": result.ModelIdentity, "policy": result.PolicyIdentity,
			"profile": result.ProfileIdentity, "schema": result.SchemaIdentity,
		} {
			if strings.TrimSpace(identity) == "" {
				e.addHold("authority_identity_missing", subject+":"+identityName)
				passing = false
			}
		}
		if result.Total <= 0 || result.Passed < 0 || result.Abstentions < 0 || result.Holds < 0 || result.Passed+result.Abstentions+result.Holds != result.Total {
			e.addHoldDetail(
				"authority_denominator_invalid",
				subject,
				"passed + abstentions + holds = total; total > 0",
				fmt.Sprintf("%d + %d + %d = %d", result.Passed, result.Abstentions, result.Holds, result.Total),
			)
			passing = false
		}
		if result.Passed != result.Total || result.Abstentions != 0 || result.Holds != 0 {
			e.addHold("authority_not_passing", subject)
			passing = false
		}
		if result.ProhibitedAdmissions != 0 {
			e.addHold("prohibited_admission", subject)
			passing = false
		}
		e.report.Certificates = append(e.report.Certificates, Certificate{
			Subject: result.Kind, Status: checkStatus(passing), Passed: result.Passed, Total: result.Total,
			Abstentions: result.Abstentions, Holds: result.Holds, ProhibitedAdmissions: result.ProhibitedAdmissions,
			ArtifactSHA256: result.Artifact.SHA256,
		})
	}
	for _, kind := range requiredAuthorities {
		if _, exists := seen[kind]; !exists {
			e.addHold("authority_missing", "authority:"+kind)
		}
	}
}

func (e *evaluation) validateJourneys(journeys []journey) {
	seen := make(map[string]struct{}, len(journeys))
	for _, result := range journeys {
		subject := "journey:" + result.Platform
		if !slices.Contains(requiredJourneys, result.Platform) {
			e.addHold("journey_unknown", subject)
			continue
		}
		if _, exists := seen[result.Platform]; exists {
			e.addHold("journey_duplicate", subject)
			continue
		}
		seen[result.Platform] = struct{}{}
		passing := e.validateCandidateBinding(result.CandidateSHA256, subject)
		passing = e.validateArtifact(result.Artifact, subject) && passing
		if strings.TrimSpace(result.BuildIdentity) == "" || strings.TrimSpace(result.DeploymentTargetIdentity) == "" {
			e.addHold("journey_identity_missing", subject)
			passing = false
		}
		if !result.Installed || !result.ChannelSelected || !result.PodSelected || !result.RangePlayback {
			e.addHold("journey_playback_incomplete", subject)
			passing = false
		}
		if !result.Passed {
			e.addHold("journey_not_passing", subject)
			passing = false
		}
		e.report.Journeys = append(e.report.Journeys, Check{
			Kind: "journey", Subject: result.Platform, Status: checkStatus(passing), Passed: boolInt(result.Passed), Total: 1,
			ArtifactSHA256: result.Artifact.SHA256,
		})
	}
	for _, platform := range requiredJourneys {
		if _, exists := seen[platform]; !exists {
			e.addHold("journey_missing", "journey:"+platform)
		}
	}
}

func (e *evaluation) validateReleaseArtifacts(artifacts []releaseArtifact) {
	seen := make(map[string]struct{}, len(artifacts))
	for _, item := range artifacts {
		subject := "release_artifact:" + item.Kind
		if !slices.Contains(requiredReleaseArtifacts, item.Kind) {
			e.addHold("release_artifact_unknown", subject)
			continue
		}
		if _, exists := seen[item.Kind]; exists {
			e.addHold("release_artifact_duplicate", subject)
			continue
		}
		seen[item.Kind] = struct{}{}
		passing := e.validateCandidateBinding(item.CandidateSHA256, subject)
		passing = e.validateArtifact(item.Artifact, subject) && passing
		if !item.Verified {
			e.addHold("release_artifact_unverified", subject)
			passing = false
		}
		e.report.ReleaseArtifacts = append(e.report.ReleaseArtifacts, Check{
			Kind: "release_artifact", Subject: item.Kind, Status: checkStatus(passing), Passed: boolInt(passing), Total: 1,
			ArtifactSHA256: item.Artifact.SHA256,
		})
	}
	for _, kind := range requiredReleaseArtifacts {
		if _, exists := seen[kind]; !exists {
			e.addHold("release_artifact_missing", "release_artifact:"+kind)
		}
	}
}

func (e *evaluation) validateCandidateBinding(actual, subject string) bool {
	if !validDigest(actual) || actual != e.report.CandidateSHA256 {
		e.addHoldDetail("candidate_binding_mismatch", subject, e.report.CandidateSHA256, actual)
		return false
	}
	return true
}

func (e *evaluation) validateArtifact(item artifact, subject string) bool {
	if item.Path == "." || !fs.ValidPath(item.Path) {
		e.addHold("artifact_path_invalid", subject)
		return false
	}
	if prior, exists := e.artifactPaths[item.Path]; exists {
		e.addHold("artifact_path_duplicate", prior)
		e.addHold("artifact_path_duplicate", subject)
		return false
	}
	e.artifactPaths[item.Path] = subject
	if !validDigest(item.SHA256) {
		e.addHold("artifact_digest_invalid", subject)
		return false
	}
	if e.root == nil {
		e.addHold("artifact_missing", subject)
		return false
	}
	file, err := e.root.Open(item.Path)
	if err != nil {
		e.addHold("artifact_missing", subject)
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		e.addHold("artifact_unreadable", subject)
		return false
	}
	observed := hex.EncodeToString(hash.Sum(nil))
	if observed != item.SHA256 {
		e.addHoldDetail("artifact_digest_mismatch", subject, item.SHA256, observed)
		return false
	}
	return true
}

func (e *evaluation) addHold(code, subject string) {
	e.addHoldDetail(code, subject, "", "")
}

func (e *evaluation) addHoldDetail(code, subject, expected, observed string) {
	key := code + "\x00" + subject + "\x00" + expected + "\x00" + observed
	e.holds[key] = Hold{Code: code, Subject: subject, Expected: expected, Observed: observed}
}

func (e *evaluation) finish() Report {
	for _, hold := range e.holds {
		e.report.Holds = append(e.report.Holds, hold)
	}
	slices.SortFunc(e.report.Holds, func(a, b Hold) int {
		if a.Code != b.Code {
			return strings.Compare(a.Code, b.Code)
		}
		if a.Subject != b.Subject {
			return strings.Compare(a.Subject, b.Subject)
		}
		if a.Expected != b.Expected {
			return strings.Compare(a.Expected, b.Expected)
		}
		return strings.Compare(a.Observed, b.Observed)
	})
	sortChecks := func(checks []Check) {
		slices.SortFunc(checks, func(a, b Check) int {
			if a.Kind != b.Kind {
				return strings.Compare(a.Kind, b.Kind)
			}
			return strings.Compare(a.Subject, b.Subject)
		})
	}
	slices.SortFunc(e.report.Certificates, func(a, b Certificate) int {
		return strings.Compare(a.Subject, b.Subject)
	})
	sortChecks(e.report.Journeys)
	sortChecks(e.report.ReleaseArtifacts)
	if len(e.report.Holds) == 0 {
		e.report.Verdict = VerdictGo
	}
	e.report.ReportSHA256 = ""
	encoded, err := json.Marshal(e.report)
	if err != nil {
		panic(fmt.Sprintf("marshal filler release report: %v", err))
	}
	sum := sha256.Sum256(encoded)
	e.report.ReportSHA256 = hex.EncodeToString(sum[:])
	return e.report
}

func candidateDigest(candidate Candidate) string {
	return digestJSON(candidate)
}

func digestJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal filler release identity: %v", err))
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validIdentity(value string) bool {
	if (len(value) != 40 && len(value) != sha256.Size*2) || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validImageDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validDigest(strings.TrimPrefix(value, "sha256:"))
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func checkStatus(passing bool) string {
	if passing {
		return "PASS"
	}
	return "HOLD"
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array is not closed")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delimiter)
	}
	return nil
}
