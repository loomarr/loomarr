import { describe, expect, it } from "vitest";
import { isOk, unwrap } from "./unwrap";

// Stands in for an orval response union: a literal-200 success arm and an error arm
// carrying a different body. The point of the helper is that narrowing one gives you
// the other's absence, so the fixtures must actually differ in shape.
type Body = { jobs: string[]; total: number };
type Res = { data: Body; status: 200 } | { data: { detail: string }; status: 404 | 500 };

const ok: Res = { data: { jobs: ["backup", "reconcile"], total: 2 }, status: 200 };
const err: Res = { data: { detail: "nope" }, status: 500 };

// Returned through a function so TypeScript cannot narrow by initializer. A `const`
// assigned an object literal is narrowed to the arm that literal matches, which would
// make the selector's parameter `never` and prove the error-path calls below impossible
// at compile time — testing the type system rather than the runtime guard the helper is.
const settled = (res: Res): Res => res;

describe("isOk", () => {
  it("accepts the 200 arm", () => {
    expect(isOk(ok)).toBe(true);
  });

  it("rejects a non-200 arm", () => {
    expect(isOk(err)).toBe(false);
  });

  // The queries this guards are `undefined` until they settle, so the loading state has
  // to be a plain false rather than a crash on `.status`.
  it("rejects undefined", () => {
    expect(isOk(undefined)).toBe(false);
  });
});

describe("unwrap", () => {
  it("returns the body of a 200", () => {
    expect(unwrap(ok)).toEqual({ jobs: ["backup", "reconcile"], total: 2 });
  });

  it("projects the body when given a selector", () => {
    expect(unwrap(ok, (b) => b.jobs)).toEqual(["backup", "reconcile"]);
  });

  it("returns undefined for a non-200, without running the selector", () => {
    let ran = false;
    const got = unwrap(settled(err), (b) => {
      ran = true;
      return b.jobs;
    });
    expect(got).toBeUndefined();
    expect(ran).toBe(false);
  });

  it("returns undefined while the query is unsettled", () => {
    expect(unwrap(undefined)).toBeUndefined();
    expect(unwrap(undefined, (b: Body) => b.jobs)).toBeUndefined();
  });

  // The fallback deliberately stays at the call site: a selector returning a real
  // `undefined` field must be distinguishable from "not loaded" only by the caller's
  // own `??`, not by the helper quietly substituting a default.
  it("passes a selector's undefined through rather than substituting", () => {
    const sparse: Res = { data: { jobs: [], total: 0 }, status: 200 };
    expect(unwrap(sparse, (b) => b.jobs)).toEqual([]);
    expect(unwrap(sparse, (b) => b.jobs) ?? ["fallback"]).toEqual([]);
  });
});
