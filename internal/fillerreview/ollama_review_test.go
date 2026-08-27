package fillerreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestRunOllamaReviewProducesOneContentBoundBlindSubmission(t *testing.T) {
	packageDir, transcript := reviewPackageFixture(t)
	digest := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"reviewer:1","model":"reviewer:1","digest":"` + digest + `","size":42,"capabilities":["vision"]}]}`))
		case "/api/chat":
			var request struct {
				Messages []struct {
					Content string   `json:"content"`
					Images  []string `json:"images"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if len(request.Messages) != 2 || len(request.Messages[1].Images) != 4 || !strings.Contains(request.Messages[1].Content, `"id":"transcript-01"`) || !strings.Contains(request.Messages[1].Content, `"text":"Drink Bright Cola"`) || strings.Contains(request.Messages[1].Content, `"id":"audio-01"`) || strings.Contains(request.Messages[1].Content, "case-secret") {
				t.Fatalf("provider-visible request = %+v", request.Messages)
			}
			content := `{"disposition":"eligible","contentRole":"commercial","taxonomy":{"product":["cola"]},"policyFlags":[],"slices":["commercial"],"evidence":[{"id":"transcript-01","kind":"transcript","claim":"content_role","value":"commercial","provenance":"cases/review-one/audio-01.wav#transcript","atMs":0}],"reviewQuestion":""}`
			response, _ := json.Marshal(map[string]any{"model": "reviewer:1", "message": map[string]string{"content": content, "thinking": ""}, "done_reason": "stop", "prompt_eval_count": 120, "eval_count": 42})
			_, _ = w.Write(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	completedAt := time.Date(2026, 8, 27, 22, 0, 0, 0, time.UTC)
	run, submissions, err := RunOllamaReview(context.Background(), OllamaReviewConfig{
		PackageDir: packageDir, Transcripts: []fillerbakeoff.TranscriptArtifact{transcript},
		BaseURL: server.URL, Model: "reviewer:1", ModelDigest: digest,
		ReviewerID: "reviewer-a-model", ExpectedCases: 1, PerCaseTimeout: time.Second, Now: func() time.Time { return completedAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(submissions) != 1 || submissions[0].Alias != "review-one" || submissions[0].Labels.ContentRole != "commercial" || submissions[0].ReviewedAt != completedAt {
		t.Fatalf("submissions = %+v", submissions)
	}
	if run.Cases != 1 || run.SubmissionSHA256 == "" || run.PackageManifestSHA256 == "" || run.TranscriptSetSHA256 == "" || run.PromptTokens != 120 || run.CompletionTokens != 42 {
		t.Fatalf("run = %+v", run)
	}
}

func TestPublishReviewAtomicallyWritesAttestationAndSubmission(t *testing.T) {
	out := filepath.Join(t.TempDir(), "completed-review")
	submissions := []fillereval.LabelSubmission{{Alias: "review-one", ReviewerID: "reviewer-a", BatchID: "blind-a", ReviewedAt: time.Date(2026, 8, 27, 22, 0, 0, 0, time.UTC), Labels: fillereval.Labels{Truth: fillereval.TruthEligible, ContentRole: "commercial", Slices: []string{"commercial"}, Evidence: []fillereval.Evidence{{ID: "frame-01", Kind: "frame", Claim: "content_role", Value: "commercial", Provenance: "cases/review-one/frame-01.jpg", AtMS: 1000}}}}}
	run := ReviewRun{SchemaVersion: ReviewRunSchemaVersion, BatchID: "blind-a", ReviewerID: "reviewer-a", ResolvedModel: "reviewer-a@sha256:test", Cases: 1, Requests: 1, Calls: []ReviewCall{{Alias: "review-one", ReviewedAt: submissions[0].ReviewedAt}}, SubmissionSHA256: submissionSHA256(submissions)}

	if err := PublishReview(out, run, submissions); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "review-run.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "labels.jsonl"))
	if err != nil || strings.Count(string(raw), "\n") != 1 || !strings.Contains(string(raw), `"alias":"review-one"`) {
		t.Fatalf("labels = %q, %v", raw, err)
	}
	if err := PublishReview(out, run, submissions); err == nil {
		t.Fatal("existing completed review was replaced")
	}
}

func TestValidateReviewPackageAcceptsDecoderOnlyUnusableCase(t *testing.T) {
	packageDir, _ := reviewPackageFixture(t)
	manifest, err := readStrictJSON[Package](filepath.Join(packageDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Cases[0].Signals = nil
	manifest.Cases[0].DecoderFacts = []DecoderFact{{Claim: "media_usability", Value: "unusable", Kind: "decoder"}}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "manifest.json"), append(raw, '\n'), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := validateReviewPackage(packageDir, manifest, 1); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOllamaReviewConfigLeavesTimeoutToPerCaseContext(t *testing.T) {
	_, client, err := validateOllamaReviewConfig(OllamaReviewConfig{PackageDir: "/tmp/review", BaseURL: "http://127.0.0.1:11434", Model: "reviewer:1", ModelDigest: strings.Repeat("a", 64), ReviewerID: "reviewer-a", ExpectedCases: 300, PerCaseTimeout: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if client.Timeout != 0 {
		t.Fatalf("client timeout = %s; per-case context must own the hard deadline", client.Timeout)
	}
}

func TestIndexReviewTranscriptsRejectsExtraHiddenCase(t *testing.T) {
	packageDir, transcript := reviewPackageFixture(t)
	manifest, err := readStrictJSON[Package](filepath.Join(packageDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	extra := transcript
	extra.CaseID = "unrelated-hidden-case"
	extra.AudioSHA256 = strings.Repeat("9", 64)

	if _, _, err := indexReviewTranscripts([]fillerbakeoff.TranscriptArtifact{transcript, extra}, manifest); err == nil || !strings.Contains(err.Error(), "exactly the package audio signals") {
		t.Fatalf("extra transcript error = %v", err)
	}
}

func TestValidateReviewEvidenceAcceptsHashBoundEmptyTranscript(t *testing.T) {
	packageDir, transcript := reviewPackageFixture(t)
	manifest, err := readStrictJSON[Package](filepath.Join(packageDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	transcript.Text = ""
	transcript.TextSHA256 = sha(nil)
	transcript.Segments = nil
	evidence := []fillereval.Evidence{{ID: "transcript-01", Kind: "transcript", Claim: "speech", Value: "none", Provenance: "cases/review-one/audio-01.wav#transcript"}}
	if err := validateReviewEvidence(manifest.Cases[0], evidence, map[string]fillerbakeoff.TranscriptArtifact{transcript.AudioSHA256: transcript}); err != nil {
		t.Fatal(err)
	}
}

func reviewPackageFixture(t *testing.T) (string, fillerbakeoff.TranscriptArtifact) {
	t.Helper()
	root := t.TempDir()
	caseDir := filepath.Join(root, "cases", "review-one")
	if err := os.MkdirAll(caseDir, 0o750); err != nil {
		t.Fatal(err)
	}
	var signals []Signal
	for i := 1; i <= 4; i++ {
		name := "frame-0" + string(rune('0'+i)) + ".jpg"
		data := []byte("jpeg-" + name)
		path := filepath.Join("cases", "review-one", name)
		if err := os.WriteFile(filepath.Join(root, path), data, 0o640); err != nil {
			t.Fatal(err)
		}
		signals = append(signals, Signal{ID: strings.TrimSuffix(name, ".jpg"), Kind: "frame", Path: path, SHA256: sha(data), Bytes: int64(len(data)), Width: 640, Height: 360, AtMS: int64(i) * 1000, ContentTypes: []string{"image/jpeg"}})
	}
	audio := []byte("wav-audio")
	audioPath := filepath.Join("cases", "review-one", "audio-01.wav")
	if err := os.WriteFile(filepath.Join(root, audioPath), audio, 0o640); err != nil {
		t.Fatal(err)
	}
	signals = append(signals, Signal{ID: "audio-01", Kind: "audio", Path: audioPath, SHA256: sha(audio), Bytes: int64(len(audio)), DurationMS: 5000, ContentTypes: []string{"audio/wav"}})
	instructions := []byte("blind instructions")
	template := []byte("empty template")
	if err := os.WriteFile(filepath.Join(root, "INSTRUCTIONS.md"), instructions, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "labels.template.jsonl"), template, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := Package{SchemaVersion: SchemaVersion, BatchID: "blind-a", DraftSHA256: strings.Repeat("b", 64), ReviewPacketSHA256: strings.Repeat("c", 64), EvidenceVersion: "evidence-v1", InstructionsPath: "INSTRUCTIONS.md", InstructionsSHA256: sha(instructions), LabelTemplatePath: "labels.template.jsonl", LabelTemplateSHA256: sha(template), Cases: []Case{{Alias: "review-one", ContentSHA256: strings.Repeat("d", 64), EvidenceSHA256: strings.Repeat("e", 64), SegmentDurationMS: 5000, Signals: signals}}}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0o640); err != nil {
		t.Fatal(err)
	}
	transcript := fillerbakeoff.TranscriptArtifact{SchemaVersion: fillerbakeoff.TranscriptSchemaVersion, CaseID: "case-secret", PacketSHA256: strings.Repeat("f", 64), EvidenceVersion: "evidence-v1", AudioSignalID: "audio-01", AudioSHA256: sha(audio), AudioBytes: int64(len(audio)), AudioDurationMS: 5000, Engine: fillerbakeoff.TranscriptEngineIdentity{Provider: "whisper.cpp", ImplementationVersion: "1", BinarySHA256: strings.Repeat("1", 64), Model: "base.en", ModelSHA256: strings.Repeat("2", 64)}, GeneratedAt: time.Date(2026, 8, 27, 20, 0, 0, 0, time.UTC), LatencyMS: 10, Segments: []fillerbakeoff.TranscriptSegment{{StartMS: 0, EndMS: 1000, Text: "Drink Bright Cola"}}, Text: "Drink Bright Cola", TextSHA256: sha([]byte("Drink Bright Cola"))}
	return root, transcript
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
