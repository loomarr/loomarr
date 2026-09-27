# Images

Formerly `design.md` §22. `internal/images` is one service that owns ingest, storage, derivatives
and serving for every image Loomarr shows: uploaded channel icons, clip stills and hover loops, and
TMDB artwork (decision [0020](decisions/0020-one-image-service.md)). Every image path uses it and
none bypasses it.

Go owns image identity, policy, jobs, authorization, storage records and publication. The required
Rust `loomarr-image` worker owns every operation that interprets pixels: validation, inspection,
static or animated decode, compositing, resize, encode, ThumbHash and dominant colour. There is one
production renderer and **no Go codec fallback**. The schema lives in the migrations, the width
ladders in code, and the image jobs in the scheduler registry. The frontend `Image` contract and the
worker's runtime certification are still in [`design.md` §22](../design.md#22-image-service--one-pipeline-for-every-image).

## The service

### Identity and storage

An image is identified by the **sha256 of its original bytes**, matching the content-hash identity
§10 arrived at for clips. Files live under `images.dir` (default `/data/images`), sharded two levels
by hash prefix so no directory accumulates a hundred thousand entries:

```text
/data/images/
  orig/ab/cd/abcdef0123….jpg          # original bytes, content-addressed
  drv/ab/cd/abcdef0123…_w320.webp     # derivatives: regenerable, evictable
  drv/ab/cd/abcdef0123…_w320.avif
```

⚠ **No image bytes are stored in the database.** The `channel_icons` table this replaces (retired-ok)
put upload bytes in the DB specifically so they would ride the §16 backup — which worked, but made the database
the wrong shape for a general image service and would not have scaled to ingested remote artwork. See
*Durability* below for what replaces that guarantee, and what deliberately does not. *(That table was
dropped outright in V52 phase 8, with no backfill: this project has no production installs, so a job
written to migrate data that does not exist would be debt rather than safety — retired-ok.)*

### Derivatives: split by cost, not by symmetry

WebP and AVIF are two orders of magnitude apart in encode cost, and treating them uniformly gets one
of them wrong:

- **Static WebP and JPEG generate lazily on first request**, behind `singleflight`. We do not know in
  advance which widths a surface will ask for, so an eager matrix would encode mostly-unserved
  renditions.
- **Animated WebP ladders generate lazily at bounded ladder widths**, behind the same singleflight and
  global worker cap as stills. The explicitly requested first-presentation-frame JPEG is an output
  contract for clients that do not display motion, not another renderer.
- **AVIF generates in a background job, never on a request.**

The load-bearing reason is **concurrency and request latency**. AVIF is produced by the pure-Rust
`ravif` path with one encoder thread, but a cold grid must still not queue expensive worker processes
in front of HTTP responses. The background job drains that work under the same global worker cap.
The one-thread limit is measured policy, not a library default: `make image-parallelism-bench`
compares equal logical-CPU budgets as multiple one-thread processes versus fewer processes with
2/4/8 rav1e threads. The worker accepts that override only on an explicit benchmark command; the
production protocol has no thread knob. Across the 2-, 4-, and 8-CPU profiles, encoder threading
reduced one process's latency but lost aggregate Image and Rendition throughput to independent
one-thread workers: the two-thread shapes were 15.6%, 12.6%, and 22.4% slower respectively in the
three-sample poster medians. They also emitted different AVIF byte counts, so adoption would require
a new immutable recipe rather than a scheduling-only change. It did not earn weighted permits, and
production remains one thread.

⚠ Therefore **AVIF coverage is eventually-consistent, and the frontend contract must tolerate it.**
`<picture>` does this natively: when no AVIF derivative exists the `<source>` is simply not emitted
and the browser takes WebP. No request ever blocks on AVIF, and no surface has to know whether the
job has caught up.

### Formats and negotiation

Three formats are emitted: **AVIF** (smallest), **WebP** (near-universal), and **JPEG** (the floor).

**Animation is a first-class ladder, not an original-file exception.** Animated WebP, GIF, and APNG
inputs produce responsive animated-WebP Renditions; a one-frame animated container is static. JPEG
and still WebP/AVIF requests use the first fully composited presentation frame. Animated AVIF is not
emitted. Motion preservation means the same displayed canvas at every frame boundary, with frame
order, per-frame duration, alpha, and finite/infinite looping preserved. GIF and WebP disposal flags
do not map one-for-one, so the worker may normalise to full-canvas replace frames after applying source
blend and disposal operations, including restore-to-previous. Container bytes and flags need not be
identical; the visible timeline does. WebP timestamps have millisecond resolution, so fractional APNG
delays are rounded on cumulative time (preventing per-frame drift); source zero-delay frames use a
deterministic 10 ms viewer floor.

Every transform uses a named recipe (`loomarr-rendition-v2` currently) fixing decoder, resize kernel,
quality, effort, animation normalisation, thread limits, and encoder versions. `v2` supersedes the
initial `v1` direct-from-source resize loop: static same-format ladders are stepped largest to
smallest. The recipe is present in the derivative key, internal filename, and public URL query, so
that intentional byte change creates a distinct immutable cache identity instead of mutating a
year-cached `v1` URL. Output SHA-256 remains recorded even when architecture-specific native encoding
produces different but contract-equivalent bytes.

### Required renderer protocol and limits

At boot the Go composition root runs `loomarr-image capabilities --protocol 1 --self-test`. The exact
Loomarr release, protocol, recipe identifier, required formats, and embedded static-plus-animation probe
must agree before readiness. The production command `loomarr-image generate --protocol 1` reads one
bounded JSON request from stdin, writes one bounded result to stdout, and uses stderr only for bounded
diagnostics. The request names a content-addressed source, its expected SHA-256, a private staging
directory, explicit format/width/motion targets, and resource limits. Rust writes only complete safe
relative files inside that staging directory and returns their metadata and SHA-256 values. Go
validates the exact target set, containment, regular-file type, signatures, sizes, and hashes before
publication. Every missing AVIF ladder is one worker request. Go atomically renames each verified
file, then commits the complete derivative-row set in one Store transaction; any file or Store
failure removes every file promoted by that request, so a partial ladder is never servable. Rust
never sees the Store or canonical publication paths.

The opt-in benchmark command may append `--benchmark-avif-threads 1..8`; it is deliberately absent
from application composition and exists only to measure the fixed production decision through the
same executable and manifest validation. Benchmark reports record the CPU profile, concurrent
processes, encoder threads, throughput, worker duration, output bytes, and aggregate child peak RSS.
They are evidence, never a timing gate.

The initial hard ceilings are an 8 MiB compressed input, 16,384 pixels per dimension, a 40-megapixel
canvas, 600 frames, 60 seconds of animation, 600 million cumulative decoded frame-pixels, 16 targets,
and 64 MiB of output. The fixture corpus may lower them before release; raising them is a design
change. Rust checks limits before large allocations and while streaming frames. Go owns the global
worker capacity and cancellation: it terminates on context cancellation and removes the private
staging directory. Inspection plus lazy JPEG/WebP work is **interactive**; scheduled AVIF work is
**background**. Background processes may occupy at most one fewer than the total capacity whenever
the host has at least two slots, leaving one slot immediately available to interactive work. On a
single-slot host the running process cannot be preempted, but an interactive waiter wins the next
admission before another background Image starts. Queue-wait metrics carry that two-value class so
operators can distinguish request pressure from an AVIF drain. Corrupt, unsupported, source-changed,
limit, decode, encode, I/O, and internal worker refusals have stable machine codes. None invokes Go
pixel processing.

⚠ **The JPEG floor is a deliberate Loomarr-specific call, not caution for its own sake.** AVIF is at
~95% and WebP ~97% global support, and a general web app could reasonably drop the fallback. The
missing few percent are concentrated in old iOS and legacy Android WebViews — which is precisely the
population of a self-hosted media server's clients: televisions, ageing tablets, embedded browsers.

Selection is by **`<picture>` with `type=`, over distinct per-format URLs** — deliberately *not*
`Accept` + `Vary: Accept`. The wider industry has moved toward `Accept` negotiation on coverage
grounds (a `<picture>` element only helps `<img>` tags you author, so CSS backgrounds and third-party
embeds keep receiving JPEG). That argument does not bind here: this design removes the app's only CSS
`background-image` image consumer, and distinct URLs keep every artifact independently cacheable and
genuinely immutable, which `Vary: Accept` does not. Revisit only if a non-`<img>` consumer appears.

**JPEG XL is deliberately not supported.** It returned to active development in 2026 — Chrome shipped
a Rust decoder, Firefox compiled one in — but **both are disabled by default**, leaving Safari as the
only default-on implementation. Track it; ship nothing.

### Serving and cache policy

Derivative URLs are content-addressed, so they are safe to declare permanently immutable:
`Cache-Control: public, max-age=31536000, immutable`, plus a **strong `ETag` equal to the content
hash**. `Last-Modified` is deliberately **not** sent — it invites heuristic freshness and adds nothing
when the URL already changes with the content.

⚠ A conditional request must be answerable **from the URL hash alone, without touching disk**. The
whole point of content addressing is that a 304 costs nothing.

### Visibility is a property of the image, not the route

Every other raw-byte route in §7.1 carries a fixed role. Images cannot: a **channel icon must be
public** (Tunarr fetches it machine-to-machine with no credentials, exactly as it would fetch a TMDB
poster), while a **clip still is member-visible**. So the serve operation mounts as `RolePublic` and
the handler enforces the row's `visibility` against the session.

⚠ **A member-visible image requested without a session is a 404, not a 403** — matching the existing
convention for clip thumbnails, where a distinct error would confirm which hashes exist.

### Security

The properties the channel-icon path already established are carried forward, not re-derived:

- **Raster-only, enforced by byte sniff**, never by the declared Content-Type or the filename. SVG
  stays refused: the serve endpoint is public and returns bytes with an image content type, so an
  uploaded SVG carrying `<script>` would be stored XSS in Loomarr's own origin. Raster-only removes
  the class rather than attempting to sanitize it.
- `X-Content-Type-Options: nosniff` on every serve.
- Path containment by resolving to absolute form and testing with `filepath.Rel`, because a `..`
  component in the result is the only reliable containment test however the input was spelled.
- Machine-client URLs (channel icons handed to Tunarr, and native/off-origin consumers) derive
  from `server.public_url`, **never** from request headers — `Host` and `X-Forwarded-Host` are
  attacker-controllable, and these URLs are stored and fetched downstream. **Image records sent
  to the in-app browser are the same-origin `/v1/images/...` paths instead.** The page already has
  an origin; making its `src`/`srcset` depend on the separately configured machine-client address
  strands every image when that address is container-only, VPN-only, or otherwise unreachable
  from the browser. This is the same browser-vs-native split as §9.1's `relativeUrl`/`url` pair.
- ⚠ **New with this section: SSRF defence.** Adopting a remote image is a new outbound request driven
  by input, which nothing in the product had before. Host allowlist, `https` only, no redirect into
  private address ranges, a response size cap, and the §6 per-service timeout.

### Durability — what survives a restore, and what does not

§16's backup is a **database** backup. It always was; this section does not change it, and adds no
second artifact. Recovery therefore differs by origin:

| origin | after losing `/data/images` |
| --- | --- |
| derivative | regenerated on next request |
| `remote` | re-fetched from `source_url` |
| `extracted` | re-derived from the clip by the existing ffmpeg pass |
| `upload` | **lost** |

⚠ **Uploads are genuinely unrecoverable, and that is an accepted tradeoff — which obliges the system
to make the loss visible rather than silent.** A row pointing at bytes that are not there must never
render as a broken image, nor as an empty box that looks like a design decision. Two things carry
that: the GC job **counts** rows with no file and no `source_url` and surfaces them as a system
warning, and the affected surfaces fall back to their real designed empty states (a channel's
monogram, the icon field's glyph).

Operator-facing consequence, which must appear in the help docs and in the `images.dir` setting's own
documentation: **`/data` is one volume, and the volume is what to back up** — the application backs up
the database only.

### TMDB compliance

⚠ **TMDB's API terms permit caching but cap it at six months.** Caching is otherwise encouraged — TMDB
staff recommend serving posters from your own cache, and the terms' "excessive bandwidth" restriction
makes a local cache the *compliant* posture. But the ceiling is real, and it interacts with the
permanently-immutable cache headers above.

The resolution: the immutable header applies to **our** content-addressed derivative URLs, which are
served from our own disk. Alongside it, the GC job expires any TMDB-origin image older than the
configured TTL, keyed on `origin_fetched_at`. Because URLs are content-addressed, a re-fetch
yielding identical bytes produces an identical URL, so revalidation is invisible downstream.

⚠ **Expiry means the bytes are DELETED and the row is requeued, not refreshed in place** (V52 phase
3b). The GC removes the original and every derivative, clears `origin_fetched_at`, and leaves the
row on `images-fetch`'s work list — which runs every minute, so the operator-visible cost is a
placeholder for well under a minute per image, once every six months.

The alternative — re-fetch first and delete only if it fails — reads as strictly nicer and is
wrong for a specific reason: it puts the compliance question inside an error branch. TMDB being
unreachable for a day would silently keep serving expired bytes, and the ceiling would then be
enforced by nothing. A ceiling that holds only while the network is up is not a ceiling. Deleting
unconditionally means no cached TMDB byte outlives the TTL regardless of what upstream is doing,
which is the only property the licence term actually asks for.

⚠ **The GC collects orphans BEFORE it expires**, because both sweeps can select the same row: an
image that is both past its TTL and no longer referenced. Expiring first would purge its bytes and
queue a fresh download moments before the orphan sweep deleted it — and if that delete failed, a
download instruction for an image no surface will ever show is what would survive.

⚠ **This must exist from the first migration.** Retrofitting expiry into a content-addressed store is
painful, and a store that has already accumulated a year of artwork cannot be brought into compliance
by adding a column.

Two further obligations:

- **Attribution is mandatory and specific.** The TMDB logo must be shown, must be *less prominent*
  than Loomarr's own branding, and this notice must appear prominently: *"This product uses TMDB and
  the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB."* This is a UI
  deliverable, not a comment.
- **Concurrency is capped at 20 simultaneous connections per IP** by TMDB. The fetcher stays
  comfortably below it and backs off with jitter on 429/5xx.

**Fetch `original` once and generate the ladder locally.** One origin request per artwork instead of
one per width: far below the connection cap, full control of resampling quality, and the periodic
re-fetch touches one file per image rather than the whole ladder.

⚠ Two TMDB API details the ladder code must not assume away: `profile` sizes use a **height** token
(`h632`), so size parsing cannot assume a `w` prefix; and **SVG assets are only offered at
`original`** — TMDB does not resize them.

### Metadata

Rich metadata is recorded in the `images.meta` column. Encoded derivatives strip source-container
metadata: smaller files, no incidental PII from operator uploads, and no ICC ambiguity. Consumers that
need attribution or provenance read the image row instead of parsing format-specific EXIF/XMP boxes.

### Two modes

- **Owned** (the default): ingested, content-addressed, permanent record, immutable URL.
- **Proxied**: a short-TTL pass-through with **no permanent row**, for high-volume ephemeral thumbnails
  such as source-search results. Ingesting hundreds of thumbnails for a search the operator abandons
  would be the wrong trade; proxying still removes the third-party origin from the browser.
