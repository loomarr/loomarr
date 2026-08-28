## ---- store conformance (Phase 3/4) --------------------------------------

.PHONY: ensure-postgres-test-image
ensure-postgres-test-image: ## use cached Postgres and Ryuk images or pull them with bounded retries
	./scripts/ensure-container-image.sh "$$(cat internal/testkit/postgresimage/image.txt)"
	./scripts/ensure-container-image.sh "$$(cat internal/testkit/ryukimage/image.txt)"

.PHONY: test-pg
test-pg: rust-dev-build ensure-postgres-test-image ## all real-Postgres integration suites (store, backend transition, app; testcontainers; requires Docker)
# ⚠ The `-run TestPostgresConformance` filter this used to carry meant every OTHER integration test
# in the package compiled and never ran — including TestMigrateSQLiteToPostgres, which its own file
# header calls "the V11 gate", plus TestMigrateCoversEveryTable and the three TestPreflight* tests.
# A filter is invisible in the output: the target printed a genuine pass and said nothing about the
# six tests it had not selected. The migrator was broken the whole time (seeded destination rows
# collided on insert) and no gate could have told anyone.
#
# This is the third variant of "green that proves nothing" this repo has hit — after a pipe masking
# an exit code, and a missing -tags=integration printing `ok … [no tests to run]`. A test existing,
# compiling, and EXECUTING are three separate facts.
	TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io TESTCONTAINERS_RYUK_DISABLED=false $(GO) test -race -tags=integration ./internal/store/ ./internal/backendtransition/ ./internal/app/
