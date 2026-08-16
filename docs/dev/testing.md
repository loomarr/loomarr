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
| LLM eval | `make eval` | Exact shipped template Intents and the wider corpus against a real model | manual |
| Rust supply chain | `make rust-audit` | Cargo advisories, licences, and sources | weekly + manual |
| Rust fuzz | `make rust-fuzz` | Bounded worker protocol and decoder do not crash | weekly + manual |
| SSO | `make test-sso` | OIDC against real Authelia + Authentik | manual |
| Maintainer smoke | `make smoke` | The real stack end to end | manual |

Store conformance is one suite over two backends — don't fork the assertions per dialect.

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

## First-channel acceptance

The acceptance path is one product journey, not a set of component demos: fresh setup → choose a
canonical template in the wizard or Guide → durable proposal-job progress → Proposal review →
approve → `building` → `live` → the same Channel appears in Guide and Watch. The browser test
submits the template's **complete typed Intent**, not just its description. While the Channel is
`building`, Watch must make no HLS request; a Channel SSE frame may accelerate the transition, but an
authoritative refetch is what permits playback.

Proposal-job interface tests pin these distinct outcomes:

1. `queued` or `running`, with no Proposal yet;
2. `done` with a `submitted` Proposal;
3. `done` with the newest already-`approved` or `denied` Proposal;
4. `failed` with no Proposal and a bounded safe failure code.

Run each recovery assertion once with SSE disabled or its terminal frame dropped. Preserve the active
`jobId` across reload and require the GET projection to restore status, full Intent, failure, and
Proposal. Retry submits that same full Intent into a **fresh** job; Edit opens a populated form.
Exercise `no_grounded_titles`, timeout/provider failure, and the generic safe fallback without
exposing raw provider text. A member can read their own job but gets 403 for another member's; an
admin can read both. Equal Intents submitted by two users must have different job ids, Proposal ids,
ownership, and decision histories even when the semantic cache supplies the content.

The end-to-end integration has two filler fixtures. With zero eligible clips, approval still creates
the Channel, reconcile writes real programs with no break slots, and playback is back-to-back. With
seeded eligible clips, the same path attaches filler through the existing assembler. Reconcile each
variant twice to prove idempotence. Filler availability is never a prerequisite assertion for first
channel success.

## Semantic template certification

`make eval` is manual and networked by design; it never joins `make check`. Its template cases load
the **same canonical product data** the wizard and Guide ship, including stable id, description,
`era`, `tone`, runtime target, and must-include/exclude fields. A separately retyped sentence is not a
template certification, even if it sounds equivalent.

Certification runs all four exact template Intents through the real grounded suggester and records
the provider, model, catalog fixture or fingerprint, and time. Every case requires a non-empty
grounded result and zero fabricated ids. It additionally asserts the promise each template makes:

- **90s Saturday Morning Cartoons:** 1990–1999 binding and a kids-safe ceiling/lineup;
- **Cozy Mystery Nights:** mystery theme with the stated non-gruesome safety intent preserved;
- **Late-Night Sci-Fi:** science-fiction theme and the canonical atmospheric tone;
- **Action Movie Marathon:** action theme and no content above the stated PG-13 ceiling.

These are semantic expectations, so certification scores theme fit without pinning one stochastic
title list. Connectivity and tool-capability probes are prerequisites, not substitutes: they can be
green while template certification fails. The run must be explicit because a hosted provider can
cost money; setup/status polling and ordinary health checks never invoke it.

## Fixtures

Fixtures in `internal/testkit/fixtures/` are captures with source-version comments. Write
parsers against them, not against remembered field names — and never let a fixture equate two
distinct identifiers, which has hidden shipped bugs by making a wrong lookup return the right
answer.
