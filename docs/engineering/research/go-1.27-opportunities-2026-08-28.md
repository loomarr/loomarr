# Go 1.27 opportunities for Loomarr

**Compiled 2026-08-28.** Scope: what moving Loomarr's module and build toolchain from Go 1.26
to Go 1.27 enables, what improves automatically, and which follow-up code changes are justified.
Sources are Go project release notes and standard-library documentation; repository observations
come from the current tree at `36294a46`.

The implemented follow-up measurements and compatibility decisions are recorded in
[`go-1.27-follow-up-2026-09-02.md`](../go-1.27-follow-up-2026-09-02.md).

## Verdict

Upgrade to Go 1.27, but treat the toolchain bump as a compatibility change rather than a version-only
edit. Go 1.27.0 is a stable release (2026-08-19), and the Go project expects almost all existing Go
programs to continue to compile and run under the Go 1 compatibility promise. The largest Loomarr
benefits require no production refactor: faster JSON unmarshaling, cheaper small allocations, faster
DEFLATE, improved HTTP connection reuse, a stronger default test/vet check, and better Darwin custom-CA
behavior. ([release history](https://go.dev/doc/devel/release),
[Go 1.27 release notes](https://go.dev/doc/go1.27))

The repository has already addressed the main byte-reproducibility risk in the merged macOS parity
work. `cmd/image-bench` writes its PNG corpus with `encodeStableBenchmarkPNG` and a hand-written
stored-block zlib stream instead of depending on Go's DEFLATE encoder. From a clean test cache at
the current HEAD, the same focused test passes under both toolchains:

```text
GOTOOLCHAIN=go1.26.0 go test ./cmd/image-bench \
  -run TestRunMeasuresTheCurrentIconAVIFLadderThroughTheWorkerProtocol -count=1
# PASS

go test ./cmd/image-bench \
  -run TestRunMeasuresTheCurrentIconAVIFLadderThroughTheWorkerProtocol -count=1
# PASS (Go 1.27.0)
```

Keep that implementation and its fixed digest test. Go 1.27 deliberately changed DEFLATE encoding,
including output from `archive/zip`, `compress/gzip`, `compress/zlib`, and `image/png`; exact
compressed bytes from those standard encoders are not a portable cross-toolchain contract. The
benchmark's explicit stored-block format is the appropriate exception because its named synthetic
corpus is itself benchmark input. ([compression change](https://go.dev/doc/go1.27#compress/flate))

## Benefits that arrive with the toolchain

### 1. Faster JSON decoding across a broad surface

The existing `encoding/json` API is now backed by the v2 implementation while preserving v1
marshal/unmarshal behavior. The Go team describes marshal performance as broadly unchanged and
unmarshal performance as significantly faster. Loomarr imports `encoding/json` in **127 production
Go files**, including store (13 files), LLM clients (8), filler (8), filler review (7), diagnostics
(6), suggestion (5), requester clients, TMDB, API routes, and command tooling. This is the clearest
automatic application-level win: upstream responses, persisted JSON, diagnostic records, and API
payloads all benefit without changing imports. ([release notes](https://go.dev/doc/go1.27#encoding-json-v2))

Compatibility caveat: v1 behavior is retained, but exact error text may differ. Loomarr generally
checks meaningful substrings such as `unknown field`, which is preferable to pinning whole errors;
the full test suite should still exercise all 117 files that use decoder options, `RawMessage`, or
custom decoder behavior. Go provides the temporary `GOEXPERIMENT=nojsonv2` escape hatch for isolating
a regression, but the release notes say it is expected to be removed, so it must not become a
permanent build setting. ([release notes](https://go.dev/doc/go1.27#encoding-json-v2))

### 2. Runtime and network improvements

- Size-specialized allocation routines reduce the cost of some allocations smaller than 80 bytes
  by up to 30%, with an expected roughly 1% overall improvement in allocation-heavy programs, at a
  fixed binary-size cost of about 60 KB. Loomarr's high-volume schedule projections, HTTP DTOs,
  JSON decoding, and diagnostic events are plausible beneficiaries, but this should be measured
  rather than attributed without a benchmark. No source change is required.
- HTTP/1 response bodies now drain a conservative amount of unread content on `Close`, improving
  connection reuse. Loomarr consistently closes upstream bodies and has bounded early-error reads
  in the LLM, requester, TMDB, archive, and image clients, so some error paths can reuse connections
  more reliably without changing those safety bounds.
- `crypto/x509.SystemCertPool` now honors `SSL_CERT_FILE` and `SSL_CERT_DIR` on Darwin. This directly
  improves native macOS development against media servers, requesters, or LLM endpoints using a
  private CA; Linux containers are unaffected by the Darwin-specific change.

These are all documented in the [Go 1.27 release notes](https://go.dev/doc/go1.27).

### 3. A stronger default gate

`go test` now runs the `stdversion` vet analyzer by default. Once `go.mod` says `go 1.27`, tests will
reject accidental use of standard-library APIs newer than the module language version, even when a
developer happens to run a later Go toolchain. That aligns well with Loomarr's existing `make check`
contract and cross-platform vet gate. ([tooling notes](https://go.dev/doc/go1.27#go-command))

`go mod tidy` also normalizes a Go 1.27 module to at most two `require` blocks (direct and indirect).
Loomarr's `go.mod` already has exactly that shape, so this is maintenance protection rather than an
immediate diff. ([tooling notes](https://go.dev/doc/go1.27#go-command))

## Recommended follow-up code changes

### P1 — expose the new goroutine-leak profile behind the existing development gate

The `goroutineleak` runtime profile is generally available in 1.27. It reports goroutines blocked on
unreachable synchronization primitives; it intentionally cannot find every leak, particularly when
the primitive remains reachable from globals or runnable goroutines. ([runtime profile docs](https://pkg.go.dev/runtime/pprof@go1.27.0#Profile))

This is a strong fit for Loomarr's process supervisors, playout sessions, background workers, event
fan-out, startup probes, and 22 files using `sync.WaitGroup`. The application already has a
development-only, boot-time `LOOMARR_PPROF=1` gate, but `internal/api/ops.go` registers a closed list
of profiler paths. Add explicit canonical and alias routes for `goroutineleak` under that same gate,
plus positive and default-off tests. Do not expose it outside the existing gate: stack traces can
contain sensitive data.

Priority: **high after the toolchain bump**. This adds diagnostic leverage with a small, bounded
surface and no production-default exposure.

### P1 — pilot in-memory HTTP test servers on timing-sensitive suites

Go 1.27 adds `httptest.NewTestServer(t, handler)`, whose default in-memory network avoids port
exhaustion and transient loopback failures, registers cleanup, turns handler panics into test
failures, and works with `testing/synctest`. ([official package docs](https://pkg.go.dev/net/http/httptest@go1.27.0#NewTestServer))

Loomarr has **79 test files** using `httptest.NewServer`; 11 of those also contain wall-clock waits,
deadlines, or sleeps. Good pilot candidates are `internal/api/events_test.go`,
`internal/api/playout_test.go`, `internal/fillerreview/openrouter_review_test.go`, and
`internal/programmer/tunarr_test.go`. This is not a mechanical global replacement: in-memory servers
must be called through `server.Client()`, while several current tests pass `server.URL` to code that
uses `http.DefaultClient`. Start with seams that already inject an HTTP client, then use
`testing/synctest.Sleep` only where the whole asynchronous operation can live inside a synctest
bubble. The new helper is valuable because it removes a source of flakes, not because it shortens
syntax.

Priority: **high for test reliability, incremental delivery**.

### P2 — evaluate direct `encoding/json/v2` only at strict trust boundaries

Direct v2 use rejects invalid UTF-8 and duplicate object names by default and uses exact,
case-sensitive field matching. Those defaults address real interoperability and security ambiguity;
the official documentation specifically calls out duplicate-name disagreement as a way two systems
can assign different meanings to one request. ([JSON v2 security considerations](https://pkg.go.dev/encoding/json/v2@go1.27.0#hdr-Security_Considerations))

The best candidates are strict immutable evidence and attestation readers in `internal/fillereval`,
`internal/fillerbakeoff`, `internal/fillerreview`, and `cmd/internal/fillerbakeoffio`, which already
reject unknown fields and trailing values. Pilot one reader with adversarial tests for duplicate
names, invalid UTF-8, unknown fields, case variants, nil/empty collections, and exact canonical
hashes. Do **not** bulk-migrate external service clients or public DTOs: upstream APIs may rely on
v1's case-insensitive matching, and Loomarr hashes or signs some JSON artifacts.

Priority: **medium, separate from the dependency/toolchain PR**. The automatic v1 speedup removes
any performance urgency.

### P3 — review, do not blindly apply, the new modernizers

Go 1.27 adds `atomictypes`, `embedlit`, `slicesbackward`, and `unsafefuncs` to `go fix`.
([tooling notes](https://go.dev/doc/go1.27#go-fix)) A read-only `go fix -diff` audit found:

- one `atomictypes` simplification in `internal/suggest/worker_test.go`;
- clear `slicesbackward` candidates in production cleanup/sequencing code in
  `internal/app/application.go`, `internal/filler/splitrescue.go`, and
  `internal/schedule/separation.go`, plus test helpers;
- no compelling `embedlit` or `unsafefuncs` migration in the inspected packages.

These are readability changes, not reasons to upgrade. Apply them only in a small follow-up with
normal behavior tests; keep them out of the toolchain/dependency diff so a regression remains easy
to bisect.

## Features that do not currently justify code changes

- **Generic methods and broader function inference:** the codebase has a small number of generic
  helpers and testkit containers, but no duplicated public API that becomes materially deeper or
  safer as a generic method. Refactoring merely to demonstrate the language feature would add churn.
- **Standard-library `uuid`:** Loomarr's own identifiers are deliberately opaque 128-bit hex strings,
  described as “uuid-like,” while Tunarr UUIDs are external opaque values. Changing their textual
  format would be a data/contract decision. It also would not remove `github.com/google/uuid`, which
  is currently transitive through Testcontainers rather than imported by Loomarr production code.
  The new package is well-specified and cryptographically random, but it solves a different problem.
  ([standard `uuid` docs](https://pkg.go.dev/uuid@go1.27.0))
- **Experimental SIMD:** Loomarr's performance-sensitive image processing is intentionally owned by
  the Rust worker. Moving a kernel into an experimental Go API would cross that architectural seam
  and opt the release build into `GOEXPERIMENT=simd`; there is no measured Go hotspot that justifies
  it.
- **ML-DSA APIs:** Loomarr delegates TLS and X.509 to the standard HTTP stack and has no custom
  signature format requiring ML-DSA. Standard-stack interoperability can improve without an
  application API migration.
- **`strings.CutLast`, `bytes.CutLast`, and URL clone helpers:** several call sites could become
  slightly shorter, but none removes a correctness hazard. Fold these into ordinary nearby work.

## Upgrade constraints and validation plan

1. Retain the cross-version `cmd/image-bench` corpus check while changing the toolchain. It is green
   under both Go 1.26.0 and Go 1.27.0 at the researched HEAD; the full image gate still needs to run
   in the upgrade PR.
2. Raise every authoritative pin together: `go.mod`, Docker's digest-pinned Go builder,
   `GO_VERSION` workflow authority and its release-verification fixtures, the separately pinned
   image-benchmark workflow, setup/README badges, doctor fixtures, and generated command/docs where
   applicable. The repository deliberately validates these values, so changing only `go.mod` is
   incomplete.
3. Run `go mod tidy` under Go 1.27 and review dependency movement independently from the toolchain
   edit.
4. Run `make check`, Postgres conformance, image certification/benchmark tests, Docker multi-platform
   build, and the macOS agent harness. Compare representative JSON-heavy benchmarks or request
   profiles before attributing a performance gain.
5. Document that native Go 1.27 development requires **macOS 13 Ventura or later**. Loomarr's shipped
   artifact remains a Linux container, but contributors running the native toolchain inherit Go's
   new Darwin floor. ([Darwin port note](https://go.dev/doc/go1.27#darwin))

## Prioritized recommendation

1. **Upgrade now**, preserving the deterministic image-benchmark corpus and updating all pins.
2. **Take the automatic wins first**; do not mix a direct JSON v2 migration into the dependency PR.
3. **Add gated `goroutineleak` access** and **pilot in-memory HTTP tests** as focused follow-ups.
4. **Trial strict JSON v2 at one evidence boundary**, backed by adversarial compatibility tests.
5. **Defer generic-method, UUID, SIMD, and cosmetic modernization churn** until a concrete module or
   measured hotspot asks for it.
