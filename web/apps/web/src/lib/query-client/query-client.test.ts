import { describe, expect, it } from "vitest";
import { backoff, createQueryClient, MAX_RETRY_DELAY_MS, retryDelay } from "./query-client";

describe("retry backoff", () => {
  const lowest = backoff(() => 0);
  const highest = backoff(() => 1);

  it("doubles with each failure, jittered within its upper half", () => {
    expect(lowest(1)).toBe(500);
    expect(highest(1)).toBe(1_000);
    expect(highest(2)).toBe(2_000);
    expect(highest(3)).toBe(4_000);
  });

  it("never waits longer than 30 seconds", () => {
    for (const failures of [6, 10, 100]) {
      expect(highest(failures)).toBe(MAX_RETRY_DELAY_MS);
      expect(lowest(failures)).toBe(MAX_RETRY_DELAY_MS / 2);
    }
  });

  it("is the client's default for every read", () => {
    expect(createQueryClient().getDefaultOptions().queries?.retryDelay).toBe(retryDelay);
  });
});
