import { guideDayStart, guideWallClock, guideWallTime } from "@loomarr/core/guide";

// The preview's quick-pick dates: pure date maths on the household's wall clock, the clock the
// server evaluates the preview on (no server round trip). An undefined zone is the device's own.
// The preview endpoint resolves holidays and dayparts; these are only starting points.

const nextWeekdayAt = (weekday: number, hour: number, now: Date = new Date(), timeZone?: string): Date => {
  const at = now.getTime();
  const today = guideWallClock(at, timeZone);
  const todayWeekday = new Date(Date.UTC(today.year, today.month - 1, today.day)).getUTCDay();
  const delta = (weekday - todayWeekday + 7) % 7;
  const candidate = guideDayStart(at, delta, timeZone, hour);
  return new Date(delta === 0 && candidate <= at ? guideDayStart(at, 7, timeZone, hour) : candidate);
};

const nextMonthDayAt = (
  month: number,
  day: number,
  hour: number,
  now: Date = new Date(),
  timeZone?: string,
): Date => {
  const at = now.getTime();
  const { year } = guideWallClock(at, timeZone);
  const thisYear = guideWallTime(year, month + 1, day, hour, 0, timeZone);
  return new Date(thisYear < at ? guideWallTime(year + 1, month + 1, day, hour, 0, timeZone) : thisYear);
};

export { nextMonthDayAt, nextWeekdayAt };
