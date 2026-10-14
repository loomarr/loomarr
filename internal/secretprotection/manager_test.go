package secretprotection_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/store"
)

func TestManagerReadsASecretAfterDatabaseAndProcessRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "secrets.db")
	installationKey := secretprotection.InstallationKey{0xb2, 0x71, 0x18, 0xef, 0x44, 0x95, 0x0a, 0x63}
	record := secretprotection.Record{Kind: "setting", ID: "library.token", Field: "value"}

	firstStore, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	first, err := secretprotection.NewManager(ctx, firstStore, secretprotection.ManagerOptions{
		InstallationKey: installationKey,
		Random:          bytes.NewReader(bytes.Repeat([]byte{0x31}, 256)),
		Now:             func() time.Time { return time.Unix(100, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("build first manager: %v", err)
	}
	envelope, err := first.Seal(record, []byte("library-api-token"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = secondStore.Close() }()
	second, err := secretprotection.NewManager(ctx, secondStore, secretprotection.ManagerOptions{
		InstallationKey: installationKey,
		Random:          bytes.NewReader(bytes.Repeat([]byte{0x72}, 256)),
		Now:             func() time.Time { return time.Unix(200, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("build second manager: %v", err)
	}
	plaintext, err := second.Open(record, envelope)
	if err != nil {
		t.Fatalf("open after restart: %v", err)
	}
	if string(plaintext) != "library-api-token" {
		t.Fatalf("open after restart = %q, want original secret", plaintext)
	}
}

func TestManagerRejectsDifferentInstallationKeyBeforeReadingSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "wrong-key.db")
	st, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x11},
	}); err != nil {
		t.Fatalf("initialize manager: %v", err)
	}
	_, err = secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x22},
	})
	if !errors.Is(err, secretprotection.ErrInstallationKeyMismatch) {
		t.Fatalf("wrong key error = %v, want ErrInstallationKeyMismatch", err)
	}
}

func TestManagerRotatesDataKeysWithoutLosingOldSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "rotation.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: secretprotection.InstallationKey{0x41}})
	if err != nil {
		t.Fatal(err)
	}
	record := secretprotection.Record{Kind: "setting", ID: "tmdb.api_key", Field: "value"}
	before, err := manager.Seal(record, []byte("before-rotation"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RotateDataKey(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := manager.Seal(record, []byte("after-rotation"))
	if err != nil {
		t.Fatal(err)
	}
	if before == after || strings.Split(before, ":")[2] == strings.Split(after, ":")[2] {
		t.Fatalf("rotation did not move new writes: before=%q after=%q", before, after)
	}
	for envelope, want := range map[string]string{before: "before-rotation", after: "after-rotation"} {
		got, err := manager.Open(record, envelope)
		if err != nil || string(got) != want {
			t.Fatalf("open rotated envelope = (%q, %v), want %q", got, err, want)
		}
	}
}

func TestManagerRefreshObservesAnotherReplicaRotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "replica-rotation.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	key := secretprotection.InstallationKey{0x45}
	first, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: key})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.RotateDataKey(ctx); err != nil {
		t.Fatal(err)
	}
	record := secretprotection.Record{Kind: "setting", ID: "seerr.api_key", Field: "value"}
	envelope, err := first.Seal(record, []byte("rotated-by-first"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.OpenLatest(ctx, record, envelope)
	if err != nil || string(got) != "rotated-by-first" || second.DataKeyCount() != 2 {
		t.Fatalf("refreshed replica = (%q, %v), keys=%d", got, err, second.DataKeyCount())
	}
}

func TestManagerRewrapsDataKeysWithoutChangingSecretCiphertext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "rewrap.db")
	oldKey := secretprotection.InstallationKey{0x51}
	newKey := secretprotection.InstallationKey{0x61}
	st, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: oldKey})
	if err != nil {
		t.Fatal(err)
	}
	record := secretprotection.Record{Kind: "setting", ID: "library.token", Field: "value"}
	envelope, err := manager.Seal(record, []byte("unchanged-ciphertext"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceInstallationKey(ctx, newKey); err != nil {
		t.Fatal(err)
	}
	if manager.Fingerprint() != newKey.Fingerprint() {
		t.Fatalf("fingerprint = %q, want %q", manager.Fingerprint(), newKey.Fingerprint())
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	withNewKey, err := secretprotection.NewManager(ctx, reopened, secretprotection.ManagerOptions{InstallationKey: newKey})
	if err != nil {
		t.Fatalf("restart with replacement key: %v", err)
	}
	got, err := withNewKey.Open(record, envelope)
	if err != nil || string(got) != "unchanged-ciphertext" {
		t.Fatalf("open unchanged ciphertext = (%q, %v)", got, err)
	}
}

func TestDatabaseBackupContainsCiphertextAndNeedsInstallationKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "source.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	key := secretprotection.InstallationKey{0x71}
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{InstallationKey: key})
	if err != nil {
		t.Fatal(err)
	}
	record := secretprotection.Record{Kind: "setting", ID: "notifications.smtp.password", Field: "value"}
	envelope, err := manager.Seal(record, []byte("backup-smtp-password"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, record.ID, envelope); err != nil {
		t.Fatal(err)
	}
	var backup bytes.Buffer
	writer := store.SQLiteBackuper(st)
	if writer == nil {
		t.Fatal("SQLite store did not expose backup writer")
	}
	if err := writer.StreamBackup(ctx, &backup); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backup.Bytes(), []byte("backup-smtp-password")) {
		t.Fatal("database backup contains known plaintext secret")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(backupPath, backup.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(ctx, "sqlite://"+backupPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if _, err := secretprotection.NewManager(ctx, restored, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x72},
	}); !errors.Is(err, secretprotection.ErrInstallationKeyMismatch) {
		t.Fatalf("restore with wrong key = %v, want mismatch", err)
	}
	recovered, err := secretprotection.NewManager(ctx, restored, secretprotection.ManagerOptions{InstallationKey: key})
	if err != nil {
		t.Fatalf("restore with preserved key: %v", err)
	}
	got, err := recovered.Open(record, envelope)
	if err != nil || string(got) != "backup-smtp-password" {
		t.Fatalf("recover backup secret = (%q, %v)", got, err)
	}
}
