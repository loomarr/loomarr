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
- Compare complete-ladder throughput, process count, output sizes, peak RSS, and certification output
  semantics against slice 1 on both release architectures.

### 3. Reserve capacity for interactive work

- Replace the undifferentiated global permits with a bounded policy that always reserves interactive
  capacity while AVIF drains in the background.
- Keep lazy JPEG/WebP and animated WebP responsive under a saturated AVIF queue.
- Prove the policy with deterministic scheduler tests and production queue-wait metrics.

### 4. Measure process and encoder parallelism

- Compare multiple one-thread worker processes with fewer workers using bounded rav1e threads at
  2-, 4-, and 8-logical-CPU profiles.
- Adopt weighted permits only if the measured throughput and p95 interactive queue wait improve
  without breaching peak-RSS ceilings. Single-thread AVIF remains the default otherwise.

### 5. Harden Rust dependency operations

- Add Cargo update grouping and review policy, advisory/license checks, an explicit unsafe-code
  policy for Loomarr-owned crates, and scheduled fuzzing of the bounded protocol/decoder boundary.
- Keep expensive supply-chain and fuzz work scheduled or manually dispatched unless it is both fast
  and deterministic enough for the required PR gate.

### 6. Choose the next Rust capability from evidence

- Use production worker duration, queue wait, peak RSS, input/output bytes, and failure outcomes to
  identify the next pixel-heavy bottleneck.
- Extend the existing Image worker protocol when the capability belongs to the same crash/resource
  boundary; do not create a second Rust service merely to increase the Rust footprint.

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
actionable only against the same corpus, recipe, release profile, and CPU profile.
