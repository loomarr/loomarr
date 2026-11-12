import { guideWindow } from "@loomarr/core/guide";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useGuideDay } from "./use-guide-day";

const timeZone = "America/New_York";
// 23:40 on 10 June 2027 in New York, and 03:10 the next day, after the Guide's window has slid.
const beforeMidnight = Date.UTC(2027, 5, 11, 3, 40);
const afterMidnight = Date.UTC(2027, 5, 11, 7, 10);

const nineAm = (anchorMs: number, dayOffset: number) =>
  guideWindow({ at: anchorMs, dayOffset, windowMinutes: 240, hourShift: 0, startHour: 9, timeZone }).from;

describe("useGuideDay", () => {
  it("keeps the picked date when the anchor slides past midnight", () => {
    const { result, rerender } = renderHook(
      ({ anchorMs }) => useGuideDay({ anchorMs, initialOffset: 0, timeZone }),
      {
        initialProps: { anchorMs: beforeMidnight },
      },
    );
    act(() => result.current[1](1));
    const tomorrowNine = nineAm(beforeMidnight, result.current[0]);
    expect(tomorrowNine).toBe(Date.UTC(2027, 5, 11, 13));

    rerender({ anchorMs: afterMidnight });

    // The day that was tomorrow is today now, and 9 AM is the same moment.
    expect(result.current[0]).toBe(0);
    expect(nineAm(afterMidnight, result.current[0])).toBe(tomorrowNine);
  });

  it("keeps a past day picked across midnight", () => {
    const { result, rerender } = renderHook(
      ({ anchorMs }) => useGuideDay({ anchorMs, initialOffset: -1, timeZone }),
      {
        initialProps: { anchorMs: beforeMidnight },
      },
    );
    const yesterdayNine = nineAm(beforeMidnight, result.current[0]);

    rerender({ anchorMs: afterMidnight });

    expect(result.current[0]).toBe(-2);
    expect(nineAm(afterMidnight, result.current[0])).toBe(yesterdayNine);
  });

  it("picks days relative to the anchor it has now", () => {
    const { result, rerender } = renderHook(
      ({ anchorMs }) => useGuideDay({ anchorMs, initialOffset: 0, timeZone }),
      {
        initialProps: { anchorMs: beforeMidnight },
      },
    );
    rerender({ anchorMs: afterMidnight });
    act(() => result.current[1](1));
    expect(result.current[0]).toBe(1);
  });
});
