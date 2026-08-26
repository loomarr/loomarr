# V59a - Rust image runtime certification

> Archived: this certification shipped; `PROGRESS.md` records its final evidence.

## Outcome

Prove the mandatory Rust image engine against a repeatable adversarial corpus and an operator's
read-only corpus, expose bounded production telemetry for the worker boundary, and certify the three
user-facing consumers that depend on it.

This phase does not add another renderer, a fallback, a daemon, new image formats, configurable
resource ceilings, or a benchmark that silently changes correctness policy.

## Public seams under test

1. `loomarr-image capabilities` and `generate` through `internal/images/rustgen`.
2. `internal/images.Service` through `Ingest` and `Rendition`.
3. Guide, Watch timeline, and Filler HTTP DTOs plus the frontend `Image` primitive.
4. `/metrics` for operational observations of the process and queue boundary.

The gate never verifies a renderer by reading its private state, querying image tables behind the
service, or replacing the worker with a same-implementation fake.

## Vertical slices

### 1. Deterministic corpus and report

- Begin with one opaque static case and a failing report assertion.
- Add transparent, static WebP, and animated cases one at a time.
- Add expected stable refusals for corrupt and bounded-resource cases.
- Record wall time, source/output bytes, peak RSS when supported, and p50/p95/max summaries.
- Prove that a failed case leaves no staged file behind.

### 2. Operator corpus mode

- Accept an explicit absolute directory through `IMAGE_CERT_CORPUS`.
- Walk regular files without following symlinks or writing beneath the source.
- Treat supported-looking raster files as expected successes; report unsupported files as skipped.
- Write JSON only to `IMAGE_CERT_REPORT` or the worktree's normal artifact directory.

### 3. Runtime telemetry

- Observe the real child process at the protocol adapter: kind, stable outcome, wall time, bytes, and
  peak RSS.
- Observe queue wait and in-flight count where the image service owns the semaphore.
- Export only bounded labels and prove the metrics through a real service operation.

### 4. Consumer certification

- Guide programme art returns a usable Image DTO and responsive sources.
- Watch timeline art returns the same service-owned record rather than a hot-linked URL.
- Filler returns a non-animated still plus an animated hover Image when motion exists.
- Each production surface renders the shared frontend `Image` primitive with explicit `sizes`.

## Required gates

```sh
make image-cert
make check
make test-pg
make fe
LOOMARR_RELEASE=dev make build
docker build --target image-worker .
```

The repository corpus must have zero unexpected failures, crashes, protocol violations, or staging
leaks. Static cases have a 10-second ceiling, animated cases 30 seconds, and child peak RSS 768 MiB.
The final report path and summary are recorded in `PROGRESS.md`; transient reports remain ignored
artifacts rather than committed measurements tied to one machine.
