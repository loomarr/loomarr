package fillerbakeoff_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestRunEscalatesOnlyNamedReasonsAndPreservesEveryStep(t *testing.T) {
	packet := basePacket()
	root := t.TempDir()
	data := []byte("frame bytes")
	if err := os.WriteFile(root+"/frame.jpg", data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	packet.Signals = []fillerbakeoff.Signal{{ID: "frame-1", Kind: string(filleradmission.KindFrame), Path: "frame.jpg", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data)), Width: 640, Height: 360}}
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{Results: []fillerbakeoff.Extraction{
		extraction("text-1", "filler_text", "text", 100, filleradmission.Evidence{ID: "role-text", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindTranscript, Source: "transcript", EvaluationID: "text-1"}),
		extraction("frames-1", "filler_frames", "frames", 200, filleradmission.Evidence{ID: "role-frame", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindFrame, Source: "frame", Derivative: "frame-1", EvaluationID: "frames-1"}),
	}}
	cfg := config(t, manifest, packet, extractor, 1000)
	cfg.CorpusRoot = root
	predictions, err := fillerbakeoff.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(predictions) != 1 || predictions[0].Verdict != fillereval.VerdictAdmit || len(predictions[0].Steps) != 2 {
		t.Fatalf("predictions = %+v", predictions)
	}
	if predictions[0].Steps[0].Rung != "text" || predictions[0].Steps[1].Rung != "frames" || len(extractor.Requests) != 2 {
		t.Fatalf("steps = %+v requests = %+v", predictions[0].Steps, extractor.Requests)
	}
}

func TestRunReservesWorstCaseSpendBeforeCallingNextRung(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{Results: []fillerbakeoff.Extraction{
		extraction("text-1", "filler_text", "text", 100, filleradmission.Evidence{ID: "role-text", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindTranscript, Source: "transcript", EvaluationID: "text-1"}),
	}}
	predictions, err := fillerbakeoff.Run(context.Background(), config(t, manifest, packet, extractor, 150))
	if err != nil {
		t.Fatal(err)
	}
	if len(extractor.Requests) != 1 || !strings.Contains(predictions[0].OperationalFailure, "budget_exhausted") || len(predictions[0].Steps) != 1 {
		t.Fatalf("calls = %d prediction = %+v", len(extractor.Requests), predictions[0])
	}
}

func TestRunRejectsChangedPacketBeforeProviderCall(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	manifest.Cases[0].EvidenceSHA256 = strings.Repeat("f", 64)
	extractor := &testkit.FillerBakeoffExtractor{}
	_, err := fillerbakeoff.Run(context.Background(), config(t, manifest, packet, extractor, 1000))
	if err == nil || !strings.Contains(err.Error(), "evidence packet digest") || len(extractor.Requests) != 0 {
		t.Fatalf("err = %v calls = %d", err, len(extractor.Requests))
	}
}

func TestDecodePacketRejectsLabelBearingOrTrailingJSON(t *testing.T) {
	packet := basePacket()
	raw := `{"schemaVersion":1,"caseId":"case-1","evidenceVersion":"evidence-1","contentSha256":"` + packet.ContentSHA256 + `","facts":[],"truth":"eligible"}`
	if _, err := fillerbakeoff.DecodePacket(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("label-bearing packet error = %v", err)
	}
	raw = `{"schemaVersion":1,"caseId":"case-1","evidenceVersion":"evidence-1","contentSha256":"` + packet.ContentSHA256 + `","facts":[]} {}`
	if _, err := fillerbakeoff.DecodePacket(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing packet error = %v", err)
	}
}

func TestRunRetainsFailedProviderAttemptAsOperationalStep(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{Errors: []error{errors.New("provider timeout")}}
	predictions, err := fillerbakeoff.Run(context.Background(), config(t, manifest, packet, extractor, 1000))
	if err != nil {
		t.Fatal(err)
	}
	got := predictions[0]
	if got.Verdict != "" || !strings.Contains(got.OperationalFailure, "provider timeout") || len(got.Steps) != 1 || got.Steps[0].Attempts != 1 || got.Steps[0].OperationalFailure != "provider timeout" {
		t.Fatalf("prediction = %+v", got)
	}
	report := fillereval.Score(manifest, predictions, config(t, manifest, packet, extractor, 1000).Run)
	if report.Certified || !strings.Contains(report.Cases[0].Failure, "operational failure") || report.Cases[0].Attempts != 1 || report.Metrics.ReviewRate != 0 || contains(report.Failures, "invalid verdict") {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunKeepsDeterministicRejectFreeOfFakeInference(t *testing.T) {
	packet := basePacket()
	packet.Facts[0].Value = filleradmission.UsabilityUnusable
	manifest := manifestFor(packet)
	manifest.Cases[0].Truth = fillereval.TruthInvalid
	manifest.Cases[0].RejectClass = fillereval.RejectDeterministic
	manifest.Cases[0].Evidence[0].Value = filleradmission.UsabilityUnusable
	relabel(&manifest.Cases[0])
	extractor := &testkit.FillerBakeoffExtractor{}
	cfg := config(t, manifest, packet, extractor, 1000)
	predictions, err := fillerbakeoff.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := predictions[0]
	if got.Verdict != fillereval.VerdictReject || got.RejectClass != fillereval.RejectDeterministic || len(got.Steps) != 0 || len(extractor.Requests) != 0 {
		t.Fatalf("prediction = %+v requests = %d", got, len(extractor.Requests))
	}
	report := fillereval.Score(manifest, predictions, cfg.Run)
	if len(report.Cases) != 1 || !report.Cases[0].Correct || report.Cases[0].Rung != "deterministic" || contains(report.Failures, "attribution are required") {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunRejectsSemanticFactsAndChangedDerivativeBeforeProviderCall(t *testing.T) {
	packet := basePacket()
	packet.Facts = append(packet.Facts, filleradmission.Evidence{ID: "leaked-label", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindFrame, Source: "review"})
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{}
	if _, err := fillerbakeoff.Run(context.Background(), config(t, manifest, packet, extractor, 1000)); err == nil || !strings.Contains(err.Error(), "only deterministic") {
		t.Fatalf("semantic fact error = %v", err)
	}

	root := t.TempDir()
	data := []byte("frame bytes")
	if err := os.WriteFile(root+"/frame.jpg", data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	packet = basePacket()
	packet.Signals = []fillerbakeoff.Signal{{ID: "frame-1", Kind: string(filleradmission.KindFrame), Path: "frame.jpg", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data)), Width: 640, Height: 360}}
	manifest = manifestFor(packet)
	if err := os.WriteFile(root+"/frame.jpg", []byte("wrong bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config(t, manifest, packet, extractor, 1000)
	cfg.CorpusRoot = root
	if _, err := fillerbakeoff.Run(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "does not match packet") {
		t.Fatalf("changed derivative error = %v", err)
	}
	if len(extractor.Requests) != 0 {
		t.Fatalf("provider calls = %d", len(extractor.Requests))
	}
}

func TestRunRejectsVideoForNonTemporalReason(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{}
	cfg := config(t, manifest, packet, extractor, 1000)
	cfg.Routes[0] = fillerbakeoff.Route{
		Class: fillerbakeoff.RouteVideo, Role: "filler_video", Rung: "video", Provider: "fixture", Model: "video-model",
		Modalities: []string{"video"}, StructuredOutput: true, MaxChargeNanoUSD: 100, MaxAttempts: 1,
		EscalateOn: []filleradmission.ReasonCode{filleradmission.ReasonMissingContentRole},
	}
	if _, err := fillerbakeoff.Run(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "temporal ambiguity") {
		t.Fatalf("route error = %v", err)
	}
	if len(extractor.Requests) != 0 {
		t.Fatalf("provider calls = %d", len(extractor.Requests))
	}
}

func TestRunPassesVerifiedDerivativeBytesToExtractor(t *testing.T) {
	root := t.TempDir()
	data := []byte("verified frame bytes")
	if err := os.WriteFile(root+"/frame.jpg", data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	packet := basePacket()
	packet.Signals = []fillerbakeoff.Signal{{ID: "frame-1", Kind: string(filleradmission.KindFrame), Path: "frame.jpg", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data)), Width: 640, Height: 360}}
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{Results: []fillerbakeoff.Extraction{
		extraction("text-1", "filler_text", "text", 100, filleradmission.Evidence{ID: "role-text", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindTranscript, Source: "transcript", EvaluationID: "text-1"}),
		extraction("frames-1", "filler_frames", "frames", 100, filleradmission.Evidence{ID: "role-frame", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleBumper, Kind: filleradmission.KindFrame, Source: "frame", Derivative: "frame-1", EvaluationID: "frames-1"}),
	}}
	cfg := config(t, manifest, packet, extractor, 1000)
	cfg.CorpusRoot = root
	if _, err := fillerbakeoff.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(extractor.Requests) != 2 || len(extractor.Requests[0].SignalData) != 0 || len(extractor.Requests[0].Packet.Signals) != 0 || string(extractor.Requests[1].SignalData["frame-1"]) != string(data) {
		t.Fatalf("verified requests = %+v", extractor.Requests)
	}
}

func TestRunRejectsOutOfOrderCascade(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{}
	cfg := config(t, manifest, packet, extractor, 1000)
	cfg.Routes[0], cfg.Routes[1] = cfg.Routes[1], cfg.Routes[0]
	if _, err := fillerbakeoff.Run(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "cascade order") {
		t.Fatalf("route order error = %v", err)
	}
	if len(extractor.Requests) != 0 {
		t.Fatalf("provider calls = %d", len(extractor.Requests))
	}
}

func TestRunFailsClosedOnHiddenRetriesAndPreservesFailedUsage(t *testing.T) {
	packet := basePacket()
	manifest := manifestFor(packet)
	extractor := &testkit.FillerBakeoffExtractor{Results: []fillerbakeoff.Extraction{{
		Attribution: filleradmission.Attribution{
			EvaluationID: "bad-retry", Role: "filler_text", Rung: "text", RequestedProvider: "fixture", RequestedModel: "filler_text",
			ResolvedProvider: "fixture", ResolvedModel: "filler_text", Modalities: []string{"text"}, Attempts: 2,
			Tokens: filleradmission.TokenUsage{Prompt: 7}, ChargedAmount: "unknown", ChargedCurrency: "USD",
		},
	}}}
	predictions, err := fillerbakeoff.Run(context.Background(), config(t, manifest, packet, extractor, 1000))
	if err != nil {
		t.Fatal(err)
	}
	got := predictions[0]
	if len(got.Steps) != 1 || got.Steps[0].Attempts != 2 || got.Steps[0].Tokens.Prompt != 7 || got.Steps[0].ChargedAmount != "unknown" || got.Steps[0].ChargedNanoUSD != 0 || got.Steps[0].ReservedNanoUSD != 100 || !strings.Contains(got.Steps[0].OperationalFailure, "hid 2 provider attempts") {
		t.Fatalf("prediction = %+v", got)
	}
}

func config(t *testing.T, manifest fillereval.Manifest, packet fillerbakeoff.Packet, extractor *testkit.FillerBakeoffExtractor, spend int64) fillerbakeoff.Config {
	t.Helper()
	evaluator, err := filleradmission.New(filleradmission.Policy{
		Version: "policy-1", TaxonomyVersion: "taxonomy-1", AllowedProducts: []string{"breakfast-cereal"},
		AllowedContentRoles: []string{filleradmission.RoleBumper},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fillerbakeoff.Config{
		Run:      fillereval.RunIdentity{Profile: "fixture", EvaluationSplit: fillereval.SplitHoldout, EvidenceVersion: "evidence-1", PromptVersion: "prompt-1", TaxonomyVersion: "taxonomy-1", PolicyVersion: "policy-1", RolePolicyVersion: "roles-1", CapabilitySnapshot: "caps-1", PriceSnapshot: "prices-1", GeneratedAt: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC), MaxRequests: 10, MaxSpendNanoUSD: spend, MaxConcurrency: 1},
		Manifest: manifest, Packets: map[string]fillerbakeoff.Packet{"case-1": packet}, Policy: evaluator, Extractor: extractor,
		Routes: []fillerbakeoff.Route{
			{Class: fillerbakeoff.RouteText, Role: "filler_text", Rung: "text", Provider: "fixture", Model: "filler_text", Modalities: []string{"text"}, StructuredOutput: true, MaxChargeNanoUSD: 100, MaxAttempts: 1, EscalateOn: []filleradmission.ReasonCode{filleradmission.ReasonMissingContentRole}},
			{Class: fillerbakeoff.RouteFrames, Role: "filler_frames", Rung: "frames", Provider: "fixture", Model: "filler_frames", Modalities: []string{"text", "image"}, StructuredOutput: true, MaxChargeNanoUSD: 100, MaxAttempts: 1, EscalateOn: []filleradmission.ReasonCode{filleradmission.ReasonInsufficientContentRole}},
		},
	}
}

func basePacket() fillerbakeoff.Packet {
	return fillerbakeoff.Packet{
		SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: "case-1", EvidenceVersion: "evidence-1", ContentSHA256: strings.Repeat("a", 64),
		Facts: []filleradmission.Evidence{
			{ID: "usable", Claim: filleradmission.ClaimMediaUsability, Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder, Source: "ffprobe"},
			{ID: "licensed", Claim: filleradmission.ClaimSourceLicense, Value: filleradmission.EligibilityEligible, Kind: filleradmission.KindSourcePolicy, Source: "rights-ledger"},
		},
	}
}

func manifestFor(packet fillerbakeoff.Packet) fillereval.Manifest {
	lockedAt := time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)
	holdout := certificationCase("case-1", fillereval.SplitHoldout, packet.ContentSHA256, fillerbakeoff.PacketSHA256(packet), lockedAt)
	development := certificationCase("development-1", fillereval.SplitDevelopment, strings.Repeat("b", 64), strings.Repeat("c", 64), lockedAt)
	return fillereval.Manifest{
		SchemaVersion: fillereval.SchemaVersion, Kind: fillereval.CorpusCertification, CorpusVersion: "fixture", LockedAt: lockedAt,
		SliceGates: []fillereval.SliceGate{{Slice: "contract", MinCases: 1, MinAccuracy: 1, MinAccuracyLower: .01}},
		Cases:      []fillereval.Case{holdout, development},
	}
}

func certificationCase(id string, split fillereval.Split, contentHash, evidenceHash string, lockedAt time.Time) fillereval.Case {
	c := fillereval.Case{
		ID: id, Split: split, Cluster: id, ContentSHA256: contentHash, EvidenceSHA256: evidenceHash,
		Source: "fixture", License: "CC0-1.0", Truth: fillereval.TruthEligible,
		ContentRole: filleradmission.RoleBumper, Slices: []string{"contract"},
		Evidence: []fillereval.Evidence{
			{ID: "usable", Kind: "decoder", Claim: "media_usability", Value: filleradmission.UsabilityUsable, Provenance: "blind review"},
			{ID: "licensed", Kind: "source_policy", Claim: "source_license", Value: filleradmission.EligibilityEligible, Provenance: "blind review"},
			{ID: "role", Kind: "frame", Claim: "content_role", Value: filleradmission.RoleBumper, Provenance: "blind review"},
		},
		Provenance: fillereval.MediaProvenance{
			Authority: "fixture", ItemID: id, ItemURL: "https://example.invalid/items/" + id,
			MetadataRetrievedAt: lockedAt, MetadataSHA256: strings.Repeat("d", 64), EvidenceURL: "https://example.invalid/evidence/" + id,
			RightsStatement: "CC0 fixture", RightsDecision: "approved", RightsReviewerID: "rights",
			RightsReviewedAt: lockedAt, Redistributable: true, SourceFilename: id + ".mp4",
			SourceURL: "https://example.invalid/media/" + id, SourceBytes: 1024, SegmentDurationMS: 30_000,
		},
	}
	labelHash := fillereval.LabelSHA256(c)
	c.LabelReviews = []fillereval.LabelReview{
		{ReviewerID: "reviewer-a", BatchID: "batch-a", ReviewedAt: lockedAt, Independent: true, SubmissionSHA256: labelHash},
		{ReviewerID: "reviewer-b", BatchID: "batch-b", ReviewedAt: lockedAt, Independent: true, SubmissionSHA256: labelHash},
	}
	return c
}

func relabel(c *fillereval.Case) {
	labelHash := fillereval.LabelSHA256(*c)
	for i := range c.LabelReviews {
		c.LabelReviews[i].SubmissionSHA256 = labelHash
	}
}

func contains(values []string, term string) bool {
	for _, value := range values {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func extraction(id, role, rung string, charge int64, evidence filleradmission.Evidence) fillerbakeoff.Extraction {
	modalities := []string{"text"}
	if rung == "frames" {
		modalities = []string{"text", "image"}
	}
	return fillerbakeoff.Extraction{Evidence: []filleradmission.Evidence{evidence}, Attribution: filleradmission.Attribution{EvaluationID: id, Role: role, Rung: rung, RequestedProvider: "fixture", RequestedModel: role, ResolvedProvider: "fixture", ResolvedModel: role, Modalities: modalities, ChargedAmount: "0.0000001", ChargedCurrency: "USD", Attempts: 1, GenerationID: id}, EstimatedNanoUSD: charge}
}
