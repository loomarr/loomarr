# Repository knowledge graph

Loomarr commits a code-only Graphify index so maintainers and agents can query
architecture and symbol relationships without loading the entire repository into
context. The graph covers maintained Go, TypeScript, JavaScript, Rust, shell, SQL,
and configuration sources. It deliberately excludes dependencies, generated
clients, build output, browser baselines, documents, images, and runtime data.
GitHub Actions YAML is the deliberate exception: `cmd/graphify-sync` projects
workflows, jobs, dependencies, triggers, and action uses into deterministic
structural nodes after Graphify's code extraction.

The committed outputs are:

- `graphify-out/graph.json` — the queryable node and edge graph;
- `graphify-out/graph.html` — an aggregated browser view for the large graph;
- `graphify-out/GRAPH_REPORT.md` — hubs, communities, inferred bridges, and
  suggested questions;
- `graphify-out/manifest.json` — hashes used by incremental extraction; and
- `.graphify_analysis.json`, `.graphify_labels.json`, and the label signature —
  clustering state used to keep community names stable across updates.

Machine-local interpreter paths, scan roots, health scratch data, and token-cost
history are ignored. The portable artifacts must not contain absolute paths.

## Install

Graphify is developer tooling, not an application dependency. Install the exact
reviewed version with SQL support so migrations contribute schema relationships:

```sh
uv tool install 'graphifyy[sql]==0.9.64'
```

## Query

Run queries from the repository root:

```sh
graphify query "What connects the data layer to the API?"
graphify path "Scheduler" "Store"
graphify explain "TunarrHTTP"
```

The graph is larger than the full-node visualization limit. `graph.html` is
therefore a community-level view; command-line queries retain node-level detail.

## Rebuild

For a complete rebuild, start from a clean current-main worktree and run:

```sh
make graphify
```

The target runs the pinned code-only extraction, injects the deterministic
GitHub Actions projection, reclusters, exports HTML, and stamps the finished
graph with a digest of every maintained source and workflow input. Review every
skipped-sensitive and parser warning before committing. Keep the persisted
community labels when updating an existing graph; Graphify uses their membership
signatures to retain valid names and replaces stale communities with current hub
labels.

CI does not install Graphify or write commits. Its always-on policy job runs:

```sh
make graphify-verify
```

That Go-only check recomputes the input digest and workflow projection. Any
source addition, deletion, edit, `.graphifyignore` change, or GitHub Actions
change fails until `make graphify` is run and the refreshed artifacts are
committed. The report's `built_at_commit` is informational because a committed
artifact cannot contain its own final commit hash; the embedded input digest is
the freshness authority.

Optional read-only diagnostics remain available:

```sh
graphify diagnose multigraph --graph graphify-out/graph.json --undirected
graphify benchmark graphify-out/graph.json
```

Never commit `.graphify_python`, `.graphify_root`, `cost.json`, or
`.graphify_health.json`.
