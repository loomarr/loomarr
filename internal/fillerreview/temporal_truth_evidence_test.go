package fillerreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestBuildTemporalTruthEvidenceCaseAllowsRepeatedFrameBytesWithoutLeakingPrivateIdentity(t *testing.T) {
	frame := temporalTruthTestJPEG(t)
	media := &temporalTruthFakeMedia{frame: frame}
	ocr := temporalTruthFakeOCR{}
	root := t.TempDir()
	draftCase := fillereval.Case{
		ID: "private-case-id", ContentSHA256: strings.Repeat("c", 64),
		Provenance: fillereval.MediaProvenance{SegmentDurationMS: 10_000},
	}
	packet := fillerbakeoff.Packet{SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: draftCase.ID, EvidenceVersion: "legacy", ContentSHA256: draftCase.ContentSHA256}
	publicCase, privateEntry, err := buildTemporalTruthEvidenceCase(context.Background(), TemporalTruthEvidenceConfig{
		SceneThreshold: 0.3, Media: media, OCR: ocr,
	}, root, "evidence-opaque", draftCase, temporalTruthDownloadedCase{LocalFile: "private-source.mp4"}, packet, fillerbakeoff.TranscriptArtifact{}, "unused", strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(publicCase.Frames) < 2 || publicCase.Frames[0].SHA256 != publicCase.Frames[1].SHA256 || len(publicCase.Frames[0].OCR) != 1 {
		t.Fatalf("repeated-frame evidence = %+v", publicCase.Frames)
	}
	publicRaw := temporalTruthJSONBytes(t, publicCase)
	if bytes.Contains(publicRaw, []byte(draftCase.ID)) || bytes.Contains(publicRaw, []byte("private-source.mp4")) || bytes.Contains(publicRaw, []byte(draftCase.ContentSHA256)) {
		t.Fatalf("public evidence leaks private identity: %s", publicRaw)
	}
	if privateEntry.CaseID != draftCase.ID || privateEntry.SourceLocalFile != "private-source.mp4" {
		t.Fatalf("private entry lost the unblinding join: %+v", privateEntry)
	}
}

func TestLoadTemporalTruthEvidenceRejectsTamperedPublicArtifact(t *testing.T) {
	root := t.TempDir()
	videoRaw, frameRaw := []byte("video"), temporalTruthTestJPEG(t)
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), videoRaw, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frame.jpg"), frameRaw, 0o640); err != nil {
		t.Fatal(err)
	}
	identity := TemporalTruthToolIdentity{Path: "/tool", Version: "v1", BinarySHA256: strings.Repeat("a", 64)}
	manifest := TemporalTruthEvidenceManifest{
		SchemaVersion: TemporalTruthEvidenceSchemaVersion, ContractVersion: TemporalTruthEvidenceContractVersion,
		EvidenceVersion: TemporalTruthEvidenceVersion, GeneratedAt: time.Unix(1, 0).UTC(), SelectionSHA256: strings.Repeat("b", 64),
		MediaTools: TemporalTruthMediaIdentity{FFmpeg: identity, FFprobe: identity}, OCR: TemporalTruthOCRStatus{Status: "unavailable"},
		Config: TemporalTruthEvidenceSettings{SceneThreshold: .3, MaximumFramesPerCase: TemporalEvidenceMaxFrames, MaximumVideoBytes: TemporalTruthMaximumVideoBytes, PerCaseTimeoutMS: 1_000},
	}
	for index := 0; index < fillereval.TemporalTruthSelectionCases; index++ {
		manifest.Cases = append(manifest.Cases, TemporalTruthEvidenceCase{
			Alias: fmt.Sprintf("evidence-%02d", index), DurationMS: 1_000,
			Plan:   TemporalEvidencePlan{SchemaVersion: TemporalEvidencePlanSchemaVersion, SourceEndMS: 1_000, EvidenceEndMS: 1_000, FrameTimesMS: []int64{0}},
			Video:  TemporalTruthEvidenceFile{Path: "video.mp4", SHA256: hashBytes(videoRaw), Bytes: int64(len(videoRaw)), DurationMS: 1_000, Width: 2, Height: 2},
			Frames: []TemporalTruthEvidenceFrame{{ID: "frame-01", Path: "frame.jpg", SHA256: hashBytes(frameRaw), Bytes: int64(len(frameRaw)), Width: 2, Height: 2, AtMS: 0}},
		})
	}
	manifestPath := filepath.Join(root, "manifest.json")
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, raw, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTemporalTruthEvidence(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frame.jpg"), []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTemporalTruthEvidence(manifestPath); err == nil || !strings.Contains(err.Error(), "binding failed") {
		t.Fatalf("tampered artifact error = %v", err)
	}
}

type temporalTruthFakeMedia struct{ frame []byte }

func (fake *temporalTruthFakeMedia) Identity() TemporalTruthMediaIdentity {
	identity := TemporalTruthToolIdentity{Path: "/fake", BinarySHA256: strings.Repeat("a", 64)}
	return TemporalTruthMediaIdentity{FFmpeg: identity, FFprobe: identity}
}

func (fake *temporalTruthFakeMedia) Analyze(context.Context, string, int64, int64, float64) ([]mediatools.Interval, []mediatools.Interval, []int64, error) {
	return nil, nil, []int64{2_000}, nil
}

func (fake *temporalTruthFakeMedia) WriteReviewVideo(_ context.Context, _ string, _, duration int64, output string) (TemporalTruthVideoInfo, error) {
	if err := os.WriteFile(output, []byte("bounded-video"), 0o640); err != nil {
		return TemporalTruthVideoInfo{}, err
	}
	return TemporalTruthVideoInfo{DurationMS: duration, Width: 320, Height: 240}, nil
}

func (fake *temporalTruthFakeMedia) Frames(_ context.Context, _ string, times []int64) ([][]byte, error) {
	result := make([][]byte, len(times))
	for index := range result {
		result[index] = bytes.Clone(fake.frame)
	}
	return result, nil
}

type temporalTruthFakeOCR struct{}

func (temporalTruthFakeOCR) Identity() TemporalTruthToolIdentity {
	return TemporalTruthToolIdentity{Path: "/fake-ocr", BinarySHA256: strings.Repeat("b", 64), SourceSHA256: strings.Repeat("c", 64)}
}

func (temporalTruthFakeOCR) Recognize(_ context.Context, inputs []TemporalTruthOCRInput) ([]TemporalTruthOCRResult, error) {
	result := make([]TemporalTruthOCRResult, len(inputs))
	for index, input := range inputs {
		result[index] = TemporalTruthOCRResult{SHA256: input.SHA256, Observations: []TemporalTruthOCRObservation{{Text: "CARD", Confidence: .9, X: .1, Y: .1, Width: .5, Height: .2}}}
	}
	return result, nil
}

func temporalTruthTestJPEG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func temporalTruthJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
