import { guideWindow } from "@loomarr/core/guide";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useGuideDay } from "./use-guide-day";

const timeZone = "America/New_York";
// The furthest back the picker offers (the server keeps one past day).
const minOffset = -1;
const DAY_MS = 24 * 60 * 60 * 1000;
// 23:40 on 10 June 2027 in New York, and 03:10 the next day, after the Guide's window has slid.
const beforeMidnight = Date.UTC(2027, 5, 11, 3, 40);
const afterMidnight = Date.UTC(2027, 5, 11, 7, 10);
// The anchor once the household's clock has crossed `midnights` midnights since beforeMidnight.
const afterMidnights = (midnights: number) => afterMidnight + (midnights - 1) * DAY_MS;

const nineAm = (anchorMs: number, dayOffset: number) =>
  guideWindow({ at: anchorMs, dayOffset, windowMinutes: 240, hourShift: 0, startHour: 9, timeZone }).from;

const renderDay = (initialOffset: number) =>
  renderHook(({ anchorMs }) => useGuideDay({ anchorMs, initialOffset, minOffset, timeZone }), {
    initialProps: { anchorMs: beforeMidnight },
  });

describe("useGuideDay", () => {
  it("keeps the picked date when the anchor slides past midnight", () => {
    const { result, rerender } = renderDay(0);
    act(() => result.current[1](1));
    const tomorrowNine = nineAm(beforeMidnight, result.current[0]);
    expect(tomorrowNine).toBe(Date.UTC(2027, 5, 11, 13));

    rerender({ anchorMs: afterMidnight });

    // The day that was tomorrow is today now, and 9 AM is the same moment.
    expect(result.current[0]).toBe(0);
    expect(nineAm(afterMidnight, result.current[0])).toBe(tomorrowNine);
  });

  it.each([0, 1, 2, 5])("stays on today across %i midnights when today was picked", (midnights) => {
    const { result, rerender } = renderDay(0);

    rerender({ anchorMs: afterMidnights(midnights) });

    expect(result.current[0]).toBe(0);
  });

  it("stays on today after picking it back across midnight", () => {
    const { result, rerender } = renderDay(1);
    act(() => result.current[1](0));

    rerender({ anchorMs: afterMidnight });

    expect(result.current[0]).toBe(0);
  });

  it("follows the clock once a tomorrow pick reaches today, instead of turning into yesterday", () => {
    const { result, rerender } = renderDay(0);
    act(() => result.current[1](1));

    rerender({ anchorMs: afterMidnight });
    expect(result.current[0]).toBe(0);

    rerender({ anchorMs: afterMidnights(2) });
    expect(result.current[0]).toBe(0);

    rerender({ anchorMs: afterMidnights(5) });
    expect(result.current[0]).toBe(0);
  });

  it("keeps a past day inside the picker's range across midnight", () => {
    const { result, rerender } = renderDay(-1);
    expect(result.current[0]).toBe(-1);

    // Yesterday's date is two days back now, which the picker no longer offers: the oldest day it
    // does offer is the new yesterday.
    rerender({ anchorMs: afterMidnight });
    expect(result.current[0]).toBe(-1);

    rerender({ anchorMs: afterMidnights(3) });
    expect(result.current[0]).toBe(-1);
  });

  it("never offers an opening day the picker does not have", () => {
    const { result } = renderDay(-4);
    expect(result.current[0]).toBe(-1);
  });

  it.each([
    // [picked offset, midnights crossed, offset shown]
    [-1, 0, -1],
    [0, 1, 0],
    [1, 0, 1],
    [1, 1, 0],
    [2, 1, 1],
    [6, 3, 3],
    // A forward pick whose date has passed is a past day, held to the picker's range.
    [1, 2, -1],
    [1, 4, -1],
  ])("shows a pick of %i after %i midnights as %i", (picked, midnights, shown) => {
    const { result, rerender } = renderDay(0);
    act(() => result.current[1](picked));

    rerender({ anchorMs: afterMidnights(midnights) });

    expect(result.current[0]).toBe(shown);
  });

  it("picks days relative to the anchor it has now", () => {
    const { result, rerender } = renderDay(0);
    rerender({ anchorMs: afterMidnight });
    act(() => result.current[1](1));
    expect(result.current[0]).toBe(1);
  });
});
