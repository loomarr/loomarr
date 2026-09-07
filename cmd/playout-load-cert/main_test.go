package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestCleanupFailureIsPersistedAsUncertifiedReport(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	report := playoutcert.Report{Certified: true, FaultProfiles: []playoutcert.FaultQualification{{Profile: playoutcert.FaultParentFailure, Status: "qualified", Outcome: "complete"}}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := finalizeAfterIsolatedCleanup(output, report, true, playoutcertfixture.CleanupFailureTarget{Err: errors.New("cleanup failed")}, time.Second, stdout, stderr); code != 1 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	blob, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted playoutcert.Report
	if err := json.Unmarshal(blob, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Certified || !strings.Contains(strings.Join(persisted.Failures, ","), "isolated_cleanup_failed") {
		t.Fatalf("persisted cleanup failure report = %+v", persisted)
	}
	if !strings.Contains(stdout.String(), "parent_failure status=unavailable outcome=cleanup_failed") {
		t.Fatalf("summary omitted persisted fault evidence: %q", stdout.String())
	}
}

func TestFinalizeWithTypedNilSyntheticTargetPublishesReport(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	var target *playoutcert.SyntheticTarget
	report := playoutcert.Report{Certified: true}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if code := finalizeAfterIsolatedCleanup(output, report, true, target, time.Second, stdout, stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatalf("report artifact: %v", err)
	}
	if !strings.Contains(stdout.String(), "Playout load certification: PASS") {
		t.Fatalf("summary = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCommandRejectsMissingSecretsAndOutputEscapeWithoutEchoingValues(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schemaVersion":1,"channels":[{"id":"private-channel"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := "must-never-be-printed"
	env := func(key string) string {
		switch key {
		case "LOOMARR_ARTIFACT_DIR":
			return dir
		case "LOOMARR_PLAYOUT_CERT_BASE_URL":
			return "http://127.0.0.1:1"
		case "LOOMARR_API_TOKEN":
			return secret
		}
		return ""
	}
	stderr := &bytes.Buffer{}
	code := run(context.Background(), []string{"--manifest", manifestPath, "--out", filepath.Join(dir, "..", "escape.json")}, env, &bytes.Buffer{}, stderr)
	if code != 2 || !strings.Contains(stderr.String(), "under LOOMARR_ARTIFACT_DIR") || strings.Contains(stderr.String(), secret) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestCommandEnforcesProgrammeAndSuiteTimingBounds(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schemaVersion":1,"channels":[{"id":"private-channel"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "boundary timeout below floor", args: []string{"--programme-boundary-timeout", "1999ms"}, want: "resource bounds"},
		{name: "boundary timeout exceeds suite", args: []string{"--programme-boundary-timeout", "30m", "--suite-timeout", "30m"}, want: "resource bounds"},
		{name: "late observation equals soak", args: []string{"--programme-boundary-timeout", "2s", "--programme-boundary-late-observation", "2s"}, want: "resource bounds"},
		{name: "synthetic period below floor", args: []string{"--synthetic-programme-duration", "1999ms"}, want: "resource bounds"},
		{name: "suite exceeds ceiling", args: []string{"--suite-timeout", "30m1ns"}, want: "resource bounds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stderr := &bytes.Buffer{}
			args := append([]string{"--manifest", manifestPath}, tc.args...)
			if code := run(context.Background(), args, func(string) string { return "" }, &bytes.Buffer{}, stderr); code != 2 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("code=%d stderr=%q", code, stderr.String())
			}
		})
	}
	// With no timing flags the documented defaults pass flag validation and
	// reach the next bounded preflight check.
	stderr := &bytes.Buffer{}
	if code := run(context.Background(), []string{"--manifest", manifestPath}, func(string) string { return "" }, &bytes.Buffer{}, stderr); code != 2 || !strings.Contains(stderr.String(), "LOOMARR_ARTIFACT_DIR is required") {
		t.Fatalf("default timing preflight code=%d stderr=%q", code, stderr.String())
	}
}

func TestReadManifestRejectsUnknownAndTrailingContent(t *testing.T) {
	for _, raw := range []string{
		`{"schemaVersion":1,"channels":[],"secret":"x"}`,
		`{"schemaVersion":1,"channels":[]} {}`,
		`{"schemaVersion":2,"channels":[]}`,
	} {
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readManifest(path); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestReadManifestEnforcesWholeFileSizeLimit(t *testing.T) {
	const limit = 1 << 20
	valid := []byte(`{"schemaVersion":1,"channels":[]}`)
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, append(valid, bytes.Repeat([]byte(" "), limit-len(valid))...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err != nil {
		t.Fatalf("exactly limited manifest rejected: %v", err)
	}
	for _, contents := range [][]byte{
		append(valid, bytes.Repeat([]byte(" "), limit-len(valid)+1)...),
		append(append([]byte(nil), valid...), append(bytes.Repeat([]byte(" "), limit-len(valid)), []byte(`{}`)...)...),
	} {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readManifest(path); err == nil {
			t.Fatal("oversized manifest was accepted")
		}
	}
}

func TestCommandRejectsSharedInputsBeforeSyntheticSetup(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schemaVersion":1,"channels":[{"id":"channel"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	env := func(key string) string {
		if key == "LOOMARR_ARTIFACT_DIR" {
			return dir
		}
		return ""
	}
	code := run(context.Background(), []string{"--manifest", manifestPath, "--synthetic", "--ffmpeg", "missing-ffmpeg", "--concurrency", "65"}, env, &bytes.Buffer{}, stderr)
	if code != 2 || !strings.Contains(stderr.String(), "resource bounds") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestWriteArtifactIsPrivate(t *testing.T) {
	dir := t.TempDir()
	output, err := openContainedOutput(dir, filepath.Join(dir, "nested", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	if err := writeArtifact(output, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "nested", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestWriteArtifactRefusesEscapedSymlinkAndParentSwap(t *testing.T) {
	rootDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootDir, "escaped")); err != nil {
		t.Fatal(err)
	}
	output, err := openContainedOutput(rootDir, filepath.Join(rootDir, "escaped", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	if err := writeArtifact(output, []byte("{}\n")); err == nil {
		t.Fatal("symlink escape was published")
	}
	if _, err := os.Stat(filepath.Join(outside, "report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside artifact stat = %v", err)
	}

	safe, err := openContainedOutput(rootDir, filepath.Join(rootDir, "nested", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = safe.Close() }()
	if err := os.Mkdir(filepath.Join(rootDir, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(rootDir, "nested"), filepath.Join(rootDir, "nested-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(rootDir, "nested")); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifact(safe, []byte("{}\n")); err == nil {
		t.Fatal("swapped parent was published outside root")
	}
	if _, err := os.Stat(filepath.Join(outside, "report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside artifact after swap stat = %v", err)
	}
}

func TestPublishReportReturnsFailureForRequiredFailureWithoutCertification(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{
		SchemaVersion: playoutcert.SchemaVersion,
		CompletedAt:   time.Now(),
		Failures:      []string{"shutdown_failed"},
	}
	if code := publishReport(output, report, false, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	blob, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), `"shutdown_failed"`) || !strings.Contains(stdout.String(), "Failures: shutdown_failed") || stderr.Len() != 0 {
		t.Fatalf("report publication lost failure: artifact=%q stdout=%q stderr=%q", blob, stdout.String(), stderr.String())
	}
}

func TestPublishReportAllowsSuccessfulDiagnosticRun(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{SchemaVersion: playoutcert.SchemaVersion, CompletedAt: time.Now()}
	if code := publishReport(output, report, false, stdout, stderr); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Playout load certification: FAIL") || stderr.Len() != 0 {
		t.Fatalf("unexpected diagnostic publication: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPublishReportRejectsUncertifiedCertification(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{SchemaVersion: playoutcert.SchemaVersion, CompletedAt: time.Now()}
	if code := publishReport(output, report, true, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatal(err)
	}
}
