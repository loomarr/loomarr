# Loomarr documentation

The docs are organized by reader and authority. Start with the page for the job you are doing;
do not read the 7,000-line design document as onboarding.

## Find the right document

| Reader or task | Start here | Authority |
| --- | --- | --- |
| Install or operate Loomarr | [`install/`](install/index.md) | Supported deployment and operations |
| Use Loomarr | [`help/`](help/quickstart.md) | Viewer- and admin-facing product behavior |
| Build and test Loomarr | [`dev/`](dev/index.md) | Contributor workflow and gates |
| Understand current behavior | [`design.md`](design.md) | Canonical system behavior |
| Use domain terminology | [`../CONTEXT.md`](../CONTEXT.md) | Canonical vocabulary |
| Check active or shipped work | [`../PROGRESS.md`](../PROGRESS.md) | Work status and gate evidence |
| Read current plans and evidence | [`engineering/`](engineering/README.md) | Dated planning and supporting evidence |
| Read release notes | [`release/`](release/README.md) | Published release contracts |

## User and operator docs

Only `help/` is embedded in the binary and served at `/v1/docs`. It is written for household
admins and members, works offline, and has two extra constraints:

- no frontmatter, because the first H1 supplies the in-app title;
- no generated diagrams, because only the help Markdown is embedded in the binary.

The help pages' anchors and claims are tested. `embed_test.go` verifies every API-emitted doc link,
and `claims_test.go` verifies high-risk behavior statements against the code.

`configuration.md` is generated from the typed settings registry. Cite it instead of restating
setting defaults in hand-written pages.

## Design authority

`design.md` owns product behavior. Companion documents own detail within their named domains:

- [`programming-design.md`](programming-design.md) — Channel Policy and scheduling heuristics;
- [`config-design.md`](config-design.md) — settings information architecture and persistence;
- [`frontend-design.md`](frontend-design.md) — shared client architecture and visual system.

If a companion contradicts `design.md`, fix the contradiction in the same PR. Engineering plans and
research explain a decision or sequence work; they are not competing behavior specifications.
Superseded plans move to [`engineering/archive/`](engineering/archive/README.md).

## Diagram standard

Use a diagram only when relationships or sequence are materially clearer than prose or a table.
Every diagram answers one question stated by its surrounding section.

- Prefer a flow for flow, a state graph for lifecycle, and a table for comparisons.
- Keep labels short and put detail in the prose below.
- Split a diagram before it becomes a wall of crossed edges.
- Keep editable D2 sources in `diagrams/` and generated SVGs in `diagrams/generated/`.
- Use the shared D2 light/dark configuration; do not hard-code colors or typography.
- Do not duplicate a generated inventory in a hand-maintained diagram.
- Do not put generated diagram references in `help/`.

D2 source and generated SVGs are committed together. GitHub and the docs site display the SVG;
reviewers read the `.d2` diff and can use GitHub's image diff for the result. If the source is not
understandable in a pull-request diff, the diagram is too complicated.

## Generated documents

Never edit these by hand:

| File | Source | Regenerate |
| --- | --- | --- |
| `configuration.md` | Settings registry | `make config-docs` |
| `dev/commands.md` | Makefile and CI workflows | `make dev-docs` |
| `design.md` §2 package map | Go package docs and imports | `make arch-docs` |
| `diagrams/generated/*.svg` | `diagrams/*.d2` | `make diagrams` |
| `../api/openapi.yaml` | Huma route definitions | `make openapi` |

Run `make docs-lint` for diagrams, structure, relative links, and Loomarr vocabulary. Generated-file
verify targets guard drift separately.
