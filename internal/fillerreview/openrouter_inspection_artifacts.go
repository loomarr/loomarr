package fillerreview

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"golang.org/x/sys/unix"
)

const maxInspectionArtifactBytes = 64 << 20

// OpenRouterReviewInspectionArtifactPaths names the local immutable inputs to
// one offline inspection. Each object is opened without following its final
// symlink and validated from that same descriptor before it is read.
type OpenRouterReviewInspectionArtifactPaths struct {
	PackageDir      string
	CheckpointDir   string
	TranscriptsPath string
	SnapshotPath    string
}

// OpenRouterReviewInspectionArtifacts is a descriptor-rooted snapshot of one
// inspection's inputs. Close releases the retained package and checkpoint
// directory descriptors.
type OpenRouterReviewInspectionArtifacts struct {
	packageRoot       *inspectionArtifactRoot
	checkpointRoot    *inspectionArtifactRoot
	manifestRaw       []byte
	checkpointRaw     []byte
	activeLockRaw     []byte
	activeLockPresent bool
	transcripts       []fillerbakeoff.TranscriptArtifact
	snapshot          fillerbakeoff.OpenRouterSnapshot
}

type inspectionArtifactRoot struct {
	file *os.File
}

// OpenOpenRouterReviewInspectionArtifacts opens every top-level input once.
// Later pathname replacement cannot redirect reads away from these objects.
func OpenOpenRouterReviewInspectionArtifacts(paths OpenRouterReviewInspectionArtifactPaths) (*OpenRouterReviewInspectionArtifacts, error) {
	packageRoot, err := openInspectionArtifactRoot(paths.PackageDir)
	if err != nil {
		return nil, fmt.Errorf("open private review package: %w", err)
	}
	artifacts := &OpenRouterReviewInspectionArtifacts{packageRoot: packageRoot}
	ok := false
	defer func() {
		if !ok {
			_ = artifacts.Close()
		}
	}()
	checkpointRoot, err := openInspectionArtifactRoot(paths.CheckpointDir)
	if err != nil {
		return nil, fmt.Errorf("open private review checkpoint: %w", err)
	}
	artifacts.checkpointRoot = checkpointRoot
	artifacts.manifestRaw, err = packageRoot.readRegular("manifest.json", maxInspectionArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("read private review manifest: %w", err)
	}
	artifacts.checkpointRaw, err = checkpointRoot.readRegular(openRouterCheckpointFilename, maxInspectionArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("read private OpenRouter review checkpoint: %w", err)
	}
	artifacts.activeLockRaw, artifacts.activeLockPresent, err = checkpointRoot.readOptionalRegular(openRouterActiveRunLockFilename, maxOpenRouterActiveRunLockBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenRouter review active run lock: %w", err)
	}
	if err := checkpointRoot.validateExactTree(map[string]inspectionObjectKind{
		openRouterCheckpointFilename:    inspectionRegular,
		openRouterActiveRunLockFilename: inspectionRegular,
	}); err != nil {
		return nil, fmt.Errorf("private checkpoint tree contains an unreferenced or unsafe object: %w", err)
	}
	transcriptRaw, err := readInspectionRegular(paths.TranscriptsPath, maxInspectionArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("read private review transcripts: %w", err)
	}
	artifacts.transcripts, err = decodeInspectionTranscripts(transcriptRaw)
	if err != nil {
		return nil, err
	}
	snapshotRaw, err := readInspectionRegular(paths.SnapshotPath, maxInspectionArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("read private review snapshot: %w", err)
	}
	if err := decodeStrictReviewJSON(snapshotRaw, &artifacts.snapshot); err != nil {
		return nil, fmt.Errorf("decode private review snapshot: %w", err)
	}
	ok = true
	return artifacts, nil
}

func (artifacts *OpenRouterReviewInspectionArtifacts) Close() error {
	if artifacts == nil {
		return nil
	}
	var errs []error
	if artifacts.packageRoot != nil && artifacts.packageRoot.file != nil {
		errs = append(errs, artifacts.packageRoot.file.Close())
		artifacts.packageRoot.file = nil
	}
	if artifacts.checkpointRoot != nil && artifacts.checkpointRoot.file != nil {
		errs = append(errs, artifacts.checkpointRoot.file.Close())
		artifacts.checkpointRoot.file = nil
	}
	return errors.Join(errs...)
}

func openInspectionArtifactRoot(path string) (*inspectionArtifactRoot, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("artifact directory path is required")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("open artifact directory descriptor")
	}
	info, err := file.Stat()
	if err != nil || !exactInspectionMode(info, true, 0o700) {
		_ = file.Close()
		return nil, fmt.Errorf("artifact directory must be a non-symlinked directory with exact mode 0700")
	}
	return &inspectionArtifactRoot{file: file}, nil
}

func readInspectionRegular(path string, limit int64) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("open artifact file descriptor")
	}
	defer func() { _ = file.Close() }()
	return readExactInspectionRegular(file, limit)
}

func (root *inspectionArtifactRoot) readRegular(relative string, limit int64) ([]byte, error) {
	file, err := root.openRegular(relative)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return readExactInspectionRegular(file, limit)
}

func (root *inspectionArtifactRoot) readOptionalRegular(relative string, limit int64) ([]byte, bool, error) {
	file, err := root.openRegular(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	raw, err := readExactInspectionRegular(file, limit)
	return raw, true, err
}

func (root *inspectionArtifactRoot) hashRegular(relative string) (string, error) {
	file, err := root.openRegular(relative)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !exactInspectionMode(info, false, 0o600) {
		return "", fmt.Errorf("artifact file must be a non-symlinked regular file with exact mode 0600")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (root *inspectionArtifactRoot) openRegular(relative string) (*os.File, error) {
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("artifact path escapes its rooted directory")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	current, err := syscall.Dup(int(root.file.Fd()))
	if err != nil {
		return nil, err
	}
	for _, part := range parts[:len(parts)-1] {
		next, openErr := syscall.Openat(current, part, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
		_ = syscall.Close(current)
		if openErr != nil {
			return nil, openErr
		}
		var stat syscall.Stat_t
		if statErr := syscall.Fstat(next, &stat); statErr != nil || !exactInspectionStatMode(stat.Mode, true, 0o700) {
			_ = syscall.Close(next)
			return nil, fmt.Errorf("artifact subdirectory must have exact mode 0700")
		}
		current = next
	}
	fd, err := syscall.Openat(current, parts[len(parts)-1], syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	_ = syscall.Close(current)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), relative)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("open rooted artifact descriptor")
	}
	return file, nil
}

type inspectionObjectKind uint8

const (
	inspectionRegular inspectionObjectKind = iota + 1
	inspectionDirectory
)

func (root *inspectionArtifactRoot) validateExactTree(allowed map[string]inspectionObjectKind) error {
	fd, err := unix.Openat(int(root.file.Fd()), ".", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(fd), root.file.Name())
	if directory == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("open rooted directory inventory descriptor")
	}
	defer func() { _ = directory.Close() }()
	return validateExactInspectionDirectory(directory, "", allowed)
}

func validateExactInspectionDirectory(directory *os.File, prefix string, allowed map[string]inspectionObjectKind) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		relative := filepath.Join(prefix, entry.Name())
		objectFD, err := unix.Openat(int(directory.Fd()), entry.Name(), unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		var stat unix.Stat_t
		statErr := unix.Fstat(objectFD, &stat)
		want, permitted := allowed[filepath.ToSlash(relative)]
		if statErr != nil || !permitted {
			_ = unix.Close(objectFD)
			return fmt.Errorf("object %q is not permitted", filepath.ToSlash(relative))
		}
		switch want {
		case inspectionDirectory:
			if !exactInspectionStatMode(stat.Mode, true, 0o700) {
				_ = unix.Close(objectFD)
				return fmt.Errorf("directory %q must have exact mode 0700", filepath.ToSlash(relative))
			}
			childFD, openErr := unix.Openat(objectFD, ".", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
			_ = unix.Close(objectFD)
			if openErr != nil {
				return openErr
			}
			child := os.NewFile(uintptr(childFD), relative)
			if child == nil {
				_ = unix.Close(childFD)
				return fmt.Errorf("open child directory inventory descriptor")
			}
			err = validateExactInspectionDirectory(child, relative, allowed)
			_ = child.Close()
			if err != nil {
				return err
			}
		case inspectionRegular:
			_ = unix.Close(objectFD)
			if !exactInspectionStatMode(stat.Mode, false, 0o600) {
				return fmt.Errorf("file %q must be regular with exact mode 0600", filepath.ToSlash(relative))
			}
		default:
			_ = unix.Close(objectFD)
			return fmt.Errorf("object %q has no permitted type", filepath.ToSlash(relative))
		}
	}
	return nil
}

func readExactInspectionRegular(file *os.File, limit int64) ([]byte, error) {
	info, err := file.Stat()
	if err != nil || !exactInspectionMode(info, false, 0o600) {
		return nil, fmt.Errorf("artifact file must be a non-symlinked regular file with exact mode 0600")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("artifact file exceeds its byte ceiling")
	}
	return raw, nil
}

func exactInspectionMode(info os.FileInfo, directory bool, perm os.FileMode) bool {
	if info == nil || info.Mode().Perm() != perm || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	if directory {
		return info.IsDir()
	}
	return info.Mode().IsRegular()
}

func exactInspectionStatMode(mode uint32, directory bool, perm uint32) bool {
	if mode&0o777 != perm || mode&(syscall.S_ISUID|syscall.S_ISGID|syscall.S_ISVTX) != 0 {
		return false
	}
	if directory {
		return mode&syscall.S_IFMT == syscall.S_IFDIR
	}
	return mode&syscall.S_IFMT == syscall.S_IFREG
}

func decodeInspectionTranscripts(raw []byte) ([]fillerbakeoff.TranscriptArtifact, error) {
	var transcripts []fillerbakeoff.TranscriptArtifact
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxInspectionArtifactBytes)
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var transcript fillerbakeoff.TranscriptArtifact
		if err := decoder.Decode(&transcript); err != nil {
			return nil, fmt.Errorf("decode private review transcripts line %d: %w", line, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("decode private review transcripts line %d: trailing JSON value", line)
		}
		if _, duplicate := seen[transcript.CaseID]; duplicate {
			return nil, fmt.Errorf("decode private review transcripts line %d: duplicate case", line)
		}
		seen[transcript.CaseID] = struct{}{}
		transcripts = append(transcripts, transcript)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return transcripts, nil
}

func validateInspectionReviewPackage(root *inspectionArtifactRoot, manifest Package, expectedCases int) error {
	artifacts, err := validateReviewPackageStructure(manifest, expectedCases)
	if err != nil {
		return err
	}
	allowed := map[string]inspectionObjectKind{"manifest.json": inspectionRegular}
	for _, artifact := range artifacts {
		digest, err := root.hashRegular(artifact.Path)
		if err != nil || digest != artifact.SHA256 {
			return fmt.Errorf("%s", artifact.Failure)
		}
		clean, _ := reviewPackageRelativePath(artifact.Path)
		allowed[filepath.ToSlash(clean)] = inspectionRegular
		for parent := filepath.Dir(clean); parent != "."; parent = filepath.Dir(parent) {
			allowed[filepath.ToSlash(parent)] = inspectionDirectory
		}
	}
	if err := root.validateExactTree(allowed); err != nil {
		return fmt.Errorf("private package tree contains an unreferenced or unsafe object: %w", err)
	}
	return nil
}
