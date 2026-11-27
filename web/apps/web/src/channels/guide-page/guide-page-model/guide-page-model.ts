import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import {
  formatGuideTimeRange,
  type GuideLayout,
  type GuideSelection,
  guideSelectionForChannel,
} from "@loomarr/core/guide";
import type { QueryClient } from "@tanstack/react-query";
import { guideDayOffset, guideDayStart } from "../../guide-window";

// How far a window may span. Kept as compact chips because this is the everyday view choice;
// the longer phrase remains in each chip's accessible name.
const WINDOW_CHOICES = [
  { value: "120", label: "2h", description: "2 hours" },
  { value: "240", label: "4h", description: "4 hours" },
  { value: "360", label: "6h", description: "6 hours" },
  { value: "720", label: "12h", description: "12 hours" },
] as const;

// How far back the day picker offers. Matches the server's `guide.retention_hours` default —
// past listings are RECOMPUTED from each channel's current lineup, so a distant past would be
// fiction rather than history. The server clamps regardless; this keeps the UI from offering
// a day it will not serve.
const RETENTION_DAYS = 1;
const FORWARD_DAYS = 6;

// The hours the START picker offers. Every third hour is enough to land anywhere in the day
// in one click; a 24-entry list is a scroll, not a choice.
const START_HOURS = [0, 3, 6, 7, 8, 9, 12, 15, 18, 20, 21, 22] as const;

const hourLabel = (h: number) => {
  const suffix = h < 12 ? "AM" : "PM";
  const twelve = h % 12 === 0 ? 12 : h % 12;
  return `${twelve} ${suffix}`;
};

// "auto" follows now; the rest pin the window's start to that hour of the chosen day.
const START_HOUR_OPTIONS = [
  { label: "Now", value: "auto" },
  ...START_HOURS.map((h) => ({ label: hourLabel(h), value: String(h) })),
];

// The guide's two views (#2766 on phones, #2787 on desktops): a Now/Next list, or the full grid.
const GUIDE_VIEWS = [
  { label: "Now & next", value: "list" },
  { label: "Full grid", value: "grid" },
] as const;
type GuideView = (typeof GUIDE_VIEWS)[number]["value"];

// The selection survives a new window or a refetch: the same channel, at the block nearest the
// same time, as the shared guide controller does. Otherwise the block airing now on the first
// channel.
const reconcileSelection = (
  layout: GuideLayout | undefined,
  selection: GuideSelection | undefined,
  atMs: number,
): GuideSelection | undefined => {
  if (!layout) return undefined;
  const kept = selection && guideSelectionForChannel(layout, selection.channelId, selection.anchorMs);
  const first = layout.channels[0]?.source.channelId;
  return kept ?? (first ? guideSelectionForChannel(layout, first, atMs) : undefined);
};

// The day picker's offset of a moment from today on the household's calendar, within the days the
// picker offers.
const dayOffsetOf = (atMs: number, timeZone?: string): number =>
  Math.min(FORWARD_DAYS, Math.max(-RETENTION_DAYS, guideDayOffset(atMs, Date.now(), timeZone)));

// The day picker's label: relative for the days people actually mean, absolute beyond that, and
// the household's date, as the axis is.
const dayLabel = (offset: number, now: number, timeZone?: string): string => {
  if (offset === 0) return "Today";
  if (offset === 1) return "Tomorrow";
  if (offset === -1) return "Yesterday";
  return new Date(guideDayStart(now, offset, timeZone)).toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    timeZone,
  });
};

// The household's zone, which the axis is labelled in and its days and hours are read on. Every
// guide answer carries it, and the route's loader has fetched one before the page renders.
const cachedGuideZone = (queryClient: QueryClient): string | undefined =>
  queryClient
    .getQueriesData<channelsApi.channelGuideResponse>({ queryKey: channelsApi.getChannelGuideQueryKey() })
    .map(([, data]) => unwrap(data)?.timezone)
    .find((zone) => zone !== undefined && zone !== "");

// A window by its day and its hours, as the controls and the axis read them: "Tomorrow 8 PM–8 AM".
const windowName = (offset: number, fromMs: number, toMs: number, now: number, timeZone?: string): string =>
  `${dayLabel(offset, now, timeZone)} ${formatGuideTimeRange(fromMs, toMs, timeZone)}`;

export type { GuideView };
export {
  cachedGuideZone,
  dayLabel,
  dayOffsetOf,
  FORWARD_DAYS,
  GUIDE_VIEWS,
  RETENTION_DAYS,
  reconcileSelection,
  START_HOUR_OPTIONS,
  WINDOW_CHOICES,
  windowName,
};
