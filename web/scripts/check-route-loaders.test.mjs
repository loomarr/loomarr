import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { compareToLedger, routeStatus } from "./check-route-loaders.mjs";

describe("routeStatus", () => {
  it("settles a route that declares a loader, as a property or a method", () => {
    const property = `export const Route = createFileRoute("/a")({ component: A, loader: ({ context }) => context.x });`;
    const method = `export const Route = createFileRoute("/a")({ component: A, async loader() {} });`;
    assert.equal(routeStatus(property), "settled");
    assert.equal(routeStatus(method), "settled");
  });

  it("leaves a route with no loader unsettled, even when its comments say loader", () => {
    const source = `
      // The loader: warms the guide.
      const Route = createFileRoute("/a")({ component: A, beforeLoad: () => {} });
    `;
    assert.equal(routeStatus(source), "unsettled");
  });

  it("settles a route that says it fetches nothing", () => {
    const source = `
      // no-loader: redirects before anything is read
      const Route = createFileRoute("/a")({ beforeLoad: () => { throw redirect({ to: "/b" }); } });
    `;
    assert.equal(routeStatus(source), "settled");
  });

  it("ignores a loader on something that is not the route's options", () => {
    const source = `
      const other = { loader: 1 };
      const Route = createFileRoute("/a")({ component: A });
    `;
    assert.equal(routeStatus(source), "unsettled");
  });

  it("reports a file with no route as none", () => {
    assert.equal(routeStatus(`export const x = { loader: 1 };`), "none");
  });
});

describe("compareToLedger", () => {
  it("accepts exactly the listed routes", () => {
    assert.deepEqual(compareToLedger(["a", "b"], ["a", "b"]), []);
  });

  it("fails an unsettled route the ledger does not list", () => {
    assert.equal(compareToLedger(["a", "c"], ["a"]).length, 1);
  });

  it("fails a ledger entry that now has a loader, so the list only shrinks", () => {
    assert.equal(compareToLedger(["a"], ["a", "b"]).length, 1);
  });
});
