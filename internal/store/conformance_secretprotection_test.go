package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

func testInitialSecretDataKey(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	firstCandidate := secretprotection.WrappedDataKey{
		ID: "dek-first", Wrapped: "loomarr-dek:v1:first-ciphertext", CreatedAt: time.Unix(100, 0).UTC(),
	}
	first, err := s.EnsureSecretDataKey(ctx, firstCandidate)
	if err != nil {
		t.Fatalf("ensure first data key: %v", err)
	}
	second, err := s.EnsureSecretDataKey(ctx, secretprotection.WrappedDataKey{
		ID: "dek-racing", Wrapped: "loomarr-dek:v1:racing-ciphertext", CreatedAt: time.Unix(200, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ensure second data key: %v", err)
	}
	if first.ID != "dek-first" || second != first || !first.Active {
		t.Fatalf("ensure results = %#v then %#v, want the first candidate active both times", first, second)
	}
}

func testRotateSecretDataKey(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	first, err := s.EnsureSecretDataKey(ctx, secretprotection.WrappedDataKey{
		ID: "dek-old", Wrapped: "loomarr-dek:v1:old-ciphertext", CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ensure old data key: %v", err)
	}
	next := secretprotection.WrappedDataKey{
		ID: "dek-new", Wrapped: "loomarr-dek:v1:new-ciphertext", CreatedAt: time.Unix(200, 0).UTC(),
	}
	if err := s.RotateSecretDataKey(ctx, next); err != nil {
		t.Fatalf("rotate data key: %v", err)
	}
	keys, err := s.ListSecretDataKeys(ctx)
	if err != nil {
		t.Fatalf("list data keys: %v", err)
	}
	if len(keys) != 2 || keys[0].ID != first.ID || keys[0].Active || keys[1].ID != next.ID || !keys[1].Active {
		t.Fatalf("keys after rotation = %#v, want old retired then new active", keys)
	}
}

func testInstallationKeyFingerprint(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:first"); err != nil {
		t.Fatalf("store first fingerprint: %v", err)
	}
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:first"); err != nil {
		t.Fatalf("recheck same fingerprint: %v", err)
	}
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:other"); !errors.Is(err, secretprotection.ErrInstallationKeyMismatch) {
		t.Fatalf("different fingerprint = %v, want mismatch", err)
	}
}

func testRewriteSettingValuesPreservesAudit(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	if err := s.UpsertSetting(ctx, SettingRow{
		Key: "library.token", Value: "plaintext", UpdatedAt: time.Unix(1234, 0).UTC(), UpdatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettingEnvOverride(ctx, "library.token", true, "", "admin"); err != nil {
		t.Fatal(err)
	}
	before, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RewriteSettingValues(ctx, []SettingMutation{{Key: "library.token", Value: "ciphertext"}}); err != nil {
		t.Fatalf("rewrite value: %v", err)
	}
	after, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != 1 || after[0].Value != "ciphertext" ||
		after[0].UpdatedAt != before[0].UpdatedAt || after[0].UpdatedBy != before[0].UpdatedBy ||
		after[0].EnvOverride != before[0].EnvOverride {
		t.Fatalf("rewrite changed more than value: before=%#v after=%#v", before, after)
	}
	if err := s.RewriteSettingValues(ctx, []SettingMutation{
		{Key: "library.token", Value: "must-roll-back"},
		{Key: "missing.setting", Value: "cannot-commit"},
	}); err == nil {
		t.Fatal("rewrite committed despite a missing row")
	}
	value, err := s.GetSetting(ctx, "library.token")
	if err != nil || value != "ciphertext" {
		t.Fatalf("failed rewrite was not atomic: value=%q err=%v", value, err)
	}
}

func testReplaceWrappedDataKeysIsAtomic(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:old"); err != nil {
		t.Fatal(err)
	}
	first, err := s.EnsureSecretDataKey(ctx, secretprotection.WrappedDataKey{
		ID: "dek-old", Wrapped: "loomarr-dek:v1:old", CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replacement := first
	replacement.Wrapped = "loomarr-dek:v1:rewrapped"
	if err := s.ReplaceWrappedDataKeys(ctx, "sha256:wrong", "sha256:new", []secretprotection.WrappedDataKey{replacement}); !errors.Is(err, secretprotection.ErrInstallationKeyMismatch) {
		t.Fatalf("wrong expected fingerprint = %v, want mismatch", err)
	}
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:old"); err != nil {
		t.Fatalf("failed replacement changed fingerprint: %v", err)
	}
	if err := s.ReplaceWrappedDataKeys(ctx, "sha256:old", "sha256:new", []secretprotection.WrappedDataKey{replacement}); err != nil {
		t.Fatalf("replace wrapped keys: %v", err)
	}
	keys, err := s.ListSecretDataKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].Wrapped != replacement.Wrapped {
		t.Fatalf("keys after replacement = (%#v, %v)", keys, err)
	}
	if err := s.EnsureInstallationKeyFingerprint(ctx, "sha256:new"); err != nil {
		t.Fatalf("new fingerprint not committed: %v", err)
	}
}
