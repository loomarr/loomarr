package fillerreview

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

const (
	TemporalStructureWindowFamilySchemaVersion   = 1
	TemporalStructureWindowFamilyContractVersion = "filler-temporal-structure-window-family-result-v1"
)

// TemporalStructureWindowFamily is the deliberately small certification seam for one assessor
// family. Implementations own paid calls, replay, evidence persistence, and source-level stitching.
type TemporalStructureWindowFamily interface {
	Profile() fillerstructure.AssessorProfile
	Assess(context.Context, filler.StructureAssessmentWindowMediaSet) (fillerstructurewindow.StitchResult, error)
}

type TemporalStructureWindowFamilyConfig struct {
	WindowSetManifestPath string
	ExpectedCases         int
	Family                TemporalStructureWindowFamily
	Now                   func() time.Time
}

// TemporalStructureWindowFamilyResult contains blinded answers from exactly one model family.
// Alias is the only case join key; construction truth never enters the family runner.
type TemporalStructureWindowFamilyResult struct {
	SchemaVersion              int                                 `json:"schemaVersion"`
	ContractVersion            string                              `json:"contractVersion"`
	WindowSetManifestSHA256    string                              `json:"windowSetManifestSha256"`
	Assessor                   fillerstructure.AssessorProfile     `json:"assessor"`
	CompletedAt                time.Time                           `json:"completedAt"`
	Cases                      []TemporalStructureWindowFamilyCase `json:"cases"`
	TrainingAllowed            bool                                `json:"trainingAllowed"`
	ProductionAdmissionAllowed bool                                `json:"productionAdmissionAllowed"`
	SHA256                     string                              `json:"sha256"`
}

type TemporalStructureWindowFamilyCase struct {
	Alias  string                             `json:"alias"`
	Stitch fillerstructurewindow.StitchResult `json:"stitch"`
}

// RunTemporalStructureWindowFamily evaluates every case in manifest order. It returns no partial
// result: durable per-window recovery belongs to the supplied family implementation.
func RunTemporalStructureWindowFamily(ctx context.Context, config TemporalStructureWindowFamilyConfig) (TemporalStructureWindowFamilyResult, error) {
	if config.ExpectedCases != TemporalStructureWindowCorpusCases || config.Family == nil || config.Now == nil {
		return TemporalStructureWindowFamilyResult{}, errors.New("window family run requires the complete certification corpus, family, and clock")
	}
	profile := config.Family.Profile()
	if err := fillerstructure.ValidateAssessorProfile(profile); err != nil {
		return TemporalStructureWindowFamilyResult{}, fmt.Errorf("window family profile: %w", err)
	}
	manifest, manifestSHA, err := LoadTemporalStructureWindowSetPublic(config.WindowSetManifestPath, config.ExpectedCases)
	if err != nil {
		return TemporalStructureWindowFamilyResult{}, err
	}
	root := filepath.Dir(config.WindowSetManifestPath)
	result := TemporalStructureWindowFamilyResult{
		SchemaVersion: TemporalStructureWindowFamilySchemaVersion, ContractVersion: TemporalStructureWindowFamilyContractVersion,
		WindowSetManifestSHA256: manifestSHA, Assessor: profile,
		Cases: make([]TemporalStructureWindowFamilyCase, 0, len(manifest.Cases)),
	}
	for index, item := range manifest.Cases {
		if err := ctx.Err(); err != nil {
			return TemporalStructureWindowFamilyResult{}, err
		}
		prepared := temporalStructureWindowFamilyMedia(root, item)
		stitched, err := config.Family.Assess(ctx, prepared)
		if err != nil {
			return TemporalStructureWindowFamilyResult{}, fmt.Errorf("assess window family case %d (%s): %w", index, item.Alias, err)
		}
		result.Cases = append(result.Cases, TemporalStructureWindowFamilyCase{Alias: item.Alias, Stitch: stitched})
	}
	result.CompletedAt = config.Now().UTC()
	result.SHA256 = temporalStructureWindowFamilySHA256(result)
	if err := validateTemporalStructureWindowFamilyResultAgainstManifest(result, manifest, manifestSHA); err != nil {
		return TemporalStructureWindowFamilyResult{}, err
	}
	return result, nil
}

func temporalStructureWindowFamilyMedia(root string, item TemporalStructureWindowSetPublicCase) filler.StructureAssessmentWindowMediaSet {
	prepared := filler.StructureAssessmentWindowMediaSet{
		Source: item.Source, Authority: item.MediaSet, Windows: make([]filler.StructureAssessmentWindowMedia, 0, len(item.Windows)),
	}
	for ordinal, entry := range item.Windows {
		prepared.Windows = append(prepared.Windows, filler.StructureAssessmentWindowMedia{
			Window: item.MediaSet.Plan.Windows[ordinal], Media: item.MediaSet.Windows[ordinal],
			FullPath: filepath.Join(root, filepath.FromSlash(entry.Path)),
		})
	}
	return prepared
}

func temporalStructureWindowFamilySHA256(result TemporalStructureWindowFamilyResult) string {
	result.SHA256 = ""
	return temporalTruthJSONSHA(result)
}
