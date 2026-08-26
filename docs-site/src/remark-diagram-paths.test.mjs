import assert from "node:assert/strict";
import test from "node:test";

import remarkDiagramPaths from "./remark-diagram-paths.mjs";

test("rewrites diagram assets without changing repository Markdown", () => {
  const tree = {
    type: "root",
    children: [
      { type: "image", url: "../diagrams/generated/ci.svg" },
      { type: "link", url: "../diagrams/ci.d2", children: [] },
      { type: "link", url: "../dev/testing.md", children: [] },
    ],
  };

  remarkDiagramPaths({ base: "/loomarr/" })(tree);

  assert.equal(tree.children[0].url, "/loomarr/generated/ci.svg");
  assert.equal(tree.children[1].url, "/loomarr/ci.d2");
  assert.equal(tree.children[2].url, "../dev/testing.md");
});
