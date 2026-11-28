import assert from "node:assert/strict";
import { test } from "node:test";

import { compare, countRoots, shrink } from "./check-test-harness.mjs";

test("counts createRoot calls, not mentions", () => {
  const source = `
    import { createRoot } from "react-dom/client";
    // createRoot(commented)
    const a: unknown = createRoot(document.body);
    const b = () => createRoot(<div />);
    const c = "createRoot(";
  `;
  assert.equal(countRoots(source), 2);
});

test("fails on a rise or a new file, passes on a fall", () => {
  assert.equal(compare({ "a.test.tsx": 1 }, { "a.test.tsx": 2 }).length, 1);
  assert.equal(compare({}, { "b.test.tsx": 1 }).length, 1);
  assert.deepEqual(compare({ "a.test.tsx": 2 }, { "a.test.tsx": 1 }), []);
});

test("writing only lowers counts and drops files", () => {
  assert.deepEqual(shrink({ a: 3, b: 1 }, { a: 1, c: 4 }), { a: 1 });
});
