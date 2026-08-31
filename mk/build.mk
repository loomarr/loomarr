## ---- build / run ---------------------------------------------------------

.PHONY: build rust-build image-cert image-bench image-parallelism-bench
build: rust-build ## build the cgo-free Go server and required Rust image worker
	release="$${LOOMARR_RELEASE:-dev}"; \
	  CGO_ENABLED=0 $(GO) build \
	    -ldflags="-X github.com/loomarr/loomarr/internal/buildinfo.version=$$release" \
	    -o $(BIN_DIR)/loomarr ./cmd/loomarr; \
	  $(BIN_DIR)/loomarr-image capabilities --protocol 1 --self-test | grep -q "\"release\":\"$$release\""

rust-build: ## build the required Rust image worker
	LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(CARGO) build --release --locked -p loomarr-image
	install -d $(BIN_DIR)
	install -m 0755 target/release/loomarr-image $(BIN_DIR)/loomarr-image

image-cert: rust-build ## certify the Rust image worker; optional IMAGE_CERT_CORPUS=/absolute/path
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${IMAGE_CERT_REPORT:-$$LOOMARR_ARTIFACT_DIR/image-certification.json}"; \
	  if [ -n "$${IMAGE_CERT_CORPUS:-}" ]; then \
	    LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-cert \
	      --worker "$(BIN_DIR)/loomarr-image" --report "$$report" --corpus "$$IMAGE_CERT_CORPUS"; \
	  else \
	    LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-cert \
	      --worker "$(BIN_DIR)/loomarr-image" --report "$$report"; \
	  fi

image-bench: rust-build ## benchmark release-worker AVIF ladders; optional IMAGE_BENCH_RUNS/ROLES/REPORT
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${IMAGE_BENCH_REPORT:-$$LOOMARR_ARTIFACT_DIR/image-benchmark.json}"; \
	  LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-bench \
	    --worker "$(BIN_DIR)/loomarr-image" --report "$$report" \
	    --roles "$${IMAGE_BENCH_ROLES:-poster,backdrop,icon}" \
	    --workers "$${IMAGE_BENCH_WORKERS:-1}" \
	    --avif-threads "$${IMAGE_BENCH_AVIF_THREADS:-1}"

image-parallelism-bench: rust-build ## compare AVIF process/thread shapes at 2/4/8 CPUs (opt-in, Linux)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report_dir="$${IMAGE_BENCH_REPORT_DIR:-$$LOOMARR_ARTIFACT_DIR/image-parallelism}"; \
	  LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" GO="$(GO)" \
	    ./scripts/image-parallelism-bench.sh "$(BIN_DIR)/loomarr-image" "$$report_dir"

.PHONY: dev
dev: ## dev compose stack (external deps: tunarr-dev; portable Mac/Linux, CPU transcode)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev: $$COMPOSE_PROJECT_NAME — Tunarr http://localhost:$$TUNARR_DEV_PORT"; \
	  docker compose -p "$$COMPOSE_PROJECT_NAME" -f docker/compose.dev.yaml up -d

.PHONY: test-sso
test-sso: ## SSO against REAL Authelia + Authentik containers (requires Docker)
	@# Not in comprehensive verification: §19 keeps the default suite Docker-free, like test-pg.
	@#
	@# TWO providers, because each found a bug the other could not. Authelia: profile claims
	@# live at userinfo, not in the id_token, so every login against a default install was
	@# refused. Authentik: its issuer is path-based WITH a trailing slash, which our
	@# normalisation stripped — discovery failed outright. A hand-written stub IdP showed
	@# neither, because it was our own reading of the spec on both sides of the wire.
	$(GO) test -count=1 -tags=integration -timeout 20m -run 'TestSSO_AgainstReal' ./internal/auth/

.PHONY: dev-be
dev-be: rust-dev-build ## backend with live reload (Air) — rebuilds + restarts on Go/Rust changes
	@# Air is a dev tool, not a dependency (§14): run via `go run` so it is never added to
	@# go.mod and needs no manual install step. A committed .air.toml with no way to run it
	@# is how this box spent a session serving a stale binary.
	@#
	@# ⚠ SINGLE-INSTANCE GUARD (scripts/dev-be-guard.sh). Air itself has no "am I already
	@# running?" check, so a SECOND `make dev-be` used to start a second Air + binary that lost
	@# the :8080 bind and exited — while the stale one kept serving OLD code. That zombie cost
	@# DAYS of "my fix didn't take". The guard refuses to start a duplicate (or, with
	@# DEV_BE_REPLACE=1, cleanly replaces ONLY the loomarr dev binary — never a blanket kill).
	@eval "$$(./scripts/dev-env.sh export)"; \
	  mkdir -p .agent-data "$$LOOMARR_ARTIFACT_DIR" "$${LOOMARR_AGENT_FILLER_DIR:-.filler-drop}" "$${LOOMARR_AGENT_PREPARED_DIR:-.agent-data/prepared}"; \
	  echo "dev-be: $$LOOMARR_INSTANCE — http://localhost:$$LOOMARR_DEV_PORT"; \
	  sh scripts/dev-be-guard.sh; \
	  sh -c 'if [ "$${DEV_BE_NO_WATCHDOG:-0}" != "1" ]; then \
	      sh scripts/dev-be-watchdog.sh & wd=$$!; trap "kill $$wd 2>/dev/null" EXIT INT TERM; fi; \
	    exec $(GO) run github.com/air-verse/air@v1.67.4'
	@# ⚠ STALE-BINARY WATCHDOG (scripts/dev-be-watchdog.sh). Even with `.air.toml`'s
	@# stop_on_error=false + poll=true, Air can still end up ALIVE but not rebuilding (poll loop
	@# stalled) — serving a frozen binary while your saves do nothing. Config can't detect its own
	@# watcher dying; this out-of-band watchdog does. It runs beside Air, notices when the running
	@# binary stays older than the newest .go source, and self-heals (nudge Air, then restart the
	@# binary via Air's own path — never a competing process). Backgrounded here; the `trap` reaps
	@# it when Air exits so `make dev-be` leaves nothing behind. Opt out with DEV_BE_NO_WATCHDOG=1.

.PHONY: rust-dev-build
rust-dev-build: ## build the required Rust worker for local development
	LOOMARR_RELEASE=dev $(CARGO) build --locked -p loomarr-image
.PHONY: dev-gpu
dev-gpu: ## dev compose stack with NVIDIA transcode overlay (Linux + nvidia-container-toolkit)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev-gpu: $$COMPOSE_PROJECT_NAME — Tunarr http://localhost:$$TUNARR_DEV_PORT"; \
	  docker compose -p "$$COMPOSE_PROJECT_NAME" -f docker/compose.dev.yaml -f docker/compose.dev.gpu.yaml up -d

.PHONY: dev-fe
dev-fe: ## frontend with HMR on this worktree's isolated port, proxying its backend
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev-fe: $$LOOMARR_INSTANCE — http://localhost:$$LOOMARR_FE_PORT -> $$LOOMARR_API"; \
	  cd $(WEB) && pnpm --filter @loomarr/web dev

.PHONY: seed
seed: ## populate a dev store via the real domain paths (approval gate honored — AGENTS.md)
	DATABASE_URL=$${DATABASE_URL:-sqlite://./loomarr-dev.db} go run ./cmd/seed
