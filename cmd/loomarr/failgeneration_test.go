package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailGeneration(t *testing.T) {
	cause := errors.New("target unavailable")
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		fallback    string
		dir         string
		wantRestart bool
		wantErr     bool
	}{
		{name: "no trial returns the cause", dir: t.TempDir(), wantErr: true},
		{name: "trial restores and restarts", fallback: "sqlite:///data/loomarr.db", dir: t.TempDir(), wantRestart: true},
		{name: "failed restore does not restart", fallback: "sqlite:///data/loomarr.db", dir: notADir, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &databaseMigrationState{fallbackSQLiteURL: tt.fallback, bootstrapDir: tt.dir}
			restart, err := state.failGeneration("open first PostgreSQL generation", cause)
			if restart != tt.wantRestart || (err != nil) != tt.wantErr {
				t.Fatalf("failGeneration = %t, %v; want restart %t, error %t", restart, err, tt.wantRestart, tt.wantErr)
			}
			if tt.fallback == "" && !errors.Is(err, cause) {
				t.Errorf("error = %v, want the cause", err)
			}
			if tt.wantRestart && !strings.HasPrefix(state.lastError, "open first PostgreSQL generation") {
				t.Errorf("lastError = %q, want it to name the stage", state.lastError)
			}
		})
	}
}
