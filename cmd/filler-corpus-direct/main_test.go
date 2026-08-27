package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestRunFreezesLocalMediaAndEvidence(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "evidence/rights.txt", "license grant")
	writeFixture(t, root, "evidence/provenance.json", `{"source":"studio export"}`)
	source := validManifest(t, root)
	manifestPath := filepath.Join(root, "manifest.json")
	writeManifest(t, manifestPath, source)
	out := filepath.Join(root, "inventory.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--manifest", manifestPath, "--root", root, "--out", out, "--snapshot-at", "2026-08-26T12:00:00Z", "--expected-items", "6", "--max-bytes", "1048576", "--max-wall-time", "1m"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run = %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := fillercorpus.DecodeInventoryBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Cases) != 6 || len(inv.Captures) != 6 || inv.Captures[0].Transport != fillercorpus.TransportLocal || len(inv.Cases[0].Evidence) != 2 || len(inv.Cases[0].Representation.SHA256) != 64 {
		t.Fatalf("inventory = %+v", inv)
	}
	if inv.Cases[0].Authority != "direct-license/example-broadcaster" {
		t.Fatalf("authority = %q", inv.Cases[0].Authority)
	}
}

func TestFreezeRejectsQuotaMismatchAndMissingEvidenceKind(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "evidence/rights.txt", "license grant")
	writeFixture(t, root, "evidence/provenance.json", "provenance")
	opts := options{root: root, snapshotAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), expectedItems: 6, maxBytes: 1 << 20, maxWallTime: time.Minute}

	source := validManifest(t, root)
	source.RoleQuotas["commercial"] = 2
	opts.manifestPath = filepath.Join(root, "quota.json")
	writeManifest(t, opts.manifestPath, source)
	if _, err := freeze(opts); err == nil || !strings.Contains(err.Error(), "role quotas total 7") {
		t.Fatalf("quota error = %v", err)
	}

	source = validManifest(t, root)
	source.Cases[0].Evidence = source.Cases[0].Evidence[:1]
	opts.manifestPath = filepath.Join(root, "evidence.json")
	writeManifest(t, opts.manifestPath, source)
	if _, err := freeze(opts); err == nil || !strings.Contains(err.Error(), "local media or evidence") {
		t.Fatalf("evidence error = %v", err)
	}
}

func TestRunRejectsRetiredFixedCohortInterface(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--max-items", "100"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined: -max-items") {
		t.Fatalf("run = %d, stderr = %q", code, stderr.String())
	}
}

func TestValidateRoleQuotasRejectsUnknownAndNonPositiveRoles(t *testing.T) {
	for name, quotas := range map[string]map[string]int{
		"unknown":  {"advert": 1},
		"zero":     {"commercial": 0},
		"negative": {"commercial": -1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRoleQuotas(quotas, 1); err == nil {
				t.Fatal("validateRoleQuotas accepted an invalid acquisition plan")
			}
		})
	}
}

func TestFreezeRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "spot.mp4")); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := hashInside(root, "spot.mp4")
	if err == nil || !strings.Contains(err.Error(), "escapes declared root") {
		t.Fatalf("escape error = %v", err)
	}
}

func validManifest(t *testing.T, root string) manifest {
	t.Helper()
	value := manifest{SchemaVersion: directManifestSchema, Authority: "example-broadcaster", Cohort: "authentic-v1", RoleQuotas: map[string]int{"commercial": 1, "promo": 1, "bumper": 1, "station_id": 1, "trailer": 1, "psa": 1}}
	for role, quota := range value.RoleQuotas {
		for index := 1; index <= quota; index++ {
			id := fmt.Sprintf("%s-%03d", role, index)
			mediaPath := filepath.Join("media", id+".mp4")
			writeFixture(t, root, mediaPath, "media bytes "+id)
			value.Cases = append(value.Cases, manifestCase{ItemID: id, Title: id, RoleHints: []string{role}, MediaPath: filepath.ToSlash(mediaPath), MIMEType: "video/mp4", RightsAssertions: []string{"written redistribution grant"}, Creator: []string{"Example Broadcaster"}, Campaign: id, SourceFamily: "master-" + id, Evidence: []manifestEvidence{{Kind: "rights", Path: "evidence/rights.txt"}, {Kind: "provenance", Path: "evidence/provenance.json"}}})
		}
	}
	return value
}

func writeFixture(t *testing.T, root, relative, value string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, path string, value manifest) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
