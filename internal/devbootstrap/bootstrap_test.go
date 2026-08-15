package devbootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

func TestEnsureCreatesOneDeveloperAndCompletesSetup(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	first, err := Ensure(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.User != Username {
		t.Fatalf("first Ensure() = %+v, want a newly created %q", first, Username)
	}
	user, err := st.GetUserByName(ctx, Username)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != store.RoleAdmin || user.PasswordHash == "" {
		t.Fatalf("developer = %+v, want a local admin created through bootstrap", user)
	}
	if completed, err := st.GetSetting(ctx, "setup.completed"); err != nil || completed != "true" {
		t.Fatalf("setup.completed = %q, %v; want true", completed, err)
	}

	second, err := Ensure(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatalf("second Ensure() = %+v, want idempotent reuse", second)
	}
	if admins, err := st.CountAdmins(ctx); err != nil || admins != 1 {
		t.Fatalf("CountAdmins() = %d, %v; want 1", admins, err)
	}
}

func TestValidateTargetAcceptsOnlyTheWorktreeDatabase(t *testing.T) {
	root := t.TempDir()
	want := "sqlite://" + filepath.Join(root, ".agent-data", "loomarr.db")
	if err := ValidateTarget(root, want); err != nil {
		t.Fatalf("ValidateTarget(isolated) = %v", err)
	}
	if err := ValidateTarget(root, "sqlite:///data/loomarr.db"); err == nil {
		t.Fatal("ValidateTarget(primary database) succeeded, want refusal")
	}
	if err := ValidateTarget("", want); err == nil {
		t.Fatal("ValidateTarget(without harness root) succeeded, want refusal")
	}
}
