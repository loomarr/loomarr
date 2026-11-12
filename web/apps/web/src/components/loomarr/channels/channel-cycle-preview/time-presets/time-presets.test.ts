import { describe, expect, it } from "vitest";
import { nextMonthDayAt, nextWeekdayAt } from "./time-presets";

// Local wall-clock dates, as the picker shows them. 14 Nov 2026 is a Saturday.
const at = (month: number, day: number, hour: number, minute = 0) => new Date(2026, month, day, hour, minute);

describe("preview time presets", () => {
  it("picks today when the weekday is today and the hour is still ahead", () => {
    expect(nextWeekdayAt(6, 9, at(10, 14, 8))).toEqual(at(10, 14, 9));
  });

  it("picks next week once today's hour has come", () => {
    expect(nextWeekdayAt(6, 9, at(10, 14, 9))).toEqual(at(10, 21, 9));
    expect(nextWeekdayAt(6, 9, at(10, 14, 21, 30))).toEqual(at(10, 21, 9));
  });

  it("picks the coming weekday on any other day", () => {
    // Thursday 12 Nov → Tuesday 17 Nov, 8pm.
    expect(nextWeekdayAt(2, 20, at(10, 12, 23))).toEqual(at(10, 17, 20));
  });

  it("counts days and hours on the household's clock, not the device's", () => {
    // 20:00 UTC on Friday 13 Nov 2026 is already Saturday 9 AM in Auckland (NZDT, UTC+13).
    const now = new Date(Date.UTC(2026, 10, 13, 20, 0));
    // Saturday 9 AM there is next week's, 9 AM on 21 Nov in Auckland = 20:00 UTC on 20 Nov.
    expect(nextWeekdayAt(6, 9, now, "Pacific/Auckland")).toEqual(new Date(Date.UTC(2026, 10, 20, 20, 0)));
    // Christmas 9 AM in Auckland is 20:00 UTC on Christmas Eve.
    expect(nextMonthDayAt(11, 25, 9, now, "Pacific/Auckland")).toEqual(
      new Date(Date.UTC(2026, 11, 24, 20, 0)),
    );
  });

  it("picks this year's date until it passes, then next year's", () => {
    expect(nextMonthDayAt(11, 25, 9, at(10, 14, 8))).toEqual(at(11, 25, 9));
    expect(nextMonthDayAt(9, 31, 20, at(10, 14, 8))).toEqual(new Date(2027, 9, 31, 20));
  });
});
