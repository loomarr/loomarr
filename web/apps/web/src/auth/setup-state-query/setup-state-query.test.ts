import { describe, expect, it, vi } from "vitest";
import { needsBootstrap } from "./setup-state-query";

// The router guards branch on this one boolean, so its failure direction matters more
// than its happy path. FINDING 1 from the maintainer smoke: a fresh install sent the
// owner to /login, which no credential could pass.
describe("needsBootstrap", () => {
  const client = (result: unknown) => ({ ensureQueryData: vi.fn().mockResolvedValue(result) });

  it("reports true for an unclaimed install, so the guard can route to the wizard", async () => {
    expect(await needsBootstrap(client({ status: 200, data: { bootstrapped: false } }))).toBe(true);
  });

  it("reports false once an owner exists", async () => {
    expect(await needsBootstrap(client({ status: 200, data: { bootstrapped: true } }))).toBe(false);
  });

  // FAILS CLOSED. The inverse would drop every visitor of a healthy, claimed install
  // into the first-run wizard the moment /v1/setup/state hiccups — a transient blip
  // rendered as "your install is brand new" to people who already have accounts.
  it("reports false when the probe throws", async () => {
    const throwing = { ensureQueryData: vi.fn().mockRejectedValue(new Error("network")) };
    expect(await needsBootstrap(throwing)).toBe(false);
  });

  it("reports false on a non-200, rather than trusting an error body's shape", async () => {
    expect(await needsBootstrap(client({ status: 500, data: { bootstrapped: false } }))).toBe(false);
  });

  it("reports false when the field is missing entirely", async () => {
    expect(await needsBootstrap(client({ status: 200, data: {} }))).toBe(false);
  });
});
