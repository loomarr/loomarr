package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
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
	report := successfulRunReport(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := finalizeAfterIsolatedCleanup(output, report, playoutcertfixture.CleanupFailureTarget{Err: errors.New("cleanup failed")}, time.Second, stdout, stderr); code != 1 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	blob, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published playoutcert.Report
	if err := json.Unmarshal(blob, &published); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(published.Failures, ","), string(playoutcert.PublicationDowngradeCleanupFailed)) || published.Certified {
		t.Fatalf("cleanup failure was not published as an uncertified downgrade: %+v", published)
	}
	foundDowngradedFault := false
	for _, row := range published.FaultProfiles {
		if row.Profile == playoutcert.FaultParentFailure {
			foundDowngradedFault = row.Status == "unavailable" && row.Outcome == "cleanup_failed"
		}
	}
	if !foundDowngradedFault || stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf("cleanup downgrade publication = %+v stdout=%q stderr=%q", published, stdout.String(), stderr.String())
	}
}

func TestFinalizeWithTypedNilSyntheticTargetPublishesReport(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	var target *app.PlayoutCertificationTarget
	report := playoutcert.Report{Certified: true}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if code := finalizeAfterIsolatedCleanup(output, report, target, time.Second, stdout, stderr); code != 1 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatalf("report artifact: %v", err)
	}
	if !strings.Contains(stdout.String(), "audit_capsule_missing") {
		t.Fatalf("forged report was not reduced to the fixed minimal publication: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPublishReportUsesActualRunPublication(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	report := successfulRunReport(t)
	publication, err := playoutcert.FinalizePublication(report)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Verdict() != playoutcert.VerdictCertified || publication.AuditStatus() != playoutcert.AuditPassed || publication.ExitStatus() != 0 {
		t.Fatalf("successful run did not finalize as a passing certification: verdict=%q audit=%q exit=%d", publication.Verdict(), publication.AuditStatus(), publication.ExitStatus())
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := publishReport(output, report, stdout, stderr); code != 0 {
		t.Fatalf("exit status = %d, want 0", code)
	}
	blob, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blob, publication.JSON()) || !bytes.Equal(stdout.Bytes(), publication.Summary()) || stderr.Len() != 0 {
		t.Fatalf("publication was not returned verbatim: artifact=%q stdout=%q stderr=%q", blob, stdout.Bytes(), stderr.Bytes())
	}
}

func TestPublishReportPreservesExistingArtifactWhenRunAuditIsUnavailable(t *testing.T) {
	fixture := playoutcertfixture.New(t, 1)
	fixture.MintRelativeURL = oversizedSignedRelativeURL()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := playoutcert.Run(ctx, playoutcert.Config{
		BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
		Channels: []playoutcert.Channel{{ID: "prepared", Roles: []string{"prepared"}}},
		Certify:  false, Concurrency: 1, SurfRounds: 1, FanInViewers: 1,
		RequestTimeout: time.Second, CleanupTimeout: 50 * time.Millisecond, CleanupPoll: time.Millisecond,
		WarmGrace: time.Millisecond, RawCaptureBytes: 188, PreparedP95: time.Millisecond,
		PreparedRawP95: time.Millisecond, ProgrammeBoundaryTimeout: 2 * time.Second,
		ProgrammeBoundaryLateObservation: 250 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if report.Target.Version != fixture.Version || len(report.Resources) == 0 || report.Resources[0].Point != "baseline" {
		t.Fatalf("Run did not complete target and resource preflight: target=%+v resources=%+v", report.Target, report.Resources)
	}
	publication, err := playoutcert.FinalizePublication(report)
	if err == nil || publication.Verdict() != playoutcert.VerdictUnavailable || publication.AuditStatus() != playoutcert.AuditUnavailable {
		t.Fatalf("first finalization = verdict %q audit %q err %v", publication.Verdict(), publication.AuditStatus(), err)
	}
	cached, cachedErr := playoutcert.FinalizePublication(report)
	if cachedErr == nil || cached.Verdict() != playoutcert.VerdictUnavailable || cached.AuditStatus() != playoutcert.AuditUnavailable {
		t.Fatalf("cached finalization = verdict %q audit %q err %v", cached.Verdict(), cached.AuditStatus(), cachedErr)
	}

	outputDir := t.TempDir()
	path := filepath.Join(outputDir, "report.json")
	prior := []byte("prior report bytes\n")
	if err := os.WriteFile(path, prior, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := openContainedOutput(outputDir, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := publishReport(output, report, stdout, stderr); code != 1 {
		t.Fatalf("publish exit code = %d, stderr=%q", code, stderr.String())
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, prior) || stdout.Len() != 0 || stderr.String() != "playout-load-cert: report finalization failed\n" {
		t.Fatalf("unpublishable report changed output: artifact=%q stdout=%q stderr=%q", actual, stdout.String(), stderr.String())
	}
}

func oversizedSignedRelativeURL() string {
	query := make(url.Values, 8195)
	query.Set("exp", "1")
	query.Set("sig", "ordinary-signed-secret")
	for index := range 8193 {
		query.Set(fmt.Sprintf("proof-%05d", index), fmt.Sprintf("dummy-secret-%05d-%s", index, strings.Repeat("x", 40)))
	}
	return "/v1/playout/hls/prepared/master.m3u8?" + query.Encode()
}

func successfulRunReport(t *testing.T) playoutcert.Report {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	channels := make([]playoutcert.Channel, 0, 106)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	channels = append(channels, playoutcert.Channel{ID: "copy-channel", Roles: []string{"copy", "audio_aac"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if closeErr := target.Close(closeCtx); closeErr != nil {
			t.Errorf("close synthetic target: %v", closeErr)
		}
	})
	report, err := playoutcert.Run(ctx, playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace: time.Second, RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(), FaultProfiles: []playoutcert.FaultProfile{playoutcert.FaultParentFailure}, FaultController: target,
		Validator: playoutcert.FFprobeValidator{Path: ffprobe}, Decoder: playoutcert.FFmpegDecoder{Path: ffmpeg},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Certified || report.AuditStatus != playoutcert.AuditMissing {
		t.Fatalf("Run exposed a final verdict before publication: certified=%t audit=%s", report.Certified, report.AuditStatus)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("required successful run failed: %v; boundary evidence: %+v; copy: %+v; overload: %+v", report.Failures, report.PhaseMust("programme_boundary").ProgrammeBoundaries, report.PhaseMust("copy_raw"), report.PhaseMust("overload"))
	}
	return report
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

func TestPublishReportRefusesForgedFailureDetails(t *testing.T) {
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
	if code := publishReport(output, report, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	blob, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "shutdown_failed") || !strings.Contains(stdout.String(), "audit_capsule_missing") || stderr.Len() != 0 {
		t.Fatalf("forged report escaped minimal publication: artifact=%q stdout=%q stderr=%q", blob, stdout.String(), stderr.String())
	}
}

func TestPublishReportRefusesForgedSuccess(t *testing.T) {
	outputDir := t.TempDir()
	output, err := openContainedOutput(outputDir, filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	report := playoutcert.Report{SchemaVersion: playoutcert.SchemaVersion, CompletedAt: time.Now(), Certified: true}
	if code := publishReport(output, report, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "audit_capsule_missing") || strings.Contains(stdout.String(), "PASS") || stderr.Len() != 0 {
		t.Fatalf("forged success escaped minimal publication: stdout=%q stderr=%q", stdout.String(), stderr.String())
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
	if code := publishReport(output, report, stdout, stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCommandOperatorInputRejectsBeforeOutputOrTargetSetup(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "channels.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schemaVersion":1,"channels":[{"id":"channel"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]string{{"--operator-cohort", ""}, {"--operator-cohort", "private-corpus.json", "--synthetic"}, {"--operator-cohort", filepath.Join(dir, "private-missing.json")}} {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		args := append([]string{"--manifest", manifestPath, "--ffmpeg", "missing-ffmpeg"}, selection...)
		code := run(t.Context(), args, func(string) string {
			t.Error("environment accessed before rejecting operator selection/input")
			return ""
		}, stdout, stderr)
		if code != 2 || stdout.Len() != 0 || strings.Contains(stderr.String(), "private-") || strings.Contains(stderr.String(), "setup failed") {
			t.Fatalf("unsafe operator rejection code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
}
