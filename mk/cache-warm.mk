## ---- CI build-cache warming (#1570) ----------------------------------------
# ⚠ Compile-only. These targets build test binaries with a gate's exact flags and execute NONE, so
# they must never stand in for a gate: releaseverify allows GO_TEST_COMPILE_ONLY only in this file
# and scripts/go-test-packages.sh, and `go-cache-warm` only in .github/workflows/ci-go-cache-warm.yml.

# test-pg's packages (mk/store.mk; its recipe is audited literally by releaseverify). A drift here
# costs cache hits, never gate coverage.
GO_CACHE_WARM_PG_PACKAGES := ./internal/store/ ./internal/backendtransition/ ./internal/app/

.PHONY: go-cache-warm
go-cache-warm: ## CI cache warming only: build one gate's Go test binaries without running them (GO_CACHE_WARM names a race lane or postgres)
	@case "$(GO_CACHE_WARM)" in \
	  postgres) GO_TEST_COMPILE_ONLY=1 GOFLAGS=-tags=integration GO_BIN="$(GO)" \
	    ./scripts/go-test-packages.sh race 20m $(GO_CACHE_WARM_PG_PACKAGES) ;; \
	  1/2|2/2|certification-1/2|certification-2/2) GO_TEST_COMPILE_ONLY=1 GO_BIN="$(GO)" \
	    GO_TEST_LANE="$(GO_CACHE_WARM)" ./scripts/go-test-lane.sh ;; \
	  *) echo "go-cache-warm: GO_CACHE_WARM must name a race lane or postgres, got '$(GO_CACHE_WARM)'" >&2; exit 2 ;; \
	esac
