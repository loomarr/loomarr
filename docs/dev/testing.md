# Testing

Unit tests never touch the network. External services are mocked through `internal/testkit` —
extend it rather than writing a private mock.

## The gate

```bash
make check    # the gate
```

Green here should mean green in CI.

The steps it runs are listed in the [command reference](commands.md), which is generated from the
Makefile — so it can't drift from what `check` actually does. Enumerating them here could, and
did.

### `make vet-tags` isn't redundant

Files behind `//go:build ffmpeg|eval|integration` are invisible to plain `go vet ./...` and to
golangci-lint — both ask the build system which files exist, and it honours the constraint.

That blind spot ran for months: `go vet ./...` exited 0 while the tagged run exited 1, and the
one test proving programmes sequence hadn't compiled in several releases. A tagged `go build`
wouldn't catch it either, since `go build` skips `_test.go` and most tagged files are tests.

## The layers

| Layer | Command | Proves | In `check`? |
| --- | --- | --- | --- |
| Go unit | `make test` | The backend, hermetically | ✅ |
| Tagged compile | `make vet-tags` | Build-tagged code compiles | ✅ |
| Doc claims | part of `make test` | Help pages don't contradict the code | ✅ |
| Store conformance | `make test-pg` | SQLite and Postgres behave identically | CI |
| Frontend units | `make fe` | Components and domain logic | CI |
| Visual + a11y | `make fe-visual` | Every story, two viewports, pixel + axe | CI |
| e2e | `make e2e` | The embedded SPA through first-run | CI |
| ffmpeg | `make test-ffmpeg` | Programmes sequence through real ffmpeg | manual |
| LLM eval | `make eval` | Real intents against a real model | manual |
| LLM certification | `make eval-cert` | Exact starter/adversarial corpus executed with a versioned scorecard | release/manual |
| LLM matrix | `make eval-matrix` | Same corpus on local and OpenRouter generation, judged through OpenRouter | release/manual |
| Rust supply chain | `make rust-audit` | Cargo advisories, licences, and sources | weekly + manual |
| Rust fuzz | `make rust-fuzz` | Bounded worker protocol and decoder do not crash | weekly + manual |
| SSO | `make test-sso` | OIDC against real Authelia + Authentik | manual |
| Maintainer smoke | `make smoke` | The real stack end to end | manual |

Store conformance is one suite over two backends — don't fork the assertions per dialect.

### Semantic evaluation versus certification

`make eval` is exploratory and exits cleanly when its real library, TMDB, or LLM configuration is
absent. `make eval-cert` is an assertion: missing configuration, a skipped/unexecuted case, a hard
grounding or negative-constraint failure, a required judge failure, or an unwritable scorecard makes
the command fail. It always bypasses Go's test cache and writes
`$LOOMARR_ARTIFACT_DIR/semantic-certification.json` unless `LOOMARR_EVAL_OUT` selects another path.
The scorecard records its schema/corpus version, requested generator/provider, trial profile, and
bounded structural observations—never credentials. Hard predicates cover
exact named includes/excludes, holiday policy, rating limits, ownership mix, and concrete scheduled
programme identities/order; a non-empty Proposal or favorable judge paragraph cannot substitute for
one. Fixture cases materialize `schedule.DesiredLineup` in the hermetic gate. Real-provider cases run
serially for an explicit trial count and report pass rate plus min/median/max relevance and
serendipity: novelty only scores when it remains defensibly on-theme. Per-case tool-call and
surfaced-candidate budgets fail deterministically. Structural diagnostics record the grounding stage,
tool mode, candidate count, generation failure, and schedule materialization failure, so a low score
points at the layer to tune. Real inference still remains outside `make check` and certifies only the
requested model/provider configuration, catalog snapshot, and corpus version in the artifact.

`make eval-matrix` prevents tuning to one local model. It requires the ordinary exported `LLM_*`
configuration plus `OPENROUTER_API_KEY` and `OPENROUTER_MODEL`; `OPENROUTER_JUDGE_MODEL` may select a
different hosted judge. It writes separate `local` and `openrouter` scorecards from the exact same
corpus. OpenRouter uses its OpenAI-compatible `https://openrouter.ai/api/v1` endpoint; keys are passed
only in request authorization and are never scorecard metadata. The command also requires
`LOOMARR_EVAL_ALLOW_LOCAL=1`: set it only after confirming the machine is idle and the configured
local runtime fits available RAM/VRAM without spilling. The two legs are sequential, and the target
does not provision or start Ollama. On a shared media server, run this manual certification during a
maintenance window; playback and transcode capacity take precedence over evaluation evidence.

### Rust dependency review

Dependabot opens one weekly grouped PR for Cargo minor and patch updates across the production and
fuzz workspaces. Major updates are deliberately excluded: update one direct crate at a time, amend
the §14 rationale when its capability or cost changes, and compare `Cargo.lock` rather than accepting
a generated diff blindly. Every Cargo update runs `make rust-audit`, `make rust-check`,
`make image-cert`, and the amd64/arm64 release build. An advisory ignore requires a reason, an owner,
and a removal issue in the same PR; an unannotated permanent ignore is not policy.

Loomarr-owned shipping Rust crates use `#![forbid(unsafe_code)]`. Transitive codecs may contain
unsafe or native code, which is why they remain behind the one-shot worker boundary. The fuzz harness
is non-shipping test infrastructure built around libFuzzer; its own source contains no unsafe block.

Auth tests need the negative cases: members get 403 on titles, approve and admin routes;
sessions die on disable.

## Green that proves nothing

Each of these has happened here:

- **A test that passes first try is suspect.** Sabotage the code and confirm it goes red.
- **`make ci-lint` needs `shellcheck` on `PATH`** — without it, actionlint skips the shell half
  and exits 0 locally while CI fails.
- **A fake that ignores its context** can't catch write-through-dead-context bugs.
- **A story on an unregistered route** snapshots "Not Found" as its baseline and stays green.
- **A pnpm config only fails on a cold install** — with `node_modules` present, pnpm doesn't
  re-evaluate build scripts.

## Determinism

Pod assembly and shuffling take an explicit seed. The visual suite pins the timezone to UTC.
Baselines are Linux-only, generated in the pinned Docker image; local macOS or Windows runs
write differently-suffixed files that are gitignored.

## Fixtures

Fixtures in `internal/testkit/fixtures/` are captures with source-version comments. Write
parsers against them, not against remembered field names — and never let a fixture equate two
distinct identifiers, which has hidden shipped bugs by making a wrong lookup return the right
answer.
