# Repository knowledge graph

Maintainers and agents can build a code-only Graphify index to query architecture and
symbol relationships without loading the entire repository into context. The graph covers
maintained Go, TypeScript, JavaScript, Rust, shell, SQL, and configuration sources. It
deliberately excludes dependencies, generated clients, build output, browser baselines,
documents, images, and runtime data (`.graphifyignore`).

The index is **generated on demand and never committed** (#1565). A committed copy went
stale behind the code it described (it still showed the retired prepared-media
architecture weeks after its removal), and every refresh added about 66 MB to the history
of a public repository. `graphify-out/` is gitignored.

## Install

Graphify is developer tooling, not an application dependency. Install the exact
reviewed version with SQL support so migrations contribute schema relationships:

```sh
uv tool install 'graphifyy[sql]==0.9.64'
```

## Build

From a clean worktree at the commit you want to ask about:

```sh
make graph
```

It writes `graphify-out/graph.json` (the queryable node and edge graph),
`GRAPH_REPORT.md` (hubs, communities, inferred bridges, and suggested questions),
`graph.html` (a community-level browser view; the graph is larger than the
full-node visualization limit), and the manifest and clustering state that later
incremental runs reuse. Rebuild after pulling significant changes: an old index answers
for the code it was built from, not the code in front of you.

## Query

Run queries from the repository root:

```sh
graphify query "What connects the data layer to the API?"
graphify path "Scheduler" "Store"
graphify explain "TunarrHTTP"
```

To check the graph's shape, run
`graphify diagnose multigraph --graph graphify-out/graph.json --undirected` or
`graphify benchmark graphify-out/graph.json`.
