import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";

type GuideWindowArgs = {
  /** Clock used to resolve the requested window. Injected so prefetch and render can agree. */
  at: number;
  dayOffset: number;
  windowMinutes: number;
  hourShift: number;
  startHour: number | null;
};

type GuideWindow = {
  from: number;
  to: number;
};

/** Geometry derived from one authoritative API airing, expressed independently of pixels. */
type GuideAiringLayout = {
  source: GuideAiring;
  channelId: string;
  scheduleBlockId: string;
  startRatio: number;
  widthRatio: number;
  clippedStartMs: number;
  clippedStopMs: number;
  startsBeforeWindow: boolean;
  endsAfterWindow: boolean;
  isOnNow: boolean;
  progressRatio?: number;
};

type GuideChannelLayout = {
  source: GuideChannelTimeline;
  airings: GuideAiringLayout[];
};

/**
 * Platform-neutral guide model. The generated response and airing objects remain the source of
 * truth; this layer adds only viewport geometry and live-progress facts.
 */
type GuideLayout = {
  source: GuideOutputBody;
  fromMs: number;
  toMs: number;
  timezone?: string;
  channels: GuideChannelLayout[];
};

type GuideSelection = {
  channelId: string;
  /** Absent when the channel has no block in the served window; the channel row still has focus. */
  scheduleBlockId?: string;
  /** Stable time column retained while moving vertically between differently sized blocks. */
  anchorMs: number;
};

type GuideNavigationDirection = "left" | "right" | "up" | "down";

type GuideNavigationResult = {
  selection: GuideSelection;
  /** Lets the platform adapter hand focus to filters or surrounding chrome without guessing. */
  boundary?: GuideNavigationDirection;
};

export type {
  GuideAiringLayout,
  GuideChannelLayout,
  GuideLayout,
  GuideNavigationDirection,
  GuideNavigationResult,
  GuideSelection,
  GuideWindow,
  GuideWindowArgs,
};
