import { describe, expect, it } from "vitest";
import { addedLine, newSince } from "./new-this-week";

describe("New this week — who made the channel (#1663)", () => {
  const createdAtMs = Date.UTC(2026, 8, 29, 12); // a Tuesday
  const channel = { id: "c1", name: "Test", number: 22, createdAtMs } as const;

  it("names the requester, or says it was yours", () => {
    expect(addedLine({ ...channel, requestedBy: "Ada" } as never, "Bo", "UTC")).toBe(
      "Requested by Ada · added Tuesday",
    );
    expect(addedLine({ ...channel, requestedBy: "Bo" } as never, "Bo", "UTC")).toBe(
      "Your request · added Tuesday",
    );
  });

  it("says only when a hand-made channel arrived", () => {
    expect(addedLine(channel as never, "Bo", "UTC")).toBe("Added Tuesday");
  });

  // The titles query key must hold still between renders within the hour.
  it("quantises the week's start to the hour", () => {
    const now = Date.UTC(2026, 8, 28, 12, 34, 56);
    expect(newSince(now)).toBe(newSince(now + 60_000));
    expect(newSince(now)).toBe(Date.UTC(2026, 8, 21, 12));
  });
});
