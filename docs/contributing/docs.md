# Writing docs

**For:** anyone adding or changing a documentation page.
**You'll get:** where a page goes, how it opens, the house style, and the rules for diagrams and
screenshots.

## One file, three readers

Every page is one Markdown file in `docs/`. GitHub renders it, the
[docs site](https://mantonx.github.io/loomarr/) renders it in place (`docs-site/` is a renderer, not
a copy), and the household pages also ship inside the binary for the in-app **Help**:
`get-started.md`, `guides/` and `explanation/` (`docs/embed.go`).

Pages in the app have three extra rules, all tested in `docs/embed_test.go`:

- **No frontmatter.** The first H1 is the title, and the app shows raw Markdown.
- **Unique file names.** A page's app slug is its file name, whichever folder it's in.
- **Relative links only to other app pages.** Link an app page by its relative `.md` path
  (`../guides/filler.md#pipeline`). Link anything else, like the settings reference, by its
  absolute site URL, or the app opens "Help page not found".

Headings in `guides/troubleshooting.md` are an API contract: setup checks deep-link to them. Don't
rename one without updating `internal/api/setup.go`. When you rename or move any app page or
heading, add the old link to `movedPages` or `movedSections` in `docs/embed.go`; the frozen list in
`docs/testdata/` fails until you do.

## Where a page goes

The site's navigation follows [Diátaxis](https://diataxis.fr/), and each section is one folder:

| Folder | Kind of page | Shape |
| --- | --- | --- |
| `get-started.md` | Tutorial | Needs, numbered `##` steps, "What's next" |
| `guides/` | How-to | Goal, numbered steps, "If it doesn't work" |
| `reference/` | Reference | Generated where possible; tables, no story |
| `explanation/` | Explanation | At most one diagram, then prose; no procedures |
| `contributing/` | Contributor how-to and explanation | As above |

Moving a published page? Add its old route to `docs-site/src/redirects.json`;
`docs-site/src/redirects.test.mjs` fails until you do. Plans, evidence and runbooks go in
[`project/`](../../project/README.md), not here.

## Every page opens the same way

```markdown
# Back up and restore

**For:** household admins.
**You'll get:** a backup you can restore on a new machine.
```

Titles are tasks in guides ("Back up and restore", not "Backups").

## Style

- **Voice.** Plain, friendly, second person. Short sentences, one idea each. Present tense,
  active voice. No "simply", "just" or "easily".
- **Spelling.** American English.
- **Headings.** Sentence case. Never skip a level.
- **Code.** Always fence with a language. One command per block when the reader might run them
  separately. Placeholders in `<angle-brackets>`. LAN addresses use `192.168.1.10`; never a real
  host, IP or domain.
- **Callouts.** GitHub alert syntax only (`> [!NOTE]`, `> [!TIP]`, `> [!WARNING]`): they render
  on GitHub, as asides on the site, and as blockquotes in the app. At most two per page. A warning
  is for data loss or security, nothing else.
- **Links.** Link text says where it goes ("the upgrade guide"), never "here".
- **Examples.** Genre words only ("90s Saturday morning cartoons"). Never a real film, show,
  person or brand, in a page, a screenshot or a release note: the repository is public.
- **Versions.** Don't hand-type the current release in more than one place.

## Diagrams

A diagram earns its place only when relationships or sequence are clearer than prose or a table.
D2 sources live in `docs/diagrams/`, generated SVGs in `docs/diagrams/generated/`, and
`make diagrams` renders both. `make diagrams-verify` fails on drift. Every diagram follows the
approved system (#1572):

- **One question per diagram,** written as the first comment in the source. About 10 nodes;
  the architecture overview, at 12, is the ceiling.
- **Roles, not colors.** Amber for Loomarr's own parts, gray for the services you run beside it,
  cyan for people and screens. Colors come from the design tokens and follow light and dark.
- **One flow direction,** top to bottom, and **zero line or label crossings**.
- **Short labels** (nouns of at most three words); the detail goes in the prose below.
- **Alt text** states the flow in a full sentence.

To draw one, start the source with `...@system/theme` and give every node and edge a class from
[`system/theme.d2`](../diagrams/system/theme.d2): `loomarr`, `service` or `audience` for nodes,
`boundary` for one running thing and `zone` for a plain group, and `hot` for the one path the
diagram is about, `flow` for the rest and `state` for reads and writes. Icons are Lucide, the
app's icon set, pre-colored per role in `docs/diagrams/icons/`; to add one, list it in
`system/icons.mjs` and run it as its header says. The render uses Geist
(`system/fonts/`, SIL Open Font License) and adds each color's dark token afterwards with
`system/darken.mjs`, so one SVG follows the reader's color scheme. Before committing, look at the
render in both schemes: an icon overlapping a label means the label is too long.

App pages don't use diagrams: only the Markdown is embedded.

## Screenshots and art

Screenshots come from the demo library, never a real media library, and are regenerated by
script rather than retaken by hand. Alt text says what to notice, not "screenshot of…".

They live in `docs/images/screenshots/` as `<name>-dark.webp`: a 1280 × 800 capture at 2×, in a
window frame drawn from the tokens, 1600 px wide, at most 150 KB. A page links one by its
relative path, and Help shows the same file from the binary, so every screenshot a page uses is
embedded (`docs/embed_test.go` checks it). To regenerate them after a UI change, on a fresh lane
store:

1. `make demo-library`, then `make dev-be`, then `make demo-seed` before anyone signs in, so the
   demo admin is the first user.
2. `make dev-fe`.
3. Wait for `watermark: GPU overlay verified` in the backend log; a stream started before it
   plays without the channel logo.
4. `make docs-capture`. The shot list is `SHOTS` in `scripts/docs-capture/capture.mjs`.

The landing page's hero and section spot art are inline SVG in `docs-site/src/components/`
(`Hero.astro`, `SpotArt.astro`), drawn only from the design tokens and the brand's parts: the set,
the screen, static and the seven bars in their fixed order. Their colors are the `--art-*`
variables in `docs-site/src/styles/custom.css`, so they follow the site's theme. No
AI-generated images. New art is shown to the maintainer before it merges.

## Generated pages

Never edit these by hand:

| File | Source | Regenerate |
| --- | --- | --- |
| `docs/reference/settings.md` | Settings registry | `make config-docs` |
| `docs/reference/make.md` | Makefile and CI workflows | `make dev-docs` |
| `docs/design.md` package map | Go package docs and imports | `make arch-docs` |
| `docs/diagrams/generated/*.svg` | `docs/diagrams/*.d2` | `make diagrams` |
| `api/openapi.yaml` | Huma route definitions | `make openapi` |

`make docs-lint` runs the diagrams check, markdownlint, lychee (offline) and Vale.
