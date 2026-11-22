# Images

Formerly `design.md` §22. `internal/media/images` is one service that owns ingest, storage, derivatives
and serving for every image Loomarr shows: uploaded channel icons, clip stills and hover loops, and
TMDB artwork (decision [0020](decisions/0020-one-image-service.md)). Every image path uses it and
none bypasses it.

Go owns image identity, policy, jobs, authorization, storage records and publication. The required
Rust `loomarr-image` worker owns every operation that interprets pixels: validation, inspection,
static or animated decode, compositing, resize, encode, ThumbHash and dominant colour. It also renders
the channel bug and decodes image subtitle tracks (see *Operations beyond renditions*). There is one
production renderer and **no Go codec fallback**. Go may measure a raw pixel buffer that ffmpeg
decoded itself (frame hints, the watermark overlay self-check); Go never decodes an encoded image. The schema lives in the migrations, the width
ladders in code, and the image jobs in the scheduler registry. The frontend `Image` contract is in
[`frontend-design.md`](../frontend-design.md#8-the-image-primitive-formerly-designmd-22s-frontend-contract),
and the worker's runtime certification in
[`contributing/releasing.md`](../contributing/releasing.md#image-worker-certification).

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

- **Boot self-test.** `loomarr-image capabilities --protocol 1 --self-test` must agree on release,
  protocol, recipe, formats and an embedded static-plus-animation probe before readiness.
- **One request, one staging directory.** `loomarr-image generate --protocol 1` reads one bounded JSON
  request on stdin (a content-addressed source and its SHA-256, a private staging directory, targets
  and limits) and writes one bounded result on stdout; stderr is bounded diagnostics. Rust writes only
  complete relative files in staging and never sees the Store or publication paths.
- **Go publishes all or nothing.** Go checks the exact target set, containment, file type,
  signatures, sizes and hashes, renames each file, then commits every derivative row in one
  transaction. Any failure removes every file the request promoted, so a partial ladder is never
  servable.
- **Hard ceilings:** 8 MiB compressed input, 16,384 px per side, 40 MP canvas, 600 frames, 60 s of
  animation, 600 million decoded frame-pixels, 16 targets, 64 MiB output. Lowering them is allowed
  before release; raising them is a design change. Rust checks them before and during allocation.
- **Capacity.** Go owns worker slots and cancellation. Inspection and lazy JPEG/WebP are interactive;
  scheduled AVIF is background and may use at most all-but-one slot, so interactive work always has
  one. Refusals carry stable machine codes, and nothing falls back to Go pixel processing.
- `--benchmark-avif-threads` exists only for measurement; benchmarks are evidence, never a gate.

### Operations beyond renditions

Two more operations run in the same worker, because both interpret pixels from bytes Loomarr does
not control. Each is its own subcommand with its own versioned request, and both follow the rules
above: one bounded JSON request on stdin, one private staging directory, complete relative files
only, Go publishing all or nothing, stable refusal codes, and the shared worker slots.
`capabilities` lists every operation the binary supports, and the boot self-test runs an embedded
probe for each, so a worker without one fails readiness instead of failing the first request.

- **`render-bug --protocol 1` draws a channel's bug** (`internal/playout/channelbug` chooses it,
  [`playout.md`](playout.md) owns the look). The request names either a custom upload (a
  content-addressed original and its SHA-256) or a callsign with its resolved look, colour and
  holiday version, plus the output height. The result is one straight-alpha RGBA PNG at exactly
  that height. The Geist Bold font is embedded in the binary; no system font is read. The recipe
  (`loomarr-bug-v1`) fixes the font, supersampling factor, resize kernel and shadow, and is part of
  Go's cache key. An upload is held to the rendition ceilings above; the output height is at most
  512 px.
- **`decode-subtitles --protocol 1` turns an image subtitle track into timed stills** for GPU
  burn-in ([`playout.md`](playout.md)). The request names the extracted track (`.sup` for
  `hdmv_pgs_subtitle`; the `.idx`/`.sub` pair for `dvd_subtitle`) and its SHA-256. The result is
  the canvas size and an ordered list of display states, each with its start and end in
  milliseconds and either one full-canvas straight-alpha PNG or none (the screen is clear). Every
  later step (ffv1 conversion, the per-seek ffconcat list, the overlay) is codec-neutral.
  Ceilings: 256 MiB input, 3840×2160 canvas, 64 objects per state, 50,000 states, 512 MiB output.
  Rust checks them before allocating each state and while decoding it.

**Format choices.** A JPEG fallback stays because the few percent of browsers without AVIF or WebP
are exactly a media server's clients (televisions, old tablets, embedded WebViews). Selection uses
`<picture>` with `type=` over distinct per-format URLs, not `Accept` negotiation, so every artifact
stays independently cacheable and immutable. JPEG XL is not supported while browsers ship it
disabled by default.

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
  a non-public address (`httpx.IsPublicIP`: private, loopback, link-local, CGNAT/Tailscale,
  reserved and IPv6 forms embedding those), a response size cap, and the §6 per-service timeout.
  `httpx.IsPublicIP` is the single "is this address public?" predicate; clients fetching
  stranger-supplied URLs (channel-proposal references, Web Push endpoints) dial through
  `httpx.PublicDialContext`, which resolves, refuses any non-public answer, and pins the checked
  address. An architecture guard keeps hand-rolled range checks out of every other package.

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

- **Six-month cache ceiling.** TMDB's terms encourage caching but cap it. Immutable headers apply to
  Loomarr's own content-addressed URLs; the GC job expires TMDB-origin images older than the TTL by
  `origin_fetched_at`.
- **Expiry deletes, then requeues.** The original and every derivative are deleted and the row goes
  back on `images-fetch`'s list (every minute). Re-fetching first would put compliance inside an error
  branch: an unreachable TMDB would keep expired bytes forever.
- **One GC run purges everything due.** `images-maintenance` runs under the 30-minute long-job
  timeout. Its purge reads expired images 500 at a time, in (fetch time, hash) order, each read
  starting after the last image of the one before. It stops at a short page. Images are purged one
  at a time, each a file delete and then a row write, so a run that has a large backlog takes longer
  but never adds load in parallel. An image whose purge fails is passed over and tried by the next
  daily run. It is never read again in the same run, and it never holds back the images behind it.
  A run that the timeout or a shutdown ends stops between images and skips the steps it has not
  reached. The job then fails, and the next run carries on.
- **The fetch queue is served by class, then by arrival.** `fetch_priority` is 0 for artwork a
  surface adopted, 1 for an image the purge requeued for a refresh, and 2 for artwork adopted in
  bulk to fetch last (episode stills). Within a class, `fetch_queued_at` sets the order: the
  adoption, or the requeue. Artwork adopted the day a purge requeues thousands of images therefore
  goes ahead of all of them. The purge requeues a bulk image in the bulk class, not the refresh
  class: stills are fetched in a burst, so they expire in one, and tens of thousands of them in
  the refresh class would keep a purged poster, which reads as pending until it is fetched again,
  waiting hours behind them. The queue's read follows `idx_images_fetch_queue` (origin,
  origin_fetched_at, fetch_priority, fetch_queued_at, hash), so a pass reads the front of the queue
  without sorting it.
- **A failed download backs off.** Each failure is recorded on the row (`fetch_failures`,
  `fetch_failed_at`) and holds the image off `images-fetch`'s list until `fetch_retry_at`. The wait
  doubles from five minutes (5m, 10m, 20m … 21h20m after the ninth failure) and is a day from the
  tenth failure on. A failure that waiting will not fix waits a day straight away: a 404 or 410, a
  URL or redirect this install refuses, a body over the size cap, or bytes the image worker refuses.
  A pass in which every attempt that went to the network got no response is the network being
  down. A permanent failure never reaches the network, so it neither makes nor unmakes an outage.
  In an outage each unreachable image waits five minutes and no failure is counted, so an outage
  does not push the whole catalogue's retries back. An image that never answers still waits five
  minutes between tries, even when it is alone in its pass. A foreground fetch skips an image
  that is still backing off, and the caller reports it as backing off, not as failed. A failing image therefore never holds the front of the queue. The
  download that lands clears all three fields. An image is never dropped for failing: it is retried
  once a day for as long as something references it.
- **Adopting a URL reuses its row.** Adoption looks the source URL up. It reuses the row with bytes
  if one exists, else the row still waiting for them: the placeholder, or an image the purge
  requeued under its content hash. Re-adopting a URL therefore never queues a second download.
- **Orphans are collected before expiry,** so an unreferenced expired image is never queued for a
  pointless download.
- **Attribution:** the TMDB logo, less prominent than Loomarr's branding, with the notice "This
  product uses the TMDB API but is not endorsed or certified by TMDB." under Settings → About →
  Data sources, and the same line in the README (#2853).
- **Connections:** well under TMDB's 20 per IP, with jittered backoff on 429 and 5xx.
- **Fetch `original` once** and build the ladder locally: one origin request per artwork. `profile`
  sizes use a height token (`h632`), and SVG assets exist only at `original`. An episode's still
  is the exception: it is fetched at `w300` (#4476), the size an episode row draws, because a
  library holds tens of thousands of them and no surface draws one larger.

### Metadata

Rich metadata is recorded in the `images.meta` column. Encoded derivatives strip source-container
metadata: smaller files, no incidental PII from operator uploads, and no ICC ambiguity. Consumers that
need attribution or provenance read the image row instead of parsing format-specific EXIF/XMP boxes.

### Two modes

- **Owned** (the default): ingested, content-addressed, permanent record, immutable URL. The
  Library's artwork is owned: a title's poster, fanart (role `backdrop`) and clear logo, and an
  episode's still (role `thumb`, #4476), each imported when the scanner writes the title or
  episode, a file beside the media ingested then and a provider's URL adopted for the fetch job
  (library.md, "Posters, backdrops and logos" and "Episode stills"). Role `thumb` is a 16:9
  still, an episode's or a filler clip's, on the 300/780/1280 ladder; role `backdrop` is fanart,
  on the ladder to 3840.
- **A rendition is never an upscale where it would show.** Fanart drops a full-screen rung (1920
  and up) its original cannot fill. A still drops every rung past the first that is wider than
  its original: a 300-wide catalogue still is served at 300 alone, and a 2x screen drawing it at
  300 CSS pixels takes that rung and scales it in the browser. A filler clip's still is a full
  frame of its video, so a 720p or larger clip keeps all three rungs. The other roles keep their
  whole ladder, and an image whose width is not yet known (adopted, bytes not here) lists the
  whole ladder.
- **Proxied**: a short-TTL pass-through with **no permanent row**, for high-volume ephemeral thumbnails
  such as source-search results. Ingesting hundreds of thumbnails for a search the operator abandons
  would be the wrong trade; proxying still removes the third-party origin from the browser.
