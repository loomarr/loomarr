package secretprotection_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

func TestKeyWrapperStoresOnlyWrappedDataKeyMaterial(t *testing.T) {
	t.Parallel()

	installationKey := secretprotection.InstallationKey{0x8e, 0x11, 0x37, 0xa4, 0x59, 0x62, 0xf0, 0x0d}
	nonces := bytes.NewReader(bytes.Repeat([]byte{0x4c}, 32))
	wrapper, err := secretprotection.NewKeyWrapper(installationKey, nonces)
	if err != nil {
		t.Fatalf("new key wrapper: %v", err)
	}
	dataKey := secretprotection.DataKey{
		ID:       "dek-2026-08-31",
		Material: [32]byte{0x71, 0x55, 0xa8, 0x03, 0xff, 0x21, 0x9c, 0x66},
	}

	wrapped, err := wrapper.Wrap(dataKey)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if wrapped == "" || bytes.Contains([]byte(wrapped), dataKey.Material[:]) {
		t.Fatalf("wrapped data key exposes raw material: %q", wrapped)
	}

	unwrapped, err := wrapper.Unwrap(dataKey.ID, wrapped)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if unwrapped != dataKey {
		t.Fatalf("unwrap returned %#v, want original data key", unwrapped)
	}
}

func TestInstallationKeyFingerprintIsStableAndNonSecret(t *testing.T) {
	t.Parallel()
	key := secretprotection.InstallationKey{0x01, 0x02, 0x03, 0x04}
	got := key.Fingerprint()
	if got != key.Fingerprint() || !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+24 {
		t.Fatalf("fingerprint = %q, want stable sha256 identity", got)
	}
}

func TestKeyWrapperRejectsWrongInstallationKeyAndIdentity(t *testing.T) {
	t.Parallel()
	dataKey := secretprotection.DataKey{ID: "dek-bound", Material: [32]byte{0x55, 0x66}}
	wrapper, err := secretprotection.NewKeyWrapper(secretprotection.InstallationKey{0x31}, bytes.NewReader(bytes.Repeat([]byte{0x12}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := wrapper.Wrap(dataKey)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := secretprotection.NewKeyWrapper(secretprotection.InstallationKey{0x32}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Unwrap(dataKey.ID, wrapped); err == nil {
		t.Fatal("wrong installation key unwrapped a data key")
	}
	if _, err := wrapper.Unwrap("dek-other", wrapped); err == nil {
		t.Fatal("wrapped data key opened under a different identity")
	}
}
