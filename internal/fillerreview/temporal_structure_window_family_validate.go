package fillerreview

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

// ValidateTemporalStructureWindowFamilyResult verifies the self-contained, truth-blind family
// artifact. Certification assembly additionally binds it to the exact public manifest.
func ValidateTemporalStructureWindowFamilyResult(result TemporalStructureWindowFamilyResult) error {
	if result.SchemaVersion != TemporalStructureWindowFamilySchemaVersion ||
		result.ContractVersion != TemporalStructureWindowFamilyContractVersion ||
		!reviewSHA256(result.WindowSetManifestSHA256) || fillerstructure.ValidateAssessorProfile(result.Assessor) != nil ||
		result.CompletedAt.IsZero() || result.CompletedAt != result.CompletedAt.UTC() ||
		len(result.Cases) != TemporalStructureWindowCorpusCases || result.TrainingAllowed ||
		result.ProductionAdmissionAllowed || !reviewSHA256(result.SHA256) ||
		result.SHA256 != temporalStructureWindowFamilySHA256(result) {
		return errors.New("window family result identity, count, or disposition is invalid")
	}
	aliases := make(map[string]struct{}, len(result.Cases))
	mediaSets := make(map[string]struct{}, len(result.Cases))
	for index, item := range result.Cases {
		if len(item.Alias) != len("case-")+24 || !strings.HasPrefix(item.Alias, "case-") || !isLowerHex(item.Alias[len("case-"):]) {
			return fmt.Errorf("window family case %d has invalid alias", index)
		}
		if _, duplicate := aliases[item.Alias]; duplicate {
			return fmt.Errorf("window family result repeats alias %q", item.Alias)
		}
		if _, duplicate := mediaSets[item.Stitch.MediaSet.SHA256]; duplicate {
			return fmt.Errorf("window family result repeats media set %q", item.Stitch.MediaSet.SHA256)
		}
		if item.Stitch.Assessor != result.Assessor {
			return fmt.Errorf("window family case %d mixes assessor identity", index)
		}
		if err := fillerstructurewindow.ValidateStitchResult(item.Stitch); err != nil {
			return fmt.Errorf("window family case %d stitch: %w", index, err)
		}
		aliases[item.Alias] = struct{}{}
		mediaSets[item.Stitch.MediaSet.SHA256] = struct{}{}
	}
	return nil
}

func validateTemporalStructureWindowFamilyResultAgainstManifest(result TemporalStructureWindowFamilyResult, manifest TemporalStructureWindowSetManifest, manifestSHA string) error {
	if err := ValidateTemporalStructureWindowFamilyResult(result); err != nil {
		return err
	}
	if result.WindowSetManifestSHA256 != manifestSHA || len(result.Cases) != len(manifest.Cases) ||
		result.CompletedAt.Before(manifest.PreparedAt) {
		return errors.New("window family result does not bind the public window set")
	}
	for index, item := range result.Cases {
		public := manifest.Cases[index]
		if item.Alias != public.Alias || !reflect.DeepEqual(item.Stitch.MediaSet, public.MediaSet) {
			return fmt.Errorf("window family case %d drifted from public window set", index)
		}
	}
	return nil
}
