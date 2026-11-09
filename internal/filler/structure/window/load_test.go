package window

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
)

func TestLoadAuthorityAcceptsOnlyReplayValidRegularFile(t *testing.T) {
	authority := loadAuthorityFixture()
	raw, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "authority.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAuthority(path)
	if err != nil || loaded == nil || loaded.SHA256 != authority.SHA256 {
		t.Fatalf("loaded=%+v error=%v", loaded, err)
	}
	if empty, err := LoadAuthority(""); err != nil || empty != nil {
		t.Fatalf("empty=%+v error=%v", empty, err)
	}
}

func TestLoadAuthorityRejectsUnknownTrailingAndSymlinkedInput(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(unknown); err == nil {
		t.Fatal("unknown authority fields were accepted")
	}
	authority := loadAuthorityFixture()
	raw, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	trailing := filepath.Join(root, "trailing.json")
	if err := os.WriteFile(trailing, append(raw, []byte("\n{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(trailing); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing error=%v", err)
	}
	link := filepath.Join(root, "authority-link.json")
	if err := os.Symlink(trailing, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(link); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink error=%v", err)
	}
}

func loadAuthorityFixture() MaterializationAuthority {
	profile := ProfileFor(cut.SoftwareProfile())
	authority := MaterializationAuthority{
		SchemaVersion:             MaterializationAuthoritySchemaVersion,
		ContractVersion:           MaterializationAuthorityContractVersion,
		WindowCertificationSHA256: strings.Repeat("a", 64), ShortLongShadowSHA256: strings.Repeat("b", 64),
		WindowProfileSHA256: profile.SHA256, AssessmentMediaProfileSHA256: profile.AssessmentMediaProfileSHA256,
		MinimumSourceDurationMS: 120_001, MaximumSourceDurationMS: 300_000,
		MaximumWindowBytes: 16 << 20, MaximumWindows: 3,
		ReducerVersion: structure.ReducerContractVersion, BoundaryToleranceMS: 2_000,
		AllowedUnits: []structure.Unit{structure.UnitCompilation},
		AllowedRoles: []structure.Role{structure.RoleCommercial, structure.RolePromo},
		ReviewerID:   "maintainer", ReviewedAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		AutomaticMaterializationAllowed: true,
	}
	for _, pair := range [][2]string{{"assessor-a", "family-a"}, {"assessor-b", "family-b"}} {
		authority.Assessors = append(authority.Assessors, structure.AssessorProfile{
			ID: pair[0], ModelFamily: pair[1], Provider: "openrouter", Model: "provider/model-" + pair[0],
			ModelDigest: strings.Repeat("c", 64), CapabilitySHA256: strings.Repeat("d", 64),
			PromptVersion:    DirectVideoPromptVersion,
			EvidenceContract: CallRecordContractVersion,
		})
	}
	authority.SHA256 = MaterializationAuthoritySHA256(authority)
	return authority
}
