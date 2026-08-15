package images

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/testkit"
)

func TestCertifyReportsARealStaticLadder(t *testing.T) {
	corpus := t.TempDir()
	data := pngBytes(t, testImage(640, 360))
	if err := os.WriteFile(filepath.Join(corpus, "opaque-landscape.png"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	limits := DefaultCertificationLimits()
	// make check exercises the intentionally unoptimized debug worker while Go packages run in
	// parallel. This test owns report correctness; the isolated make image-cert release-profile
	// gate owns the design's 10-second static ceiling.
	limits.StaticMaxDuration = time.Minute
	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: corpus,
		Renderer:  testkit.RustImageRenderer(t),
		Limits:    limits,
	})
	if err != nil {
		t.Fatalf("Certify: %v; report = %+v", err, report)
	}
	if !report.Passed || report.Summary.Passed != 1 || report.Summary.Failed != 0 {
		t.Fatalf("report summary = %+v, want one passed case", report.Summary)
	}
	if len(report.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(report.Cases))
	}
	got := report.Cases[0]
	if got.Path != "opaque-landscape.png" || got.SourceBytes != int64(len(data)) {
		t.Errorf("source = %+v", got)
	}
	if got.MIME != "image/png" || got.Width != 640 || got.Height != 360 || got.Animated {
		t.Errorf("inspection = %+v", got)
	}
	if len(got.Outputs) != 3 {
		t.Fatalf("outputs = %+v, want JPEG/WebP/AVIF", got.Outputs)
	}
	formats := map[string]bool{}
	for _, output := range got.Outputs {
		formats[output.Format] = true
		if output.Width != 320 || output.Height != 180 || output.Bytes <= 0 || output.SHA256 == "" {
			t.Errorf("invalid output = %+v", output)
		}
	}
	for _, format := range []string{"jpeg", "webp", "avif"} {
		if !formats[format] {
			t.Errorf("missing %s output", format)
		}
	}
	if got.WallTimeMS <= 0 || got.PeakRSSBytes <= 0 {
		t.Errorf("runtime observation = wall %dms, peak RSS %d", got.WallTimeMS, got.PeakRSSBytes)
	}
	if report.Summary.SourceBytes != int64(len(data)) || report.Summary.OutputBytes <= 0 ||
		report.Summary.P95WallTimeMS <= 0 || report.Summary.MaxPeakRSSBytes <= 0 {
		t.Errorf("summary measurements = %+v", report.Summary)
	}
}

func TestCertifyPreservesAnimatedWebPTimeline(t *testing.T) {
	corpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(corpus, "finite-hover.webp"), animatedWebPBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: corpus,
		Renderer:  testkit.RustImageRenderer(t),
		Limits:    DefaultCertificationLimits(),
	})
	if err != nil {
		t.Fatalf("Certify: %v", err)
	}
	got := report.Cases[0]
	if !got.Animated || got.FrameCount != 2 || got.DurationMS != 400 || got.LoopCount == nil {
		t.Fatalf("animation timeline = %+v", got)
	}
	for _, output := range got.Outputs {
		wantAnimated := output.Format == "webp"
		if output.Animated != wantAnimated {
			t.Errorf("%s animated = %t, want %t", output.Format, output.Animated, wantAnimated)
		}
	}
	if !got.StagingClean {
		t.Error("certification left its private staging behind")
	}
}

func TestCertifyRecordsAnExpectedStableRefusal(t *testing.T) {
	corpus := t.TempDir()
	corrupt := append([]byte("\x89PNG\r\n\x1a\n"), []byte("truncated")...)
	if err := os.WriteFile(filepath.Join(corpus, "corrupt.png"), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir:        corpus,
		Renderer:         testkit.RustImageRenderer(t),
		Limits:           DefaultCertificationLimits(),
		ExpectedRefusals: map[string]string{"corrupt.png": "corrupt_input"},
	})
	if err != nil {
		t.Fatalf("expected refusal should certify: %v", err)
	}
	if !report.Passed || report.Summary.Refused != 1 || report.Summary.Failed != 0 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	got := report.Cases[0]
	if got.Outcome != "refused" || got.ErrorCode != "corrupt_input" || !got.StagingClean {
		t.Errorf("refusal = %+v", got)
	}
}

func TestRepositoryCertificationCorpusCoversStaticAnimationAndLimits(t *testing.T) {
	corpus := t.TempDir()
	manifest, err := WriteCertificationCorpus(corpus)
	if err != nil {
		t.Fatalf("WriteCertificationCorpus: %v", err)
	}
	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: corpus, Renderer: testkit.RustImageRenderer(t),
		Limits: DefaultCertificationLimits(), ExpectedRefusals: manifest.ExpectedRefusals,
		BoundaryCases: manifest.BoundaryCases,
	})
	if err != nil {
		t.Fatalf("repository corpus: %v; report = %+v", err, report)
	}
	if report.Summary.Passed != 7 || report.Summary.Refused != 11 || report.Summary.Failed != 0 {
		t.Fatalf("summary = %+v, want 7 accepted and 11 stable refusals", report.Summary)
	}
	byPath := map[string]CertificationCase{}
	for _, result := range report.Cases {
		byPath[result.Path] = result
	}
	if got := byPath["fractional-zero-delay.apng"]; !got.Animated || got.FrameCount != 2 || got.DurationMS != 27 {
		t.Errorf("APNG timeline = %+v", got)
	}
	if got := byPath["finite-loop.gif"]; !got.Animated || got.LoopCount == nil || *got.LoopCount != 2 {
		t.Errorf("finite GIF loop = %+v", got)
	}
	if got := byPath["one-frame.gif"]; got.Animated || got.FrameCount != 1 || got.LoopCount != nil {
		t.Errorf("one-frame GIF should be static: %+v", got)
	}
	if got := byPath["static.webp"]; got.MIME != "image/webp" || got.Animated {
		t.Errorf("static WebP = %+v", got)
	}
	for path, code := range manifest.ExpectedRefusals {
		if got := byPath[path]; got.Outcome != "refused" || got.ErrorCode != code {
			t.Errorf("%s refusal = %+v, want %s", path, got, code)
		}
	}
	for _, boundary := range manifest.BoundaryCases {
		if got := byPath["@limits/"+boundary.Name]; got.Outcome != "refused" || got.ErrorCode != boundary.ExpectedCode {
			t.Errorf("%s boundary = %+v, want %s", boundary.Name, got, boundary.ExpectedCode)
		}
	}
}

func TestCertifyRejectsAnEmptyCorpus(t *testing.T) {
	_, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: t.TempDir(), Renderer: testkit.RustImageRenderer(t), Limits: DefaultCertificationLimits(),
	})
	if err == nil {
		t.Fatal("empty corpus certified successfully")
	}
}

func TestCertifyTreatsAnOversizedOperatorInputAsAStableBudgetRefusal(t *testing.T) {
	corpus := t.TempDir()
	path := filepath.Join(corpus, "oversized.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(certificationMaxInputBytes + 1024); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: corpus, Renderer: testkit.RustImageRenderer(t), Limits: DefaultCertificationLimits(),
	})
	if err != nil {
		t.Fatalf("oversized input should be an expected policy refusal: %v", err)
	}
	got := report.Cases[0]
	if got.Outcome != "refused" || got.ErrorCode != "limit_exceeded" || got.SourceBytes != certificationMaxInputBytes+1024 {
		t.Fatalf("oversized case = %+v", got)
	}
}

func TestCertifyReportsUnsupportedFilesAndDoesNotFollowSymlinks(t *testing.T) {
	corpus := t.TempDir()
	want := pngBytes(t, testImage(32, 18))
	imagePath := filepath.Join(corpus, "owned.png")
	if err := os.WriteFile(imagePath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "notes.txt"), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(imagePath, filepath.Join(corpus, "alias.png")); err != nil {
		t.Fatal(err)
	}

	report, err := Certify(context.Background(), CertificationOptions{
		CorpusDir: corpus, Renderer: testkit.RustImageRenderer(t), Limits: DefaultCertificationLimits(),
	})
	if err != nil {
		t.Fatalf("Certify: %v", err)
	}
	if report.Summary.Passed != 1 || report.Summary.Skipped != 1 || len(report.Cases) != 2 {
		t.Fatalf("summary = %+v; cases = %+v", report.Summary, report.Cases)
	}
	if report.Cases[0].Path != "notes.txt" || report.Cases[0].Outcome != "skipped" {
		t.Errorf("unsupported file = %+v", report.Cases[0])
	}
	got, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("certification modified the operator corpus")
	}
}
