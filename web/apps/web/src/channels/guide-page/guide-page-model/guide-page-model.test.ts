import { describe, expect, it } from "vitest";
import {
  dayLabel,
  dayOffsetOf,
  FORWARD_DAYS,
  RETENTION_DAYS,
  reconcileSelection,
  START_HOUR_OPTIONS,
  windowName,
} from "./guide-page-model";

describe("guide page model", () => {
  it("labels the days people mean relatively and the rest by date", () => {
    const now = Date.UTC(2026, 6, 25, 12, 0, 0);
    expect(dayLabel(0, now, "UTC")).toBe("Today");
    expect(dayLabel(1, now, "UTC")).toBe("Tomorrow");
    expect(dayLabel(-1, now, "UTC")).toBe("Yesterday");
    expect(dayLabel(3, now, "UTC")).not.toMatch(/Today|Tomorrow|Yesterday/);
  });

  it("clamps the day offset to the days the picker offers", () => {
    const day = 86_400_000;
    expect(dayOffsetOf(Date.now() + 30 * day, "UTC")).toBe(FORWARD_DAYS);
    expect(dayOffsetOf(Date.now() - 30 * day, "UTC")).toBe(-RETENTION_DAYS);
  });

  it("names a window by its day and hours", () => {
    const from = Date.UTC(2026, 6, 25, 20, 0, 0);
    const to = Date.UTC(2026, 6, 26, 8, 0, 0);
    expect(windowName(1, from, to, from, "UTC")).toMatch(/^Tomorrow /);
  });

  it("offers Now first, then the start hours in 12-hour labels", () => {
    expect(START_HOUR_OPTIONS[0]).toEqual({ label: "Now", value: "auto" });
    expect(START_HOUR_OPTIONS.find((o) => o.value === "0")?.label).toBe("12 AM");
    expect(START_HOUR_OPTIONS.find((o) => o.value === "12")?.label).toBe("12 PM");
    expect(START_HOUR_OPTIONS.find((o) => o.value === "21")?.label).toBe("9 PM");
  });

  it("has no selection without a layout", () => {
    expect(reconcileSelection(undefined, { channelId: "ch", anchorMs: 1 }, 2)).toBeUndefined();
  });
});
