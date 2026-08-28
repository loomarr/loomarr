package fillerreview

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	openRouterActiveRunLockSchemaVersion = 1
	openRouterActiveRunLockFilename      = "active-run.lock"
	maxOpenRouterActiveRunLockBytes      = 16 << 10
)

type openRouterActiveRunLockRecord struct {
	SchemaVersion            int       `json:"schemaVersion"`
	CheckpointIdentitySHA256 string    `json:"checkpointIdentitySha256"`
	StartedAt                time.Time `json:"startedAt"`
	ProcessID                int       `json:"processId"`
}

type openRouterActiveRunLock struct {
	dir    string
	path   string
	digest string
}

func acquireOpenRouterActiveRunLock(dir string, identity openRouterCheckpointIdentity, now func() time.Time, beforeCheckpointDirCreate func()) (openRouterActiveRunLock, error) {
	if err := ensureOpenRouterCheckpointDirBeforeCreate(dir, beforeCheckpointDirCreate); err != nil {
		return openRouterActiveRunLock{}, err
	}
	identityRaw, err := json.Marshal(identity)
	if err != nil {
		return openRouterActiveRunLock{}, err
	}
	path := filepath.Join(dir, openRouterActiveRunLockFilename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		digest := existingOpenRouterActiveRunLockDigest(path)
		return openRouterActiveRunLock{}, fmt.Errorf("OpenRouter review active run lock exists (sha256=%s); explicit operator recovery is required", digest)
	}
	if err != nil {
		return openRouterActiveRunLock{}, fmt.Errorf("create OpenRouter review active run lock: %w", err)
	}
	created := true
	defer func() {
		_ = file.Close()
		if created {
			_ = os.Remove(path)
		}
	}()
	record := openRouterActiveRunLockRecord{
		SchemaVersion: openRouterActiveRunLockSchemaVersion, CheckpointIdentitySHA256: hashBytes(identityRaw),
		StartedAt: now().UTC(), ProcessID: os.Getpid(),
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return openRouterActiveRunLock{}, err
	}
	raw = append(raw, '\n')
	if _, err := file.Write(raw); err != nil {
		return openRouterActiveRunLock{}, fmt.Errorf("write OpenRouter review active run lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		return openRouterActiveRunLock{}, fmt.Errorf("sync OpenRouter review active run lock: %w", err)
	}
	if err := file.Close(); err != nil {
		return openRouterActiveRunLock{}, fmt.Errorf("close OpenRouter review active run lock: %w", err)
	}
	if err := syncOpenRouterReviewDir(dir); err != nil {
		return openRouterActiveRunLock{}, err
	}
	created = false
	return openRouterActiveRunLock{dir: dir, path: path, digest: hashBytes(raw)}, nil
}

func (lock openRouterActiveRunLock) release() error {
	raw, err := readOpenRouterActiveRunLock(lock.path)
	if err != nil {
		return fmt.Errorf("read owned OpenRouter review active run lock: %w", err)
	}
	if hashBytes(raw) != lock.digest {
		return fmt.Errorf("OpenRouter review active run lock changed while owned; explicit operator recovery is required")
	}
	if err := os.Remove(lock.path); err != nil {
		return fmt.Errorf("remove OpenRouter review active run lock: %w", err)
	}
	return syncOpenRouterReviewDir(lock.dir)
}

// RecoverOpenRouterReviewLock explicitly retires a crash-stale lock only when
// the operator supplies the exact digest reported by the blocked runner. It
// never guesses liveness from a PID or timestamp.
func RecoverOpenRouterReviewLock(checkpointDir, expectedSHA256 string) (string, error) {
	if !reviewSHA256(expectedSHA256) {
		return "", fmt.Errorf("exact active run lock SHA-256 is required")
	}
	if err := ensureOpenRouterCheckpointDir(checkpointDir); err != nil {
		return "", err
	}
	path := filepath.Join(checkpointDir, openRouterActiveRunLockFilename)
	raw, err := readOpenRouterActiveRunLock(path)
	if err != nil {
		return "", fmt.Errorf("read crash-stale OpenRouter review active run lock: %w", err)
	}
	actual := hashBytes(raw)
	if actual != expectedSHA256 {
		return "", fmt.Errorf("active run lock SHA-256 changed: got %s", actual)
	}
	recoveredPath := path + ".recovered-" + actual[:12]
	if _, err := os.Lstat(recoveredPath); err == nil || !os.IsNotExist(err) {
		return "", fmt.Errorf("recovered active run lock audit already exists")
	}
	if err := os.Rename(path, recoveredPath); err != nil {
		return "", fmt.Errorf("recover crash-stale OpenRouter review active run lock: %w", err)
	}
	if err := syncOpenRouterReviewDir(checkpointDir); err != nil {
		return "", err
	}
	return recoveredPath, nil
}

func readOpenRouterActiveRunLock(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() < 0 || info.Size() > maxOpenRouterActiveRunLockBytes {
		return nil, fmt.Errorf("active run lock must be a bounded private regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxOpenRouterActiveRunLockBytes+1))
	if err != nil || len(raw) > maxOpenRouterActiveRunLockBytes {
		return nil, fmt.Errorf("read bounded active run lock")
	}
	return raw, nil
}

func existingOpenRouterActiveRunLockDigest(path string) string {
	raw, err := readOpenRouterActiveRunLock(path)
	if err != nil {
		return "unavailable"
	}
	return hashBytes(raw)
}

func syncOpenRouterReviewDir(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open private OpenRouter review directory for sync: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync private OpenRouter review directory: %w", err)
	}
	return nil
}
