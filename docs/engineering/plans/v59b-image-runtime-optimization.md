# V59b - Rust image runtime optimization

## Outcome

Make the required Rust Image worker faster without weakening the bounded process boundary, changing
Rendition bytes accidentally, or moving AVIF onto request latency. Measurement lands before behavior:
each optimization is compared against a repeatable release-profile baseline through the production
Go-to-Rust protocol seam.

This phase does not add a Go or ffmpeg renderer fallback, a persistent daemon, cgo, a new image
format, configurable resource ceilings, or timing assertions to the required PR gate.

## Public seams under test

1. `internal/images/rustgen.Generator.Generate`, using the installed release worker and its bounded
   request/result manifest.
2. `internal/images.AVIFJob`, which owns background AVIF coverage and publication.
3. The Image service's global worker capacity, queue wait, in-flight, duration, and peak-RSS metrics.
4. `make image-bench`, which writes machine-readable evidence without deciding pass or fail from a
   shared runner's wall clock.

Correctness remains owned by `make image-cert`; the benchmark never substitutes for certification.

## Vertical slices

### 1. Establish the release-worker baseline

- Generate deterministic poster, backdrop, and icon sources outside the timed region.
- Warm each role once, then measure three complete AVIF ladders by default.
- Exercise the current production shape: one bounded worker invocation per missing AVIF Rendition.
- Record the recipe, architecture, logical CPU count, source dimensions, ladder widths, process and
  Rendition counts, output bytes, p50/p95/max worker time, median complete-ladder time and throughput,
  and maximum child peak RSS.
- Write JSON beneath the worktree artifact directory locally. A manually dispatched workflow runs
  the same Make target on native amd64 and arm64 runners and retains one report per architecture.
- Keep the benchmark opt-in. It is evidence for comparisons, not a noisy `make check` timing gate.

### 2. Batch each missing AVIF ladder

- Send all missing widths for one Image in one worker request and accept only a complete manifest.
- Resize the largest rung from the decoded source, then step downward from the preceding rung.
- Preserve Go's validation, atomic publication, Store writes, cancellation, and all-or-nothing cleanup.
- Advance the immutable recipe from `v1` to `v2`: stepped resampling intentionally changes pixels,
  so it must never overwrite a year-cached `v1` URL.
- Compare complete-ladder throughput, process count, output sizes, peak RSS, and certification output
  semantics against slice 1 on both release architectures.

### 3. Reserve capacity for interactive work

- Replace the undifferentiated global permits with a bounded policy that reserves one interactive
  slot while AVIF drains whenever total capacity is at least two; on a single-slot host, prioritize
  an interactive waiter at the next process boundary because the running worker cannot be preempted.
- Keep lazy JPEG/WebP and animated WebP responsive under a saturated AVIF queue.
- Prove the policy with deterministic scheduler tests and production queue-wait metrics.

### 4. Measure process and encoder parallelism

- Compare multiple one-thread worker processes with fewer workers using bounded rav1e threads at
  2-, 4-, and 8-logical-CPU profiles.
- Adopt weighted permits only if the measured throughput and p95 interactive queue wait improve
  without breaching peak-RSS ceilings. Single-thread AVIF remains the default otherwise.

**Checkpoint 4 decision:** retain one-thread AVIF and do not add weighted permits. The opt-in
`make image-parallelism-bench` matrix invokes the release worker through the production manifest
seam while varying only an explicit benchmark CLI argument. At equal CPU budgets the multi-process,
one-thread shape delivered the highest aggregate throughput in every local 2/4/8-CPU profile. Three
measured poster runs produced 15.57 versus 13.15 Images/min at 2 CPUs (2x1-thread versus 1x2-thread),
27.71 versus 24.20 at 4 CPUs (4x1 versus 2x2), and 60.58 versus 47.02 at 8 CPUs (8x1 versus 4x2).
Larger per-worker thread counts fell further. Per-Image output bytes also changed with the thread
count (49,151 at one thread; 50,043 at two; 51,355 at four; 52,732 at eight), so adoption would
require a recipe advance and cache-identity migration.
Interactive queue wait is already held at zero while capacity is at least two by checkpoint 3's
reserved slot, so a weighted policy cannot improve that admission result; it would add policy for a
throughput loser. The production CLI and recipe remain unchanged.

### 5. Harden Rust dependency operations

- Add Cargo update grouping and review policy, advisory/license checks, an explicit unsafe-code
  policy for Loomarr-owned crates, and scheduled fuzzing of the bounded protocol/decoder boundary.
- Keep expensive supply-chain and fuzz work scheduled or manually dispatched unless it is both fast
  and deterministic enough for the required PR gate.

**Checkpoint 5 implementation:** Cargo minor/patch updates are one weekly Dependabot group across
the production and excluded fuzz workspaces; majors remain deliberate one-crate reviews. Pinned
cargo-deny checks RustSec advisories, an explicit SPDX allow-list, and crates.io-only sources for
both lockfiles. Both owned shipping crate roots forbid unsafe code. A pinned cargo-fuzz/libFuzzer job
drives valid bounded JSON around arbitrary and seed-mutated image bytes for 60 seconds weekly or on
manual dispatch, retaining crash reproducers. These network-sensitive tools remain outside
`make check`; lock enforcement, clippy/tests, and the unsafe prohibition stay in the fast gate.
The first audit found RUSTSEC-2026-0204 in the transitive `crossbeam-epoch` graph and advanced the
lockfile to 0.9.20. The non-shipping fuzz workspace is excluded from the container context, keeping
its large local compile cache out of ordinary image builds.

### 6. Choose the next Rust capability from evidence

- Use production worker duration, queue wait, peak RSS, input/output bytes, and failure outcomes to
  identify the next pixel-heavy bottleneck.
- Extend the existing Image worker protocol when the capability belongs to the same crash/resource
  boundary; do not create a second Rust service merely to increase the Rust footprint.

**Checkpoint 6 result:** no additional capability is justified. The production Image service has
no Go pixel path; its Go codec imports came only from deterministic certification-corpus generation,
which now lives with the non-shipping `cmd/image-cert` tool. A source architecture test prevents
those imports from returning to `internal/images`. The only other production Go pixel consumer is
the filler-era heuristic in `internal/mediatools`; it samples at most 1,024 pixels per keyframe in a
background job, lies outside the measured Image-worker boundary, and has no evidence of meaningful
duration, queue, RSS, byte, or failure cost. Moving it would expand the protocol on novelty rather
than evidence. The next proposal must bring a production metrics capture and a same-corpus
certification/benchmark reproduction; if it qualifies, it extends the existing worker.

## Required gates per optimization PR

```sh
make image-bench
make image-cert
make check
LOOMARR_RELEASE=dev make build
docker build --target image-worker .
```

Slices that change Store behavior also run `make test-pg`. A benchmark comparison records hardware,
architecture, process count, throughput, p50/p95/max duration, output bytes, and peak RSS. A result is
actionable only against the same corpus, release profile, and CPU profile. A comparison across recipe
identifiers must identify the intentional pixel-policy change and use certification for semantic
equivalence; it is not a byte-for-byte regression comparison.
