// Package devbootstrap prepares an isolated agent worktree for UI development.
// It is invoked by the repository harness, never by the application at startup.
package devbootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/store"
)

const Username = "developer"

// ValidateTarget keeps this development command incapable of touching a
// configured or primary database. The worktree harness owns exactly this path.
func ValidateTarget(worktree, databaseURL string) error {
	if worktree == "" {
		return fmt.Errorf("LOOMARR_REPO_ROOT is required")
	}
	root, err := filepath.Abs(worktree)
	if err != nil {
		return fmt.Errorf("resolve worktree: %w", err)
	}
	want := "sqlite://" + filepath.Join(root, ".agent-data", "loomarr.db")
	if databaseURL != want {
		return fmt.Errorf("refusing database %q; worktree dev identity requires %q", databaseURL, want)
	}
	return nil
}

// Result describes the idempotent preparation without exposing the random local
// password. Agent worktrees enter through the separately gated dev-login route.
type Result struct {
	Created bool
	User    string
}

// Ensure creates the first admin through the production bootstrap service, then
// marks the first-run flow complete. Existing worktree databases keep their admin;
// the harness only needs an allowlisted account for dev-login to borrow.
func Ensure(ctx context.Context, st store.Store) (Result, error) {
	admins, err := st.CountAdmins(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("count admins: %w", err)
	}

	result := Result{User: Username}
	if admins == 0 {
		id, err := randomHex(16)
		if err != nil {
			return Result{}, fmt.Errorf("generate user id: %w", err)
		}
		password, err := randomHex(32)
		if err != nil {
			return Result{}, fmt.Errorf("generate password: %w", err)
		}
		p := auth.NewProvisioner(st, nil, func() string { return "dev_" + id }, time.Now)
		if _, err := p.Bootstrap(ctx, Username, password); err != nil {
			return Result{}, fmt.Errorf("bootstrap developer: %w", err)
		}
		result.Created = true
	} else {
		result.User = "existing admin"
	}

	if err := st.SetSetting(ctx, "setup.completed", "true"); err != nil {
		return Result{}, fmt.Errorf("complete setup: %w", err)
	}
	return result, nil
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
