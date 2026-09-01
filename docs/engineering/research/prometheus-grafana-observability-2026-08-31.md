# Prometheus and Grafana observability overhaul

Tracking: [#751](https://github.com/loomarr/loomarr/issues/751)  
Date: 2026-08-31

## Decision

Keep Prometheus as Loomarr's only application telemetry protocol, deepen `internal/metrics` into the
owner of a documented and executable metric contract, and ship one source-controlled Grafana
overview dashboard plus optional Prometheus recording and alert examples. Do **not** make Prometheus
or Grafana part of Loomarr's required runtime and do not add either container to the default Compose
topology. Operators commonly already have an observability stack; Loomarr should provide portable
artifacts that work with it without owning its retention, credentials, upgrades, or host exposure.

The dashboard is a supported operational view, not the source of metric semantics. Its queries,
recording rules, and alerts must be validated against the same metric contract that tests the scrape
endpoint. Start with one overview rather than a gallery: multiple dashboards would multiply query and
compatibility maintenance before distinct operator workflows justify them.

## Current surface

`GET /v1/metrics` and its permanent `/metrics` alias expose the default Go registry through a public
LAN route ([ops.go](../../../internal/api/ops.go)). The registry includes Go and process collectors
plus these Loomarr families:

| Family | Type | Labels | Operator question |
| --- | --- | --- | --- |
| `loomarr_http_requests_total` | counter | `method`, `route`, `code` | What traffic and errors reach each route? |
| `loomarr_http_request_duration_seconds` | histogram | `method`, `route` | How long do requests take? |
| `loomarr_http_requests_in_flight` | gauge | none | Is HTTP serving saturated? |
| `loomarr_http_outbound_fanout` | histogram | `method`, `route` | Which inbound routes fan out excessively? |
| `loomarr_outbound_requests_total` | counter | `target`, `code` | Which dependencies fail? |
| `loomarr_outbound_request_duration_seconds` | histogram | `target` | Which dependencies are slow? |
| `loomarr_outbound_retries_total` | counter | `target`, `reason` | Where are retries hiding instability? |
| `loomarr_channel_reconciles_total` | counter | `result` | Do channel reconciles succeed? |
| `loomarr_channel_reconcile_duration_seconds` | histogram | none | How long does one reconcile take? |
| `loomarr_channel_slot_substitutions_total` | counter | none | How often does library drift replace programming? |
| `loomarr_auth_logins_total` | counter | `result` | Are login attempts succeeding? |
| `loomarr_metrics_scrape_errors_total` | counter | `source` | Did store-backed collection fail? |
| `loomarr_titles` | gauge | `state` | How many acquisition records occupy each state? |
| `loomarr_jobs` | gauge | `status` | How many proposal jobs occupy each state? |
| `loomarr_active_sessions` | gauge | none | How many unexpired sessions exist? |
| `loomarr_proposal_job_oldest_age_seconds` | gauge | `status` | How long has the oldest proposal job waited? |
| `loomarr_proposal_job_attempts` | gauge | `outcome` | What retained attempt outcomes exist? |
| `loomarr_proposal_job_failures` | gauge | `code` | Why did retained proposal jobs fail? |
| `loomarr_llm_tokens_total` | counter | `kind` | How many prompt/completion tokens were consumed? |
| `loomarr_filler_pods_total` | counter | `match_level` | How often does filler selection degrade? |
| `loomarr_filler_rotation_airings_total` | counter | `repeat`, `cooldown` | How healthy is filler rotation? |
| `loomarr_image_worker_operations_total` | counter | `kind`, `result` | Do image operations succeed? |
| `loomarr_image_worker_duration_seconds` | histogram | `kind` | How long do image processes run? |
| `loomarr_image_worker_input_bytes` | histogram | `kind` | What input sizes reach image processing? |
| `loomarr_image_worker_output_bytes` | histogram | `kind` | What output sizes leave image processing? |
| `loomarr_image_worker_peak_rss_bytes` | histogram | `kind` | What peak memory does image processing use? |
| `loomarr_image_worker_queue_wait_seconds` | histogram | `class` | Is image capacity delaying work? |
| `loomarr_image_worker_in_flight` | gauge | none | How many image processes hold capacity? |

The implementation is concentrated in [internal/metrics](../../../internal/metrics), with natural
call sites at HTTP, outbound transport, authentication, LLM, reconciliation, filler, playout, and
image-process seams. Baseline tests pass in `internal/metrics`, `internal/api`, and `internal/app`.

## Findings

### 1. The documented contract has drifted

Design §17 still promises requester/webhook, acquisition decision, grounding, janitor, diagnostics,
process-output, filler-catalog, and per-channel slot signals that the current constructor inventory
does not emit. The webhook subsystem and its metric were deliberately retired, but the design list
was not reconciled. Package comments also claim already-shipped latency and domain work remains
deferred and point to a nonexistent `docs/help/runbook`.

This is more dangerous than a missing chart: operators cannot tell whether a silent series means
zero, broken wiring, or a metric that never existed. Replace the prose wish list with a table that
classifies every family as shipped, deprecated, or deliberately deferred.

### 2. Metric construction is global while the product lifecycle is generation-scoped

All ordinary collectors are `promauto` process singletons. The store collector needs special atomic
rebinding when an app generation restarts so the process-global registry does not retain a closed
store. Tests use deltas against global counters because state leaks between test servers.

Deepen the metrics module around an owned `Recorder`: it owns one registry, the handler, middleware,
typed semantic recorders, Go/process collectors, and store collection. The composition root creates
one recorder per generation and injects its narrow callbacks/adapters. Callers continue to know
semantic events, not Prometheus collectors or label strings. Deleting this module would redistribute
registry lifecycle, label bounding, zero initialization, and exposition logic across every caller;
that is useful depth rather than wrapper indirection.

### 3. Several bounded-label claims are not enforced at the module interface

HTTP routes are matched templates and retry reasons are an enum, but outbound `target`, filler
`match_level`, image `kind`/`result`, and image admission `class` enter the metrics module as strings.
Today their callers happen to use closed values. A future caller can silently create unbounded
series. Prometheus warns that every label combination is a new time series and recommends examining
alternatives when cardinality can exceed 100; most metrics should remain below ten labelsets
([metric naming](https://prometheus.io/docs/practices/naming/),
[instrumentation](https://prometheus.io/docs/practices/instrumentation/)).

Make these dimensions typed or classify unknown inputs into one `other` value inside the metrics
module. Add contract tests with deliberately hostile identifiers, URLs, errors, user data, media ids,
channel ids, and filesystem paths and prove none appear in gathered label values.

### 4. Known series are often absent until the first event

Store-backed gauges zero-fill known states, but labelled counters and histograms do not initialize
their closed combinations. Before the first login, retry, reconcile, filler event, or LLM call,
simple queries return an absent vector rather than zero. Prometheus explicitly recommends exporting
default zeroes for known series ([instrumentation](https://prometheus.io/docs/practices/instrumentation/#avoid-missing-metrics)).

Initialize closed counter labelsets when a recorder is built. Do not manufacture histogram
observations: dashboards should use zero-safe queries for histograms with no samples.

### 5. Names mostly follow Prometheus conventions, but the state gauges need clarity

The application prefix, seconds, bytes, and counter `_total` suffixes are sound. Prometheus recommends
one quantity and base unit per family, with unit and type visible in the name
([naming](https://prometheus.io/docs/practices/naming/)). `loomarr_titles`, `loomarr_jobs`,
`loomarr_proposal_job_attempts`, and `loomarr_proposal_job_failures` are retained object counts, but
their names do not say so. Rename them to explicit `_current`/`_info`-free gauge names such as
`loomarr_acquisition_titles_current` and `loomarr_proposal_jobs_current` only with a compatibility
window; dashboards must not guess whether a count is cumulative.

For oldest work, prefer exporting the oldest creation timestamp and deriving age as
`time() - timestamp`. Prometheus recommends timestamps rather than continuously maintained
time-since gauges because the query stays correct even if update logic stalls
([instrumentation](https://prometheus.io/docs/practices/instrumentation/#timestamps-not-time-since)).

### 6. The most valuable missing signals are operational, not a mirror of every domain event

Prioritize questions that lead to action:

1. Build and backend identity: a single `loomarr_build_info` series with bounded version, revision,
   and database backend labels.
2. Database saturation: open/in-use/idle connections, waits, wait duration, max-open, and close
   counts from `database/sql.DBStats`.
3. Scheduler health: executions and duration by code-defined job name/outcome, running jobs, and
   last-success timestamps. Job names are a closed registry; errors never become labels.
4. Playout health: active streams, starts by bounded result, transcode/process failures by bounded
   stage, and fallback/offline transitions. Never label by channel, programme, client, or file.
5. Acquisition pipeline: current state already exists; add transition/outcome counters only where
   they distinguish stuck work from ordinary waiting.
6. Diagnostics retention/drop and notification delivery only when each has a concrete runbook action.

Do not implement the old design list mechanically. Prometheus recommends request count, errors, and
latency for online systems and start/running/last-success/completion signals for offline and batch
work ([instrumentation](https://prometheus.io/docs/practices/instrumentation/)).

### 7. Public scrape access is an explicit deployment contract

The supported Traefik router accepts every path on the trusted-LAN listener, and the security policy
explicitly declares metrics public. Metrics expose aggregate operational state but currently no
credentials or user/media identities. Do not quietly add application authentication that breaks
existing scrape jobs. Document that the endpoint belongs on a private scrape network or a separately
hardened operator edge; Prometheus scrape configuration supports TLS and authorization when an
operator needs them ([configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)).

### 8. Grafana artifacts are worthwhile if they remain portable and testable

Grafana supports file-provisioned, version-controlled dashboards and stable dashboard UIDs
([provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)). It also warns
that later file updates overwrite UI edits. Ship the dashboard as read-only source (`allowUiUpdates:
false`) and use a Prometheus datasource variable rather than a hard-coded UID. Variables allow one
dashboard to work across datasource instances ([variables](https://grafana.com/docs/grafana/latest/visualizations/dashboards/variables/)).

Use the broadly provisionable classic JSON model for the first artifact unless the minimum supported
Grafana version is raised to one that accepts the v2 Kubernetes resource format everywhere. Grafana's
new observability-as-code stack is promising, but adopting its Foundation SDK would add an
application-repository dependency solely to generate JSON. Official guidance still supports file
provisioning, while noting v2 is the fully compatible future model
([observability as code](https://grafana.com/docs/grafana/latest/as-code/observability-as-code/),
[JSON model](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/view-dashboard-json-model/)).

## Proposed compatibility policy

- Treat every shipped family, label name, label value set, type, and unit as a compatibility contract.
- Additive families and additive closed label values are minor-compatible.
- Never change a family type or unit in place.
- For a rename, emit old and new families from the same observation for one documented release line;
  mark the old family deprecated in HELP text and remove it only in a planned breaking release.
- Dashboard queries use the new contract. Optional compatibility recording rules may bridge older
  releases, but should not conceal a mixed-version fleet indefinitely.
- No raw errors, URLs, ids, usernames, emails, titles, paths, prompt text, or secrets in labels or HELP.

## Verification contract

1. Gather a fresh recorder through its public interface and compare the complete family/type/help/
   label schema to a reviewed manifest.
2. Drive representative success, failure, retry, restart, scheduler, database, and playout
   transitions through their real module seams and assert scrape results.
3. Prove all closed series initialize as intended and hostile values collapse to `other` or never
   reach exposition.
4. Parse every dashboard PromQL expression against the contract and assert every referenced raw or
   recording family exists.
5. Check rule syntax with `promtool check rules` and behavior with `promtool test rules`; Prometheus
   documents both the checker and rule-test format
   ([recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/),
   [rule tests](https://prometheus.io/docs/prometheus/latest/configuration/unit_testing_rules/)).
6. Provision the dashboard into a pinned Grafana container in a bounded verification target and
   fail on provisioning or datasource errors. This is a test dependency, not a shipped service.

## Implementation slices

1. Design-first contract table and compatibility policy; correct stale package/README/security text.
2. Test-first owned recorder/registry and scrape-manifest gate; preserve existing names initially.
3. Typed label classifiers and zero initialization; negative cardinality/privacy tests.
4. Add build/backend, database-pool, scheduler, and playout metrics at their natural seams.
5. Reconcile or deprecate unclear state-gauge names with dual emission where needed.
6. Add one Grafana overview, optional recording/alert rules, example scrape/provisioning files, and a
   pinned `make observability-verify` target.
7. Run affected verification and the opt-in observability integration target; record evidence.

## Explicit deferrals

- No per-user, per-channel, per-title, per-provider URL, or per-process-id labels.
- No hosted-LLM dollar metric: model prices change independently; tokens remain the stable source.
- No tracing or OpenTelemetry expansion in this goal.
- No bundled Prometheus retention policy, Alertmanager routing, Grafana credentials, or published
  monitoring port.
- No dashboard gallery until one overview has real operator use and a second distinct workflow.
