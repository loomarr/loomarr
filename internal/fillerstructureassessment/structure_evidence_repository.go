package fillerstructureassessment

import (
	"fmt"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

const structureAssessmentResponseMaxBytes = fillerstructure.AssessmentMaximumResponseBytes

// FileStructureAssessmentEvidenceRepository is the private content-addressed store for window
// call records, stitches, decisions and response bytes. Blobs publish before their record, so a
// visible record is always loadable after a crash; unreferenced blobs are harmless and may be
// swept later.
type FileStructureAssessmentEvidenceRepository struct {
	root string
}

func NewFileStructureAssessmentEvidenceRepository(root string) (*FileStructureAssessmentEvidenceRepository, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("structure assessment evidence requires a clean absolute root")
	}
	return &FileStructureAssessmentEvidenceRepository{root: root}, nil
}

func (r *FileStructureAssessmentEvidenceRepository) blobPath(kind, digest string) string {
	return filepath.Join(r.root, kind, digest[:2], digest+".json")
}
