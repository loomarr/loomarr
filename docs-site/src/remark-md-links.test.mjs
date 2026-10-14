import assert from "node:assert/strict";
import test from "node:test";

import remarkMdLinks from "./remark-md-links.mjs";

const options = {
  base: "/loomarr/",
  docsRoot: "/repo/docs",
  include: ["get-started.md", "guides", "contributing"],
  repoUrl: "https://github.com/loomarr/loomarr",
};

const rewrite = (from, url) => {
  const tree = { type: "root", children: [{ type: "link", url, children: [] }] };
  remarkMdLinks(options)(tree, { path: `/repo/docs/${from}` });
  return tree.children[0].url;
};

test("a published page becomes its site route, fragment kept", () => {
  assert.equal(rewrite("guides/upgrade.md", "backups.md#restore"), "/loomarr/guides/backups/#restore");
  assert.equal(rewrite("guides/upgrade.md", "../get-started.md"), "/loomarr/get-started/");
});

test("an index page is its folder", () => {
  assert.equal(rewrite("get-started.md", "contributing/index.md"), "/loomarr/contributing/");
});

test("anything unpublished goes to GitHub", () => {
  assert.equal(
    rewrite("guides/upgrade.md", "../../docker/compose.yaml"),
    "https://github.com/loomarr/loomarr/blob/main/docker/compose.yaml",
  );
  assert.equal(
    rewrite("contributing/index.md", "../agents/harness.md"),
    "https://github.com/loomarr/loomarr/blob/main/docs/agents/harness.md",
  );
});

test("absolute, same-page and diagram links are left alone", () => {
  assert.equal(rewrite("guides/upgrade.md", "https://example.com/x"), "https://example.com/x");
  assert.equal(rewrite("guides/upgrade.md", "#restore"), "#restore");
  assert.equal(rewrite("guides/upgrade.md", "../diagrams/ci.d2"), "../diagrams/ci.d2");
});
