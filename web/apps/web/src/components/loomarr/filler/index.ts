export * from "./clip-card";
export * from "./coverage-meter";
// ⚠ `discover-panel` is DELETED (V35), not merely unrendered. Discover stopped being a tab —
// searching is something you do to a source — and the redesigned per-source search is a
// different shape (a result row carries date · duration · quality and a "Queue download"),
// so the old panel was not reusable.
//
// Left barrel-exported it would have been "built and imported by nothing", which is the exact
// state the reachability suite exists to catch — and a reviewer caught it here. Its replacement
// is `source-search` below, which is why `GET /v1/filler/discover` is no longer API-only.
export * from "./filler-sources";
export * from "./incoming-panel";
export * from "./pod-timeline";
export * from "./pool-health";
export * from "./pull-card";
export * from "./source-search";
export * from "./split-review-editor";
