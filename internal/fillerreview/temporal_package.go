package fillerreview

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const TemporalReviewPackageSchemaVersion = 2

// TemporalReviewPackage is the public, identity-blind diagnostic package. The
// source map and previous labels are deliberately absent from this contract.
type TemporalReviewPackage struct {
	SchemaVersion          int                  `json:"schemaVersion"`
	BatchID                string               `json:"batchId"`
	GeneratedAt            time.Time            `json:"generatedAt"`
	EvidenceVersion        string               `json:"evidenceVersion"`
	TemporalManifestSHA256 string               `json:"temporalManifestSha256"`
	OCRManifestSHA256      string               `json:"ocrManifestSha256"`
	OCRPacketsSHA256       string               `json:"ocrPacketsSha256"`
	SelectionSHA256        string               `json:"selectionSha256"`
	InstructionsPath       string               `json:"instructionsPath"`
	InstructionsSHA256     string               `json:"instructionsSha256"`
	TemplatePath           string               `json:"templatePath"`
	TemplateSHA256         string               `json:"templateSha256"`
	Cases                  []TemporalReviewCase `json:"cases"`
}

type TemporalReviewCase struct {
	Alias              string                     `json:"alias"`
	DurationMS         int64                      `json:"durationMs"`
	Frames             []TemporalReviewFrame      `json:"frames"`
	TranscriptSegments []TemporalReviewTranscript `json:"transcriptSegments"`
}

type TemporalReviewFrame struct {
	ID                   string                         `json:"id"`
	Path                 string                         `json:"path"`
	SHA256               string                         `json:"sha256"`
	Bytes                int64                          `json:"bytes"`
	Width                int                            `json:"width"`
	Height               int                            `json:"height"`
	AtMS                 int64                          `json:"atMs"`
	OCRSignalID          string                         `json:"ocrSignalId"`
	OCR                  []TemporalReviewOCRObservation `json:"ocr"`
	CardCandidate        bool                           `json:"cardCandidate"`
	CardCandidateReasons []string                       `json:"cardCandidateReasons"`
}

type TemporalReviewOCRObservation struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
}

type TemporalReviewTranscript struct {
	ID      string `json:"id"`
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// LoadTemporalReviewPackage strictly decodes and content-verifies a package,
// then exposes the only signal identifiers assessments may cite.
func LoadTemporalReviewPackage(packagePath string, expectedCases int) (TemporalReviewPackage, []fillereval.TemporalCaseSignals, string, error) {
	if strings.TrimSpace(packagePath) == "" || expectedCases <= 0 {
		return TemporalReviewPackage{}, nil, "", fmt.Errorf("package path and positive expected case count are required")
	}
	pack, err := readStrictJSON[TemporalReviewPackage](packagePath)
	if err != nil {
		return TemporalReviewPackage{}, nil, "", fmt.Errorf("read temporal review package: %w", err)
	}
	packageSHA256, err := hashFile(packagePath)
	if err != nil {
		return TemporalReviewPackage{}, nil, "", fmt.Errorf("hash temporal review package: %w", err)
	}
	if err := validateTemporalReviewPackage(filepath.Dir(packagePath), pack, expectedCases); err != nil {
		return TemporalReviewPackage{}, nil, "", err
	}
	cases := make([]fillereval.TemporalCaseSignals, 0, len(pack.Cases))
	for _, item := range pack.Cases {
		caseSignals := fillereval.TemporalCaseSignals{Alias: item.Alias, DurationMS: item.DurationMS}
		for _, frame := range item.Frames {
			caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: frame.ID, Kind: "frame", AtMS: frame.AtMS})
			if frame.OCRSignalID != "" {
				caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: frame.OCRSignalID, Kind: "ocr", AtMS: frame.AtMS})
			}
		}
		for _, segment := range item.TranscriptSegments {
			caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: segment.ID, Kind: "transcript", AtMS: segment.StartMS})
		}
		cases = append(cases, caseSignals)
	}
	return pack, cases, packageSHA256, nil
}

func validateTemporalReviewPackage(root string, pack TemporalReviewPackage, expectedCases int) error {
	if pack.SchemaVersion != TemporalReviewPackageSchemaVersion || strings.TrimSpace(pack.BatchID) == "" || pack.GeneratedAt.IsZero() || strings.TrimSpace(pack.EvidenceVersion) == "" {
		return fmt.Errorf("temporal review package identity is invalid")
	}
	for name, digest := range map[string]string{
		"temporal manifest": pack.TemporalManifestSHA256,
		"OCR manifest":      pack.OCRManifestSHA256,
		"OCR packets":       pack.OCRPacketsSHA256,
		"selection":         pack.SelectionSHA256,
	} {
		if !reviewSHA256(digest) {
			return fmt.Errorf("temporal review package %s digest is invalid", name)
		}
	}
	if len(pack.Cases) != expectedCases {
		return fmt.Errorf("temporal review package has %d cases; want exactly %d", len(pack.Cases), expectedCases)
	}
	for _, public := range []struct {
		name   string
		path   string
		digest string
	}{
		{name: "instructions", path: pack.InstructionsPath, digest: pack.InstructionsSHA256},
		{name: "template", path: pack.TemplatePath, digest: pack.TemplateSHA256},
	} {
		if !reviewSHA256(public.digest) {
			return fmt.Errorf("temporal review package %s digest is invalid", public.name)
		}
		path, err := resolveWithin(root, public.path)
		if err != nil {
			return fmt.Errorf("resolve temporal review %s: %w", public.name, err)
		}
		digest, err := hashFile(path)
		if err != nil || digest != public.digest {
			return fmt.Errorf("temporal review %s fails its content binding", public.name)
		}
	}

	aliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		if strings.TrimSpace(item.Alias) == "" || item.DurationMS <= 0 || len(item.Frames) == 0 {
			return fmt.Errorf("temporal review package contains an invalid case")
		}
		if _, duplicate := aliases[item.Alias]; duplicate {
			return fmt.Errorf("temporal review package repeats alias %q", item.Alias)
		}
		aliases[item.Alias] = struct{}{}
		if err := validateTemporalReviewCase(root, item); err != nil {
			return fmt.Errorf("temporal review alias %q: %w", item.Alias, err)
		}
	}
	return nil
}

func validateTemporalReviewCase(root string, item TemporalReviewCase) error {
	signals := make(map[string]struct{}, len(item.Frames)*2+len(item.TranscriptSegments))
	previousFrameMS := int64(-1)
	for _, frame := range item.Frames {
		if strings.TrimSpace(frame.ID) == "" || frame.ID == frame.OCRSignalID || !reviewSHA256(frame.SHA256) || frame.Bytes <= 0 || frame.Width <= 0 || frame.Height <= 0 || frame.AtMS < previousFrameMS || frame.AtMS < 0 || frame.AtMS > item.DurationMS {
			return fmt.Errorf("frame %q has invalid identity, dimensions, or timestamp", frame.ID)
		}
		previousFrameMS = frame.AtMS
		ids := []string{frame.ID}
		if frame.OCRSignalID != "" {
			ids = append(ids, frame.OCRSignalID)
		}
		for _, id := range ids {
			if _, duplicate := signals[id]; duplicate {
				return fmt.Errorf("duplicate signal id %q", id)
			}
			signals[id] = struct{}{}
		}
		path, err := resolveWithin(root, frame.Path)
		if err != nil {
			return fmt.Errorf("resolve frame %q: %w", frame.ID, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() != frame.Bytes {
			return fmt.Errorf("frame %q fails its byte binding", frame.ID)
		}
		digest, err := hashFile(path)
		if err != nil || digest != frame.SHA256 {
			return fmt.Errorf("frame %q fails its content binding", frame.ID)
		}
		for _, observation := range frame.OCR {
			if strings.TrimSpace(observation.Text) == "" || !boundedUnitFloat(observation.Confidence) || !boundedOCRCoordinate(observation.X) || !boundedOCRCoordinate(observation.Y) || !boundedPositiveOCRSize(observation.Width) || !boundedPositiveOCRSize(observation.Height) || observation.X+observation.Width > 1.05 || observation.Y+observation.Height > 1.05 {
				return fmt.Errorf("frame %q contains an invalid OCR observation", frame.ID)
			}
		}
	}
	previousTranscriptEnd := int64(0)
	for _, segment := range item.TranscriptSegments {
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" || segment.StartMS < previousTranscriptEnd || segment.StartMS < 0 || segment.EndMS <= segment.StartMS || segment.EndMS > item.DurationMS {
			return fmt.Errorf("transcript %q has invalid text or timestamps", segment.ID)
		}
		if _, duplicate := signals[segment.ID]; duplicate {
			return fmt.Errorf("duplicate signal id %q", segment.ID)
		}
		signals[segment.ID] = struct{}{}
		previousTranscriptEnd = segment.EndMS
	}
	return nil
}

func boundedUnitFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func boundedOCRCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -0.05 && value <= 1.05
}

func boundedPositiveOCRSize(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= 1.05
}
