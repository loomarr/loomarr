import { guideDayOffset } from "@loomarr/core/guide";
import { useCallback, useState } from "react";

interface GuideDayArgs {
  /** The pinned guide clock; it slides forward as the Guide's window does. */
  anchorMs: number;
  /** The day offset the Guide opens on, from the anchor's day. */
  initialOffset: number;
  /** The furthest back the day picker offers (a negative offset); a kept pick never goes past it. */
  minOffset: number;
  timeZone?: string;
}

/**
 * The Guide's day picker as [offset from the anchor's day, setter].
 *
 * "Today" follows the clock: left open across midnight it is the new today. Any other pick names a
 * date, so it is stored with the anchor it was made against and its offset re-derived when the anchor
 * slides to a later day: a tab left on "Tomorrow" across midnight keeps showing that same date. When
 * that date is today it becomes "Today" and follows the clock from then on, so it does not turn into
 * yesterday at the next midnight. A past pick is held to the picker's range (`minOffset`): its date
 * ages out of what the picker offers and the server keeps, so the oldest offered day stands in.
 */
const useGuideDay = ({ anchorMs, initialOffset, minOffset, timeZone }: GuideDayArgs) => {
  const [picked, setPicked] = useState({ anchorMs, offset: initialOffset });
  const drift = guideDayOffset(anchorMs, picked.anchorMs, timeZone);
  const derived = picked.offset === 0 ? 0 : picked.offset - drift;
  const reachedToday = picked.offset > 0 && derived === 0;
  if (reachedToday) setPicked({ anchorMs, offset: 0 });
  const dayOffset = Math.max(minOffset, derived);
  const setDayOffset = useCallback((offset: number) => setPicked({ anchorMs, offset }), [anchorMs]);
  return [dayOffset, setDayOffset] as const;
};

export { useGuideDay };
