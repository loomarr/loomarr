package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishBundleIsPrivateAndImmutable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "refresh")
	files := map[string][]byte{
		"manifest.json":       []byte("manifest"),
		"packets.jsonl":       []byte("packets"),
		"mapping.json":        []byte("mapping"),
		"content-review.json": []byte("review"),
		"refresh-report.json": []byte("report"),
	}
	if err := publishBundle(target, files); err != nil {
		t.Fatal(err)
	}
	assertPrivateBundle(t, target, files)
	if err := publishBundle(target, files); err == nil {
		t.Fatal("existing output bundle was overwritten")
	}
}

func assertPrivateBundle(t *testing.T, target string, files map[string][]byte) {
	t.Helper()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode=%#o", got)
	}
	for name, want := range files {
		path := filepath.Join(target, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s=%q", name, got)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%#o", name, got)
		}
	}
}
