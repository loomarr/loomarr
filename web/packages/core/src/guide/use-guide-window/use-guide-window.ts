import { useEffect, useMemo, useState } from "react";
import { DEFAULT_WINDOW_MINUTES, guideWindow } from "../guide";
import type { GuideWindow } from "../guide.type";
import type { GuideWindowClock } from "./use-guide-window.type";

// How often the live clock moves: the "on now" progress and minutes-left move on it.
const GUIDE_CLOCK_TICK_MS = 30_000;

// The window slides forward once now is this close to its end, so the viewer never reaches a
// guide that has run out of schedule.
const GUIDE_SLIDE_MARGIN_MS = 30 * 60_000;

const todayWindow = (anchorMs: number, windowMinutes: number): GuideWindow =>
  guideWindow({ at: anchorMs, dayOffset: 0, windowMinutes, hourShift: 0, startHour: null });

/**
 * The guide window a screen asks `GET /v1/guide` for, plus its live clock.
 *
 * The window is part of the react-query key, so its identity IS the request count. It is pinned at
 * mount and slides forward only when the clock gets within 30 minutes of the window end: a tab
 * left open on a four-hour window requests the guide about once every three and a half hours
 * instead of once a minute. Pair it with `placeholderData: keepPreviousData` so the slide
 * refetches behind the data already on screen instead of blanking it.
 *
 * `nowMs` ticks on its own timer outside the key, so the now-line and minutes-left stay live
 * without touching the request. `anchorMs` is the pinned clock a screen with its own day, hour or
 * span controls feeds to `guideWindow`.
 */
const useGuideWindow = (windowMinutes: number = DEFAULT_WINDOW_MINUTES): GuideWindowClock => {
  const [clock, setClock] = useState(() => {
    const now = Date.now();
    return { nowMs: now, anchorMs: now };
  });

  useEffect(() => {
    const id = setInterval(() => {
      const now = Date.now();
      setClock((previous) => ({
        nowMs: now,
        anchorMs:
          now >= todayWindow(previous.anchorMs, windowMinutes).to - GUIDE_SLIDE_MARGIN_MS
            ? now
            : previous.anchorMs,
      }));
    }, GUIDE_CLOCK_TICK_MS);
    return () => clearInterval(id);
  }, [windowMinutes]);

  const window = useMemo(() => todayWindow(clock.anchorMs, windowMinutes), [clock.anchorMs, windowMinutes]);
  return { nowMs: clock.nowMs, anchorMs: clock.anchorMs, window };
};

export { GUIDE_CLOCK_TICK_MS, GUIDE_SLIDE_MARGIN_MS, useGuideWindow };
