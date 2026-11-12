// The preview's quick-pick dates: pure date maths in the browser's wall clock (no server round
// trip). The preview endpoint resolves holidays and dayparts; these are only starting points.

const nextWeekdayAt = (weekday: number, hour: number, now: Date = new Date()): Date => {
  const d = new Date(now);
  d.setHours(hour, 0, 0, 0);
  const delta = (weekday - d.getDay() + 7) % 7;
  d.setDate(d.getDate() + (delta === 0 && d.getTime() <= now.getTime() ? 7 : delta));
  return d;
};

const nextMonthDayAt = (month: number, day: number, hour: number, now: Date = new Date()): Date => {
  const thisYear = new Date(now.getFullYear(), month, day, hour, 0, 0, 0);
  return thisYear.getTime() < now.getTime()
    ? new Date(now.getFullYear() + 1, month, day, hour, 0, 0, 0)
    : thisYear;
};

export { nextMonthDayAt, nextWeekdayAt };
