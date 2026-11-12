import { guideDayOffset } from "@loomarr/core/guide";
import { useCallback, useState } from "react";

interface GuideDayArgs {
  /** The pinned guide clock; it slides forward as the Guide's window does. */
  anchorMs: number;
  /** The day offset the Guide opens on, from the anchor's day. */
  initialOffset: number;
  timeZone?: string;
}

/**
 * The Guide's day picker as [offset from the anchor's day, setter].
 *
 * A pick is stored with the anchor it was made against, so the offset is re-derived when the anchor
 * slides to a later day: a tab left on "Tomorrow" across midnight keeps showing that same date,
 * which reads "Today" once the household's clock reaches it.
 */
const useGuideDay = ({ anchorMs, initialOffset, timeZone }: GuideDayArgs) => {
  const [picked, setPicked] = useState({ anchorMs, offset: initialOffset });
  const dayOffset = picked.offset - guideDayOffset(anchorMs, picked.anchorMs, timeZone);
  const setDayOffset = useCallback((offset: number) => setPicked({ anchorMs, offset }), [anchorMs]);
  return [dayOffset, setDayOffset] as const;
};

export { useGuideDay };
