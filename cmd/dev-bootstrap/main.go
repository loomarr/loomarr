// Command dev-bootstrap prepares only the isolated database selected by the
// agent-worktree harness. The harness refuses to invoke it for the primary tree.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/devbootstrap"
	"github.com/mantonx/loomarr/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("dev-bootstrap: load config: %v", err)
	}
	if err := devbootstrap.ValidateTarget(os.Getenv("LOOMARR_REPO_ROOT"), cfg.DatabaseURL); err != nil {
		log.Fatalf("dev-bootstrap: %v", err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL, true)
	if err != nil {
		log.Fatalf("dev-bootstrap: open isolated store: %v", err)
	}
	defer func() { _ = st.Close() }()

	result, err := devbootstrap.Ensure(ctx, st)
	if err != nil {
		log.Fatalf("dev-bootstrap: %v", err)
	}
	if result.Created {
		fmt.Printf("dev-bootstrap: created %s and completed setup\n", result.User)
		return
	}
	fmt.Printf("dev-bootstrap: kept %s and completed setup\n", result.User)
}
