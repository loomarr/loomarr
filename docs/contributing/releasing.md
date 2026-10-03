# Publishing a release

**For:** maintainers cutting a release.
**You'll get:** what a `v*` tag publishes, and which workflow owns each part.

Server image publication and GitHub Release publication intentionally run in separate workflows from
the same `v*` tag. The image workflow owns package write and keyless-signing permissions. The release
notes workflow owns only `contents: write`; it cannot build, sign, or promote an image.

## One-time repository setup

Add a dedicated OpenRouter key as the masked Actions secret used only by release notes:

```sh
gh secret set OPENROUTER_RELEASE_API_KEY --repo loomarr/loomarr
```

The default model is `openai/gpt-5-mini`. To change it without editing the workflow, set the optional
repository variable `RELEASE_NOTES_MODEL` to an OpenRouter model that supports strict structured
outputs. The release process is designed for a small classification call, not model-authored prose.

## Preview before tagging

Authenticate `gh`, export the release-specific key locally, and generate a preview for the proposed
tag. The tag may already exist or be a commit-ish accepted by GitHub's generated-notes endpoint.

```sh
export OPENROUTER_RELEASE_API_KEY='...'
make release-notes-preview TAG=v0.2.0 PREVIOUS_TAG=v0.1.0-beta.1
```

The default output is `.artifacts/release-notes-v0.2.0.md`. Set `OUTPUT=/path/to/notes.md` to choose
another destination. Do not commit or paste the key. A tag-specific
`project/release/<tag>.md` file, when present, is prepended for human-authored framing and known
limitations.

## Docs pass

The docs ship with every release (#1682, maintainer decision D3; step 5 of #1572). Two things
happen without any extra action:

- **The docs site** ([Pages workflow](../../.github/workflows/pages.yml)) redeploys on every `v*`
  tag push, not only on pushes to `main`. It renders `docs/**` in place, so the published site
  always matches the tagged commit's documentation, even for a release that changed no docs.
- **The in-app Help set** is `//go:embed`-ded into the server binary (`docs/embed.go`), and the
  release image ([`release.yml`](../../.github/workflows/release.yml)) builds from the tagged
  source. Help is current the moment the image is current — there is no separate build step.

**The gate runs automatically on the tag.** The [Release notes workflow](../../.github/workflows/release-notes.yml)
runs `scripts/check-release-docs-gate.sh` right after it renders the notes and before it creates
the GitHub Release. It fails that workflow — publishing no Release — when the rendered notes
contain a New Features, Improvements, Bug Fixes, or Security Fixes entry and the tag range
touched nothing under `docs/`, `docs-site/`, or `README.md`. It reuses the same seven-category
classification the release notes already show (`internal/releasenotes`) rather than a second
taxonomy, and it is unconditional like every other step in that workflow: fixing the docs, or
the PR classification that misled it, is the only way past it. A release that only ships
Documentation, Dependencies, or Maintenance changes is not held to a docs change it has no
user-facing reason to need. `release.yml`, the separately hardened image-publication workflow, is
unchanged — it still owns only build, sign, and promote, with no added step or job.

What still needs a human: catch it *before* tagging, not after a failed Release publication.
Generate the preview notes and run the same gate locally over the same range:

```sh
make release-notes-preview TAG=v0.2.0 PREVIOUS_TAG=v0.1.0-beta.1
make docs-release-gate TAG=v0.2.0 PREVIOUS_TAG=v0.1.0-beta.1
```

See [`scripts/check-release-docs-gate.sh`](../../scripts/check-release-docs-gate.sh) for the gate
itself, shared by both the local preview and the workflow.

## What the model can and cannot do

The helper first asks GitHub to generate the exact merged-PR list, contributor list, and compare link.
It sends only each PR number and title to OpenRouter. The structured response is one closed object:
every exact PR number is a required property, and its value must be one of these fixed sections. That
shape encodes one assignment per PR even for unusually large releases instead of asking the model to
repeat PR numbers across seven independent arrays:

- New Features
- Improvements
- Bug Fixes
- Security Fixes
- Documentation
- Dependencies
- Maintenance

Repository code renders GitHub's original bullet for each assignment. It independently rejects unknown
JSON fields, invented PRs, duplicate keys, omitted PRs, invalid categories, malformed output, and
unrecognized GitHub change lines. It retries inference three times and then fails without creating a
GitHub Release. It never silently publishes uncategorized or model-authored notes.

## Tag and verify

Before requesting remote release certification, test the exact current `main` commit on the
maintainer's local machine. Record the commit, commands, and results with the release evidence. At
minimum, run every release-relevant gate this host supports: Go/Rust contracts and tests, Postgres,
web unit/build, visual/e2e/tuner browser evidence, shared clients, both Expo Android app builds,
legacy Android TV including its release bundle contract, image-worker certification, release
policy verification, and the [docs pass](#docs-pass) (`make docs-release-gate`). Host-incompatible
evidence such as iOS/tvOS and native arm64 image builds must be named explicitly and remain
required in protected CI; a local Linux pass cannot stand in for them. Agent sessions still never
run the maintainer's live-stack `make smoke*` targets.

Push the protected version tag only after that local evidence is green, the required commit gates
are green, and the exact current `main` commit has passed the proportional release-candidate scope:

```sh
gh workflow run ci.yml --repo loomarr/loomarr --ref main -f scope=release-candidate
```

That scope runs repository contracts, real-codec image-worker certification, and the native amd64
and arm64 image builds. Each image build also compares the packaged `LICENSE` and
`THIRD_PARTY_NOTICES.md` with the tagged source, then runs the unchanged `make test-ffmpeg` gate
against the `ffmpeg` and `ffprobe` copied out of that image (#661). That ties the gate to the exact
bytes the published GPL source releases correspond to. It deliberately leaves unrelated platform and UI matrices to their normal
change-based CI. A normal push run or `-f scope=full` run is never accepted as Docker-release
evidence, even when green, because neither proves that publication is isolated from those unrelated
jobs. Use `-f scope=full` only when a complete manual rerun is independently required.

Both release workflows start from the tag. If OpenRouter is temporarily unavailable, rerun the
failed **Release notes** workflow after service recovers; the separately hardened image publication
is unaffected.

Before signing, the publish helper (`scripts/publish-release-image.sh`) refuses an image index unless
both platform images carry a non-empty SPDX SBOM and a build provenance whose every recorded source
revision is the tagged commit of this repository. After both workflows finish, verify the GitHub
Release body (including its links to the third-party source releases named in
`THIRD_PARTY_NOTICES.md`), the GHCR manifest, signature, SBOM, and provenance against the tagged
commit. The tag-specific header remains the place to state limitations
that cannot be derived from pull requests.

## Image-worker certification

Moved verbatim from `design.md` §22 (#1572). The image service's design is in
[`design/images.md`](../design/images.md).

The required worker is release infrastructure, so its gate is broader than unit codec coverage.
`make image-cert` drives the installed `loomarr-image` executable through the same bounded protocol
and manifest validation used by the application, then writes a machine-readable report under this
worktree's `.artifacts/<instance>/` directory. It has two corpus modes:

- With no arguments, it uses the repository's deterministic certification corpus. That corpus
  covers opaque JPEG, transparent PNG, static WebP, animated GIF, APNG and WebP, finite and infinite
  loops, fractional and zero frame delays, a one-frame animated container, corrupt input, and every
  resource ceiling whose refusal is observable without allocating the forbidden resource.
- `make image-cert IMAGE_CERT_CORPUS=/absolute/read-only/path` scans an operator's existing raster
  corpus. It never modifies source files, follows no symlinks, performs no network I/O, and treats a
  supported-looking file that cannot complete inspection plus the requested ladder as a failure.
  Unsupported files are reported as skipped; stable budget refusals are reported separately from
  crashes, malformed manifests, and I/O failures.

Every accepted case produces an inspection plus a 320-pixel JPEG/WebP/AVIF ladder. Motion-preserving
WebP is required for an animated source; JPEG and AVIF must be the first composited presentation
frame. The certifier independently verifies the source hash, output signatures, dimensions, hashes,
motion flag, and that staging is empty after each case. A run fails on a worker crash, protocol or
manifest violation, unexpected refusal, leaked staging file, incorrect visible timeline, or a
resource ceiling breach.

The deterministic gate is intentionally generous enough to survive shared CI hardware while still
catching runaway work: each static case must complete within 10 seconds, each animated case within
30 seconds, and a worker process must remain below 768 MiB peak resident memory. The report records
per-case wall time, source and output bytes, peak RSS where the host exposes it, plus p50/p95/max
summaries. These are certification ceilings, not product SLOs; lowering them requires corpus evidence
and raising them is a design change.

Production exposes the same boundary at `/metrics`: worker operations and stable outcomes, wall
time, input/output bytes, peak RSS, queue wait, and in-flight count. Labels are bounded vocabulary
(`inspect`/`render` and stable result classes), never an image hash, path, URL, MIME supplied by a
caller, or free-form error text.

Those measurements are also the admission gate for another Rust capability. A proposal must name
the production operation that dominates a captured worker-duration, queue-wait, peak-RSS, byte, or
failure distribution, then reproduce it through the certification or benchmark seam. An operation
does not move merely because it handles media or because a Rust implementation is possible. The
Image service itself imports no Go `image` package; its deterministic corpus generator lives only in
`cmd/image-cert`, and a source architecture test keeps that boundary closed. Filler-era frame hints
remain in `internal/mediatools`: they sample at most 1,024 pixels per keyframe in background work and
no captured Image-worker evidence identifies them as a bottleneck. Therefore V59b selects no next
capability. A future measured case extends this worker protocol and resource boundary rather than
creating a second Rust service.

`make image-bench` is the performance companion to certification. It drives the installed release
worker through `internal/images/rustgen` with deterministic poster, backdrop, and icon sources and
the complete AVIF width ladder for each role. Source creation and the capabilities self-test happen
outside the timed region. After one warm-up per role, three runs record the recipe, host architecture
and logical CPU count, process/Rendition counts, output bytes, complete-ladder throughput,
p50/p95/max worker time, median ladder time, and maximum child peak RSS in a machine-readable report
under the worktree artifact directory. The current baseline deliberately performs one worker request
per missing AVIF Rendition, matching the background job shape that later batching will replace.

The benchmark is opt-in locally and manually dispatched on native amd64 and arm64 CI runners. It is
not part of comprehensive verification and has no wall-clock pass/fail threshold: shared-runner timing is comparative
evidence, not a product SLO or correctness gate. Comparisons are valid only for the same corpus,
recipe, release profile, architecture, and CPU profile. `make image-cert` remains the authority for
protocol, output, and resource-ceiling correctness.

The consumer half of the gate is observable through public seams. Guide programme art, Watch
timeline art, and Filler still/hover art must carry a real Image record through their HTTP DTO and
render through the shared frontend `Image` primitive. An animated filler hover must offer an
animated WebP rendition while its still fallback remains non-animated. Tests exercise those HTTP
responses and rendered elements; querying image tables or asserting private renderer calls is not
certification evidence.
