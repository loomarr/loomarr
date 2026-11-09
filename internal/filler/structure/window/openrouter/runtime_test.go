package openrouter

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/assessment"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/openroutercatalog"
)

func TestCertifiedRuntimeRefreshesMetadataAndRequiresReviewedProfiles(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	snapshot, authority, deployment := certifiedRuntimeFixture(t, now)
	fetches := 0
	runtime, err := NewCertifiedRuntime(CertifiedRuntimeConfig{
		Authority: authority, Deployment: deployment, APIKey: "secret",
		SourceRoot: t.TempDir(), MediaRoot: t.TempDir(), EvidenceRoot: t.TempDir(), FFmpegPath: "ffmpeg",
		Ledger: runtimeNoopLedger{}, Now: func() time.Time { return now }, CutFamily: softwareCuts,
		FetchSnapshot: func(_ context.Context, config openroutercatalog.OpenRouterSnapshotConfig) (openroutercatalog.OpenRouterSnapshot, error) {
			fetches++
			if len(config.Models) != 2 || config.Models[0] != "vendor/model-a" || config.Models[1] != "vendor/model-b" || config.APIKey != "secret" {
				t.Fatalf("snapshot config=%+v", config)
			}
			value := snapshot
			value.RetrievedAt = config.RetrievedAt
			return value, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.freshSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.assessors(first, now); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.freshSnapshot(t.Context()); err != nil || fetches != 1 {
		t.Fatalf("cached snapshot fetches=%d error=%v", fetches, err)
	}
	now = now.Add(snapshotRefreshAge)
	if _, err := runtime.freshSnapshot(t.Context()); err != nil || fetches != 2 {
		t.Fatalf("refreshed snapshot fetches=%d error=%v", fetches, err)
	}

	drifted := first
	drifted.Models = append([]openroutercatalog.OpenRouterModelSnapshot(nil), first.Models...)
	drifted.Models[0].CanonicalSlug += "-different"
	if _, err := runtime.assessors(drifted, first.RetrievedAt); err == nil || !strings.Contains(err.Error(), "profiles") {
		t.Fatalf("profile drift error=%v", err)
	}
}

func TestCertifiedRuntimeRejectsOutOfEnvelopeSourceBeforeMetadata(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	_, authority, deployment := certifiedRuntimeFixture(t, now)
	fetched := false
	runtime, err := NewCertifiedRuntime(CertifiedRuntimeConfig{
		Authority: authority, Deployment: deployment, APIKey: "secret",
		SourceRoot: t.TempDir(), MediaRoot: t.TempDir(), EvidenceRoot: t.TempDir(), FFmpegPath: "ffmpeg",
		Ledger: runtimeNoopLedger{}, Now: func() time.Time { return now }, CutFamily: softwareCuts,
		FetchSnapshot: func(context.Context, openroutercatalog.OpenRouterSnapshotConfig) (openroutercatalog.OpenRouterSnapshot, error) {
			fetched = true
			return openroutercatalog.OpenRouterSnapshot{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Assess(t.Context(), split.AssessmentSource{Source: split.SourceAsset{
		SHA256: strings.Repeat("a", 64), ClipHash: strings.Repeat("a", 64), Bytes: 1_000,
		DurationMs: authority.MinimumSourceDurationMS - 1,
	}})
	if err == nil || fetched {
		t.Fatalf("error=%v fetched=%v", err, fetched)
	}
}

// #2121: an authority certifies cuts made on one family. A host that cuts on another holds every
// source before any media is prepared or any provider is asked, and an authority pinning a profile
// no family produces never builds a runtime.
func TestCertifiedRuntimeCutsOnlyOnTheCertifiedFamily(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	_, authority, deployment := certifiedRuntimeFixture(t, now)
	nvenc, _ := cut.ProfileFor(encode.FamilyNVENC)
	authority.AssessmentMediaProfileSHA256 = nvenc.SHA256
	authority.WindowProfileSHA256 = window.ProfileFor(nvenc).SHA256
	authority.SHA256 = window.MaterializationAuthoritySHA256(authority)
	deployment.AuthoritySHA256 = authority.SHA256
	deployment.SHA256 = DeploymentSHA256(deployment)
	sourceRoot := t.TempDir()
	host := encode.FamilySoftware
	fetched := false
	config := CertifiedRuntimeConfig{
		Authority: authority, Deployment: deployment, APIKey: "secret",
		SourceRoot: sourceRoot, MediaRoot: t.TempDir(), EvidenceRoot: t.TempDir(), FFmpegPath: "ffmpeg",
		Ledger: runtimeNoopLedger{}, Now: func() time.Time { return now }, CutFamily: func() encode.Family { return host },
		FetchSnapshot: func(context.Context, openroutercatalog.OpenRouterSnapshotConfig) (openroutercatalog.OpenRouterSnapshot, error) {
			fetched = true
			return openroutercatalog.OpenRouterSnapshot{}, nil
		},
	}
	runtime, err := NewCertifiedRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	preparer := &capturedRuntimePreparer{err: errors.New("media preflight failed")}
	runtime.preparer = preparer
	input := split.AssessmentSource{
		Source: split.SourceAsset{
			Role: split.SourceLegacyPlayback, SHA256: strings.Repeat("a", 64), ClipHash: strings.Repeat("b", 64),
			Bytes: 1_000, DurationMs: authority.MaximumSourceDurationMS, Path: "source.mp4",
		},
		FullPath: filepath.Join(sourceRoot, "source.mp4"),
	}
	if _, err := runtime.Assess(t.Context(), input); err == nil || !strings.Contains(err.Error(), "this host cuts on software") ||
		preparer.calls != 0 || fetched {
		t.Fatalf("software host: error=%v preparer calls=%d metadata fetched=%v", err, preparer.calls, fetched)
	}
	host = encode.FamilyNVENC
	if _, err := runtime.Assess(t.Context(), input); err == nil || preparer.calls != 1 || fetched {
		t.Fatalf("NVENC host: error=%v preparer calls=%d metadata fetched=%v", err, preparer.calls, fetched)
	}

	unknown := authority
	unknown.AssessmentMediaProfileSHA256 = strings.Repeat("e", 64)
	unknown.SHA256 = window.MaterializationAuthoritySHA256(unknown)
	config.Authority = unknown
	config.Deployment.AuthoritySHA256 = unknown.SHA256
	config.Deployment.SHA256 = DeploymentSHA256(config.Deployment)
	if _, err := NewCertifiedRuntime(config); err == nil {
		t.Fatal("a runtime was built over an authority pinning no known media profile")
	}
}

func softwareCuts() encode.Family { return encode.FamilySoftware }

type capturedRuntimePreparer struct {
	calls    int
	prepared assessment.StructureAssessmentWindowMediaSet
	err      error
}

func (p *capturedRuntimePreparer) PrepareWindows(context.Context, split.AssessmentSource, window.Plan) (assessment.StructureAssessmentWindowMediaSet, error) {
	p.calls++
	return p.prepared, p.err
}

func TestCertifiedRuntimePreflightsMediaBeforeProviderMetadata(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	_, authority, deployment := certifiedRuntimeFixture(t, now)
	sourceRoot := t.TempDir()
	fetched := false
	runtime, err := NewCertifiedRuntime(CertifiedRuntimeConfig{
		Authority: authority, Deployment: deployment, APIKey: "secret",
		SourceRoot: sourceRoot, MediaRoot: t.TempDir(), EvidenceRoot: t.TempDir(), FFmpegPath: "ffmpeg",
		Ledger: runtimeNoopLedger{}, Now: func() time.Time { return now }, CutFamily: softwareCuts,
		FetchSnapshot: func(context.Context, openroutercatalog.OpenRouterSnapshotConfig) (openroutercatalog.OpenRouterSnapshot, error) {
			fetched = true
			return openroutercatalog.OpenRouterSnapshot{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	preparer := &capturedRuntimePreparer{err: errors.New("media preflight failed")}
	runtime.preparer = preparer
	_, err = runtime.Assess(t.Context(), split.AssessmentSource{
		Source: split.SourceAsset{
			Role: split.SourceLegacyPlayback, SHA256: strings.Repeat("a", 64), ClipHash: strings.Repeat("b", 64),
			Bytes: 1_000, DurationMs: authority.MaximumSourceDurationMS, Path: "source.mp4",
		},
		FullPath: filepath.Join(sourceRoot, "source.mp4"),
	})
	if err == nil || preparer.calls != 1 || fetched {
		t.Fatalf("error=%v preparer calls=%d metadata fetched=%v", err, preparer.calls, fetched)
	}
}

func TestCertifiedRuntimeChecksReviewedMediaEnvelopeBeforeProviderMetadata(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	_, authority, deployment := certifiedRuntimeFixture(t, now)
	authority.MaximumWindowBytes = 999
	authority.SHA256 = window.MaterializationAuthoritySHA256(authority)
	deployment.AuthoritySHA256 = authority.SHA256
	deployment.SHA256 = DeploymentSHA256(deployment)
	sourceRoot := t.TempDir()
	input, prepared := certifiedRuntimePreparedFixture(t, sourceRoot, authority.MaximumSourceDurationMS)
	fetched := false
	runtime, err := NewCertifiedRuntime(CertifiedRuntimeConfig{
		Authority: authority, Deployment: deployment, APIKey: "secret",
		SourceRoot: sourceRoot, MediaRoot: t.TempDir(), EvidenceRoot: t.TempDir(), FFmpegPath: "ffmpeg",
		Ledger: runtimeNoopLedger{}, Now: func() time.Time { return now }, CutFamily: softwareCuts,
		FetchSnapshot: func(context.Context, openroutercatalog.OpenRouterSnapshotConfig) (openroutercatalog.OpenRouterSnapshot, error) {
			fetched = true
			return openroutercatalog.OpenRouterSnapshot{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	preparer := &capturedRuntimePreparer{prepared: prepared}
	runtime.preparer = preparer
	if _, err := runtime.Assess(t.Context(), input); err == nil || preparer.calls != 1 || fetched {
		t.Fatalf("error=%v preparer calls=%d metadata fetched=%v", err, preparer.calls, fetched)
	}
}

func certifiedRuntimePreparedFixture(t *testing.T, sourceRoot string, durationMS int64) (split.AssessmentSource, assessment.StructureAssessmentWindowMediaSet) {
	t.Helper()
	source := split.SourceAsset{
		Role: split.SourceLegacyPlayback, SHA256: strings.Repeat("a", 64), ClipHash: strings.Repeat("b", 64),
		Bytes: 1_000, DurationMs: durationMS, Path: "source.mp4",
	}
	input := split.AssessmentSource{Source: source, FullPath: filepath.Join(sourceRoot, source.Path)}
	plan, err := window.NewPlan(structure.Source{SHA256: source.SHA256, Bytes: source.Bytes, DurationMS: durationMS}, cut.SoftwareProfile())
	if err != nil {
		t.Fatal(err)
	}
	identities := make([]structure.AssessmentMedia, len(plan.Windows))
	for ordinal, window := range plan.Windows {
		identities[ordinal] = structure.AssessmentMedia{
			SHA256: strings.Repeat(string(rune('1'+ordinal)), 64), Bytes: 1_000,
			DurationMS:    window.MediaEndMS - window.MediaStartMS,
			ProfileSHA256: plan.Profile.AssessmentMediaProfileSHA256, LineageSHA256: strings.Repeat(string(rune('a'+ordinal)), 64),
		}
	}
	set, err := window.NewMediaSet(plan, identities)
	if err != nil {
		t.Fatal(err)
	}
	prepared := assessment.StructureAssessmentWindowMediaSet{Source: source, Authority: set}
	for ordinal, window := range plan.Windows {
		prepared.Windows = append(prepared.Windows, assessment.StructureAssessmentWindowMedia{
			Window: window, Media: set.Windows[ordinal], FullPath: filepath.Join(t.TempDir(), "window.mp4"),
		})
	}
	return input, prepared
}

type runtimeNoopLedger struct{}

func (runtimeNoopLedger) Reserve(context.Context, window.CallReservation) (window.CallReservationState, error) {
	return window.CallReservationAccepted, nil
}

func (runtimeNoopLedger) Settle(context.Context, window.CallRecord) error { return nil }

func certifiedRuntimeFixture(t *testing.T, now time.Time) (openroutercatalog.OpenRouterSnapshot, window.MaterializationAuthority, Deployment) {
	t.Helper()
	snapshot := openroutercatalog.OpenRouterSnapshot{
		SchemaVersion: openroutercatalog.OpenRouterSnapshotSchemaVersion, SourceBaseURL: openroutercatalog.OpenRouterBaseURL,
		RetrievedAt: now, Requests: 4, ResponseBytes: 1_000,
	}
	deployment := Deployment{
		SchemaVersion: DeploymentSchemaVersion, ContractVersion: DeploymentContractVersion,
		PerSourceBudgetNanoUSD: 144_000_000, PerDayBudgetNanoUSD: 1_440_000_000,
		AutomaticAssessmentAllowed: true,
	}
	for _, suffix := range []string{"a", "b"} {
		modelID := "vendor/model-" + suffix
		provider := "Provider " + strings.ToUpper(suffix)
		providerSlug := "provider/" + suffix
		snapshot.Models = append(snapshot.Models, openroutercatalog.OpenRouterModelSnapshot{
			ID: modelID, CanonicalSlug: modelID + "-20260914", Name: "Model " + suffix, Created: 1,
			InputModalities: []string{"text", "video"}, OutputModalities: []string{"text"},
			Endpoints: []openroutercatalog.OpenRouterEndpointSnapshot{{
				Name: provider + " endpoint", ModelID: modelID, ProviderName: provider, ProviderSlug: providerSlug,
				Quantization: "fp16", ContextLength: 32_768, MaxCompletionTokens: MaximumOutputTokens,
				MaxPromptTokens: 20_000, SupportedParameters: []string{"reasoning", "response_format", "structured_outputs"},
				Pricing: map[string]string{"prompt": "0.000001", "completion": "0.000002"}, Status: 0, ZDR: true,
			}},
		})
		deployment.Families = append(deployment.Families, DeploymentFamily{
			AssessorID: "assessor-" + suffix, ModelFamily: "family-" + suffix, Model: modelID,
			UpstreamProvider: provider, UpstreamProviderSlug: providerSlug, ReasoningMode: ReasoningDisabled,
			MaximumInputTokens: 20_000, ReservationNanoUSD: 24_000_000,
		})
	}
	windowProfile := window.ProfileFor(cut.SoftwareProfile())
	authority := window.MaterializationAuthority{
		SchemaVersion:             window.MaterializationAuthoritySchemaVersion,
		ContractVersion:           window.MaterializationAuthorityContractVersion,
		WindowCertificationSHA256: strings.Repeat("1", 64), ShortLongShadowSHA256: strings.Repeat("2", 64),
		WindowProfileSHA256: windowProfile.SHA256, AssessmentMediaProfileSHA256: windowProfile.AssessmentMediaProfileSHA256,
		MinimumSourceDurationMS: 120_001, MaximumSourceDurationMS: 300_000,
		MaximumWindowBytes: 16 << 20, MaximumWindows: 3,
		ReducerVersion: structure.ReducerContractVersion, BoundaryToleranceMS: 2_000,
		AllowedUnits: []structure.Unit{structure.UnitCompilation},
		AllowedRoles: []structure.Role{structure.RoleCommercial},
		ReviewerID:   "maintainer", ReviewedAt: now, AutomaticMaterializationAllowed: true,
	}
	for _, family := range deployment.Families {
		modelDigest, capabilitySHA, err := openroutercatalog.OpenRouterAssessorIdentity(
			snapshot, family.Model, family.UpstreamProvider, family.UpstreamProviderSlug, family.ReasoningMode,
		)
		if err != nil {
			t.Fatal(err)
		}
		authority.Assessors = append(authority.Assessors, structure.AssessorProfile{
			ID: family.AssessorID, ModelFamily: family.ModelFamily, Provider: "openrouter", Model: family.Model,
			ModelDigest: modelDigest, CapabilitySHA256: capabilitySHA,
			PromptVersion: window.DirectVideoPromptVersion, EvidenceContract: window.CallRecordContractVersion,
		})
	}
	authority.SHA256 = window.MaterializationAuthoritySHA256(authority)
	deployment.AuthoritySHA256 = authority.SHA256
	deployment.SHA256 = DeploymentSHA256(deployment)
	return snapshot, authority, deployment
}
