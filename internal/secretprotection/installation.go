package secretprotection

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	installationKeyEnv     = "LOOMARR_ENCRYPTION_KEY"
	installationKeyFileEnv = "LOOMARR_ENCRYPTION_KEY_FILE"
	previousKeyEnv         = "LOOMARR_ENCRYPTION_KEY_PREVIOUS"
	previousKeyFileEnv     = "LOOMARR_ENCRYPTION_KEY_PREVIOUS_FILE"
	installationKeyName    = "encryption.key"
)

// InstallationKeySource explains where the current process obtained its key.
type InstallationKeySource string

const (
	InstallationKeyEnvironment InstallationKeySource = "environment"
	InstallationKeyFile        InstallationKeySource = "file"
	InstallationKeyGenerated   InstallationKeySource = "generated"
)

// InstallationKeyOptions supplies boot-only dependencies.
type InstallationKeyOptions struct {
	DataDir   string
	LookupEnv func(string) (string, bool)
	Random    io.Reader
}

// LoadPreviousInstallationKey resolves the optional one-boot old key used to
// atomically replace an installation key. It never generates one.
func LoadPreviousInstallationKey(lookup func(string) (string, bool)) (InstallationKey, bool, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	raw, hasRaw := nonemptyEnv(lookup, previousKeyEnv)
	path, hasPath := nonemptyEnv(lookup, previousKeyFileEnv)
	if hasRaw && hasPath {
		return InstallationKey{}, false, errors.New("secret protection: LOOMARR_ENCRYPTION_KEY_PREVIOUS and LOOMARR_ENCRYPTION_KEY_PREVIOUS_FILE are both set")
	}
	if hasRaw {
		key, err := decodeInstallationKey(raw)
		return key, true, err
	}
	if hasPath {
		key, err := readInstallationKeyFile(path)
		return key, true, err
	}
	return InstallationKey{}, false, nil
}

// LoadedInstallationKey is the resolved key and non-secret provenance.
type LoadedInstallationKey struct {
	Key    InstallationKey
	Source InstallationKeySource
}

// LoadInstallationKey resolves environment, explicit file, or the generated
// data-directory key in that order. The installation key never enters the DB.
func LoadInstallationKey(options InstallationKeyOptions) (LoadedInstallationKey, error) {
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	random := options.Random
	if random == nil {
		random = rand.Reader
	}
	rawEnv, hasEnv := nonemptyEnv(lookup, installationKeyEnv)
	fileEnv, hasFileEnv := nonemptyEnv(lookup, installationKeyFileEnv)
	if hasEnv && hasFileEnv {
		return LoadedInstallationKey{}, errors.New("secret protection: LOOMARR_ENCRYPTION_KEY and LOOMARR_ENCRYPTION_KEY_FILE are both set")
	}
	if hasEnv {
		key, err := decodeInstallationKey(rawEnv)
		return LoadedInstallationKey{Key: key, Source: InstallationKeyEnvironment}, err
	}
	if hasFileEnv {
		key, err := readInstallationKeyFile(fileEnv)
		return LoadedInstallationKey{Key: key, Source: InstallationKeyFile}, err
	}
	if options.DataDir == "" {
		return LoadedInstallationKey{}, errors.New("secret protection: no data directory for generated installation key")
	}
	path := filepath.Join(options.DataDir, installationKeyName)
	key, err := readInstallationKeyFile(path)
	if err == nil {
		return LoadedInstallationKey{Key: key, Source: InstallationKeyFile}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return LoadedInstallationKey{}, err
	}
	if err := os.MkdirAll(options.DataDir, 0o755); err != nil {
		return LoadedInstallationKey{}, fmt.Errorf("secret protection: create data directory: %w", err)
	}
	var generated InstallationKey
	if _, err := io.ReadFull(random, generated[:]); err != nil {
		return LoadedInstallationKey{}, fmt.Errorf("secret protection: generate installation key: %w", err)
	}
	created, err := installKeyFile(path, generated)
	if err != nil {
		return LoadedInstallationKey{}, err
	}
	if !created {
		key, err = readInstallationKeyFile(path)
		return LoadedInstallationKey{Key: key, Source: InstallationKeyFile}, err
	}
	return LoadedInstallationKey{Key: generated, Source: InstallationKeyGenerated}, nil
}

func nonemptyEnv(lookup func(string) (string, bool), name string) (string, bool) {
	value, ok := lookup(name)
	return value, ok && value != ""
}

func readInstallationKeyFile(path string) (InstallationKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return InstallationKey{}, fmt.Errorf("secret protection: read installation key file: %w", err)
	}
	return decodeInstallationKey(strings.TrimSpace(string(raw)))
}

func decodeInstallationKey(encoded string) (InstallationKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) != 32 {
		return InstallationKey{}, errors.New("secret protection: installation key must be 32 bytes encoded as unpadded base64url")
	}
	var key InstallationKey
	copy(key[:], raw)
	return key, nil
}

func installKeyFile(path string, key InstallationKey) (bool, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".encryption-key-*")
	if err != nil {
		return false, fmt.Errorf("secret protection: create temporary installation key: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("secret protection: protect temporary installation key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key[:])
	if _, err := io.WriteString(tmp, encoded); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("secret protection: write temporary installation key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("secret protection: sync temporary installation key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("secret protection: close temporary installation key: %w", err)
	}
	if err := os.Link(tmpPath, path); errors.Is(err, fs.ErrExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("secret protection: install generated key: %w", err)
	}
	return true, nil
}
