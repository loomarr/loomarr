package fillerreference_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerreference"
)

const (
	legacyContract = "filler-reference-cohort-2026-08-31-v3"
	legacyEvidence = "filler-evidence-v1"
)

func TestRefreshProducesCurrentArtifactsThatReplayThroughAudit(t *testing.T) {
	raw, downloads, refreshedAt := legacyRefreshFixture(t)
	result, err := fillerreference.Refresh(raw, refreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.From.ReferenceContract != legacyContract || result.Report.To.ReferenceContract != fillerreference.ContractVersion || result.Report.To.EvidenceVersion != fillerreference.PacketEvidenceVersion {
		t.Fatalf("contract transition = %+v -> %+v", result.Report.From, result.Report.To)
	}
	if result.Report.Denominators.Cases != 300 || result.Report.Denominators.Packets != 300 || result.Report.Denominators.RemovedSourceLicenseFacts != 300 || result.Report.Denominators.LabelReviews != 600 {
		t.Fatalf("denominators = %+v", result.Report.Denominators)
	}
	if strings.Contains(string(result.Packets), "source_license") || strings.Contains(string(result.Packets), "source-license") {
		t.Fatal("refreshed packets retained retired licensing evidence")
	}
	if _, err := fillerreference.BuildAudit(fillerreference.RawAuditInputs{
		Manifest: result.Manifest, Packets: result.Packets, Mapping: result.Mapping,
		DownloadLedger: downloads, ContentReview: result.ContentReview,
	}, refreshedAt); err != nil {
		t.Fatalf("refreshed artifacts do not pass the current audit: %v", err)
	}

	again, err := fillerreference.Refresh(raw, refreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(result)
	second, _ := json.Marshal(again)
	if !bytes.Equal(first, second) {
		t.Fatal("same refresh inputs and time produced different artifacts")
	}
}

func TestRefreshPreservesSemanticAndNegativeReviewAuthority(t *testing.T) {
	raw, _, refreshedAt := legacyRefreshFixture(t)
	var beforeManifest fillereval.Manifest
	var beforeMapping fillerreference.MappingArtifact
	var beforeReview fillerreference.ContentReviewArtifact
	if err := json.Unmarshal(raw.Manifest, &beforeManifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw.ContentReview, &beforeReview); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw.Mapping, &beforeMapping); err != nil {
		t.Fatal(err)
	}
	beforeLabels := make(map[string]string, len(beforeManifest.Cases))
	for _, item := range beforeManifest.Cases {
		beforeLabels[item.ID] = fillereval.LabelSHA256(item)
	}
	result, err := fillerreference.Refresh(raw, refreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	var afterManifest fillereval.Manifest
	var afterMapping fillerreference.MappingArtifact
	var afterReview fillerreference.ContentReviewArtifact
	if err := json.Unmarshal(result.Manifest, &afterManifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(result.ContentReview, &afterReview); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(result.Mapping, &afterMapping); err != nil {
		t.Fatal(err)
	}
	for _, item := range afterManifest.Cases {
		if got := fillereval.LabelSHA256(item); got != beforeLabels[item.ID] {
			t.Fatalf("case %q label changed: %s", item.ID, got)
		}
	}
	beforeReview.ContractVersion, beforeReview.SourceManifestSHA256 = afterReview.ContractVersion, afterReview.SourceManifestSHA256
	if got, want := mustJSON(t, afterReview), mustJSON(t, beforeReview); !bytes.Equal(got, want) {
		t.Fatal("refresh edited the negative content review beyond its declared bindings")
	}
	beforeMapping.SourceManifestSHA256 = afterMapping.SourceManifestSHA256
	if got, want := mustJSON(t, afterMapping), mustJSON(t, beforeMapping); !bytes.Equal(got, want) {
		t.Fatal("refresh edited the product mapping beyond its declared binding")
	}
}

func TestRefreshRejectsAnythingBeyondDeclaredLicenseRemoval(t *testing.T) {
	tests := map[string]func(t *testing.T, raw *fillerreference.RawRefreshInputs){
		"missing license fact": func(t *testing.T, raw *fillerreference.RawRefreshInputs) {
			mutateLegacyPacket(t, raw, func(packet *fillerbakeoff.Packet) { packet.Facts = packet.Facts[:1] })
		},
		"ineligible license fact": func(t *testing.T, raw *fillerreference.RawRefreshInputs) {
			mutateLegacyPacket(t, raw, func(packet *fillerbakeoff.Packet) { packet.Facts[1].Value = "ineligible" })
		},
		"extra fact": func(t *testing.T, raw *fillerreference.RawRefreshInputs) {
			mutateLegacyPacket(t, raw, func(packet *fillerbakeoff.Packet) { packet.Facts = append(packet.Facts, packet.Facts[0]) })
		},
		"unknown manifest field": func(_ *testing.T, raw *fillerreference.RawRefreshInputs) {
			raw.Manifest = bytes.Replace(raw.Manifest, []byte(`"schemaVersion":5`), []byte(`"schemaVersion":5,"surprise":true`), 1)
		},
		"duplicate manifest field": func(_ *testing.T, raw *fillerreference.RawRefreshInputs) {
			raw.Manifest = bytes.Replace(raw.Manifest, []byte(`"schemaVersion":5`), []byte(`"schemaVersion":5,"schemaVersion":5`), 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			raw, _, refreshedAt := legacyRefreshFixture(t)
			mutate(t, &raw)
			if _, err := fillerreference.Refresh(raw, refreshedAt); err == nil {
				t.Fatal("invalid refresh input was accepted")
			}
		})
	}
}

func TestRefreshRejectsASecondRefresh(t *testing.T) {
	raw, _, refreshedAt := legacyRefreshFixture(t)
	result, err := fillerreference.Refresh(raw, refreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fillerreference.Refresh(fillerreference.RawRefreshInputs{Manifest: result.Manifest, Packets: result.Packets, Mapping: result.Mapping, ContentReview: result.ContentReview}, refreshedAt.Add(time.Hour))
	if err == nil {
		t.Fatal("current artifacts entered the one-way legacy refresh")
	}
}

func legacyRefreshFixture(t *testing.T) (fillerreference.RawRefreshInputs, []byte, time.Time) {
	t.Helper()
	manifest, packets, mapping, downloads, review := fixture()
	for id, packet := range packets {
		packet.SchemaVersion = 1
		packet.EvidenceVersion = legacyEvidence
		packet.Facts = append(packet.Facts, filleradmission.Evidence{
			ID: "source-license", Claim: filleradmission.Claim("source_license"), Value: "eligible",
			Kind: filleradmission.KindSourcePolicy, Source: "rights:" + id,
		})
		packets[id] = packet
		rebindPacket(&manifest, packet)
	}
	review.ContractVersion = legacyContract
	raw := rawAuditInputs(t, manifest, packets, mapping, downloads, review)
	return fillerreference.RawRefreshInputs{Manifest: raw.Manifest, Packets: raw.Packets, Mapping: raw.Mapping, ContentReview: raw.ContentReview}, raw.DownloadLedger, manifest.LockedAt.Add(time.Hour)
}

func mutateLegacyPacket(t *testing.T, raw *fillerreference.RawRefreshInputs, mutate func(*fillerbakeoff.Packet)) {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(raw.Packets), []byte{'\n'})
	var packet fillerbakeoff.Packet
	if err := json.Unmarshal(lines[0], &packet); err != nil {
		t.Fatal(err)
	}
	mutate(&packet)
	lines[0] = mustJSON(t, packet)
	raw.Packets = append(bytes.Join(lines, []byte{'\n'}), '\n')
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
