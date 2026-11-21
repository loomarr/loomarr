package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A database is the file at its path whatever characters the path holds: a "#", a "?" or a "%"
// in a folder's name is part of the name, not where a URI's fragment, query or escape starts. Two
// stores in folders that differ only after one are two databases, each opened, read-only opened
// and backed up at its own path.
func TestADatabaseIsTheFileAtItsPathWhateverItsCharacters(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	for _, folder := range []string{"data#1", "data#2", "data?mode=memory", "data%41", "dataA"} {
		dir := filepath.Join(parent, folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "loomarr.db")
		if err := os.WriteFile(path, sharedSQLiteTemplate(t), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := Open(ctx, "sqlite://"+path, false)
		if err != nil {
			t.Fatalf("%s: %v", folder, err)
		}
		if got, err := s.GetSetting(ctx, "path.probe"); err == nil && got != "" {
			t.Errorf("%s opened a database %s wrote", folder, got)
		}
		if err := s.SetSetting(ctx, "path.probe", folder); err != nil {
			t.Fatal(err)
		}
		backup := filepath.Join(parent, folder+".bak")
		if err := s.(*sqlStore).vacuumInto(ctx, backup); err != nil {
			t.Fatalf("%s: back up: %v", folder, err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		for _, at := range []string{path, backup} {
			ro, err := openSQLiteReadOnly(ctx, at)
			if err != nil {
				t.Fatalf("%s: open %s read-only: %v", folder, at, err)
			}
			if got, err := ro.GetSetting(ctx, "path.probe"); err != nil || got != folder {
				t.Errorf("%s read back %q (err %v), want %q", at, got, err, folder)
			}
			_ = ro.Close()
		}
	}
	// Nothing was written beside the folders, where a cut-short path would have put it.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) != ".bak" {
			t.Errorf("a stray file %s", e.Name())
		}
	}
}
