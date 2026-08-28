package fillerreview

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

func TestInspectOpenRouterReviewCheckpointEmitsOnlySanitizedReadiness(t *testing.T) {
	config, checkpointDir, _ := openRouterInspectionFixture(t)
	manifest, err := readStrictJSON[Package](filepath.Join(config.ArtifactPaths.PackageDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkpointPath := filepath.Join(checkpointDir, openRouterCheckpointFilename)
	before, err := hashFile(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	if attestation.Status != OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval || attestation.ProviderExecutionAuthorized || attestation.AcceptedCases != 0 || attestation.FailedAttempts != 1 || attestation.PendingCases != 300 || attestation.HistoricalRequestsUsed != 1 || attestation.HistoricalRequestsRemaining != 300 || attestation.HistoricalRequestCeiling != 301 || attestation.HistoricalSpendCeilingNanoUSD != 4_000_000 || attestation.HistoricalPerCallCeilingNanoUSD != 2_000_000 || attestation.HistoricalRemainingAllowanceNanoUSD != 3_000_000 || len(attestation.AttestationSHA256) != 64 {
		t.Fatalf("attestation = %+v", attestation)
	}
	after, err := hashFile(checkpointPath)
	if err != nil || after != before {
		t.Fatalf("inspection mutated checkpoint: before=%s after=%s err=%v", before, after, err)
	}
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{manifest.Cases[0].Alias, config.ArtifactPaths.PackageDir, checkpointDir, "labels", "generationId", "requestSha256"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("attestation leaked %q: %s", private, raw)
		}
	}
}

func TestInspectOpenRouterReviewCheckpointDoesNotAuthorizeRecoveryOrClaimSpendReservation(t *testing.T) {
	config, _, _ := openRouterInspectionFixture(t)

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	if attestation.Status != "awaiting_explicit_maintainer_approval" || attestation.ProviderExecutionAuthorized {
		t.Fatalf("attestation = %+v", attestation)
	}
	if attestation.HistoricalSpendCeilingNanoUSD != 4_000_000 || attestation.HistoricalPerCallCeilingNanoUSD != 2_000_000 || attestation.HistoricalRemainingAllowanceNanoUSD != 3_000_000 {
		t.Fatalf("historical monetary accounting = %+v", attestation)
	}
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ready_to_resume", "reservedSpend", "spendReservation", "chargedNanoUsd", "maxSpendNanoUsd", "maxChargeNanoUsd"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("attestation claims unauthorized recovery or spend through %q: %s", forbidden, raw)
		}
	}
}

func TestInspectOpenRouterReviewCheckpointRequiresExactReviewerB300CaseContract(t *testing.T) {
	for name, mutate := range map[string]func(*OpenRouterReviewInspectionConfig){
		"reviewer": func(config *OpenRouterReviewInspectionConfig) { config.ReviewerID = "hosted-reviewer-a" },
		"count":    func(config *OpenRouterReviewInspectionConfig) { config.ExpectedCases = 299 },
	} {
		t.Run(name, func(t *testing.T) {
			config, _, _ := openRouterInspectionFixture(t)
			mutate(&config)
			attestation, err := InspectOpenRouterReviewCheckpoint(config)
			if err == nil || !strings.Contains(err.Error(), "Reviewer B 300-case contract") || attestation != (OpenRouterReviewAttestation{}) {
				t.Fatalf("attestation=%+v error=%v", attestation, err)
			}
		})
	}
}

func TestInspectOpenRouterReviewCheckpointAcceptsHistoricalSnapshotIdentity(t *testing.T) {
	config, _, _ := openRouterInspectionFixture(t)

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil || attestation.Status != "awaiting_explicit_maintainer_approval" {
		t.Fatalf("attestation=%+v error=%v", attestation, err)
	}
}

func TestInspectOpenRouterReviewCheckpointUsesClosedInspectionStatus(t *testing.T) {
	config, _, _ := openRouterInspectionFixture(t)
	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	var status OpenRouterReviewInspectionStatus = attestation.Status
	if status != OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval {
		t.Fatalf("status = %q", status)
	}
}

func TestInspectOpenRouterReviewCheckpointExposesOnlySanitizedFields(t *testing.T) {
	config, _, _ := openRouterInspectionFixture(t)
	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(fields))
	for field := range fields {
		got = append(got, field)
	}
	slices.Sort(got)
	want := []string{
		"acceptedCases", "attestationSha256", "capabilitySnapshotSha256", "checkpointIdentitySha256",
		"checkpointSha256", "expectedCases", "failedAttempts", "historicalPerCallCeilingNanoUsd",
		"historicalRemainingAllowanceNanoUsd", "historicalRequestCeiling", "historicalRequestsRemaining",
		"historicalRequestsUsed", "historicalSpendCeilingNanoUsd", "packageManifestSha256", "pendingCases",
		"promptSha256", "providerExecutionAuthorized", "schemaVersion", "status", "transcriptSetSha256",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("attestation fields = %q, want %q; JSON=%s", got, want, raw)
	}
	for _, forbidden := range []string{config.Model, config.UpstreamProvider, config.UpstreamProviderSlug, config.ReviewerID, "batchId", "reviewerId", "model", "resolvedModel", "upstreamProvider", "upstreamProviderSlug", "promptVersion", "recoveryRequired"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("attestation leaked forbidden field or value %q: %s", forbidden, raw)
		}
	}
}

func TestInspectOpenRouterReviewCheckpointDigestRecomputesIndependently(t *testing.T) {
	config, _, _ := openRouterInspectionFixture(t)
	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	got := attestation.AttestationSHA256
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "attestationSha256")
	canonical, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(canonical))
	if got != want {
		t.Fatalf("attestation digest = %s, independently recomputed %s", got, want)
	}
}

func TestInspectOpenRouterReviewCheckpointFailsClosed(t *testing.T) {
	tests := map[string]struct {
		mutate func(*testing.T, OpenRouterReviewInspectionConfig, string)
		want   string
	}{
		"checkpoint directory mode": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				if err := os.Chmod(checkpointDir, 0o750); err != nil {
					t.Fatal(err)
				}
			},
			want: "mode 0700",
		},
		"checkpoint file mode": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(checkpointDir, openRouterCheckpointFilename), 0o640); err != nil {
					t.Fatal(err)
				}
			},
			want: "mode 0600",
		},
		"checkpoint schema": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				mutateInspectionCheckpoint(t, checkpointDir, func(checkpoint *openRouterCheckpoint) {
					checkpoint.Identity.SchemaVersion++
				})
			},
			want: "identity drift",
		},
		"settled accounting": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				mutateInspectionCheckpoint(t, checkpointDir, func(checkpoint *openRouterCheckpoint) {
					checkpoint.Attempts[0].ChargedAmountUSD = "0.002"
				})
			},
			want: "attempt ledger is invalid",
		},
		"unsettled reservation": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				mutateInspectionCheckpoint(t, checkpointDir, func(checkpoint *openRouterCheckpoint) {
					checkpoint.Attempts[0].State = openRouterAttemptReserved
					checkpoint.Attempts[0].ChargedAmountUSD = ""
					checkpoint.Attempts[0].ChargedNanoUSD = 0
				})
			},
			want: "unsettled reserved request",
		},
		"ceiling identity": {
			mutate: func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				config.MaxSpendNanoUSD--
				if attestation, err := InspectOpenRouterReviewCheckpoint(config); err == nil || !strings.Contains(err.Error(), "identity drift") || attestation != (OpenRouterReviewAttestation{}) {
					t.Fatalf("attestation=%+v error=%v", attestation, err)
				}
			},
			want: "already asserted",
		},
		"capability artifact": {
			mutate: func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				snapshot := readInspectionSnapshot(t, config.ArtifactPaths.SnapshotPath)
				snapshot.RetrievedAt = snapshot.RetrievedAt.Add(time.Minute)
				writeInspectionSnapshot(t, config.ArtifactPaths.SnapshotPath, snapshot)
				if attestation, err := InspectOpenRouterReviewCheckpoint(config); err == nil || !strings.Contains(err.Error(), "identity drift") || attestation != (OpenRouterReviewAttestation{}) {
					t.Fatalf("attestation=%+v error=%v", attestation, err)
				}
			},
			want: "already asserted",
		},
		"active lock identity": {
			mutate: func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(checkpointDir, openRouterActiveRunLockFilename), []byte("{}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "lock is invalid",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, checkpointDir, _ := openRouterInspectionFixture(t)
			test.mutate(t, config, checkpointDir)
			if test.want == "already asserted" {
				return
			}
			attestation, err := InspectOpenRouterReviewCheckpoint(config)
			if err == nil || !strings.Contains(err.Error(), test.want) || attestation != (OpenRouterReviewAttestation{}) {
				t.Fatalf("attestation=%+v error=%v", attestation, err)
			}
		})
	}
}

func TestInspectOpenRouterReviewCheckpointRejectsArtifactIdentityAndOrderDrift(t *testing.T) {
	tests := map[string]struct {
		mutate func(*testing.T, *OpenRouterReviewInspectionConfig, string)
		want   string
	}{
		"package": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				path := filepath.Join(config.ArtifactPaths.PackageDir, "manifest.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, ' '), 0o640); err != nil {
					t.Fatal(err)
				}
			},
			want: "identity drift",
		},
		"transcript": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				transcripts := readInspectionTranscriptFixture(t, config.ArtifactPaths.TranscriptsPath)
				transcripts[0].Segments[0].Text = "Different words"
				transcripts[0].Text = "Different words"
				transcripts[0].TextSHA256 = hashBytes([]byte("Different words"))
				writeInspectionTranscripts(t, config.ArtifactPaths.TranscriptsPath, transcripts)
			},
			want: "identity drift",
		},
		"snapshot": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				snapshot := readInspectionSnapshot(t, config.ArtifactPaths.SnapshotPath)
				snapshot.RetrievedAt = snapshot.RetrievedAt.Add(time.Minute)
				writeInspectionSnapshot(t, config.ArtifactPaths.SnapshotPath, snapshot)
			},
			want: "identity drift",
		},
		"route": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				config.UpstreamProvider = "Different Provider"
			},
			want: "route is absent",
		},
		"prompt": {
			mutate: func(t *testing.T, _ *OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				mutateInspectionCheckpoint(t, checkpointDir, func(checkpoint *openRouterCheckpoint) {
					checkpoint.Identity.PromptSHA256 = strings.Repeat("9", 64)
				})
			},
			want: "identity drift",
		},
		"duplicate package alias": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, _ string) {
				t.Helper()
				mutateInspectionManifest(t, config.ArtifactPaths.PackageDir, func(manifest *Package) {
					manifest.Cases[1].Alias = manifest.Cases[0].Alias
				})
			},
			want: "duplicate alias",
		},
		"checkpoint package order": {
			mutate: func(t *testing.T, config *OpenRouterReviewInspectionConfig, checkpointDir string) {
				t.Helper()
				mutateInspectionManifest(t, config.ArtifactPaths.PackageDir, func(manifest *Package) {
					manifest.Cases[0], manifest.Cases[1] = manifest.Cases[1], manifest.Cases[0]
				})
				manifestSHA256, err := hashFile(filepath.Join(config.ArtifactPaths.PackageDir, "manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				mutateInspectionCheckpoint(t, checkpointDir, func(checkpoint *openRouterCheckpoint) {
					checkpoint.Identity.PackageManifestSHA256 = manifestSHA256
				})
			},
			want: "package order",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, checkpointDir, _ := openRouterInspectionFixture(t)
			test.mutate(t, &config, checkpointDir)
			attestation, err := InspectOpenRouterReviewCheckpoint(config)
			if err == nil || !strings.Contains(err.Error(), test.want) || attestation != (OpenRouterReviewAttestation{}) {
				t.Fatalf("attestation=%+v error=%v", attestation, err)
			}
		})
	}
}

func TestInspectOpenRouterReviewCheckpointRejectsNonExactTypesModesAndSymlinks(t *testing.T) {
	tests := map[string]func(*testing.T, OpenRouterReviewInspectionConfig, string, openRouterCheckpointIdentity){
		"package directory mode": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(config.ArtifactPaths.PackageDir, 0o750); err != nil {
				t.Fatal(err)
			}
		},
		"package directory setuid": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(config.ArtifactPaths.PackageDir, 0o700|os.ModeSetuid); err != nil {
				t.Fatal(err)
			}
		},
		"package manifest symlink": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			path := filepath.Join(config.ArtifactPaths.PackageDir, "manifest.json")
			target := path + ".target"
			if err := os.Rename(path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"package signal mode": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(filepath.Join(config.ArtifactPaths.PackageDir, "cases", "review-one", "frame-01.jpg"), 0o640); err != nil {
				t.Fatal(err)
			}
		},
		"package subdirectory sticky": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(filepath.Join(config.ArtifactPaths.PackageDir, "cases", "review-one"), 0o700|os.ModeSticky); err != nil {
				t.Fatal(err)
			}
		},
		"transcripts symlink": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			target := config.ArtifactPaths.TranscriptsPath + ".target"
			if err := os.Rename(config.ArtifactPaths.TranscriptsPath, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, config.ArtifactPaths.TranscriptsPath); err != nil {
				t.Fatal(err)
			}
		},
		"snapshot setgid": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(config.ArtifactPaths.SnapshotPath, 0o600|os.ModeSetgid); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint directory regular file": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.RemoveAll(checkpointDir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(checkpointDir, []byte("not a directory"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint directory symlink": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			target := checkpointDir + "-target"
			if err := os.Rename(checkpointDir, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, checkpointDir); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint directory sticky": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(checkpointDir, 0o700|os.ModeSticky); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint file mode 0400": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(filepath.Join(checkpointDir, openRouterCheckpointFilename), 0o400); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint file directory": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			path := filepath.Join(checkpointDir, openRouterCheckpointFilename)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint file symlink": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			path := filepath.Join(checkpointDir, openRouterCheckpointFilename)
			target := path + ".target"
			if err := os.Rename(path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint file setuid": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Chmod(filepath.Join(checkpointDir, openRouterCheckpointFilename), 0o600|os.ModeSetuid); err != nil {
				t.Fatal(err)
			}
		},
		"active lock directory": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string, _ openRouterCheckpointIdentity) {
			t.Helper()
			if err := os.Mkdir(filepath.Join(checkpointDir, openRouterActiveRunLockFilename), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"active lock symlink": func(t *testing.T, config OpenRouterReviewInspectionConfig, checkpointDir string, identity openRouterCheckpointIdentity) {
			t.Helper()
			target := filepath.Join(checkpointDir, "lock.target")
			if err := os.WriteFile(target, validInspectionLock(t, config, identity), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(checkpointDir, openRouterActiveRunLockFilename)); err != nil {
				t.Fatal(err)
			}
		},
		"active lock setgid": func(t *testing.T, config OpenRouterReviewInspectionConfig, checkpointDir string, identity openRouterCheckpointIdentity) {
			t.Helper()
			path := filepath.Join(checkpointDir, openRouterActiveRunLockFilename)
			if err := os.WriteFile(path, validInspectionLock(t, config, identity), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o600|os.ModeSetgid); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			config, checkpointDir, identity := openRouterInspectionFixture(t)
			mutate(t, config, checkpointDir, identity)
			attestation, err := InspectOpenRouterReviewCheckpoint(config)
			if err == nil || attestation != (OpenRouterReviewAttestation{}) {
				t.Fatalf("attestation=%+v error=%v", attestation, err)
			}
		})
	}
}

func TestInspectOpenRouterReviewCheckpointDoesNotMutateInputDirectories(t *testing.T) {
	config, checkpointDir, _ := openRouterInspectionFixture(t)
	before := inspectionTreeFingerprint(t, config.ArtifactPaths.PackageDir, checkpointDir)
	if _, err := InspectOpenRouterReviewCheckpoint(config); err != nil {
		t.Fatal(err)
	}
	after := inspectionTreeFingerprint(t, config.ArtifactPaths.PackageDir, checkpointDir)
	if before != after {
		t.Fatalf("inspection mutated an input directory: before=%s after=%s", before, after)
	}
}

func TestInspectOpenRouterReviewCheckpointUsesOpenedArtifactsAfterPathReplacement(t *testing.T) {
	config, checkpointDir, _ := openRouterInspectionFixture(t)
	artifacts, err := OpenOpenRouterReviewInspectionArtifacts(OpenRouterReviewInspectionArtifactPaths{
		PackageDir: config.ArtifactPaths.PackageDir, CheckpointDir: checkpointDir,
		TranscriptsPath: config.ArtifactPaths.TranscriptsPath, SnapshotPath: config.ArtifactPaths.SnapshotPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = artifacts.Close() }()

	for _, path := range []string{config.ArtifactPaths.PackageDir, checkpointDir, config.ArtifactPaths.TranscriptsPath, config.ArtifactPaths.SnapshotPath} {
		original := path + ".opened"
		if err := os.Rename(path, original); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, "review-package") || path == config.ArtifactPaths.PackageDir || path == checkpointDir {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			writeInspectionExtra(t, filepath.Join(path, "replacement-only-extra"), 0o600)
			continue
		}
		if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config.OpenedArtifacts = artifacts
	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil || attestation.Status != OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval {
		t.Fatalf("attestation=%+v error=%v", attestation, err)
	}
}

func TestInspectOpenRouterReviewCheckpointReportsExactLockRecoveryBoundary(t *testing.T) {
	config, checkpointDir, identity := openRouterInspectionFixture(t)
	identityRaw, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	lock := openRouterActiveRunLockRecord{
		SchemaVersion: openRouterActiveRunLockSchemaVersion, CheckpointIdentitySHA256: hashBytes(identityRaw),
		StartedAt: time.Date(2026, 7, 1, 2, 0, 0, 0, time.UTC), ProcessID: 1234,
	}
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(checkpointDir, openRouterActiveRunLockFilename), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	if attestation.Status != "active_run_lock_present" || attestation.ProviderExecutionAuthorized || attestation.ActiveRunLockSHA256 != hashBytes(raw) {
		t.Fatalf("attestation = %+v", attestation)
	}
	again, err := InspectOpenRouterReviewCheckpoint(config)
	if err != nil || again.AttestationSHA256 != attestation.AttestationSHA256 {
		t.Fatalf("second inspection = %+v, %v", again, err)
	}
}

func TestInspectOpenRouterReviewCheckpointRequiresExactActiveLockMode(t *testing.T) {
	config, checkpointDir, identity := openRouterInspectionFixture(t)
	raw := validInspectionLock(t, config, identity)
	if err := os.WriteFile(filepath.Join(checkpointDir, openRouterActiveRunLockFilename), raw, 0o400); err != nil {
		t.Fatal(err)
	}

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err == nil || !strings.Contains(err.Error(), "mode 0600") || attestation != (OpenRouterReviewAttestation{}) {
		t.Fatalf("attestation=%+v error=%v", attestation, err)
	}
}

func TestInspectOpenRouterReviewCheckpointRejectsEmptyPresentActiveLock(t *testing.T) {
	config, checkpointDir, _ := openRouterInspectionFixture(t)
	if err := os.WriteFile(filepath.Join(checkpointDir, openRouterActiveRunLockFilename), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	attestation, err := InspectOpenRouterReviewCheckpoint(config)
	if err == nil || !strings.Contains(err.Error(), "active run lock") || attestation != (OpenRouterReviewAttestation{}) {
		t.Fatalf("attestation=%+v error=%v", attestation, err)
	}
}

func TestInspectOpenRouterReviewCheckpointRejectsUnreferencedPrivateTreeObjects(t *testing.T) {
	tests := map[string]func(*testing.T, OpenRouterReviewInspectionConfig, string){
		"package regular file": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			writeInspectionExtra(t, filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced.txt"), 0o600)
		},
		"package directory": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			if err := os.Mkdir(filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"package unsafe mode": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			writeInspectionExtra(t, filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced.txt"), 0o640)
		},
		"package symlink": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			if err := os.Symlink("manifest.json", filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced")); err != nil {
				t.Fatal(err)
			}
		},
		"package fifo": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			if err := syscall.Mkfifo(filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"package device": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			if err := syscall.Mknod(filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced"), syscall.S_IFCHR|0o600, 0x103); err != nil {
				if err == syscall.EPERM {
					t.Skip("creating a device node is not permitted in this test environment")
				}
				t.Fatal(err)
			}
		},
		"package special-bit directory": func(t *testing.T, config OpenRouterReviewInspectionConfig, _ string) {
			t.Helper()
			path := filepath.Join(config.ArtifactPaths.PackageDir, "unreferenced")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o700|os.ModeSticky); err != nil {
				t.Fatal(err)
			}
		},
		"checkpoint regular file": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
			t.Helper()
			writeInspectionExtra(t, filepath.Join(checkpointDir, "unreferenced.txt"), 0o600)
		},
		"checkpoint directory": func(t *testing.T, _ OpenRouterReviewInspectionConfig, checkpointDir string) {
			t.Helper()
			if err := os.Mkdir(filepath.Join(checkpointDir, "unreferenced"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			config, checkpointDir, _ := openRouterInspectionFixture(t)
			mutate(t, config, checkpointDir)
			attestation, err := InspectOpenRouterReviewCheckpoint(config)
			if err == nil || !strings.Contains(err.Error(), "unreferenced or unsafe object") || attestation != (OpenRouterReviewAttestation{}) {
				t.Fatalf("attestation=%+v error=%v", attestation, err)
			}
		})
	}
}

func writeInspectionExtra(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("unreferenced\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func validInspectionLock(t *testing.T, config OpenRouterReviewInspectionConfig, identity openRouterCheckpointIdentity) []byte {
	t.Helper()
	identityRaw, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	lock := openRouterActiveRunLockRecord{
		SchemaVersion: openRouterActiveRunLockSchemaVersion, CheckpointIdentitySHA256: hashBytes(identityRaw),
		StartedAt: time.Date(2026, 7, 1, 2, 0, 0, 0, time.UTC), ProcessID: 1234,
	}
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func openRouterInspectionFixture(t *testing.T) (OpenRouterReviewInspectionConfig, string, openRouterCheckpointIdentity) {
	t.Helper()
	packageDir, transcript := reviewerBPackageFixture(t)
	manifestPath := filepath.Join(packageDir, "manifest.json")
	manifest, err := readStrictJSON[Package](manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	transcripts := []fillerbakeoff.TranscriptArtifact{transcript}
	_, transcriptSetSHA256, err := indexReviewTranscripts(transcripts, manifest)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 1, 2, 0, 0, 0, time.UTC)
	snapshot := openRouterReviewSnapshot(fillerbakeoff.OpenRouterBaseURL, now)
	artifactDir := t.TempDir()
	transcriptsPath := filepath.Join(artifactDir, "transcripts.jsonl")
	transcriptRaw, err := json.Marshal(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcriptsPath, append(transcriptRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(artifactDir, "snapshot.json")
	snapshotRaw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, append(snapshotRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpointDir := filepath.Join(t.TempDir(), "private-review-state")
	if err := os.Mkdir(checkpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identity := openRouterCheckpointIdentity{
		SchemaVersion: openRouterCheckpointSchemaVersion, PackageManifestSHA256: manifestSHA256,
		CapabilitySnapshotSHA256: fillerbakeoff.OpenRouterSnapshotSHA256(snapshot), TranscriptSetSHA256: transcriptSetSHA256,
		BaseURL: fillerbakeoff.OpenRouterBaseURL, Model: "review/vendor-model", ResolvedModel: "review/vendor-model",
		UpstreamProvider: "Provider Route", UpstreamProviderSlug: "provider/route",
		PromptVersion: OpenRouterReviewPromptVersion, PromptSHA256: hashBytes([]byte(reviewerSystemPrompt)),
		ReviewerID: OpenRouterReviewerBID, BatchID: manifest.BatchID, ExpectedCases: OpenRouterReviewerBExpectedCases,
		MaxRequests: 301, MaxSpendNanoUSD: 4_000_000, MaxChargeNanoUSD: 2_000_000,
	}
	checkpoint := openRouterCheckpoint{
		Identity: identity,
		Attempts: []ReviewAttempt{{
			Alias: manifest.Cases[0].Alias, Attempt: 1, RequestedAt: now,
			RequestSHA256: strings.Repeat("1", 64), State: openRouterAttemptFailed,
			ChargedAmountUSD: "0.001", ChargedNanoUSD: 1_000_000,
		}},
	}
	if err := persistOpenRouterCheckpoint(checkpointDir, checkpoint); err != nil {
		t.Fatal(err)
	}
	config := OpenRouterReviewInspectionConfig{
		ArtifactPaths: OpenRouterReviewInspectionArtifactPaths{
			PackageDir: packageDir, CheckpointDir: checkpointDir, TranscriptsPath: transcriptsPath, SnapshotPath: snapshotPath,
		},
		Model: identity.Model, UpstreamProvider: identity.UpstreamProvider, UpstreamProviderSlug: identity.UpstreamProviderSlug,
		ReviewerID: identity.ReviewerID, ExpectedCases: identity.ExpectedCases, MaxRequests: identity.MaxRequests,
		MaxSpendNanoUSD: identity.MaxSpendNanoUSD, MaxChargeNanoUSD: identity.MaxChargeNanoUSD,
	}
	return config, checkpointDir, identity
}

func reviewerBPackageFixture(t *testing.T) (string, fillerbakeoff.TranscriptArtifact) {
	t.Helper()
	packageDir, transcript := reviewPackageFixture(t)
	manifestPath := filepath.Join(packageDir, "manifest.json")
	manifest, err := readStrictJSON[Package](manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(manifest.Cases); index < OpenRouterReviewerBExpectedCases; index++ {
		manifest.Cases = append(manifest.Cases, Case{
			Alias: fmt.Sprintf("review-%03d", index+1), ContentSHA256: strings.Repeat("d", 64),
			EvidenceSHA256: strings.Repeat("e", 64), SegmentDurationMS: 5_000,
			DecoderFacts: []DecoderFact{{Claim: "media_usability", Value: "unusable", Kind: "decoder"}},
		})
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := filepath.Walk(packageDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return os.Chmod(path, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	return packageDir, transcript
}

func mutateInspectionCheckpoint(t *testing.T, checkpointDir string, mutate func(*openRouterCheckpoint)) {
	t.Helper()
	path := filepath.Join(checkpointDir, openRouterCheckpointFilename)
	checkpoint, err := readStrictJSON[openRouterCheckpoint](path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&checkpoint)
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mutateInspectionManifest(t *testing.T, packageDir string, mutate func(*Package)) {
	t.Helper()
	path := filepath.Join(packageDir, "manifest.json")
	manifest, err := readStrictJSON[Package](path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o640); err != nil {
		t.Fatal(err)
	}
}

func inspectionTreeFingerprint(t *testing.T, roots ...string) string {
	t.Helper()
	hasher := sha256.New()
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(hasher, "%s\x00%s\x00", relative, info.Mode())
			if info.Mode().IsRegular() {
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				_, _ = hasher.Write(raw)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func writeInspectionSnapshot(t *testing.T, path string, snapshot fillerbakeoff.OpenRouterSnapshot) {
	t.Helper()
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readInspectionSnapshot(t *testing.T, path string) fillerbakeoff.OpenRouterSnapshot {
	t.Helper()
	snapshot, err := readStrictJSON[fillerbakeoff.OpenRouterSnapshot](path)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func readInspectionTranscriptFixture(t *testing.T, path string) []fillerbakeoff.TranscriptArtifact {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	transcripts, err := decodeInspectionTranscripts(raw)
	if err != nil {
		t.Fatal(err)
	}
	return transcripts
}

func writeInspectionTranscripts(t *testing.T, path string, transcripts []fillerbakeoff.TranscriptArtifact) {
	t.Helper()
	var raw []byte
	for _, transcript := range transcripts {
		line, err := json.Marshal(transcript)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
