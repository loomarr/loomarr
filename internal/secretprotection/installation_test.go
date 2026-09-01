package secretprotection_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

func TestLoadInstallationKeyGeneratesOneStableProtectedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	random := bytes.NewReader(bytes.Repeat([]byte{0x6b}, 64))
	first, err := secretprotection.LoadInstallationKey(secretprotection.InstallationKeyOptions{
		DataDir: dir,
		Random:  random,
	})
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := secretprotection.LoadInstallationKey(secretprotection.InstallationKeyOptions{
		DataDir: dir,
		Random:  random,
	})
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if first.Key != second.Key {
		t.Fatal("second load did not reuse the generated installation key")
	}
	if first.Source != secretprotection.InstallationKeyGenerated || second.Source != secretprotection.InstallationKeyFile {
		t.Fatalf("sources = %q then %q, want generated then file", first.Source, second.Source)
	}

	info, err := os.Stat(filepath.Join(dir, "encryption.key"))
	if err != nil {
		t.Fatalf("stat generated key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("generated key mode = %o, want 600", got)
	}
}

func TestLoadInstallationKeyHonorsEnvironmentAndFileForms(t *testing.T) {
	t.Parallel()
	var want secretprotection.InstallationKey
	copy(want[:], bytes.Repeat([]byte{0x3c}, len(want)))
	encoded := base64.RawURLEncoding.EncodeToString(want[:])
	for _, tc := range []struct {
		name string
		env  map[string]string
		want secretprotection.InstallationKeySource
	}{
		{name: "value", env: map[string]string{"LOOMARR_ENCRYPTION_KEY": encoded}, want: secretprotection.InstallationKeyEnvironment},
		{name: "file", env: map[string]string{"LOOMARR_ENCRYPTION_KEY_FILE": filepath.Join(t.TempDir(), "mounted-key")}, want: secretprotection.InstallationKeyFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if path := tc.env["LOOMARR_ENCRYPTION_KEY_FILE"]; path != "" {
				if err := os.WriteFile(path, []byte(encoded+"\n"), 0o400); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := secretprotection.LoadInstallationKey(secretprotection.InstallationKeyOptions{
				LookupEnv: func(name string) (string, bool) { value, ok := tc.env[name]; return value, ok },
			})
			if err != nil || loaded.Key != want || loaded.Source != tc.want {
				t.Fatalf("load = (%#v, %v), want source %q", loaded, err, tc.want)
			}
		})
	}
}

func TestLoadInstallationKeyRejectsAmbiguousOrMalformedInput(t *testing.T) {
	t.Parallel()
	for _, env := range []map[string]string{
		{"LOOMARR_ENCRYPTION_KEY": "bad"},
		{"LOOMARR_ENCRYPTION_KEY": base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "LOOMARR_ENCRYPTION_KEY_FILE": "/mounted/key"},
	} {
		_, err := secretprotection.LoadInstallationKey(secretprotection.InstallationKeyOptions{
			LookupEnv: func(name string) (string, bool) { value, ok := env[name]; return value, ok },
		})
		if err == nil {
			t.Fatalf("accepted invalid installation-key environment: %#v", env)
		}
	}
}

func TestLoadPreviousInstallationKeyIsOptionalAndNeverGenerated(t *testing.T) {
	t.Parallel()
	lookup := func(string) (string, bool) { return "", false }
	if _, ok, err := secretprotection.LoadPreviousInstallationKey(lookup); err != nil || ok {
		t.Fatalf("absent previous key = (present %v, %v), want absent", ok, err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x7a}, 32))
	lookup = func(name string) (string, bool) {
		if name == "LOOMARR_ENCRYPTION_KEY_PREVIOUS" {
			return encoded, true
		}
		return "", false
	}
	key, ok, err := secretprotection.LoadPreviousInstallationKey(lookup)
	if err != nil || !ok || key[0] != 0x7a {
		t.Fatalf("previous key = (%#v, %v, %v)", key, ok, err)
	}
}
