package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("LOOMARR_IMAGE_BENCH_TEST_WORKER") == "1" {
		os.Exit(runTestWorker(os.Args[1:], os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func TestRunMeasuresTheCurrentIconAVIFLadderThroughTheWorkerProtocol(t *testing.T) {
	t.Setenv("LOOMARR_IMAGE_BENCH_TEST_WORKER", "1")
	reportPath := filepath.Join(t.TempDir(), "image-benchmark.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--worker", os.Args[0],
		"--report", reportPath,
		"--roles", "icon",
		"--runs", "1",
		"--warmups", "0",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run = %d; stderr = %s", code, stderr.String())
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		SchemaVersion int    `json:"schemaVersion"`
		Corpus        string `json:"corpus"`
		Strategy      string `json:"strategy"`
		Profiles      []struct {
			Role         string `json:"role"`
			SourceSHA256 string `json:"sourceSha256"`
			Widths       []int  `json:"widths"`
			Samples      []struct {
				Processes     int     `json:"processes"`
				Renditions    int     `json:"renditions"`
				OutputBytes   int64   `json:"outputBytes"`
				WallTimeMS    int64   `json:"wallTimeMs"`
				PeakRSSBytes  int64   `json:"peakRssBytes"`
				WorkerTimesMS []int64 `json:"workerTimesMs"`
			} `json:"samples"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SchemaVersion != 1 || report.Corpus != "synthetic-role-v1" ||
		report.Strategy != "serial-per-rendition-v1" || len(report.Profiles) != 1 {
		t.Fatalf("report envelope = %+v", report)
	}
	profile := report.Profiles[0]
	if profile.Role != "icon" || len(profile.Samples) != 1 {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.SourceSHA256 != "5e3f7a75ad5f3e19b331274eb13c800ad8c9637492450edd4a6f2255c25f0bac" {
		t.Errorf("source SHA-256 = %q, want the synthetic-role-v1 icon source", profile.SourceSHA256)
	}
	if got, want := profile.Widths, []int{92, 185, 500}; !equalInts(got, want) {
		t.Errorf("widths = %v, want %v", got, want)
	}
	sample := profile.Samples[0]
	if sample.Processes != 3 || sample.Renditions != 3 || len(sample.WorkerTimesMS) != 3 {
		t.Errorf("sample counts = %+v, want one real worker process per Rendition", sample)
	}
	if sample.OutputBytes <= 0 || sample.WallTimeMS <= 0 {
		t.Errorf("sample measurements = %+v, want positive output and elapsed time", sample)
	}
	if !strings.Contains(stdout.String(), reportPath) || !strings.Contains(stdout.String(), "3 renditions") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func runTestWorker(args []string, stdin io.Reader, stdout io.Writer) int {
	if len(args) > 0 && args[0] == "capabilities" {
		_, _ = io.WriteString(stdout, `{"protocol":1,"release":"dev","recipe":"loomarr-rendition-v1","formats":["avif","jpeg","webp"],"animation":true,"selfTest":true}`+"\n")
		return 0
	}
	var request struct {
		RequestID  string `json:"requestId"`
		StagingDir string `json:"stagingDir"`
		Source     struct {
			Path           string `json:"path"`
			ExpectedSHA256 string `json:"expectedSha256"`
		} `json:"source"`
		Targets []struct {
			ID     string `json:"id"`
			Format string `json:"format"`
			Width  int    `json:"width"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(stdin).Decode(&request); err != nil {
		return 1
	}
	source, err := os.Stat(request.Source.Path)
	if err != nil {
		return 1
	}
	avif := []byte("\x00\x00\x00\x0cftypavif")
	digest := fmt.Sprintf("%x", sha256.Sum256(avif))
	type output struct {
		TargetID       string `json:"targetId"`
		RelativePath   string `json:"relativePath"`
		RecipeID       string `json:"recipeId"`
		Format         string `json:"format"`
		MIME           string `json:"mime"`
		RequestedWidth int    `json:"requestedWidth"`
		Width          int    `json:"width"`
		Height         int    `json:"height"`
		Bytes          int64  `json:"bytes"`
		SHA256         string `json:"sha256"`
		Animated       bool   `json:"animated"`
	}
	outputs := make([]output, 0, len(request.Targets))
	for _, target := range request.Targets {
		name := target.ID + ".avif"
		if err := os.WriteFile(filepath.Join(request.StagingDir, name), avif, 0o600); err != nil {
			return 1
		}
		outputs = append(outputs, output{
			TargetID: target.ID, RelativePath: name, RecipeID: "loomarr-rendition-v1",
			Format: target.Format, MIME: "image/avif", SHA256: digest,
			RequestedWidth: target.Width, Width: target.Width, Height: target.Width,
			Bytes: int64(len(avif)), Animated: false,
		})
	}
	response := struct {
		Protocol  int    `json:"protocol"`
		Status    string `json:"status"`
		RequestID string `json:"requestId"`
		Source    struct {
			SHA256      string `json:"sha256"`
			MIME        string `json:"mime"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Bytes       int64  `json:"bytes"`
			Animated    bool   `json:"animated"`
			FrameCount  int    `json:"frameCount"`
			DurationMS  int64  `json:"durationMs"`
			LoopCount   *int   `json:"loopCount"`
			Placeholder string `json:"placeholder"`
			DominantHex string `json:"dominantHex"`
		} `json:"source"`
		Outputs []output `json:"outputs"`
	}{Protocol: 1, Status: "ok", RequestID: request.RequestID, Outputs: outputs}
	response.Source.SHA256 = request.Source.ExpectedSHA256
	response.Source.MIME = "image/png"
	response.Source.Placeholder = "AQID"
	response.Source.DominantHex = "#112233"
	response.Source.Width, response.Source.Height = 1000, 1000
	response.Source.FrameCount = 1
	response.Source.Bytes = source.Size()
	if err := json.NewEncoder(stdout).Encode(response); err != nil {
		return 1
	}
	return 0
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
