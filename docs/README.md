# Loomarr docs

Four audiences, kept deliberately separate. All of it renders on GitHub and on the docs site;
only `help/` is embedded in the binary.

## For operators

- **[`install/`](install/index.md)** — what Loomarr needs, the Docker walkthrough, hardware
  acceleration, upgrading.
- **[`configuration.md`](configuration.md)** — every setting. **Generated** from the settings
  registry by `make config-docs`; CI fails on drift. Cite it, don't restate it.
- **[`integrations/`](integrations/media-server-livetv.md)** — Emby/Jellyfin Live TV wiring.

## For users — `help/`, embedded in the binary

Rendered offline in the app under **Help**, and served at `/v1/docs`. Written lean for a
household admin: [Quickstart](help/quickstart.md), [Concepts](help/concepts.md),
[Integrations](help/integrations.md), [Programming](help/programming.md),
[Filler](help/filler.md), [Member guide](help/member-guide.md),
[Troubleshooting](help/troubleshooting.md).

Two constraints on this directory specifically:

- **No frontmatter.** `embed.go` derives the title from the first H1, and YAML would render as
  literal text in the in-app viewer.
- **No mermaid.** The in-app renderer has no mermaid support, so a diagram would show an
  operator its own source. Use prose and tables here; diagrams belong in the site-rendered set.

Their **anchors and their claims are both contracts**, and both are tested: `embed_test.go`
proves every `docHref` the API emits resolves to a real heading, and `claims_test.go` proves the
pages don't contradict the code.

## For contributors

- **[`dev/`](dev/index.md)** — setup, the dev loop, testing, CI, codegen, and the generated
  command reference. The single home for these facts.

## For the build team

- **[`design.md`](design.md)** — **the single source of truth** (§1–§22). Doc-first: if code
  must deviate, update this in the same PR, first.
- Companion designs, authoritative for their own domains:
  [`programming-design.md`](programming-design.md) (ChannelPolicy heuristics),
  [`config-design.md`](config-design.md) (settings subsystem),
  [`frontend-design.md`](frontend-design.md) (the "Test Card" design system).
- **[`engineering/`](engineering/)** — dated findings and decision records that support the
  build. Superseded plans live in [`engineering/archive/`](engineering/archive/README.md) with a
  banner saying what replaced them.
- **[`agents/`](agents/domain.md)** — configuration the coding-agent skills read.

Precedence: `design.md` wins on behaviour; each companion wins on its own domain.

## What's generated

Never hand-edit these; each has a verify target that fails CI on drift.

| File | From | Regenerate |
| --- | --- | --- |
| `configuration.md` | the settings registry | `make config-docs` |
| `dev/commands.md` | the Makefile + CI workflows | `make dev-docs` |
| `../api/openapi.yaml` | the Huma definitions | `make openapi` |
