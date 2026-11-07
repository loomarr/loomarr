package assessment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
)

const (
	structureAssessmentMediaDirName        = "structure-assessment"
	structureAssessmentLineageMaximumBytes = 64 << 10
)

// StructureAssessmentMediaSourceIdentity deliberately excludes a pathname. A retained path is a
// location at which bytes may be found; it is not part of the decision authority.
type StructureAssessmentMediaSourceIdentity struct {
	Role       filler.SplitSourceRole `json:"role"`
	SHA256     string                 `json:"sha256"`
	Bytes      int64                  `json:"bytes"`
	ClipHash   string                 `json:"clipHash"`
	DurationMS int64                  `json:"durationMs"`
}

// StructureAssessmentMediaDerivative identifies one complete normalized output without naming
// the machine-local location at which it was published.
type StructureAssessmentMediaDerivative struct {
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
	DurationMS int64  `json:"durationMs"`
}

func structureAssessmentSourceIdentity(source filler.SplitSourceAsset) StructureAssessmentMediaSourceIdentity {
	return StructureAssessmentMediaSourceIdentity{
		Role: source.Role, SHA256: source.SHA256, Bytes: source.Bytes,
		ClipHash: source.ClipHash, DurationMS: source.DurationMs,
	}
}

func structureAssessmentMediaPath(clipDir, digest string) string {
	return filepath.Join(clipDir, filler.MediaAssetRootName, structureAssessmentMediaDirName, "media", digest[:2], digest+".mp4")
}

func strictStructureAssessmentJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

func readBoundedRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, errors.New("artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(raw)) != info.Size() || int64(len(raw)) > maximum {
		return nil, errors.New("artifact changed or exceeded its byte ceiling while read")
	}
	return raw, nil
}

func publishStructureAssessmentArtifact(ctx context.Context, staged, target string, expected []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Link(staged, target); err == nil {
		_ = os.Remove(staged)
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, err := readBoundedRegularFile(target, max(int64(len(expected)), structureAssessmentLineageMaximumBytes))
	if err != nil || !bytes.Equal(existing, expected) {
		return errors.New("existing structure assessment artifact does not match exact bytes")
	}
	_ = os.Remove(staged)
	return nil
}

func publishStructureAssessmentMedia(ctx context.Context, staged, target, digest string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Link(staged, target); err == nil {
		_ = os.Remove(staged)
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existingDigest, existingSize, err := filler.FileSHA256(target)
	if err != nil || existingDigest != digest || existingSize != size {
		return errors.New("existing structure assessment media does not match its content address")
	}
	equal, err := filler.ExactFileBytesEqual(ctx, staged, target, cut.MaximumVideoBytes)
	if err != nil || !equal {
		return errors.New("existing structure assessment media does not match exact bytes")
	}
	_ = os.Remove(staged)
	return nil
}

func writeStructureAssessmentJSON(stageDir, pattern string, value any) (string, []byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 || len(raw) > structureAssessmentLineageMaximumBytes {
		return "", nil, errors.New("encode bounded structure assessment artifact")
	}
	file, err := os.CreateTemp(stageDir, pattern)
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	return path, raw, nil
}

func absoluteDurationDifference(left, right int64) int64 {
	if left < right {
		return right - left
	}
	return left - right
}
