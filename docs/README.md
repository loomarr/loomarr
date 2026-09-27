# Loomarr documentation

**For:** anyone browsing the docs on GitHub.
**You'll get:** where each kind of page lives. The same pages are on the
[docs site](https://mantonx.github.io/loomarr/) and, for the household pages, in the app under **Help**.

The folders follow the site's navigation. Each holds one kind of page:

| Folder | Kind of page | Start with |
| --- | --- | --- |
| [`get-started.md`](get-started.md) | Tutorial: install to first channel playing | [Get started](get-started.md) |
| [`guides/`](guides/install-docker.md) | How-to guides, one task each | [Add a channel](guides/add-a-channel.md) |
| [`reference/`](reference/settings.md) | Generated reference: settings, `make` targets | [Settings](reference/settings.md) |
| [`explanation/`](explanation/how-loomarr-works.md) | How and why Loomarr works the way it does | [How Loomarr works](explanation/how-loomarr-works.md) |
| [`contributing/`](contributing/index.md) | Building, testing and releasing Loomarr | [Contributing](contributing/index.md) |
| [`design/`](design/README.md) and [`design.md`](design.md) | The system design, moving section by section into `design/` (#779) | [Design](design/README.md) |

Not on the site:

- [`agents/`](agents/harness.md): the contract coding agents work under. Vendored skills cite
  these paths, so the folder doesn't move.
- [`../project/`](../project/README.md): maintainer runbooks, live plans, evidence and release-note
  headers.
- [`diagrams/`](diagrams/architecture.d2): D2 sources and their generated SVGs.

How to write and place a page: [Writing docs](contributing/docs.md).
