export * from "./clip-card";
export * from "./clip-preview";
export * from "./clip-row";
export * from "./coverage-meter";
// ⚠ There is no `discover-panel` (V35). Discover is not a tab — searching is something you do
// to a source — and the per-source search is its own shape (a result row carries date ·
// duration · quality and a "Queue download"): `source-search` below, the UI path for
// `GET /v1/filler/discover`. A component exported here but rendered by nothing is "built and
// imported by nothing", the exact state the reachability suite exists to catch.
export * from "./filler-sources";
export * from "./pod-timeline";
export * from "./pool-health";
export * from "./pull-card";
export * from "./segment-filmstrip";
export * from "./segment-preview";
export * from "./source-search";
export * from "./split-review-editor";
export * from "./watch-line";
