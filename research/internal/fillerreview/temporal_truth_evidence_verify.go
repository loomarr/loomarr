package fillerreview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

// LoadTemporalTruthEvidence strictly decodes and re-hashes a public evidence
// set before a reviewer or model may consume it.
func LoadTemporalTruthEvidence(manifestPath string) (TemporalTruthEvidenceManifest, string, error) {
	manifest, err := readStrictJSON[TemporalTruthEvidenceManifest](manifestPath)
	if err != nil {
		return TemporalTruthEvidenceManifest{}, "", fmt.Errorf("read temporal truth evidence: %w", err)
	}
	digest, err := hashFile(manifestPath)
	if err != nil {
		return TemporalTruthEvidenceManifest{}, "", err
	}
	if err := validateTemporalTruthEvidence(filepath.Dir(manifestPath), manifest); err != nil {
		return TemporalTruthEvidenceManifest{}, "", err
	}
	return manifest, digest, nil
}

func validateTemporalTruthEvidence(root string, manifest TemporalTruthEvidenceManifest) error {
	if manifest.SchemaVersion != TemporalTruthEvidenceSchemaVersion || manifest.ContractVersion != TemporalTruthEvidenceContractVersion || manifest.EvidenceVersion != TemporalTruthEvidenceVersion || manifest.GeneratedAt.IsZero() || !reviewSHA256(manifest.SelectionSHA256) || len(manifest.Cases) != fillereval.TemporalTruthSelectionCases {
		return fmt.Errorf("temporal truth evidence identity or case count is invalid")
	}
	for name, identity := range map[string]TemporalTruthToolIdentity{"ffmpeg": manifest.MediaTools.FFmpeg, "ffprobe": manifest.MediaTools.FFprobe} {
		if strings.TrimSpace(identity.Path) == "" || strings.TrimSpace(identity.Version) == "" || !reviewSHA256(identity.BinarySHA256) {
			return fmt.Errorf("temporal truth evidence %s identity is invalid", name)
		}
	}
	if manifest.Config.SceneThreshold <= 0 || manifest.Config.SceneThreshold >= 1 || manifest.Config.MaximumFramesPerCase != TemporalEvidenceMaxFrames || manifest.Config.MaximumVideoBytes != TemporalTruthMaximumVideoBytes || manifest.Config.PerCaseTimeoutMS <= 0 {
		return fmt.Errorf("temporal truth evidence settings are invalid")
	}
	if manifest.OCR.Status == "available" {
		if manifest.OCR.Engine == nil || !reviewSHA256(manifest.OCR.Engine.BinarySHA256) || !reviewSHA256(manifest.OCR.Engine.SourceSHA256) || manifest.OCR.MinimumConfidence != 0.5 {
			return fmt.Errorf("temporal truth evidence OCR identity is invalid")
		}
	} else if manifest.OCR.Status != "unavailable" || manifest.OCR.Engine != nil || manifest.OCR.MinimumConfidence != 0 {
		return fmt.Errorf("temporal truth evidence OCR status is invalid")
	}
	aliases := make(map[string]struct{}, len(manifest.Cases))
	for _, item := range manifest.Cases {
		if strings.TrimSpace(item.Alias) == "" || item.DurationMS <= 0 || len(item.Frames) == 0 || len(item.Frames) > TemporalEvidenceMaxFrames || item.Plan.SchemaVersion != TemporalEvidencePlanSchemaVersion || item.Plan.SourceStartMS != 0 || item.Plan.SourceEndMS != item.DurationMS || len(item.Plan.FrameTimesMS) != len(item.Frames) {
			return fmt.Errorf("temporal truth evidence contains an invalid case")
		}
		if _, duplicate := aliases[item.Alias]; duplicate {
			return fmt.Errorf("temporal truth evidence repeats alias %q", item.Alias)
		}
		aliases[item.Alias] = struct{}{}
		if err := verifyTemporalTruthEvidenceFile(root, item.Video, TemporalTruthMaximumVideoBytes); err != nil || item.Video.DurationMS <= 0 || item.Video.DurationMS > item.DurationMS+1_000 || item.Video.Width <= 0 || item.Video.Height <= 0 {
			return fmt.Errorf("temporal truth evidence alias %q video is invalid: %w", item.Alias, err)
		}
		for index, frame := range item.Frames {
			if frame.ID != fmt.Sprintf("frame-%02d", index+1) || frame.AtMS != item.Plan.FrameTimesMS[index] || frame.AtMS < 0 || frame.AtMS >= item.DurationMS || frame.Width <= 0 || frame.Height <= 0 {
				return fmt.Errorf("temporal truth evidence alias %q frame %d is invalid", item.Alias, index+1)
			}
			if err := verifyTemporalTruthEvidenceFile(root, TemporalTruthEvidenceFile{Path: frame.Path, SHA256: frame.SHA256, Bytes: frame.Bytes}, 16<<20); err != nil {
				return fmt.Errorf("temporal truth evidence alias %q frame %d: %w", item.Alias, index+1, err)
			}
			for _, observation := range frame.OCR {
				if err := validateTemporalTruthOCRObservation(observation); err != nil {
					return fmt.Errorf("temporal truth evidence alias %q frame %d: %w", item.Alias, index+1, err)
				}
			}
		}
		if _, err := temporalTruthTranscriptSegments(item.TranscriptSegments, item.DurationMS); err != nil {
			return fmt.Errorf("temporal truth evidence alias %q: %w", item.Alias, err)
		}
	}
	return nil
}

func verifyTemporalTruthEvidenceFile(root string, artifact TemporalTruthEvidenceFile, maximumBytes int64) error {
	if !reviewSHA256(artifact.SHA256) || artifact.Bytes <= 0 || artifact.Bytes > maximumBytes {
		return fmt.Errorf("artifact identity or byte bound is invalid")
	}
	path, err := resolveWithin(root, artifact.Path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != artifact.Bytes {
		return fmt.Errorf("artifact byte binding failed")
	}
	digest, err := hashFile(path)
	if err != nil || digest != artifact.SHA256 {
		return fmt.Errorf("artifact content binding failed")
	}
	return nil
}
