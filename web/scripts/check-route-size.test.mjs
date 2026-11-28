import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { LIMIT, lineCount, shrink, violations } from "./check-route-size.mjs";

describe("route-size ratchet", () => {
  it("counts lines without the trailing newline", () => {
    assert.equal(lineCount("a\nb\n"), 2);
  });

  it("fails an unlisted file over the limit and a listed file that grew", () => {
    assert.equal(violations({ a: LIMIT }, {}).length, 0);
    assert.equal(violations({ a: LIMIT + 1 }, {}).length, 1);
    assert.equal(violations({ a: 100 }, { a: 99 }).length, 1);
    assert.equal(violations({ a: 90 }, { a: 99 }).length, 0);
  });

  it("only lowers or drops ledger entries, never adds", () => {
    assert.deepEqual(shrink({ a: 80, b: 40, c: 200, d: 120 }, { a: 90, b: 90, d: 100 }), { a: 80, d: 100 });
  });
});
