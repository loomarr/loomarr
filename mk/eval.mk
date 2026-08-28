eval-contract: ## hermetic semantic-evaluation contracts; never contacts a model, Library, or TMDB
	LOOMARR_EVAL_CONTRACT_ONLY=1 $(GO) test -tags=eval ./internal/eval/

eval: ## semantic eval: real intents → real LLM → scored (needs LLM_*/LIBRARY_*/TMDB_API_KEY; NOT in the hermetic gate)
	$(GO) test -tags=eval -v -timeout 20m ./internal/eval/

eval-cert: ## certify exact intents and mandatory scheduled viewer outcomes; fails closed and writes a scorecard
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${LOOMARR_EVAL_OUT:-$$LOOMARR_ARTIFACT_DIR/semantic-certification.json}"; \
	  mkdir -p "$$(dirname "$$report")"; \
	  LOOMARR_EVAL_REQUIRED=1 LOOMARR_EVAL_OUT="$$report" \
	    $(GO) test -count=1 -tags=eval -v -timeout 20m ./internal/eval/

eval-matrix: ## explicitly certify local + OpenRouter generation sequentially (manual, resource-heavy)
	@test -n "$$OPENROUTER_API_KEY" || { echo "eval-matrix: OPENROUTER_API_KEY is required" >&2; exit 2; }; \
	  test -n "$$OPENROUTER_MODEL" || { echo "eval-matrix: OPENROUTER_MODEL is required" >&2; exit 2; }; \
	  test -n "$$OPENROUTER_GENERATOR_PROVIDER" || { echo "eval-matrix: OPENROUTER_GENERATOR_PROVIDER is required" >&2; exit 2; }; \
	  test -n "$$OPENROUTER_JUDGE_PROVIDER" || { echo "eval-matrix: OPENROUTER_JUDGE_PROVIDER is required" >&2; exit 2; }; \
	  test "$$LOOMARR_EVAL_ALLOW_LOCAL" = "1" || { echo "eval-matrix: refusing local inference; confirm an idle host with sufficient RAM/VRAM, then set LOOMARR_EVAL_ALLOW_LOCAL=1" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  judge_model="$${OPENROUTER_JUDGE_MODEL:-$$OPENROUTER_MODEL}"; \
	  status=0; \
	  LOOMARR_EVAL_PROFILE=local \
	  LOOMARR_EVAL_OUT="$$LOOMARR_ARTIFACT_DIR/semantic-certification-local.json" \
	  LOOMARR_EVAL_JUDGE="$$judge_model" LOOMARR_EVAL_JUDGE_PROVIDER=openrouter \
	  LOOMARR_EVAL_JUDGE_UPSTREAM_PROVIDER="$$OPENROUTER_JUDGE_PROVIDER" \
	  LOOMARR_EVAL_JUDGE_URL=https://openrouter.ai/api/v1 \
	  LOOMARR_EVAL_JUDGE_API_KEY="$$OPENROUTER_API_KEY" \
	    $(MAKE) eval-cert || status=$$?; \
	  LLM_PROVIDER=openrouter LLM_URL=https://openrouter.ai/api/v1 \
	  LLM_MODEL="$$OPENROUTER_MODEL" LLM_API_KEY="$$OPENROUTER_API_KEY" \
	  LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER="$$OPENROUTER_GENERATOR_PROVIDER" \
	  LOOMARR_EVAL_PROFILE=openrouter \
	  LOOMARR_EVAL_OUT="$$LOOMARR_ARTIFACT_DIR/semantic-certification-openrouter.json" \
	  LOOMARR_EVAL_JUDGE="$$judge_model" LOOMARR_EVAL_JUDGE_PROVIDER=openrouter \
	  LOOMARR_EVAL_JUDGE_UPSTREAM_PROVIDER="$$OPENROUTER_JUDGE_PROVIDER" \
	  LOOMARR_EVAL_JUDGE_URL=https://openrouter.ai/api/v1 \
	  LOOMARR_EVAL_JUDGE_API_KEY="$$OPENROUTER_API_KEY" \
	    $(MAKE) eval-cert || status=$$?; \
	  exit "$$status"

filler-eval-contract: ## hermetic filler-admission corpus and selective-risk contracts
	$(GO) test ./internal/filleradmission/ ./internal/fillerbakeoff/ ./internal/fillercorpus/ ./internal/fillereval/ ./internal/fillerreview/ ./cmd/filler-bakeoff-ollama/ ./cmd/filler-bakeoff-openrouter/ ./cmd/filler-bakeoff-transcribe/ ./cmd/filler-cert/ ./cmd/filler-openrouter-snapshot/ ./cmd/filler-corpus/ ./cmd/filler-corpus-archive/ ./cmd/filler-corpus-commons/ ./cmd/filler-corpus-direct/ ./cmd/filler-corpus-download/ ./cmd/filler-corpus-inventory/ ./cmd/filler-corpus-loc/ ./cmd/filler-corpus-nasa/ ./cmd/filler-corpus-pages/ ./cmd/filler-corpus-pilot/ ./cmd/filler-corpus-pilot-rights-lock/ ./cmd/filler-corpus-pilot-rights-review/ ./cmd/filler-corpus-prepare/ ./cmd/filler-corpus-review/ ./cmd/filler-corpus-review-ollama/ ./cmd/filler-corpus-review-openrouter/ ./cmd/filler-corpus-rights-review/ ./cmd/filler-corpus-rights-lock/

filler-corpus-commons: ## freeze bounded Commons pilot and full-inventory artifacts
	@eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-commons \
	    --category "$${LOOMARR_FILLER_CORPUS_COMMONS_CATEGORY:-Advertising videos}" \
	    --role-hint "$${LOOMARR_FILLER_CORPUS_COMMONS_ROLE_HINT:-commercial}" \
	    --out "$${LOOMARR_FILLER_CORPUS_COMMONS_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-commons.json}" \
	    --inventory-out "$${LOOMARR_FILLER_CORPUS_COMMONS_INVENTORY_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-commons-inventory.json}" \
	    --cache-dir "$${LOOMARR_FILLER_CORPUS_COMMONS_CACHE:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-commons-cache}" \
	    --user-agent "$$LOOMARR_FILLER_CORPUS_USER_AGENT" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_COMMONS_SNAPSHOT_AT" \
	    --max-requests "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_REQUESTS:-10}" \
	    --max-pages "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_PAGES:-5}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_ITEMS:-10}" \
	    --max-response-bytes "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_RESPONSE_BYTES:-33554432}" \
	    --max-item-bytes "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_ITEM_BYTES:-536870912}" \
	    --max-total-bytes "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_TOTAL_BYTES:-3221225472}" \
	    --delay "$${LOOMARR_FILLER_CORPUS_COMMONS_DELAY:-250ms}" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_COMMONS_MAX_WALL_TIME:-2m}"

filler-corpus-cdc: ## freeze bounded CDC pilot and full-inventory artifacts
	@eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-pages \
	    --in internal/fillercorpus/corpus/seeds/cdc.json \
	    --out "$${LOOMARR_FILLER_CORPUS_CDC_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-cdc.json}" \
	    --inventory-out "$${LOOMARR_FILLER_CORPUS_CDC_INVENTORY_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-cdc-inventory.json}" \
	    --cache-dir "$${LOOMARR_FILLER_CORPUS_CDC_CACHE:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-cdc-cache}" \
	    --user-agent "$$LOOMARR_FILLER_CORPUS_USER_AGENT" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_CDC_SNAPSHOT_AT" \
	    --page-host www.cdc.gov \
	    --media-host www.cdc.gov \
	    --max-requests "$${LOOMARR_FILLER_CORPUS_CDC_MAX_REQUESTS:-20}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_CDC_MAX_ITEMS:-10}" \
	    --max-response-bytes "$${LOOMARR_FILLER_CORPUS_CDC_MAX_RESPONSE_BYTES:-16777216}" \
	    --max-item-bytes "$${LOOMARR_FILLER_CORPUS_CDC_MAX_ITEM_BYTES:-104857600}" \
	    --max-total-bytes "$${LOOMARR_FILLER_CORPUS_CDC_MAX_TOTAL_BYTES:-1073741824}" \
	    --delay "$${LOOMARR_FILLER_CORPUS_CDC_DELAY:-250ms}" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_CDC_MAX_WALL_TIME:-2m}"

filler-corpus-loc: ## freeze bounded LOC pilot and full-inventory artifacts
	@eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-loc \
	    --query "$$LOOMARR_FILLER_CORPUS_LOC_QUERY" \
	    --role-hint "$$LOOMARR_FILLER_CORPUS_LOC_ROLE_HINT" \
	    --out "$${LOOMARR_FILLER_CORPUS_LOC_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-loc.json}" \
	    --inventory-out "$${LOOMARR_FILLER_CORPUS_LOC_INVENTORY_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-loc-inventory.json}" \
	    --cache-dir "$${LOOMARR_FILLER_CORPUS_LOC_CACHE:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-loc-cache}" \
	    --user-agent "$$LOOMARR_FILLER_CORPUS_USER_AGENT" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_LOC_SNAPSHOT_AT" \
	    --max-requests "$${LOOMARR_FILLER_CORPUS_LOC_MAX_REQUESTS:-25}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_LOC_MAX_ITEMS:-10}" \
	    --max-response-bytes "$$LOOMARR_FILLER_CORPUS_LOC_MAX_RESPONSE_BYTES" \
	    --max-item-bytes "$$LOOMARR_FILLER_CORPUS_LOC_MAX_ITEM_BYTES" \
	    --max-total-bytes "$$LOOMARR_FILLER_CORPUS_LOC_MAX_TOTAL_BYTES" \
	    --delay "$${LOOMARR_FILLER_CORPUS_LOC_DELAY:-3100ms}" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_LOC_MAX_WALL_TIME:-3m}"

filler-corpus-nasa: ## freeze bounded NASA pilot and full-inventory artifacts
	@eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-nasa \
	    --query "$${LOOMARR_FILLER_CORPUS_NASA_QUERY:-trailer}" \
	    --role-hint "$${LOOMARR_FILLER_CORPUS_NASA_ROLE_HINT:-trailer}" \
	    --out "$${LOOMARR_FILLER_CORPUS_NASA_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-nasa.json}" \
	    --inventory-out "$${LOOMARR_FILLER_CORPUS_NASA_INVENTORY_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-nasa-inventory.json}" \
	    --cache-dir "$${LOOMARR_FILLER_CORPUS_NASA_CACHE:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-nasa-cache}" \
	    --user-agent "$$LOOMARR_FILLER_CORPUS_USER_AGENT" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_NASA_SNAPSHOT_AT" \
	    --max-requests "$${LOOMARR_FILLER_CORPUS_NASA_MAX_REQUESTS:-80}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_NASA_MAX_ITEMS:-10}" \
	    --max-response-bytes "$${LOOMARR_FILLER_CORPUS_NASA_MAX_RESPONSE_BYTES:-33554432}" \
	    --max-item-bytes "$${LOOMARR_FILLER_CORPUS_NASA_MAX_ITEM_BYTES:-536870912}" \
	    --max-total-bytes "$${LOOMARR_FILLER_CORPUS_NASA_MAX_TOTAL_BYTES:-3221225472}" \
	    --delay "$${LOOMARR_FILLER_CORPUS_NASA_DELAY:-250ms}" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_NASA_MAX_WALL_TIME:-2m}"

filler-corpus-pilot: ## lock the qualified metadata-only filler rights-yield pilot
	@test -n "$$LOOMARR_FILLER_CORPUS_PILOT_SNAPSHOT_AT" || { echo "filler-corpus-pilot: LOOMARR_FILLER_CORPUS_PILOT_SNAPSHOT_AT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PILOT_LOCKED_AT" || { echo "filler-corpus-pilot: LOOMARR_FILLER_CORPUS_PILOT_LOCKED_AT is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-pilot \
	    --lane internal/fillercorpus/corpus/pilot/prelinger.json \
	    --lane internal/fillercorpus/corpus/pilot/loc.json \
	    --lane internal/fillercorpus/corpus/pilot/nasa.json \
	    --lane internal/fillercorpus/corpus/pilot/cdc.json \
	    --lane internal/fillercorpus/corpus/pilot/commons.json \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_PILOT_SNAPSHOT_AT" \
	    --locked-at "$$LOOMARR_FILLER_CORPUS_PILOT_LOCKED_AT" \
	    --out "$${LOOMARR_FILLER_CORPUS_PILOT_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-pilot.json}"

filler-corpus-pilot-rights-review: ## prepare the inert five-lane pilot review packet
	@test -n "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_PREPARED_AT" || { echo "filler-corpus-pilot-rights-review: LOOMARR_FILLER_CORPUS_PILOT_REVIEW_PREPARED_AT is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-pilot-rights-review \
	    --pilot "$${LOOMARR_FILLER_CORPUS_PILOT:-internal/fillercorpus/corpus/pilot/locked.json}" \
	    --out "$${LOOMARR_FILLER_CORPUS_PILOT_REVIEW_WORKSHEET:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-pilot-rights-review.json}" \
	    --csv-out "$${LOOMARR_FILLER_CORPUS_PILOT_REVIEW_CSV:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-pilot-rights-review.csv}" \
	    --prepared-at "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_PREPARED_AT"

filler-corpus-pilot-rights-lock: ## lock completed pilot review into a non-authorizing yield report
	@test -n "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_WORKSHEET" || { echo "filler-corpus-pilot-rights-lock: LOOMARR_FILLER_CORPUS_PILOT_REVIEW_WORKSHEET is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_CSV" || { echo "filler-corpus-pilot-rights-lock: LOOMARR_FILLER_CORPUS_PILOT_REVIEW_CSV is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_LOCKED_AT" || { echo "filler-corpus-pilot-rights-lock: LOOMARR_FILLER_CORPUS_PILOT_REVIEW_LOCKED_AT is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-pilot-rights-lock \
	    --pilot "$${LOOMARR_FILLER_CORPUS_PILOT:-internal/fillercorpus/corpus/pilot/locked.json}" \
	    --worksheet "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_WORKSHEET" \
	    --completed-csv "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_CSV" \
	    --out "$${LOOMARR_FILLER_CORPUS_PILOT_REVIEW_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-pilot-rights-result.json}" \
	    --locked-at "$$LOOMARR_FILLER_CORPUS_PILOT_REVIEW_LOCKED_AT"

filler-corpus-archive: ## freeze a bounded rights-filtered Archive.org corpus inventory
	@test -n "$$LOOMARR_FILLER_CORPUS_ARCHIVE_COLLECTION" || { echo "filler-corpus-archive: LOOMARR_FILLER_CORPUS_ARCHIVE_COLLECTION is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-archive \
	    --collection "$$LOOMARR_FILLER_CORPUS_ARCHIVE_COLLECTION" \
	    --query "$$LOOMARR_FILLER_CORPUS_ARCHIVE_QUERY" \
	    --out "$${LOOMARR_FILLER_CORPUS_ARCHIVE_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-archive.json}" \
	    --pilot-out "$$LOOMARR_FILLER_CORPUS_ARCHIVE_PILOT_OUT" \
	    --role-hint "$$LOOMARR_FILLER_CORPUS_ARCHIVE_ROLE_HINT" \
	    --cache-dir "$${LOOMARR_FILLER_CORPUS_ARCHIVE_CACHE:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-archive-cache}" \
	    --user-agent "$${LOOMARR_FILLER_CORPUS_USER_AGENT:-$$LOOMARR_FILLER_CORPUS_ARCHIVE_USER_AGENT}" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_ARCHIVE_SNAPSHOT_AT" \
	    --max-requests "$$LOOMARR_FILLER_CORPUS_ARCHIVE_MAX_REQUESTS" \
	    --max-items "$$LOOMARR_FILLER_CORPUS_ARCHIVE_MAX_ITEMS" \
	    --max-item-bytes "$$LOOMARR_FILLER_CORPUS_ARCHIVE_MAX_ITEM_BYTES" \
	    --max-total-bytes "$$LOOMARR_FILLER_CORPUS_ARCHIVE_MAX_TOTAL_BYTES" \
	    --delay "$${LOOMARR_FILLER_CORPUS_ARCHIVE_DELAY:-1s}" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_ARCHIVE_MAX_WALL_TIME:-1m}"

filler-corpus-inventory: ## combine strict source inventories for mixed-authority rights review
	@test -n "$$LOOMARR_FILLER_CORPUS_INVENTORIES" || { echo "filler-corpus-inventory: LOOMARR_FILLER_CORPUS_INVENTORIES is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  set --; \
	  for path in $$LOOMARR_FILLER_CORPUS_INVENTORIES; do set -- "$$@" --inventory "$$path"; done; \
	  $(GO) run ./cmd/filler-corpus-inventory "$$@" \
	    --out "$${LOOMARR_FILLER_CORPUS_INVENTORY:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-inventory.json}"

filler-corpus-direct: ## freeze an authored local cohort with rights and provenance evidence
	@test -n "$$LOOMARR_FILLER_CORPUS_DIRECT_MANIFEST" || { echo "filler-corpus-direct: LOOMARR_FILLER_CORPUS_DIRECT_MANIFEST is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_DIRECT_ROOT" || { echo "filler-corpus-direct: LOOMARR_FILLER_CORPUS_DIRECT_ROOT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_DIRECT_SNAPSHOT_AT" || { echo "filler-corpus-direct: LOOMARR_FILLER_CORPUS_DIRECT_SNAPSHOT_AT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_DIRECT_ITEMS" || { echo "filler-corpus-direct: LOOMARR_FILLER_CORPUS_DIRECT_ITEMS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_DIRECT_MAX_BYTES" || { echo "filler-corpus-direct: LOOMARR_FILLER_CORPUS_DIRECT_MAX_BYTES is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-direct \
	    --manifest "$$LOOMARR_FILLER_CORPUS_DIRECT_MANIFEST" \
	    --root "$$LOOMARR_FILLER_CORPUS_DIRECT_ROOT" \
	    --out "$${LOOMARR_FILLER_CORPUS_DIRECT_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-direct.json}" \
	    --snapshot-at "$$LOOMARR_FILLER_CORPUS_DIRECT_SNAPSHOT_AT" \
	    --expected-items "$$LOOMARR_FILLER_CORPUS_DIRECT_ITEMS" \
	    --max-bytes "$$LOOMARR_FILLER_CORPUS_DIRECT_MAX_BYTES" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_DIRECT_MAX_WALL_TIME:-1m}"

filler-corpus-prepare: ## build an unlabeled corpus draft and bounded evidence packets
	@test -n "$$LOOMARR_FILLER_CORPUS_PROFILE" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PROFILE is required" >&2; exit 2; }; \
	  case "$$LOOMARR_FILLER_CORPUS_PROFILE" in \
	    development) default_min=300; default_max=300 ;; \
	    certification) default_min=1426; default_max=1600 ;; \
	    *) echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PROFILE must be development or certification" >&2; exit 2 ;; \
	  esac; \
	  test -n "$$LOOMARR_FILLER_CORPUS_INVENTORY" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_INVENTORY is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_RIGHTS_APPROVALS" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_RIGHTS_APPROVALS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PREPARATION_PLAN" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PREPARATION_PLAN is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_LOCAL_ROOT" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_LOCAL_ROOT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_MEDIA_DIR" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_MEDIA_DIR is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PREPARED_AT" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PREPARED_AT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PREP_MAX_INPUT_BYTES" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PREP_MAX_INPUT_BYTES is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_PREP_MAX_OUTPUT_BYTES" || { echo "filler-corpus-prepare: LOOMARR_FILLER_CORPUS_PREP_MAX_OUTPUT_BYTES is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-prepare \
	    --profile "$$LOOMARR_FILLER_CORPUS_PROFILE" \
	    --inventory "$$LOOMARR_FILLER_CORPUS_INVENTORY" \
	    --rights-approvals "$$LOOMARR_FILLER_CORPUS_RIGHTS_APPROVALS" \
	    --plan "$$LOOMARR_FILLER_CORPUS_PREPARATION_PLAN" \
	    --local-root "$$LOOMARR_FILLER_CORPUS_LOCAL_ROOT" \
	    --remote-root "$$LOOMARR_FILLER_CORPUS_MEDIA_DIR" \
	    --draft-out "$${LOOMARR_FILLER_CORPUS_DRAFT:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-draft.json}" \
	    --packets-out "$${LOOMARR_FILLER_CORPUS_PACKETS:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-packets.jsonl}" \
	    --derivatives-root "$${LOOMARR_FILLER_CORPUS_DERIVATIVES:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-derivatives}" \
	    --prepared-at "$$LOOMARR_FILLER_CORPUS_PREPARED_AT" \
	    --ffmpeg "$${LOOMARR_FILLER_CORPUS_FFMPEG:-ffmpeg}" \
	    --min-items "$${LOOMARR_FILLER_CORPUS_PREP_MIN_ITEMS:-$$default_min}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_PREP_MAX_ITEMS:-$$default_max}" \
	    --max-input-bytes "$$LOOMARR_FILLER_CORPUS_PREP_MAX_INPUT_BYTES" \
	    --max-output-bytes "$$LOOMARR_FILLER_CORPUS_PREP_MAX_OUTPUT_BYTES" \
	    --max-wall-time "$${LOOMARR_FILLER_CORPUS_PREP_MAX_WALL_TIME:-6h}"

filler-corpus-download: ## download only rights-approved corpus media under hard ceilings
	@eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-download \
	    --inventory "$$LOOMARR_FILLER_CORPUS_INVENTORY" \
	    --rights-approvals "$$LOOMARR_FILLER_CORPUS_RIGHTS_APPROVALS" \
	    --out-dir "$$LOOMARR_FILLER_CORPUS_MEDIA_DIR" \
	    --ledger "$$LOOMARR_FILLER_CORPUS_DOWNLOAD_LEDGER" \
	    --user-agent "$${LOOMARR_FILLER_CORPUS_USER_AGENT:-$$LOOMARR_FILLER_CORPUS_ARCHIVE_USER_AGENT}" \
	    --generated-at "$$LOOMARR_FILLER_CORPUS_DOWNLOAD_GENERATED_AT" \
	    --max-requests "$$LOOMARR_FILLER_CORPUS_DOWNLOAD_MAX_REQUESTS" \
	    --max-items "$$LOOMARR_FILLER_CORPUS_DOWNLOAD_MAX_ITEMS" \
	    --max-bytes "$$LOOMARR_FILLER_CORPUS_DOWNLOAD_MAX_BYTES" \
	    --delay "$${LOOMARR_FILLER_CORPUS_DOWNLOAD_DELAY:-1s}"

filler-corpus-rights-review: ## prepare an inert worksheet from a frozen filler inventory
	@test -n "$$LOOMARR_FILLER_CORPUS_INVENTORY" || { echo "filler-corpus-rights-review: LOOMARR_FILLER_CORPUS_INVENTORY is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-rights-review \
	    --inventory "$$LOOMARR_FILLER_CORPUS_INVENTORY" \
	    --out "$${LOOMARR_FILLER_CORPUS_RIGHTS_WORKSHEET:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-rights-review.json}" \
	    --csv-out "$${LOOMARR_FILLER_CORPUS_RIGHTS_CSV:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-rights-review.csv}" \
	    --prepared-at "$$LOOMARR_FILLER_CORPUS_RIGHTS_PREPARED_AT" \
	    --min-items "$${LOOMARR_FILLER_CORPUS_RIGHTS_MIN_ITEMS:-1426}" \
	    --max-items "$${LOOMARR_FILLER_CORPUS_RIGHTS_MAX_ITEMS:-1600}"

filler-corpus-rights-lock: ## validate completed rights review CSV into approval JSONL
	@test -n "$$LOOMARR_FILLER_CORPUS_INVENTORY" || { echo "filler-corpus-rights-lock: LOOMARR_FILLER_CORPUS_INVENTORY is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_RIGHTS_WORKSHEET" || { echo "filler-corpus-rights-lock: LOOMARR_FILLER_CORPUS_RIGHTS_WORKSHEET is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_RIGHTS_CSV" || { echo "filler-corpus-rights-lock: LOOMARR_FILLER_CORPUS_RIGHTS_CSV is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-rights-lock \
	    --inventory "$$LOOMARR_FILLER_CORPUS_INVENTORY" \
	    --worksheet "$$LOOMARR_FILLER_CORPUS_RIGHTS_WORKSHEET" \
	    --completed-csv "$$LOOMARR_FILLER_CORPUS_RIGHTS_CSV" \
	    --approvals-out "$${LOOMARR_FILLER_CORPUS_RIGHTS_APPROVALS:-$$LOOMARR_ARTIFACT_DIR/filler-corpus-rights-approvals.jsonl}" \
	    --locked-at "$$LOOMARR_FILLER_CORPUS_RIGHTS_LOCKED_AT"

filler-corpus-lock: ## lock two blind filler-label batches into a certification manifest
	@test -n "$$LOOMARR_FILLER_CORPUS_DRAFT" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_DRAFT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_A" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_REVIEW_A is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP_A" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_REVIEW_MAP_A is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_B" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_REVIEW_B is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP_B" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_REVIEW_MAP_B is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_LOCKED_AT" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_LOCKED_AT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_OUT" || { echo "filler-corpus-lock: LOOMARR_FILLER_CORPUS_OUT is required" >&2; exit 2; }; \
	  $(GO) run ./cmd/filler-corpus \
	    --draft "$$LOOMARR_FILLER_CORPUS_DRAFT" \
	    --review-a "$$LOOMARR_FILLER_CORPUS_REVIEW_A" \
	    --map-a "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP_A" \
	    --review-b "$$LOOMARR_FILLER_CORPUS_REVIEW_B" \
	    --map-b "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP_B" \
	    --adjudications "$$LOOMARR_FILLER_CORPUS_ADJUDICATIONS" \
	    --locked-at "$$LOOMARR_FILLER_CORPUS_LOCKED_AT" \
	    --out "$$LOOMARR_FILLER_CORPUS_OUT"

filler-corpus-review: ## prepare one opaque randomized filler-label review batch
	@test -n "$$LOOMARR_FILLER_CORPUS_DRAFT" || { echo "filler-corpus-review: LOOMARR_FILLER_CORPUS_DRAFT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_BATCH" || { echo "filler-corpus-review: LOOMARR_FILLER_CORPUS_REVIEW_BATCH is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKET" || { echo "filler-corpus-review: LOOMARR_FILLER_CORPUS_REVIEW_PACKET is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP" || { echo "filler-corpus-review: LOOMARR_FILLER_CORPUS_REVIEW_MAP is required" >&2; exit 2; }; \
	  $(GO) run ./cmd/filler-corpus-review \
	    --draft "$$LOOMARR_FILLER_CORPUS_DRAFT" \
	    --batch-id "$$LOOMARR_FILLER_CORPUS_REVIEW_BATCH" \
	    --packet-out "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKET" \
	    --map-out "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP"

filler-corpus-review-package: ## materialize one verified identity-blind reviewer evidence package
	@test -n "$$LOOMARR_FILLER_CORPUS_DRAFT" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_DRAFT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKET" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_REVIEW_PACKET is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_REVIEW_MAP is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_EVIDENCE_PACKETS" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_EVIDENCE_PACKETS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_DERIVATIVES" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_DERIVATIVES is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKAGE" || { echo "filler-corpus-review-package: LOOMARR_FILLER_CORPUS_REVIEW_PACKAGE is required" >&2; exit 2; }; \
	  $(GO) run ./cmd/filler-corpus-review-package \
	    --draft "$$LOOMARR_FILLER_CORPUS_DRAFT" \
	    --review-packet "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKET" \
	    --alias-map "$$LOOMARR_FILLER_CORPUS_REVIEW_MAP" \
	    --evidence-packets "$$LOOMARR_FILLER_CORPUS_EVIDENCE_PACKETS" \
	    --corpus-root "$$LOOMARR_FILLER_CORPUS_DERIVATIVES" \
	    --out "$$LOOMARR_FILLER_CORPUS_REVIEW_PACKAGE" \
	    --materialize "$${LOOMARR_FILLER_CORPUS_REVIEW_MATERIALIZE:-hardlink}"

filler-corpus-review-ollama: ## complete one blind package with a digest-pinned local reviewer
	@test -n "$$LOOMARR_FILLER_REVIEW_PACKAGE" || { echo "filler-corpus-review-ollama: LOOMARR_FILLER_REVIEW_PACKAGE is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_TRANSCRIPTS" || { echo "filler-corpus-review-ollama: LOOMARR_FILLER_REVIEW_TRANSCRIPTS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_MODEL" || { echo "filler-corpus-review-ollama: LOOMARR_FILLER_REVIEW_MODEL is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_MODEL_DIGEST" || { echo "filler-corpus-review-ollama: LOOMARR_FILLER_REVIEW_MODEL_DIGEST is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEWER_ID" || { echo "filler-corpus-review-ollama: LOOMARR_FILLER_REVIEWER_ID is required" >&2; exit 2; }; \
	  $(GO) run ./cmd/filler-corpus-review-ollama \
	    --package "$$LOOMARR_FILLER_REVIEW_PACKAGE" \
	    --transcripts "$$LOOMARR_FILLER_REVIEW_TRANSCRIPTS" \
	    --model "$$LOOMARR_FILLER_REVIEW_MODEL" \
	    --model-digest "$$LOOMARR_FILLER_REVIEW_MODEL_DIGEST" \
	    --reviewer-id "$$LOOMARR_FILLER_REVIEWER_ID" \
	    --expected-cases "$${LOOMARR_FILLER_REVIEW_EXPECTED_CASES:-300}" \
	    --per-case-timeout "$${LOOMARR_FILLER_REVIEW_CASE_TIMEOUT:-5m}" \
	    --base-url "$${LOOMARR_FILLER_REVIEW_BASE_URL:-http://127.0.0.1:11434}" \
	    --out "$${LOOMARR_FILLER_REVIEW_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-completed-review}"

filler-corpus-review-openrouter: ## complete one blind package through a bounded pinned hosted reviewer
	@test -n "$$OPENROUTER_API_KEY" || { echo "filler-corpus-review-openrouter: OPENROUTER_API_KEY is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_PACKAGE" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_PACKAGE is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_TRANSCRIPTS" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_TRANSCRIPTS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_SNAPSHOT" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_SNAPSHOT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_MODEL" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_MODEL is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_PROVIDER" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_PROVIDER is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_PROVIDER_SLUG" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_PROVIDER_SLUG is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEWER_ID" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEWER_ID is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_MAX_SPEND_NANOUSD" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_MAX_SPEND_NANOUSD is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_REVIEW_MAX_CHARGE_NANOUSD" || { echo "filler-corpus-review-openrouter: LOOMARR_FILLER_REVIEW_MAX_CHARGE_NANOUSD is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-corpus-review-openrouter \
	    --package "$$LOOMARR_FILLER_REVIEW_PACKAGE" \
	    --transcripts "$$LOOMARR_FILLER_REVIEW_TRANSCRIPTS" \
	    --snapshot "$$LOOMARR_FILLER_REVIEW_SNAPSHOT" \
	    --model "$$LOOMARR_FILLER_REVIEW_MODEL" \
	    --provider "$$LOOMARR_FILLER_REVIEW_PROVIDER" \
	    --provider-slug "$$LOOMARR_FILLER_REVIEW_PROVIDER_SLUG" \
	    --reviewer-id "$$LOOMARR_FILLER_REVIEWER_ID" \
	    --expected-cases "$${LOOMARR_FILLER_REVIEW_EXPECTED_CASES:-300}" \
	    --max-requests "$${LOOMARR_FILLER_REVIEW_MAX_REQUESTS:-301}" \
	    --max-spend-nanousd "$$LOOMARR_FILLER_REVIEW_MAX_SPEND_NANOUSD" \
	    --max-charge-nanousd "$$LOOMARR_FILLER_REVIEW_MAX_CHARGE_NANOUSD" \
	    --per-case-timeout "$${LOOMARR_FILLER_REVIEW_CASE_TIMEOUT:-5m}" \
	    --base-url "$${LOOMARR_FILLER_REVIEW_BASE_URL:-https://openrouter.ai/api/v1}" \
	    --out "$${LOOMARR_FILLER_REVIEW_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-completed-review}"

filler-openrouter-snapshot: ## lock OpenRouter capability, endpoint-price, and ZDR metadata
	@test -n "$$OPENROUTER_API_KEY" || { echo "filler-openrouter-snapshot: OPENROUTER_API_KEY is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_OPENROUTER_MODELS" || { echo "filler-openrouter-snapshot: LOOMARR_FILLER_OPENROUTER_MODELS is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-openrouter-snapshot \
	    --models "$$LOOMARR_FILLER_OPENROUTER_MODELS" \
	    --out "$${LOOMARR_FILLER_OPENROUTER_SNAPSHOT:-$$LOOMARR_ARTIFACT_DIR/filler-openrouter-snapshot.json}" \
	    --base-url "$${LOOMARR_FILLER_BAKEOFF_BASE_URL:-https://openrouter.ai/api/v1}"

filler-bakeoff-openrouter: ## capture a bounded label-blind OpenRouter prediction ledger (paid/manual)
	@test -n "$$OPENROUTER_API_KEY" || { echo "filler-bakeoff-openrouter: OPENROUTER_API_KEY is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" || { echo "filler-bakeoff-openrouter: LOOMARR_FILLER_BAKEOFF_MANIFEST is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_PACKETS" || { echo "filler-bakeoff-openrouter: LOOMARR_FILLER_BAKEOFF_PACKETS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CONFIG" || { echo "filler-bakeoff-openrouter: LOOMARR_FILLER_BAKEOFF_CONFIG is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_SNAPSHOT" || { echo "filler-bakeoff-openrouter: LOOMARR_FILLER_BAKEOFF_SNAPSHOT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" || { echo "filler-bakeoff-openrouter: LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-bakeoff-openrouter \
	    --manifest "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" \
	    --packets "$$LOOMARR_FILLER_BAKEOFF_PACKETS" \
	    --config "$$LOOMARR_FILLER_BAKEOFF_CONFIG" \
	    --snapshot "$$LOOMARR_FILLER_BAKEOFF_SNAPSHOT" \
	    --corpus-root "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" \
	    --transcripts "$${LOOMARR_FILLER_BAKEOFF_TRANSCRIPTS:-}" \
	    --predictions "$${LOOMARR_FILLER_BAKEOFF_PREDICTIONS:-$$LOOMARR_ARTIFACT_DIR/filler-bakeoff-predictions.jsonl}" \
	    --base-url "$${LOOMARR_FILLER_BAKEOFF_BASE_URL:-https://openrouter.ai/api/v1}"

filler-bakeoff-ollama: ## capture a digest-pinned local filler prediction ledger (manual)
	@test -n "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" || { echo "filler-bakeoff-ollama: LOOMARR_FILLER_BAKEOFF_MANIFEST is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_PACKETS" || { echo "filler-bakeoff-ollama: LOOMARR_FILLER_BAKEOFF_PACKETS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CONFIG" || { echo "filler-bakeoff-ollama: LOOMARR_FILLER_BAKEOFF_CONFIG is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" || { echo "filler-bakeoff-ollama: LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT is required" >&2; exit 2; }; \
	  $(GO) run ./cmd/filler-bakeoff-ollama \
	    --manifest "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" \
	    --packets "$$LOOMARR_FILLER_BAKEOFF_PACKETS" \
	    --config "$$LOOMARR_FILLER_BAKEOFF_CONFIG" \
	    --corpus-root "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" \
	    --transcripts "$${LOOMARR_FILLER_BAKEOFF_TRANSCRIPTS:-}" \
	    --predictions "$${LOOMARR_FILLER_BAKEOFF_PREDICTIONS:-$$LOOMARR_ARTIFACT_DIR/filler-bakeoff-ollama-predictions.jsonl}" \
	    --base-url "$${LOOMARR_FILLER_BAKEOFF_BASE_URL:-http://127.0.0.1:11434}"

filler-bakeoff-transcribe: ## capture digest-pinned shared filler transcripts (manual)
	@test -n "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_BAKEOFF_MANIFEST is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_PACKETS" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_BAKEOFF_PACKETS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CONFIG" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_BAKEOFF_CONFIG is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_WHISPER_PATH" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_WHISPER_PATH is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_WHISPER_MODEL" || { echo "filler-bakeoff-transcribe: LOOMARR_FILLER_WHISPER_MODEL is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  $(GO) run ./cmd/filler-bakeoff-transcribe \
	    --manifest "$$LOOMARR_FILLER_BAKEOFF_MANIFEST" \
	    --packets "$$LOOMARR_FILLER_BAKEOFF_PACKETS" \
	    --config "$$LOOMARR_FILLER_BAKEOFF_CONFIG" \
	    --corpus-root "$$LOOMARR_FILLER_BAKEOFF_CORPUS_ROOT" \
	    --whisper "$$LOOMARR_FILLER_WHISPER_PATH" \
	    --model "$$LOOMARR_FILLER_WHISPER_MODEL" \
	    --transcripts "$${LOOMARR_FILLER_BAKEOFF_TRANSCRIPTS:-$$LOOMARR_ARTIFACT_DIR/filler-bakeoff-transcripts.jsonl}"

filler-eval-cert: ## score captured filler decisions; never contacts a model or media source
	@test -n "$$LOOMARR_FILLER_EVAL_PREDICTIONS" || { echo "filler-eval-cert: LOOMARR_FILLER_EVAL_PREDICTIONS is required" >&2; exit 2; }; \
	  test -n "$$LOOMARR_FILLER_EVAL_GENERATED_AT" || { echo "filler-eval-cert: LOOMARR_FILLER_EVAL_GENERATED_AT is required" >&2; exit 2; }; \
	  test "$${LOOMARR_FILLER_EVAL_MAX_REQUESTS:-0}" -gt 0 || { echo "filler-eval-cert: positive LOOMARR_FILLER_EVAL_MAX_REQUESTS is required" >&2; exit 2; }; \
	  test "$${LOOMARR_FILLER_EVAL_MAX_SPEND_NANO_USD:-0}" -gt 0 || { echo "filler-eval-cert: positive LOOMARR_FILLER_EVAL_MAX_SPEND_NANO_USD is required" >&2; exit 2; }; \
	  test "$${LOOMARR_FILLER_EVAL_MAX_CONCURRENCY:-0}" -gt 0 || { echo "filler-eval-cert: positive LOOMARR_FILLER_EVAL_MAX_CONCURRENCY is required" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${LOOMARR_FILLER_EVAL_OUT:-$$LOOMARR_ARTIFACT_DIR/filler-certification.json}"; \
	  $(GO) run ./cmd/filler-cert \
	    --manifest "$${LOOMARR_FILLER_EVAL_MANIFEST:-internal/fillereval/corpus/seed-v1.json}" \
	    --predictions "$$LOOMARR_FILLER_EVAL_PREDICTIONS" --report "$$report" \
	    --profile "$${LOOMARR_FILLER_EVAL_PROFILE:-replay}" \
	    --split "$${LOOMARR_FILLER_EVAL_SPLIT:-holdout}" \
	    --evidence-version "$$LOOMARR_FILLER_EVAL_EVIDENCE_VERSION" \
	    --prompt-version "$$LOOMARR_FILLER_EVAL_PROMPT_VERSION" \
	    --taxonomy-version "$$LOOMARR_FILLER_EVAL_TAXONOMY_VERSION" \
	    --policy-version "$$LOOMARR_FILLER_EVAL_POLICY_VERSION" \
	    --role-policy-version "$$LOOMARR_FILLER_EVAL_ROLE_POLICY_VERSION" \
	    --capability-snapshot "$$LOOMARR_FILLER_EVAL_CAPABILITY_SNAPSHOT" \
	    --price-snapshot "$$LOOMARR_FILLER_EVAL_PRICE_SNAPSHOT" \
	    --generated-at "$$LOOMARR_FILLER_EVAL_GENERATED_AT" \
	    --max-requests "$$LOOMARR_FILLER_EVAL_MAX_REQUESTS" \
	    --max-spend-nano-usd "$$LOOMARR_FILLER_EVAL_MAX_SPEND_NANO_USD" \
	    --max-concurrency "$$LOOMARR_FILLER_EVAL_MAX_CONCURRENCY"
