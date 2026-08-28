// Package fillerreview materializes identity-blind evidence for independent
// semantic review. It never calls a reviewer or reads gold labels.
package fillerreview

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	SchemaVersion = 1
	maxJSONLLine  = 64 << 20
)

type MaterializationMode string

const (
	MaterializeHardlink MaterializationMode = "hardlink"
	MaterializeCopy     MaterializationMode = "copy"
)

// Config is the complete interface to the packaging module. OutputDir must not
// exist; Build publishes it atomically only after every input and output hash
// has been verified.
type Config struct {
	DraftPath           string
	ReviewPacketPath    string
	AliasMapPath        string
	EvidencePacketsPath string
	CorpusRoot          string
	OutputDir           string
	Mode                MaterializationMode
}

type Result struct {
	Cases               int
	Files               int
	Bytes               int64
	ManifestSHA256      string
	LabelTemplateSHA256 string
}

type Package struct {
	SchemaVersion       int    `json:"schemaVersion"`
	BatchID             string `json:"batchId"`
	DraftSHA256         string `json:"draftSha256"`
	ReviewPacketSHA256  string `json:"reviewPacketSha256"`
	EvidenceVersion     string `json:"evidenceVersion"`
	InstructionsPath    string `json:"instructionsPath"`
	InstructionsSHA256  string `json:"instructionsSha256"`
	LabelTemplatePath   string `json:"labelTemplatePath"`
	LabelTemplateSHA256 string `json:"labelTemplateSha256"`
	Cases               []Case `json:"cases"`
}

type Case struct {
	Alias             string        `json:"alias"`
	ContentSHA256     string        `json:"contentSha256"`
	EvidenceSHA256    string        `json:"evidenceSha256"`
	SegmentStartMS    int64         `json:"segmentStartMs,omitempty"`
	SegmentDurationMS int64         `json:"segmentDurationMs"`
	DecoderFacts      []DecoderFact `json:"decoderFacts,omitempty"`
	Signals           []Signal      `json:"signals,omitempty"`
}

type DecoderFact struct {
	Claim    string `json:"claim"`
	Value    string `json:"value"`
	Kind     string `json:"kind"`
	Location string `json:"location,omitempty"`
}

type Signal struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Path         string   `json:"path"`
	SHA256       string   `json:"sha256"`
	Bytes        int64    `json:"bytes"`
	DurationMS   int64    `json:"durationMs,omitempty"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	AtMS         int64    `json:"atMs,omitempty"`
	ContentTypes []string `json:"contentTypes"`
}

const instructions = `# Blind filler semantic review

Review only the evidence in this package. Do not attempt to recover a source identity or use an
external title, filename, catalog entry, or prior label. Fill every line of labels.template.jsonl,
preserving alias and batchId. Use one reviewerId for the entire file and the actual UTC reviewedAt
for each completed review.

truth is eligible, invalid, or ambiguous. invalid requires rejectClass deterministic or semantic;
other truth values must omit rejectClass. Eligible and semantic-invalid cases require one contentRole;
deterministic-invalid and ambiguous cases may leave it empty only when the supplied evidence cannot
establish a role. A non-empty contentRole is one of commercial,
promo, bumper, psa, station_id, trailer, interstitial, programme_excerpt, or compilation. Use
ambiguous only when the supplied evidence supports a concrete answerable reviewQuestion.

commercial advertises a product, brand, or paid proposition. promo promotes a programme, channel,
or scheduled event. bumper is a short transition into or out of a programme or break. psa is a
public-service announcement. station_id identifies a station or network. trailer previews a film or
programme. interstitial is standalone connective filler not covered above. programme_excerpt is
ordinary programme material rather than a standalone filler unit. compilation contains multiple
distinct clips whose boundary cannot be represented as one role.

Include at least one slice and one evidence entry. Evidence must name an id, kind, claim, value, and
package-relative provenance; use atMs when the observation is temporal. Taxonomy and policy flags
must be literal observations from the supplied media, never guesses. The empty template is
intentionally invalid and cannot be locked as a completed review.
`

// Build verifies all private joins and external derivatives, then publishes a
// package containing only reviewer-visible aliases and media.
func Build(config Config) (Result, error) {
	if err := validateConfig(config); err != nil {
		return Result{}, err
	}
	draft, err := readStrictJSON[fillereval.Manifest](config.DraftPath)
	if err != nil {
		return Result{}, fmt.Errorf("read draft: %w", err)
	}
	review, err := readStrictJSON[fillereval.BlindReviewPacket](config.ReviewPacketPath)
	if err != nil {
		return Result{}, fmt.Errorf("read review packet: %w", err)
	}
	mapping, err := readStrictJSON[fillereval.BlindReviewMap](config.AliasMapPath)
	if err != nil {
		return Result{}, fmt.Errorf("read alias map: %w", err)
	}
	packets, err := readPackets(config.EvidencePacketsPath)
	if err != nil {
		return Result{}, fmt.Errorf("read evidence packets: %w", err)
	}

	draftDigest := fillereval.ManifestSHA256(draft)
	if failures := fillereval.ValidateReviewDraft(draft); len(failures) > 0 {
		return Result{}, fmt.Errorf("invalid review draft: %s", strings.Join(failures, "; "))
	}
	if review.SchemaVersion != fillereval.BlindReviewSchemaVersion || mapping.SchemaVersion != fillereval.BlindReviewSchemaVersion ||
		review.BatchID == "" || review.BatchID != mapping.BatchID || review.DraftSHA256 != draftDigest || mapping.DraftSHA256 != draftDigest {
		return Result{}, fmt.Errorf("review packet and private map must bind the exact draft and batch")
	}
	draftCases := make(map[string]fillereval.Case, len(draft.Cases))
	for _, item := range draft.Cases {
		if _, duplicate := draftCases[item.ID]; duplicate {
			return Result{}, fmt.Errorf("duplicate draft case %q", item.ID)
		}
		draftCases[item.ID] = item
	}
	aliasToCase, err := indexAliases(mapping, review, draftCases)
	if err != nil {
		return Result{}, err
	}
	if len(packets) != len(draftCases) {
		return Result{}, fmt.Errorf("evidence packet set has %d cases; draft requires %d", len(packets), len(draftCases))
	}
	evidenceVersion := ""
	for id, item := range draftCases {
		packet, ok := packets[id]
		if !ok {
			return Result{}, fmt.Errorf("draft case %q has no evidence packet", id)
		}
		if evidenceVersion == "" {
			evidenceVersion = packet.EvidenceVersion
		}
		if packet.EvidenceVersion == "" || packet.EvidenceVersion != evidenceVersion {
			return Result{}, fmt.Errorf("evidence packets must share one non-empty evidence version")
		}
		if err := fillerbakeoff.ValidatePacketAgainstCase(item, packet, evidenceVersion, config.CorpusRoot); err != nil {
			return Result{}, err
		}
	}

	output, err := filepath.Abs(config.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output: %w", err)
	}
	if _, err := os.Lstat(output); err == nil || !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("output already exists: %s", output)
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return Result{}, fmt.Errorf("create output parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, ".filler-review-package-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary package: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.Chmod(temporary, 0o750); err != nil {
		return Result{}, err
	}

	instructionsPath := "INSTRUCTIONS.md"
	instructionsBytes := []byte(instructions)
	if err := os.WriteFile(filepath.Join(temporary, instructionsPath), instructionsBytes, 0o640); err != nil {
		return Result{}, fmt.Errorf("write instructions: %w", err)
	}
	templatePath := "labels.template.jsonl"
	templateBytes, err := labelTemplate(review)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(temporary, templatePath), templateBytes, 0o640); err != nil {
		return Result{}, fmt.Errorf("write label template: %w", err)
	}

	manifest := Package{
		SchemaVersion: SchemaVersion, BatchID: review.BatchID, DraftSHA256: draftDigest,
		ReviewPacketSHA256: hashJSON(review), EvidenceVersion: evidenceVersion, InstructionsPath: instructionsPath,
		InstructionsSHA256: hashBytes(instructionsBytes), LabelTemplatePath: templatePath,
		LabelTemplateSHA256: hashBytes(templateBytes),
	}
	result := Result{Cases: len(review.Cases), LabelTemplateSHA256: manifest.LabelTemplateSHA256}
	for _, reviewCase := range review.Cases {
		caseID := aliasToCase[reviewCase.Alias]
		packet := packets[caseID]
		packaged, files, materializedBytes, err := packageCase(temporary, config.CorpusRoot, config.Mode, reviewCase, packet)
		if err != nil {
			return Result{}, fmt.Errorf("package alias %q: %w", reviewCase.Alias, err)
		}
		manifest.Cases = append(manifest.Cases, packaged)
		result.Files += files
		result.Bytes += materializedBytes
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal package manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "manifest.json"), manifestBytes, 0o640); err != nil {
		return Result{}, fmt.Errorf("write package manifest: %w", err)
	}
	result.ManifestSHA256 = hashBytes(manifestBytes)
	if err := os.Rename(temporary, output); err != nil {
		return Result{}, fmt.Errorf("publish package: %w", err)
	}
	published = true
	return result, nil
}

func validateConfig(config Config) error {
	paths := []struct{ name, value string }{
		{"draft", config.DraftPath}, {"review packet", config.ReviewPacketPath}, {"alias map", config.AliasMapPath},
		{"evidence packets", config.EvidencePacketsPath}, {"corpus root", config.CorpusRoot}, {"output", config.OutputDir},
	}
	for _, path := range paths {
		if strings.TrimSpace(path.value) == "" {
			return fmt.Errorf("%s path is required", path.name)
		}
	}
	if config.Mode != MaterializeHardlink && config.Mode != MaterializeCopy {
		return fmt.Errorf("materialization mode must be hardlink or copy")
	}
	return nil
}

func indexAliases(mapping fillereval.BlindReviewMap, review fillereval.BlindReviewPacket, draft map[string]fillereval.Case) (map[string]string, error) {
	aliases := make(map[string]string, len(mapping.Entries))
	mappedCases := make(map[string]struct{}, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		if !validAlias(entry.Alias) {
			return nil, fmt.Errorf("invalid opaque alias %q", entry.Alias)
		}
		if _, duplicate := aliases[entry.Alias]; duplicate {
			return nil, fmt.Errorf("duplicate alias %q", entry.Alias)
		}
		if _, duplicate := mappedCases[entry.CaseID]; duplicate {
			return nil, fmt.Errorf("duplicate mapped case %q", entry.CaseID)
		}
		if _, ok := draft[entry.CaseID]; !ok {
			return nil, fmt.Errorf("mapped case %q is absent from draft", entry.CaseID)
		}
		aliases[entry.Alias] = entry.CaseID
		mappedCases[entry.CaseID] = struct{}{}
	}
	if len(aliases) != len(draft) || len(review.Cases) != len(draft) {
		return nil, fmt.Errorf("review packet and alias map must cover all %d draft cases", len(draft))
	}
	seen := make(map[string]struct{}, len(review.Cases))
	for _, item := range review.Cases {
		caseID, ok := aliases[item.Alias]
		if !ok {
			return nil, fmt.Errorf("review alias %q is absent from private map", item.Alias)
		}
		if _, duplicate := seen[item.Alias]; duplicate {
			return nil, fmt.Errorf("duplicate review alias %q", item.Alias)
		}
		seen[item.Alias] = struct{}{}
		c := draft[caseID]
		if item.ContentSHA256 != c.ContentSHA256 || item.EvidenceSHA256 != c.EvidenceSHA256 || item.SegmentStartMS != c.Provenance.SegmentStartMS || item.SegmentDurationMS != c.Provenance.SegmentDurationMS {
			return nil, fmt.Errorf("review alias %q does not bind its mapped draft case", item.Alias)
		}
	}
	return aliases, nil
}

func validAlias(alias string) bool {
	if len(alias) != len("review-")+32 || !strings.HasPrefix(alias, "review-") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(alias, "review-"))
	return err == nil
}

func packageCase(root, corpusRoot string, mode MaterializationMode, reviewCase fillereval.BlindReviewCase, packet fillerbakeoff.Packet) (Case, int, int64, error) {
	packaged := Case{
		Alias: reviewCase.Alias, ContentSHA256: reviewCase.ContentSHA256, EvidenceSHA256: reviewCase.EvidenceSHA256,
		SegmentStartMS: reviewCase.SegmentStartMS, SegmentDurationMS: reviewCase.SegmentDurationMS,
	}
	for _, fact := range packet.Facts {
		if fact.Claim == filleradmission.ClaimMediaUsability && fact.Kind == filleradmission.KindDecoder {
			if fact.Value != filleradmission.UsabilityUsable && fact.Value != filleradmission.UsabilityUnusable {
				return Case{}, 0, 0, fmt.Errorf("decoder usability fact has unsupported value")
			}
			packaged.DecoderFacts = append(packaged.DecoderFacts, DecoderFact{Claim: string(fact.Claim), Value: fact.Value, Kind: string(fact.Kind), Location: sanitizeDecoderLocation(fact.Location)})
		}
	}
	caseDir := filepath.Join(root, "cases", reviewCase.Alias)
	if err := os.MkdirAll(caseDir, 0o750); err != nil {
		return Case{}, 0, 0, err
	}
	counts := map[string]int{}
	var files int
	var total int64
	for _, source := range packet.Signals {
		if source.Kind != string(filleradmission.KindFrame) && source.Kind != string(filleradmission.KindAudio) && source.Kind != string(filleradmission.KindVideo) {
			continue
		}
		extension, err := signalExtension(source)
		if err != nil {
			return Case{}, 0, 0, err
		}
		counts[source.Kind]++
		name := fmt.Sprintf("%s-%02d%s", source.Kind, counts[source.Kind], extension)
		publicID := strings.TrimSuffix(name, extension)
		relative := filepath.ToSlash(filepath.Join("cases", reviewCase.Alias, name))
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := materialize(corpusRoot, source, target, mode); err != nil {
			return Case{}, 0, 0, err
		}
		packaged.Signals = append(packaged.Signals, Signal{
			ID: publicID, Kind: source.Kind, Path: relative, SHA256: source.SHA256, Bytes: source.Bytes,
			DurationMS: source.DurationMS, Width: source.Width, Height: source.Height, AtMS: source.AtMS,
			ContentTypes: slices.Clone(source.ContentTypes),
		})
		files++
		total += source.Bytes
	}
	return packaged, files, total, nil
}

func sanitizeDecoderLocation(location string) string {
	allowed := map[string]bool{
		"source_duration_ms": true, "segment_start_ms": true, "segment_duration_ms": true,
		"no_video": true, "no_audio": true, "black_percent": true, "silence_percent": true,
	}
	var sanitized []string
	for _, field := range strings.Split(location, ";") {
		key, value, ok := strings.Cut(field, "=")
		if !ok || !allowed[key] {
			continue
		}
		if key == "no_video" || key == "no_audio" {
			if _, err := strconv.ParseBool(value); err != nil {
				continue
			}
		} else if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			continue
		}
		sanitized = append(sanitized, key+"="+value)
	}
	return strings.Join(sanitized, ";")
}

func signalExtension(signal fillerbakeoff.Signal) (string, error) {
	expected := ""
	extension := ""
	switch signal.Kind {
	case string(filleradmission.KindFrame):
		expected, extension = "image/jpeg", ".jpg"
	case string(filleradmission.KindAudio):
		expected, extension = "audio/wav", ".wav"
	case string(filleradmission.KindVideo):
		expected, extension = "video/mp4", ".mp4"
	}
	if len(signal.ContentTypes) != 1 || signal.ContentTypes[0] != expected {
		return "", fmt.Errorf("signal %q requires content type %q", signal.ID, expected)
	}
	return extension, nil
}

func materialize(corpusRoot string, signal fillerbakeoff.Signal, target string, mode MaterializationMode) error {
	source, err := resolveWithin(corpusRoot, signal.Path)
	if err != nil {
		return fmt.Errorf("resolve signal %q: %w", signal.ID, err)
	}
	if mode == MaterializeHardlink {
		if err := os.Link(source, target); err != nil {
			return fmt.Errorf("hard-link signal %q (use copy mode across filesystems): %w", signal.ID, err)
		}
	} else if err := copyFile(source, target); err != nil {
		return fmt.Errorf("copy signal %q: %w", signal.ID, err)
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Size() != signal.Bytes {
		return fmt.Errorf("materialized signal %q is not the expected regular file", signal.ID)
	}
	digest, err := hashFile(target)
	if err != nil || digest != signal.SHA256 {
		return fmt.Errorf("materialized signal %q failed SHA-256 verification", signal.ID)
	}
	return nil
}

func resolveWithin(root, relative string) (string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil || strings.TrimSpace(root) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid root or absolute signal path")
	}
	path := filepath.Join(rootAbsolute, filepath.Clean(relative))
	rel, err := filepath.Rel(rootAbsolute, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("signal path escapes corpus root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbsolute)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("signal symlink escapes corpus root")
	}
	return resolvedPath, nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func labelTemplate(review fillereval.BlindReviewPacket) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, item := range review.Cases {
		submission := fillereval.LabelSubmission{Alias: item.Alias, BatchID: review.BatchID, Labels: fillereval.Labels{}}
		if err := encoder.Encode(submission); err != nil {
			return nil, fmt.Errorf("encode label template: %w", err)
		}
	}
	return output.Bytes(), nil
}

func readPackets(path string) (map[string]fillerbakeoff.Packet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	packets := map[string]fillerbakeoff.Packet{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLine)
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		packet, err := fillerbakeoff.DecodePacket(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if _, duplicate := packets[packet.CaseID]; duplicate {
			return nil, fmt.Errorf("line %d: duplicate case %q", line, packet.CaseID)
		}
		packets[packet.CaseID] = packet
	}
	return packets, scanner.Err()
}

func readStrictJSON[T any](path string) (T, error) {
	var value T
	file, err := os.Open(path)
	if err != nil {
		return value, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("trailing JSON value")
		}
		return value, err
	}
	return value, nil
}

func hashJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return hashBytes(data)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
