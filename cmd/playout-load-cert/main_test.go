package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

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

func TestWriteArtifactIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	if err := writeArtifact(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestPublishReportReturnsFailureForRequiredFailureWithoutCertification(t *testing.T) {
	output := filepath.Join(t.TempDir(), "report.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{
		SchemaVersion: playoutcert.SchemaVersion,
		CompletedAt:   time.Now(),
		Failures:      []string{"shutdown_failed"},
	}
	if code := publishReport(output, report, false, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	blob, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), `"shutdown_failed"`) || !strings.Contains(stdout.String(), "Failures: shutdown_failed") || stderr.Len() != 0 {
		t.Fatalf("report publication lost failure: artifact=%q stdout=%q stderr=%q", blob, stdout.String(), stderr.String())
	}
}

func TestPublishReportAllowsSuccessfulDiagnosticRun(t *testing.T) {
	output := filepath.Join(t.TempDir(), "report.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{SchemaVersion: playoutcert.SchemaVersion, CompletedAt: time.Now()}
	if code := publishReport(output, report, false, stdout, stderr); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Playout load certification: FAIL") || stderr.Len() != 0 {
		t.Fatalf("unexpected diagnostic publication: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPublishReportRejectsUncertifiedCertification(t *testing.T) {
	output := filepath.Join(t.TempDir(), "report.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{SchemaVersion: playoutcert.SchemaVersion, CompletedAt: time.Now()}
	if code := publishReport(output, report, true, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}
