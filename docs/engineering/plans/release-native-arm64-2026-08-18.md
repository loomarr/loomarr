# Plan: native arm64 in release.yml (drop QEMU) — 2026-08-18

**Goal:** the release image build (`release.yml`) currently builds `linux/arm64` under **QEMU
emulation** on an amd64 runner — ~30 min of emulated Rust+ffmpeg compile, felt directly cutting
`v0.1.0-beta.1`. CI's *verify* build already went native (`ci.yml` `image` job: matrix over
`ubuntu-24.04` + `ubuntu-24.04-arm`, `push: false`). Port that native pattern into the *publish*
path, preserving every signing guarantee.

**Why this is a careful change, not a quick edit:** `release.yml` is locked by
`internal/releaseverify.VerifyReleaseWorkflow`, backed by **28 adversarial test cases** that protect
the keyless-cosign signing identity (`packages: write` + `id-token: write`). The gate asserts a
SINGLE `images` job, an exact 6-action sequence, `buildCount == 1`, digest-only build, and
"no public tags before signing." Going native means two jobs → the verifier and its `good` fixture
and all 28 cases must be re-derived. Weakening any invariant is a prime-directive violation.

## Current flow (one job, QEMU)

`images` (ubuntu-latest): checkout → validate-source → setup-qemu → setup-buildx → GHCR login →
protect-tag → **build both arches (QEMU), push by DIGEST only** (`push-by-digest=true`, no public
tag, `provenance: mode=max`, `sbom: true`) → install cosign → `publish-release-image.sh`.

`publish-release-image.sh` takes ONE multi-arch **index** digest, asserts it holds exactly
`linux/amd64 + linux/arm64` (jq over the manifest list), `cosign sign` + identity-bound
`cosign verify` the index, then `imagetools create --tag` the public SemVer/latest tags and
re-inspects. **The helper does not care how the index was built — only that the digest is a valid
2-arch index.** ← this is the key: the helper stays UNCHANGED.

## Target flow (two jobs, native)

### Job 1 — `build` (matrix, native per arch, digest-only, no signing)
- `strategy.matrix.include`: `{platform: linux/amd64, runner: ubuntu-24.04}`,
  `{platform: linux/arm64, runner: ubuntu-24.04-arm}`; `fail-fast: false`.
- `runs-on: ${{ matrix.runner }}`.
- Steps: checkout → validate-source (only meaningful once, but running per-arch is harmless and
  keeps each leg self-contained — OR gate it to one arch; decide in impl) → setup-buildx (NO
  setup-qemu — native) → GHCR login → build-push-action with `platforms: ${{ matrix.platform }}`,
  `outputs: type=image,name=$IMAGE,push-by-digest=true,push=true`, `provenance: mode=max`,
  `sbom: true`. **Push by digest only, per arch, no tag.**
- Output each arch's digest: `outputs.digest-${{ matrix.platform }}` via a step output +
  `strategy` fan-in. (GitHub matrix outputs are awkward — likely write each digest to an artifact
  or use `jobs.build.outputs` keyed per arch; settle mechanism in impl.)

### Job 2 — `publish` (needs: build; single runner; merge + sign)
- `runs-on: ubuntu-latest`.
- Steps: checkout → validate-source (the authoritative one) → GHCR login → protect-tag →
  **merge the two per-arch digests into ONE index** via `docker buildx imagetools create` (push by
  digest, capture the index digest) → install cosign → `publish-release-image.sh` (UNCHANGED; fed
  the merged index digest). The helper's jq check already proves exactly-2-arch, so a bad merge is
  caught before signing.

## The releaseverify gate rewrite (`internal/releaseverify/workflow.go`)

`VerifyReleaseWorkflow` must accept the two-job shape while keeping every invariant. Changes:
1. **Job set**: was `len(jobs)==2 && jobs[0]=="images"` (one job). New: exactly two jobs named
   `build` and `publish`, in that order. (`jobs.Content` has 2 entries per job → len == 4.)
2. **`build` job**: matrix over the two native runners; steps = checkout, (validate-source),
   setup-buildx (NO qemu — actively REJECT `docker/setup-qemu-action` now, the opposite of today),
   login, one `docker/build-push-action` that is digest-only (`verifyDigestOnlyBuild` reused) and
   builds a SINGLE `${{ matrix.platform }}`. No cosign, no public tag, no publish helper here.
3. **`publish` job**: checkout, validate-source (the required one), login, protect-tag, the
   merge step (a `run:` calling `imagetools create ... push-by-digest`), cosign-installer (exactly
   v2.6.5, once), `publish-release-image.sh` (once, via `verifyPublisherEnvironment`). Order asserted
   as today. `buildCount` becomes a per-job assertion (one build in `build`, zero in `publish`).
4. **Preserve ALL 28 invariants**, re-mapped: "no second publication job" now means "no job other
   than build/publish"; "raw signing bypass"/"public tags before signing" now guard the `publish`
   job; digest-only guards the `build` job; identity-bound cosign stays in `publish`.
5. **The `good` fixture** (`goodWorkflow()` in the test) is rewritten to the new two-job release.yml,
   and each of the 28 mutation cases is re-pointed at the right job/step. Add new cases: "build job
   must not sign", "publish job must not build a public tag", "qemu is rejected" (native-only),
   "merge step must push by digest".

## Verification loop (cheap, local — no heavy gates)
- `go test ./internal/releaseverify/` reads the file + fixture; iterate release.yml ↔ workflow.go ↔
  fixture until green. This is a fast Go unit test, safe to run locally.
- `make ci-lint` (actionlint) on the new release.yml.
- Do NOT dry-run the release workflow (it publishes). Its correctness is proven by the verifier +
  the helper's own manifest assertions; the first real proof is the NEXT tag.

## Sequencing
Land after the cancelled-gate fix (#429) and any easier CI wins. This one is its own PR, reviewed
carefully, because it edits the signing path. The helper `publish-release-image.sh` should not need
changes — if it does, that's a signal the design drifted from "helper signs an index digest."
