package fillerreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

type fakeTemporalStructureWindowFamily struct {
	profile fillerstructure.AssessorProfile
	calls   []string
	paths   [][]string
	failAt  int
}

func (f *fakeTemporalStructureWindowFamily) Profile() fillerstructure.AssessorProfile {
	return f.profile
}

func (f *fakeTemporalStructureWindowFamily) Assess(_ context.Context, prepared filler.StructureAssessmentWindowMediaSet) (fillerstructurewindow.StitchResult, error) {
	f.calls = append(f.calls, prepared.Source.SHA256)
	paths := make([]string, len(prepared.Windows))
	for index, window := range prepared.Windows {
		paths[index] = window.FullPath
	}
	f.paths = append(f.paths, paths)
	if f.failAt > 0 && len(f.calls) == f.failAt {
		return fillerstructurewindow.StitchResult{}, errors.New("fixture family failure")
	}
	assessments := make([]fillerstructurewindow.Assessment, 0, len(prepared.Windows))
	for _, window := range prepared.Windows {
		assessment, err := fillerstructurewindow.NewAssessment(fillerstructurewindow.AssessmentInput{
			MediaSet: prepared.Authority, WindowOrdinal: window.Window.Ordinal, Assessor: f.profile,
			Segments: []fillerstructure.Segment{{
				StartMS: window.Window.MediaStartMS, EndMS: window.Window.MediaEndMS, Role: fillerstructure.RoleCommercial,
			}},
			AssessedAt: time.Date(2026, time.September, 13, 1, 0, window.Window.Ordinal, 0, time.UTC),
		})
		if err != nil {
			return fillerstructurewindow.StitchResult{}, err
		}
		assessments = append(assessments, assessment)
	}
	return fillerstructurewindow.Stitch(prepared.Authority, assessments, 2_000)
}

func TestRunTemporalStructureWindowFamilyUsesOnlyCompletePublicMediaSets(t *testing.T) {
	suiteConfig, _ := temporalStructureWindowSuiteFixture(t, filepath.Join(t.TempDir(), "suite"))
	manifest := readStrictTestJSON[TemporalStructureWindowSetManifest](t, suiteConfig.WindowSetManifestPath)
	family := &fakeTemporalStructureWindowFamily{profile: temporalStructureWindowFamilyProfile("family-a")}
	completedAt := time.Date(2026, time.September, 13, 2, 0, 0, 0, time.UTC)
	result, err := RunTemporalStructureWindowFamily(t.Context(), TemporalStructureWindowFamilyConfig{
		WindowSetManifestPath: suiteConfig.WindowSetManifestPath,
		ExpectedCases:         TemporalStructureWindowCorpusCases,
		Family:                family,
		Now:                   func() time.Time { return completedAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTemporalStructureWindowFamilyResult(result); err != nil {
		t.Fatal(err)
	}
	if result.CompletedAt != completedAt || result.Assessor != family.profile ||
		len(result.Cases) != TemporalStructureWindowCorpusCases || len(family.calls) != len(result.Cases) ||
		result.ProductionAdmissionAllowed || result.TrainingAllowed || !reviewSHA256(result.SHA256) {
		t.Fatalf("result=%+v calls=%d", result, len(family.calls))
	}
	for index, item := range result.Cases {
		if item.Alias != manifest.Cases[index].Alias || item.Stitch.MediaSet.SHA256 != manifest.Cases[index].MediaSet.SHA256 ||
			item.Stitch.Assessor != family.profile || fillerstructurewindow.ValidateStitchResult(item.Stitch) != nil ||
			family.calls[index] != manifest.Cases[index].Source.SHA256 {
			t.Fatalf("case %d=%+v call=%s", index, item, family.calls[index])
		}
		for ordinal, path := range family.paths[index] {
			want := filepath.Join(filepath.Dir(suiteConfig.WindowSetManifestPath), filepath.FromSlash(manifest.Cases[index].Windows[ordinal].Path))
			if path != want {
				t.Fatalf("case %d window %d path=%q want=%q", index, ordinal, path, want)
			}
		}
	}
}

func TestTemporalStructureWindowFamilyArtifactRoundTripsAgainstManifest(t *testing.T) {
	suiteConfig, _ := temporalStructureWindowSuiteFixture(t, filepath.Join(t.TempDir(), "suite"))
	family := &fakeTemporalStructureWindowFamily{profile: temporalStructureWindowFamilyProfile("family-a")}
	result, err := RunTemporalStructureWindowFamily(t.Context(), TemporalStructureWindowFamilyConfig{
		WindowSetManifestPath: suiteConfig.WindowSetManifestPath,
		ExpectedCases:         TemporalStructureWindowCorpusCases,
		Family:                family,
		Now:                   func() time.Time { return time.Date(2026, time.September, 13, 2, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "family.json")
	digest, err := PublishTemporalStructureWindowFamilyResult(path, suiteConfig.WindowSetManifestPath, result)
	if err != nil {
		t.Fatal(err)
	}
	loaded, fileDigest, err := LoadTemporalStructureWindowFamilyResult(path, suiteConfig.WindowSetManifestPath)
	if err != nil || !reflect.DeepEqual(loaded, result) || digest != fileDigest || !reviewSHA256(digest) {
		t.Fatalf("loaded=%+v digest=%q fileDigest=%q error=%v", loaded, digest, fileDigest, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode=%v", info.Mode())
	}
	if _, err := PublishTemporalStructureWindowFamilyResult(path, suiteConfig.WindowSetManifestPath, result); err == nil {
		t.Fatal("expected immutable publication failure")
	}
}

func TestValidateTemporalStructureWindowFamilyResultRejectsDrift(t *testing.T) {
	suiteConfig, _ := temporalStructureWindowSuiteFixture(t, filepath.Join(t.TempDir(), "suite"))
	manifest, manifestSHA, err := LoadTemporalStructureWindowSetPublic(suiteConfig.WindowSetManifestPath, TemporalStructureWindowCorpusCases)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunTemporalStructureWindowFamily(t.Context(), TemporalStructureWindowFamilyConfig{
		WindowSetManifestPath: suiteConfig.WindowSetManifestPath,
		ExpectedCases:         TemporalStructureWindowCorpusCases,
		Family:                &fakeTemporalStructureWindowFamily{profile: temporalStructureWindowFamilyProfile("family-a")},
		Now:                   func() time.Time { return time.Date(2026, time.September, 13, 2, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*TemporalStructureWindowFamilyResult){
		"manifest": func(value *TemporalStructureWindowFamilyResult) {
			value.WindowSetManifestSHA256 = strings.Repeat("f", 64)
		},
		"alias": func(value *TemporalStructureWindowFamilyResult) { value.Cases[0].Alias = value.Cases[1].Alias },
		"assessor": func(value *TemporalStructureWindowFamilyResult) {
			value.Cases[0].Stitch.Assessor = temporalStructureWindowFamilyProfile("family-b")
		},
		"training": func(value *TemporalStructureWindowFamilyResult) { value.TrainingAllowed = true },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := result
			changed.Cases = append([]TemporalStructureWindowFamilyCase(nil), result.Cases...)
			mutate(&changed)
			changed.SHA256 = temporalStructureWindowFamilySHA256(changed)
			if err := validateTemporalStructureWindowFamilyResultAgainstManifest(changed, manifest, manifestSHA); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestRunTemporalStructureWindowFamilyReturnsNoPartialResult(t *testing.T) {
	suiteConfig, _ := temporalStructureWindowSuiteFixture(t, filepath.Join(t.TempDir(), "suite"))
	family := &fakeTemporalStructureWindowFamily{profile: temporalStructureWindowFamilyProfile("family-a"), failAt: 3}
	result, err := RunTemporalStructureWindowFamily(t.Context(), TemporalStructureWindowFamilyConfig{
		WindowSetManifestPath: suiteConfig.WindowSetManifestPath,
		ExpectedCases:         TemporalStructureWindowCorpusCases,
		Family:                family,
		Now:                   func() time.Time { return time.Date(2026, time.September, 13, 2, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "fixture family failure") ||
		!reflect.DeepEqual(result, TemporalStructureWindowFamilyResult{}) || len(family.calls) != 3 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, len(family.calls))
	}
}

func temporalStructureWindowFamilyProfile(id string) fillerstructure.AssessorProfile {
	return fillerstructure.AssessorProfile{
		ID: id, Provider: "openrouter", Model: "provider/model", ModelFamily: id,
		ModelDigest: strings.Repeat("a", 64), CapabilitySHA256: strings.Repeat("b", 64),
		PromptVersion: fillerstructurewindow.DirectVideoPromptVersion, EvidenceContract: fillerstructurewindow.CallRecordContractVersion,
	}
}
