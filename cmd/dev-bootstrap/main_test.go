package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDevEncryptionKeyCreatesPrivateStableKeyBesideSQLite(t *testing.T) {
	dir := t.TempDir()
	databaseURL := "sqlite://" + filepath.Join(dir, "loomarr.db")
	if err := ensureDevEncryptionKey(databaseURL); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "encryption.key")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("generated key is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v; want 0600", info.Mode().Perm())
	}
	if err := ensureDevEncryptionKey(databaseURL); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Fatal("second bootstrap replaced the worktree encryption key")
	}
}
