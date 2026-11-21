import { describe, expect, it } from "vitest";
import { soonestExpiry } from "./soonest-expiry";

describe("soonestExpiry", () => {
  it("is the first pending link to run out", () => {
    expect(
      soonestExpiry([
        { status: "pending", expiresAt: 9_000 },
        { status: "pending", expiresAt: 4_000 },
      ]),
    ).toBe(4_000);
  });

  it("ignores a link already redeemed, revoked or expired", () => {
    expect(
      soonestExpiry([
        { status: "redeemed", expiresAt: 1_000 },
        { status: "revoked", expiresAt: 2_000 },
        { status: "expired", expiresAt: 3_000 },
        { status: "pending", expiresAt: 9_000 },
      ]),
    ).toBe(9_000);
    expect(soonestExpiry([{ status: "redeemed", expiresAt: 1_000 }])).toBeUndefined();
  });
});
