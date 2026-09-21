package fillerrelease_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/loomarr/loomarr/internal/fillerrelease"
)

func TestEvaluateCompleteCandidateIsGo(t *testing.T) {
	files, manifest := completeBundle(t)

	report := fillerrelease.Evaluate(files, manifest, time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC))

	if report.Verdict != fillerrelease.VerdictGo {
		t.Fatalf("Verdict = %q, want GO; holds: %#v", report.Verdict, report.Holds)
	}
	if report.Cohort.ClipCount != 32 {
		t.Fatalf("ClipCount = %d, want 32", report.Cohort.ClipCount)
	}
	if report.ReportSHA256 == "" {
		t.Fatal("ReportSHA256 is empty")
	}
	if report.ManifestSHA256 == "" || report.Cohort.SHA256 == "" {
		t.Fatal("manifest or cohort digest is empty")
	}
	for _, check := range report.PipelineChecks {
		if check.Status != "PASS" {
			t.Fatalf("certificate %#v is not passing", check)
		}
	}
	checks := append(report.SourceJourneys, report.Journeys...)
	checks = append(checks, report.ReleaseArtifacts...)
	for _, check := range checks {
		if check.Status != "PASS" {
			t.Fatalf("check %#v is not passing", check)
		}
	}
	firstDigest := report.ReportSHA256
	report = fillerrelease.Evaluate(files, manifest, report.GeneratedAt)
	if report.ReportSHA256 != firstDigest {
		t.Fatalf("report digest changed: %q != %q", report.ReportSHA256, firstDigest)
	}
}

func TestEvaluateMalformedManifestIsHold(t *testing.T) {
	report := fillerrelease.Evaluate(fstest.MapFS{}, []byte(`{"schema_version":`), time.Time{})

	assertHold(t, report, "manifest_malformed")
	if report.ReportSHA256 == "" {
		t.Fatal("ReportSHA256 is empty")
	}
}

func TestEvaluateRejectsUnknownManifestFields(t *testing.T) {
	files, manifest := completeBundle(t)
	var raw map[string]any
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	raw["quietly_ignored"] = true
	manifest, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	report := fillerrelease.Evaluate(files, manifest, time.Time{})

	assertHold(t, report, "manifest_malformed")
}

func TestEvaluateRejectsDuplicateJSONFields(t *testing.T) {
	report := fillerrelease.Evaluate(fstest.MapFS{}, []byte(`{
  "schema_version": 2,
  "schema_version": 2
}`), time.Time{})

	assertHold(t, report, "manifest_malformed")
}

func TestEvaluateChangedArtifactIsHold(t *testing.T) {
	files, manifest := completeBundle(t)
	files["evidence/media.json"] = &fstest.MapFile{Data: []byte("changed")}

	report := fillerrelease.Evaluate(files, manifest, time.Time{})

	assertHold(t, report, "artifact_digest_mismatch")
}

func TestEvaluateMissingArtifactIsHold(t *testing.T) {
	files, manifest := completeBundle(t)
	delete(files, "evidence/media.json")

	report := fillerrelease.Evaluate(files, manifest, time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC))

	assertHold(t, report, "artifact_missing")
}

func TestEvaluateUnsafeArtifactPathIsHold(t *testing.T) {
	files, manifest := completeBundle(t)
	var raw manifestFixture
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	raw.PipelineChecks[0].Artifact.Path = "../outside.json"
	manifest, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	report := fillerrelease.Evaluate(files, manifest, time.Time{})

	assertHold(t, report, "artifact_path_invalid")
}

func TestEvaluateMissingPipelineCheckAndResidualDecisionAreHolds(t *testing.T) {
	files, manifest := completeBundle(t)
	var raw manifestFixture
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	raw.PipelineChecks = raw.PipelineChecks[1:]
	raw.ResidualHumanDecisions = 1
	manifest, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	report := fillerrelease.Evaluate(files, manifest, time.Time{})

	assertHold(t, report, "pipeline_check_missing")
	assertHold(t, report, "residual_human_decisions")
}

func TestEvaluateCandidateBindingMismatchIsHold(t *testing.T) {
	files, manifest := completeBundle(t)
	var raw manifestFixture
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	raw.Journeys[0].CandidateSHA256 = digest("another candidate")
	manifest, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	report := fillerrelease.Evaluate(files, manifest, time.Time{})

	assertHold(t, report, "candidate_binding_mismatch")
}

func TestEvaluateRequiredClassesFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		mutate func(*manifestFixture)
	}{
		{"candidate identity", "candidate_identity_invalid", func(m *manifestFixture) { m.Candidate.ServerImageDigest = "" }},
		{"candidate tag", "candidate_tag_mismatch", func(m *manifestFixture) { m.Candidate.Tag = "v0.2.0-beta.5" }},
		{"evidence freshness", "evidence_stale", func(m *manifestFixture) { m.ValidUntil = time.Date(2026, 9, 20, 17, 30, 0, 0, time.UTC) }},
		{"exact clip count", "clip_count_invalid", func(m *manifestFixture) { m.Cohort.Clips = m.Cohort.Clips[:31] }},
		{"too many clips", "clip_count_invalid", func(m *manifestFixture) { m.Cohort.Clips = append(m.Cohort.Clips, m.Cohort.Clips[0]) }},
		{"source lineage", "clip_source_unbound", func(m *manifestFixture) { m.Cohort.Clips[0].SourceIdentity = "unbound" }},
		{"clip readiness", "clip_not_ready", func(m *manifestFixture) { m.Cohort.Clips[0].Ready = false }},
		{"clip range playback", "clip_range_playback_failed", func(m *manifestFixture) { m.Cohort.Clips[0].RangePlayback = false }},
		{"duplicate family", "duplicate_family_repeated", func(m *manifestFixture) {
			m.Cohort.Clips[0].DuplicateFamilyIdentity = "family-a"
			m.Cohort.Clips[1].DuplicateFamilyIdentity = "family-a"
		}},
		{"repeated pipeline check", "pipeline_check_duplicate", func(m *manifestFixture) { m.PipelineChecks = append(m.PipelineChecks, m.PipelineChecks[0]) }},
		{"pipeline denominator", "pipeline_check_denominator_invalid", func(m *manifestFixture) { m.PipelineChecks[0].Total++ }},
		{"pipeline abstention", "pipeline_check_not_passing", func(m *manifestFixture) {
			m.PipelineChecks[0].Passed--
			m.PipelineChecks[0].Abstentions++
		}},
		{"prohibited admission", "prohibited_admission", func(m *manifestFixture) { m.PipelineChecks[0].ProhibitedAdmissions = 1 }},
		{"pipeline identity", "pipeline_check_identity_missing", func(m *manifestFixture) { m.PipelineChecks[0].PolicyIdentity = "" }},
		{"source journey", "source_journey_incomplete", func(m *manifestFixture) { m.SourceJourneys[0].LibraryReady = false }},
		{"installed playback", "journey_playback_incomplete", func(m *manifestFixture) { m.Journeys[0].RangePlayback = false }},
		{"physical Android TV target", "journey_target_invalid", func(m *manifestFixture) {
			m.Journeys[0].DeploymentTargetIdentity = "shield-physical-device"
		}},
		{"release verification", "release_artifact_unverified", func(m *manifestFixture) { m.ReleaseArtifacts[0].Verified = false }},
		{"residual human decision", "residual_human_decisions", func(m *manifestFixture) { m.ResidualHumanDecisions = 1 }},
		{"operational failure", "operational_failures", func(m *manifestFixture) { m.OperationalFailures = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files, encoded := completeBundle(t)
			var manifest manifestFixture
			if err := json.Unmarshal(encoded, &manifest); err != nil {
				t.Fatal(err)
			}
			test.mutate(&manifest)
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}

			report := fillerrelease.Evaluate(files, encoded, time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC))

			assertHold(t, report, test.code)
		})
	}
}

func TestEvaluateEveryRequiredPipelineSourceClientAndReleaseArtifact(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		mutate func(*manifestFixture, int)
		count  int
	}{
		{"pipeline check", "pipeline_check_missing", func(m *manifestFixture, i int) {
			m.PipelineChecks = append(m.PipelineChecks[:i], m.PipelineChecks[i+1:]...)
		}, 7},
		{"source journey", "source_journey_missing", func(m *manifestFixture, i int) {
			m.SourceJourneys = append(m.SourceJourneys[:i], m.SourceJourneys[i+1:]...)
		}, 2},
		{"journey", "journey_missing", func(m *manifestFixture, i int) { m.Journeys = append(m.Journeys[:i], m.Journeys[i+1:]...) }, 2},
		{"release artifact", "release_artifact_missing", func(m *manifestFixture, i int) {
			m.ReleaseArtifacts = append(m.ReleaseArtifacts[:i], m.ReleaseArtifacts[i+1:]...)
		}, 6},
	}
	for _, test := range tests {
		for i := range test.count {
			t.Run(fmt.Sprintf("%s-%d", test.name, i), func(t *testing.T) {
				files, encoded := completeBundle(t)
				var manifest manifestFixture
				if err := json.Unmarshal(encoded, &manifest); err != nil {
					t.Fatal(err)
				}
				test.mutate(&manifest, i)
				encoded, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}

				report := fillerrelease.Evaluate(files, encoded, time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC))

				assertHold(t, report, test.code)
			})
		}
	}
}

func TestReportOmitsPrivateEvidenceDetails(t *testing.T) {
	files, encoded := completeBundle(t)
	var manifest manifestFixture
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	privateMarker := "private-household-title-and-transcript"
	oldPath := manifest.PipelineChecks[0].Artifact.Path
	newPath := "private/" + privateMarker + ".json"
	files[newPath] = files[oldPath]
	delete(files, oldPath)
	manifest.PipelineChecks[0].Artifact.Path = newPath
	manifest.Cohort.SourceIdentities[0] = privateMarker
	for i := range manifest.Cohort.Clips {
		manifest.Cohort.Clips[i].SourceIdentity = privateMarker
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	report := fillerrelease.Evaluate(files, encoded, time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC))
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(reportJSON), privateMarker) {
		t.Fatalf("report leaked private evidence detail: %s", reportJSON)
	}
}

func assertHold(t *testing.T, report fillerrelease.Report, code string) {
	t.Helper()
	if report.Verdict != fillerrelease.VerdictHold {
		t.Fatalf("Verdict = %q, want HOLD", report.Verdict)
	}
	for _, hold := range report.Holds {
		if hold.Code == code {
			return
		}
	}
	t.Fatalf("hold %q absent from %#v", code, report.Holds)
}

type artifactFixture struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type resultFixture struct {
	Kind                 string          `json:"kind"`
	CandidateSHA256      string          `json:"candidate_sha256"`
	Artifact             artifactFixture `json:"artifact"`
	SchemaIdentity       string          `json:"schema_identity"`
	PolicyIdentity       string          `json:"policy_identity"`
	ModelIdentity        string          `json:"model_identity"`
	PromptIdentity       string          `json:"prompt_identity"`
	ProfileIdentity      string          `json:"profile_identity"`
	BuildIdentity        string          `json:"build_identity"`
	Passed               int             `json:"passed"`
	Total                int             `json:"total"`
	Abstentions          int             `json:"abstentions"`
	Holds                int             `json:"holds"`
	ProhibitedAdmissions int             `json:"prohibited_admissions"`
}

type sourceJourneyFixture struct {
	Provider           string          `json:"provider"`
	CandidateSHA256    string          `json:"candidate_sha256"`
	Artifact           artifactFixture `json:"artifact"`
	SourceIdentity     string          `json:"source_identity"`
	SourceMasterSHA256 string          `json:"source_master_sha256"`
	PlaybackSHA256     string          `json:"playback_sha256"`
	Acquired           bool            `json:"acquired"`
	Prepared           bool            `json:"prepared"`
	LibraryReady       bool            `json:"library_ready"`
	RangePlayback      bool            `json:"range_playback"`
}

type journeyFixture struct {
	Platform                 string          `json:"platform"`
	CandidateSHA256          string          `json:"candidate_sha256"`
	Artifact                 artifactFixture `json:"artifact"`
	BuildIdentity            string          `json:"build_identity"`
	DeploymentTargetIdentity string          `json:"deployment_target_identity"`
	Installed                bool            `json:"installed"`
	ChannelSelected          bool            `json:"channel_selected"`
	PodSelected              bool            `json:"pod_selected"`
	RangePlayback            bool            `json:"range_playback"`
	Passed                   bool            `json:"passed"`
}

type releaseArtifactFixture struct {
	Kind            string          `json:"kind"`
	CandidateSHA256 string          `json:"candidate_sha256"`
	Artifact        artifactFixture `json:"artifact"`
	Verified        bool            `json:"verified"`
}

type candidateFixture struct {
	GitCommit                  string `json:"git_commit"`
	Tag                        string `json:"tag"`
	ServerImageDigest          string `json:"server_image_digest"`
	WebBuildIdentity           string `json:"web_build_identity"`
	AndroidTVArtifactSHA256    string `json:"android_tv_artifact_sha256"`
	AndroidTVVersion           string `json:"android_tv_version"`
	ConfigurationProfileSHA256 string `json:"configuration_profile_sha256"`
}

type clipFixture struct {
	SourceMasterSHA256       string `json:"source_master_sha256"`
	LineageSHA256            string `json:"lineage_sha256"`
	PlaybackDerivativeSHA256 string `json:"playback_derivative_sha256"`
	SidecarSHA256            string `json:"sidecar_sha256"`
	SourceIdentity           string `json:"source_identity"`
	DuplicateFamilyIdentity  string `json:"duplicate_family_identity,omitempty"`
	Ready                    bool   `json:"ready"`
	RangePlayback            bool   `json:"range_playback"`
}

type cohortFixture struct {
	SourceIdentities []string      `json:"source_identities"`
	Clips            []clipFixture `json:"clips"`
}

type manifestFixture struct {
	SchemaVersion          int                      `json:"schema_version"`
	Release                string                   `json:"release"`
	AssembledAt            time.Time                `json:"assembled_at"`
	ValidUntil             time.Time                `json:"valid_until"`
	Candidate              candidateFixture         `json:"candidate"`
	Cohort                 cohortFixture            `json:"cohort"`
	PipelineChecks         []resultFixture          `json:"pipeline_checks"`
	SourceJourneys         []sourceJourneyFixture   `json:"source_journeys"`
	Journeys               []journeyFixture         `json:"journeys"`
	ReleaseArtifacts       []releaseArtifactFixture `json:"release_artifacts"`
	ResidualHumanDecisions int                      `json:"residual_human_decisions"`
	OperationalFailures    int                      `json:"operational_failures"`
}

func completeBundle(t *testing.T) (fstest.MapFS, []byte) {
	t.Helper()
	files := fstest.MapFS{}
	candidate := candidateFixture{
		GitCommit:                  digest("git")[:40],
		Tag:                        "v0.2.0-beta.6",
		ServerImageDigest:          "sha256:" + digest("server"),
		WebBuildIdentity:           digest("web")[:40],
		AndroidTVArtifactSHA256:    digest("android-tv"),
		AndroidTVVersion:           "0.2.0-beta.6",
		ConfigurationProfileSHA256: digest("config"),
	}
	candidateBytes, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidateDigest := digest(string(candidateBytes))

	manifest := manifestFixture{
		SchemaVersion: 2,
		Release:       "v0.2.0-beta.6",
		AssembledAt:   time.Date(2026, 9, 20, 17, 0, 0, 0, time.UTC),
		ValidUntil:    time.Date(2026, 9, 21, 17, 0, 0, 0, time.UTC),
		Candidate:     candidate,
		Cohort:        cohortFixture{SourceIdentities: []string{"archive:starter"}},
	}
	for i := range 32 {
		manifest.Cohort.Clips = append(manifest.Cohort.Clips, clipFixture{
			SourceMasterSHA256:       digest(fmt.Sprintf("source-%02d", i)),
			LineageSHA256:            digest(fmt.Sprintf("lineage-%02d", i)),
			PlaybackDerivativeSHA256: digest(fmt.Sprintf("playback-%02d", i)),
			SidecarSHA256:            digest(fmt.Sprintf("sidecar-%02d", i)),
			SourceIdentity:           "archive:starter",
			Ready:                    true,
			RangePlayback:            true,
		})
	}
	for _, kind := range []string{"duplicates", "enrichment", "language", "media", "playback", "readiness", "suitability"} {
		path := "evidence/" + kind + ".json"
		artifact := addArtifact(files, path, []byte("evidence for "+kind))
		manifest.PipelineChecks = append(manifest.PipelineChecks, resultFixture{
			Kind: kind, CandidateSHA256: candidateDigest, Artifact: artifact,
			SchemaIdentity: "schema-v1", PolicyIdentity: "policy-v1", ModelIdentity: "none", PromptIdentity: "none",
			ProfileIdentity: "household-beta", BuildIdentity: "build-v1", Passed: 32, Total: 32,
		})
	}
	for _, provider := range []string{"archive_org", "youtube"} {
		path := "sources/" + provider + ".json"
		manifest.SourceJourneys = append(manifest.SourceJourneys, sourceJourneyFixture{
			Provider: provider, CandidateSHA256: candidateDigest,
			Artifact:       addArtifact(files, path, []byte("source journey for "+provider)),
			SourceIdentity: provider + ":fixture", SourceMasterSHA256: digest(provider + "-source"),
			PlaybackSHA256: digest(provider + "-playback"), Acquired: true, Prepared: true,
			LibraryReady: true, RangePlayback: true,
		})
	}
	for _, platform := range []string{"android_tv_emulator", "web"} {
		path := "journeys/" + platform + ".json"
		deploymentTarget := "web-localhost"
		if platform == "android_tv_emulator" {
			deploymentTarget = "emulator-5554"
		}
		manifest.Journeys = append(manifest.Journeys, journeyFixture{
			Platform: platform, CandidateSHA256: candidateDigest,
			Artifact:      addArtifact(files, path, []byte("journey for "+platform)),
			BuildIdentity: "build-v1", DeploymentTargetIdentity: deploymentTarget,
			Installed: true, ChannelSelected: true, PodSelected: true, RangePlayback: true, Passed: true,
		})
	}
	for _, kind := range []string{"deployment", "rollback", "sbom", "signature", "provenance", "notices"} {
		path := "release/" + kind + ".json"
		manifest.ReleaseArtifacts = append(manifest.ReleaseArtifacts, releaseArtifactFixture{
			Kind: kind, CandidateSHA256: candidateDigest,
			Artifact: addArtifact(files, path, []byte("release artifact for "+kind)), Verified: true,
		})
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return files, encoded
}

func addArtifact(files fstest.MapFS, path string, content []byte) artifactFixture {
	files[path] = &fstest.MapFile{Data: content}
	sum := sha256.Sum256(content)
	return artifactFixture{Path: path, SHA256: hex.EncodeToString(sum[:])}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
