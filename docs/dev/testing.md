# Testing

Loomarr has several test layers with genuinely different jobs. The rule that shapes all of
them: **unit tests never touch the network.** Every external service is mocked through
`internal/testkit` — one shared implementation per service. Extend it; don't invent a private
mock.

> This page describes what each layer *proves*. It deliberately quotes no test counts — four
> files used to state the size of the visual suite and no two agreed. For the target list, see
> [commands](commands.md), which is generated.

## The gate

```bash
make check    # fmt + vet + vet-tags + lint + test
```

Green here should mean green in CI. Everything below is either part of it or deliberately
outside it.

### `make vet-tags` is not redundant with `make vet`

Files behind `//go:build ffmpeg|eval|integration` are **invisible** to plain `go vet ./...` and
to golangci-lint, because both ask the build system which files exist and the build system
honours the constraint.

That blind spot ran for months: `go vet ./...` exited 0 while `go vet -tags '…' ./...` exited 1,
and the one test that proves programmes actually sequence had not compiled for several releases.

A tagged `go build` would *not* have caught it either — `go build ./...` skips `_test.go`
entirely, and most tagged files are tests. Only vet typechecks them.

## The layers

| Layer | Command | What it proves | In `check`? |
| --- | --- | --- | --- |
| Go unit | `make test` | The whole backend, hermetically | ✅ |
| Tagged compile | `make vet-tags` | Build-tagged code still compiles | ✅ |
| Doc claims | part of `make test` | The embedded help pages don't contradict the code | ✅ |
| Store conformance | `make test-pg` | SQLite and Postgres behave identically (Docker) | CI |
| Frontend units | `make fe` | Components and domain logic, jsdom | CI |
| Visual + a11y | `make fe-visual` | Every story, two viewports, pixel + axe (Docker) | CI |
| e2e | `make e2e` | The real embedded SPA through first-run | CI |
| ffmpeg | `make test-ffmpeg` | Programmes sequence through real ffmpeg | manual |
| LLM eval | `make eval` | Real intents against a real model, scored | manual |
| SSO | `make test-sso` | OIDC against real Authelia + Authentik | manual |
| Maintainer smoke | `make smoke` | The real stack end to end | manual |

**Store conformance is one suite, two backends.** Never fork the assertions per dialect — that
would let the two drift, which is the entire thing it exists to prevent.

**Auth tests must include the negative cases**: members get 403 on titles, approve and admin
routes; sessions die on disable.

## Things that look green and aren't

This repo has been bitten by each of these, so they're worth knowing before you trust a pass.

- **A test that passes on its first run is suspect.** Sabotage the code and confirm it goes red.
  A gate that has never failed may not be connected to anything.
- **`make ci-lint` needs `shellcheck` on `PATH`.** Without it, actionlint silently skips the
  shell half and exits 0 locally while CI fails.
- **A fake that ignores its context can't catch write-through-dead-context bugs.** This has hit
  twice, in filler and in the scheduler.
- **A story on an unregistered route snapshots "Not Found"** as its baseline and stays green
  forever.
- **"Flaky" usually means the retry harness working.** `--update-snapshots` is not verification.

## Determinism

Pod assembly and any shuffling take an **explicit seed** in tests. The visual suite pins the
timezone to UTC — without it, wall-clock labels drift and every snapshot fails on a different
machine. Baselines are Linux-only, generated in the pinned Docker image; local macOS or Windows
runs write differently-suffixed files that are gitignored.

## Fixtures are pinned truth

Webhook payloads and API responses in `internal/testkit/fixtures/` are captures with
source-version comments. **Write parsers against the fixtures, not against remembered field
names** — and never let a fixture equate two distinct identifiers, which has hidden shipped bugs
by making a wrong lookup produce a right answer.
