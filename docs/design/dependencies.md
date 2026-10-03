# Dependencies

Formerly `design.md` §14. Every direct dependency and bundled tool, with the reason it earns its
place. A new dependency adds its row here in the same PR (AGENTS.md).

**Pins are exact.** Manifests use one concrete version, never a range; container images and CI
actions use digests or commit SHAs; Renovate proposes the next exact pin. Peer ranges are
compatibility contracts only. A platform hold (for example Expo's) stays exact and is documented next
to the dependency. The Expo/Metro graph carries a local source patch on `image-size@1.2.1` (#1162) so
malformed ICNS/JXL/HEIF boxes fail instead of looping; retire it only when a compatible upstream fix
passes the same consumer tests.

## Server (Go 1.27+)

| Concern | Choice | Why |
| --- | --- | --- |
| HTTP router | stdlib `net/http` ServeMux via Huma's `humago` | no third-party router; same-origin SPA means no CORS layer |
| API framework | Huma v2 | code-first OpenAPI 3.1 with validation and docs from one definition ([0002](decisions/0002-code-first-openapi.md)) |
| Bootstrap config | `caarlos0/env` | struct tags for the env layer feeding the settings registry ([`config-design.md`](../config-design.md)) |
| Database access | `database/sql` with `modernc.org/sqlite` and `pgx` (stdlib shim) | one store code path; no cgo; dialect differences live in migrations and `ClaimDue*` |
| Migrations | `goose` with `embed.FS`, one directory per dialect | simple embedded story |
| Background jobs | `riverqueue/river` + `riverdriver/riversqlite` | durable jobs, retries and run history; SQLite support is upstream-experimental, accepted deliberately; schema applied by `rivermigrate`, never the River CLI |
| Cron parsing | `adhocore/gronx` | correct next-tick for 6-field cron schedules with no transitive dependencies |
| Sessions | hand-rolled in the Store | random 256-bit token, SHA-256 at rest; revocation per user on both backends |
| Human passwords | `x/crypto/argon2` (Argon2id), `x/crypto/bcrypt` read-only | memory-hard verifier; legacy bcrypt rows upgrade on next login |
| Unicode matching | `x/text` (`cases.Fold`, `unicode/norm`) | canonical non-ASCII phrase matching without locale guesses |
| Location data | GeoNames and DB-IP Country Lite, generated into embedded indexes | offline place search and IP fallback without sending a location to a geocoder; digests verified by the generator |
| Rate limiting | `x/time/rate` | login throttling, per instance |
| Metrics and logs | `prometheus/client_golang`, `slog` | standard |
| Secret encryption | stdlib `crypto/*` | envelope encryption needs nothing outside the standard library |
| Startup report | `jedib0t/go-pretty/v6/table` | terminal formatting of the startup report only |
| Windows job objects | `x/sys/windows` | compile-isolated legacy adapter; not a support claim |
| SSO | `coreos/go-oidc/v3`, `x/oauth2`, `go-jose/v4` | JWKS verification, rotation and claim checks are not code to hand-roll |
| Email | `wneessen/go-mail` | context-aware SMTP with explicit TLS policy and MIME composition, behind the delivery port |
| Web Push | `SherClockHolmes/webpush-go` | RFC 8291 encryption and VAPID without local crypto code, behind the push adapter |
| Leak detection (tests) | `go.uber.org/goleak` | proves the in-process restart loop leaks no goroutines |
| Model providers | hand-written clients over plain `net/http` for the Ollama API and OpenAI-compatible endpoints | one compatible client covers hosted providers and local servers; no vendor SDK |
| Web references, release notes, model discovery | plain `net/http` | public page evidence for Intents; release-note classification; Hugging Face model listing; no SDKs |
| TMDB, Seerr, media server, Tunarr | hand-written thin clients | each uses a handful of endpoints; generating from full upstream specs couples us to their churn |
| Image rendering | required Rust `loomarr-image` worker over a versioned manifest protocol | contains decoder crashes and memory outside the Go process without cgo ([images](images.md)) |
| Rust image crates | `serde`, `sha2`, `base64`, `thumbhash`, `image`, `fast_image_resize`, `webp`, `webp-animation`, `ravif` (via `image`) | bounded protocol, content identity, placeholders, static and animated decode/encode; locked by `Cargo.lock`, fuzz graph separate |
| Backend tests | stdlib `testing`, `testcontainers-go` | Postgres conformance |

## Bundled executables

Exec'd, never linked, and shipped in the one image, so every feature is always available.

| Tool | Why |
| --- | --- |
| `ffmpeg`, `ffprobe` | playout encoding and packaging, probing, yt-dlp stream merging |
| `yt-dlp`, `deno` | YouTube and playlist filler sources (`internal/clipfetch`); deno is required by yt-dlp |
| `whisper-cli` (whisper.cpp) with `small.en` | transcripts for compilation splitting; smaller models measurably dropped whole adverts. Ships its `libggml` backends beside it, and the image proves it by transcribing at build time |
| Traefik (Compose edge) | one health-aware HTTP edge, digest-pinned; one Loomarr replica remains the supported boundary |

`sherpa-onnx` is approved as a development-only exec'd runtime. Its keyword-spotting weights have no
confirmed redistribution licence, so Loomarr neither bundles nor fetches them. Hosted providers can
replace local transcription and vision; those modalities stay operator choices.

## Clients (Node 22.5+, pnpm)

| Concern | Choice | Why |
| --- | --- | --- |
| Workspace | pnpm workspaces + Turborepo, under the Make targets | content-aware task graph inside `web/` |
| Shared styling | `@tamagui/core` behind Loomarr's `design-system` and `ui` | one token and variant implementation across web and native; direct imports fail the import-graph gate |
| Vectors and icons | `react-native-svg`, `lucide-react-native`, `react-native-web` | shared brand and glyph geometry on every platform |
| Pairing QR | `qrcode` behind a `QrCode` interface | standards-correct matrix rendered through the SVG layer |
| Native runtime | Expo + React Native, `react-native-tvos`, Expo Router, `expo-video` | one toolchain for phones and TVs ([0024](decisions/0024-react-native-clients.md)) |
| Native credentials | `expo-secure-store` | the device token stays in Keystore/Keychain |
| Native session ids | `expo-crypto` | Hermes has no `crypto.randomUUID`; the playback session id needs a cryptographically secure source, never `Math.random()` |
| TV server discovery | `grandcat/zeroconf` (server) and Android `NsdManager` | find a local server without typing a URL; manual entry stays |
| Android tooling | ccache (pinned), `bundletool` (pinned) | compiler cache; installing split APKs from the exact CI bundle |
| API client | TanStack Query with hooks, zod schemas and MSW handlers generated by `orval` | one generator for types, hooks, wire schemas and test wiring; generated mock data is never trusted |
| Import boundaries | `dependency-cruiser` (with TypeScript 6 for its parser) | packages are deep modules; the gate blocks bypassing their entry points |
| Unused code | `knip` 6 (root devDependency, `web/knip.ts`) and Go `deadcode` (`go run`, never a `go.mod` dependency) | knip fails `make fe` on a new unused file, export or dependency; `deadcode` cannot see code reached only from build-tagged tests, so it reports monthly against a committed baseline (`deadcode.yml`) instead of gating. Neither ships in the product |
| Web routing | TanStack Router | typed routes sharing the Query client |
| Legacy web styling | Tailwind + shadcn/ui on Base UI + CVA | only on surfaces not yet migrated (#970); removed when migration completes |
| Legacy usage ledger | `@babel/parser` (root devDependency; TypeScript 7 has no JS parser API) | `web/scripts/check-legacy-usage.mjs` counts the legacy stack per production file against `web/apps/web/legacy-usage.json`; `make fe` fails when a count rises or a new file appears, and `pnpm legacy:update` only shrinks it (#970). Never ships in the product |
| Lineup reordering | `@dnd-kit` | accessible, keyboard-capable sortable list |
| Web previews | `react-dom` portal behind the shared design system | keep contextual detail outside scrolling and clipping ancestors; native adapters do not load the web renderer |
| DOM test environment | jsdom 30.0.1 | exercise preview focus, pointer and viewport lifecycles without a browser; the shared UI package uses the same exact pin |
| Guide rows | `@tanstack/react-virtual` | windows the guide's rows |
| Image placeholders | `thumbhash` | decodes the placeholder the server stores, without a canvas |
| Forms | `@tanstack/react-form` | uses the zod schemas directly |
| Help | `react-markdown` + `remark-gfm` | renders the embedded help offline |
| Component workshop | Storybook 10 with a11y and themes addons | the component contract; the visual gate runs Playwright over its static build |
| Tests | Vitest, Testing Library, Playwright, `@axe-core/playwright` | units, visual, accessibility and e2e |
| Browser HLS | `hls.js` | MSE transport for the Watch player; Safari plays HLS natively |

The private visual-corpus review board is a self-contained page rendered by Go's `html/template`;
it adds no service or dependency.

## Documentation tooling

Build-time only; nothing ships. Astro + Starlight renders `docs/` in place; D2 (pinned, containerized)
renders committed SVG diagrams; lychee checks links and anchors offline; markdownlint-cli2 and Vale
(this repository's own style only) lint the reader-facing set; `cmd/dev-docs` generates the command
reference.

Model training lives in the separate `loomarr-models` repository. No trainer, converter or weight
enters this repository or its image; Loomarr consumes a certified model only through its HTTP
provider boundary.
