import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import type {
  GuideAiringLayout,
  GuideChannelLayout,
  GuideChannelState,
  GuideLayout,
  GuideNavigationDirection,
  GuideNavigationResult,
  GuideSelection,
  GuideWindow,
  GuideWindowArgs,
} from "./guide.type";

// Kept equal to the server's arrangement bucket so a prefetched window and its schedule expire
// together. The client does not depend on that equality for correctness.
const GUIDE_BUCKET_MS = 60_000;
const LOOKBACK_MINUTES = 30;
const DEFAULT_WINDOW_MINUTES = 240;

const startOfDay = (date: Date): number =>
  new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();

/** Resolve the stable API window shared by route prefetch and every client presentation. */
const guideWindow = ({
  at,
  dayOffset,
  windowMinutes,
  hourShift,
  startHour,
}: GuideWindowArgs): GuideWindow => {
  const span = windowMinutes * 60_000;
  const shift = hourShift * 3_600_000;
  const midnight = startOfDay(new Date(at)) + dayOffset * 86_400_000;

  if (startHour !== null) {
    const start = midnight + startHour * 3_600_000 + shift;
    return { from: start, to: start + span };
  }
  if (dayOffset === 0) {
    const start = Math.floor((at - LOOKBACK_MINUTES * 60_000 + shift) / GUIDE_BUCKET_MS) * GUIDE_BUCKET_MS;
    return { from: start, to: start + span };
  }
  return { from: midnight + shift, to: midnight + shift + span };
};

const defaultGuideWindow = (at: number): GuideWindow =>
  guideWindow({
    at,
    dayOffset: 0,
    windowMinutes: DEFAULT_WINDOW_MINUTES,
    hourShift: 0,
    startHour: null,
  });

/** Household guide clocks are explicit and consistent across browser, mobile, and TV. */
const guideTimeFormatter = (timeZone?: string): Intl.DateTimeFormat =>
  new Intl.DateTimeFormat("en-US", {
    hour: "numeric",
    minute: "2-digit",
    hourCycle: "h12",
    timeZone,
  });

const formatGuideTime = (at: number, timeZone?: string): string => guideTimeFormatter(timeZone).format(at);

const formatGuideTimeRange = (startMs: number, stopMs: number, timeZone?: string): string => {
  const formatter = guideTimeFormatter(timeZone);
  return `${formatter.format(startMs)}–${formatter.format(stopMs)}`;
};

/** Compact episode identity, including season zero specials and partially known metadata. */
const formatGuideEpisode = (season?: number, episode?: number): string | undefined => {
  const seasonLabel = season === undefined ? "" : `S${String(season).padStart(2, "0")}`;
  const episodeLabel = episode === undefined ? "" : `E${String(episode).padStart(2, "0")}`;
  return seasonLabel || episodeLabel ? `${seasonLabel}${episodeLabel}` : undefined;
};

const GUIDE_KIND_LABEL = {
  program: "Programme",
  filler: "Commercials",
  pending: "Coming soon",
  flex: "Filler",
} as const;

/** The compact identity shared by Guide blocks and their accessible names. */
const guideAiringLabel = (airing: GuideAiring): string => {
  const title = airing.title.trim();
  if (!title) return GUIDE_KIND_LABEL[airing.kind];
  return airing.series ? `${airing.series} · ${title}` : title;
};

/** Broadcast and health answer different questions and must not collapse into one status. */
const guideChannelState = (channel: GuideChannelTimeline): GuideChannelState => {
  let broadcast: GuideChannelState["broadcast"] = "off";
  if (channel.status === "live" || channel.status === "drifted") broadcast = "live";
  if (channel.status === "building") broadcast = "reconciling";

  let health: GuideChannelState["health"] = null;
  if (channel.status === "building") health = "creating";
  else if (channel.status === "drifted") health = "drift";
  else if (channel.status === "detached") health = "error";
  else if (channel.status === "paused") health = "paused";
  else if (channel.pendingCount > 0) health = "pending-slots";

  return { broadcast, health };
};

const clamp = (value: number, min: number, max: number): number => Math.min(max, Math.max(min, value));

/**
 * Add platform-neutral geometry to the generated guide response.
 *
 * Ratios are measured against the window the server actually served, not the one the client
 * requested. Airing order is preserved exactly; a presentation must not silently rearrange the
 * authoritative schedule. Blocks wholly outside the served window cannot render and are omitted.
 */
const layoutGuide = (source: GuideOutputBody, nowMs: number): GuideLayout => {
  const spanMs = Math.max(1, source.toMs - source.fromMs);

  const channels = source.channels.map((channel) => {
    const airings: GuideAiringLayout[] = [];
    for (const airing of channel.airings) {
      if (
        airing.stopMs <= source.fromMs ||
        airing.startMs >= source.toMs ||
        airing.stopMs <= airing.startMs
      ) {
        continue;
      }

      const clippedStartMs = clamp(airing.startMs, source.fromMs, source.toMs);
      const clippedStopMs = clamp(airing.stopMs, source.fromMs, source.toMs);
      const isOnNow = airing.startMs <= nowMs && nowMs < airing.stopMs;
      airings.push({
        source: airing,
        channelId: channel.channelId,
        scheduleBlockId: airing.scheduleBlockId,
        startRatio: (clippedStartMs - source.fromMs) / spanMs,
        widthRatio: (clippedStopMs - clippedStartMs) / spanMs,
        clippedStartMs,
        clippedStopMs,
        startsBeforeWindow: airing.startMs < source.fromMs,
        endsAfterWindow: airing.stopMs > source.toMs,
        isOnNow,
        progressRatio: isOnNow
          ? clamp((nowMs - airing.startMs) / (airing.stopMs - airing.startMs), 0, 1)
          : undefined,
      });
    }
    return { source: channel, airings };
  });

  return {
    source,
    fromMs: source.fromMs,
    toMs: source.toMs,
    timezone: source.timezone,
    channels,
  };
};

const distanceFromAiring = (airing: GuideAiringLayout, atMs: number): number => {
  if (airing.source.startMs <= atMs && atMs < airing.source.stopMs) return 0;
  if (atMs < airing.source.startMs) return airing.source.startMs - atMs;
  return atMs - airing.source.stopMs;
};

const nearestAiring = (channel: GuideChannelLayout, atMs: number): GuideAiringLayout | undefined => {
  const containing = channel.airings.find(
    (airing) => airing.source.startMs <= atMs && atMs < airing.source.stopMs,
  );
  if (containing) return containing;

  let nearest: GuideAiringLayout | undefined;
  let nearestDistance = Number.POSITIVE_INFINITY;
  for (const airing of channel.airings) {
    const distance = distanceFromAiring(airing, atMs);
    if (distance < nearestDistance) {
      nearest = airing;
      nearestDistance = distance;
    }
  }
  return nearest;
};

/** Build a focus target for a channel while retaining the requested time column. */
const guideSelectionForChannel = (
  layout: GuideLayout,
  channelId: string,
  anchorMs: number,
): GuideSelection | undefined => {
  const channel = layout.channels.find((candidate) => candidate.source.channelId === channelId);
  if (!channel) return undefined;
  const airing = nearestAiring(channel, anchorMs);
  return {
    channelId,
    scheduleBlockId: airing?.scheduleBlockId,
    anchorMs,
  };
};

const horizontalSelection = (
  channel: GuideChannelLayout,
  selection: GuideSelection,
  direction: "left" | "right",
): GuideNavigationResult => {
  const currentIndex = selection.scheduleBlockId
    ? channel.airings.findIndex((airing) => airing.scheduleBlockId === selection.scheduleBlockId)
    : -1;
  const current =
    currentIndex >= 0 ? channel.airings[currentIndex] : nearestAiring(channel, selection.anchorMs);
  const resolvedIndex = current ? channel.airings.indexOf(current) : -1;
  const nextIndex = resolvedIndex + (direction === "left" ? -1 : 1);
  const next = channel.airings[nextIndex];
  if (!next) return { selection, boundary: direction };

  return {
    selection: {
      channelId: selection.channelId,
      scheduleBlockId: next.scheduleBlockId,
      anchorMs: next.source.startMs + (next.source.stopMs - next.source.startMs) / 2,
    },
  };
};

/**
 * Resolve one grid move. Platform adapters own key/focus events and use boundary results to enter
 * filters or surrounding chrome; the time-column and block-neighbor rules remain identical.
 */
const moveGuideSelection = (
  layout: GuideLayout,
  selection: GuideSelection,
  direction: GuideNavigationDirection,
): GuideNavigationResult => {
  const rowIndex = layout.channels.findIndex((channel) => channel.source.channelId === selection.channelId);
  if (rowIndex < 0) return { selection, boundary: direction };

  const currentChannel = layout.channels[rowIndex];
  if (!currentChannel) return { selection, boundary: direction };
  if (direction === "left" || direction === "right") {
    return horizontalSelection(currentChannel, selection, direction);
  }

  const targetIndex = rowIndex + (direction === "up" ? -1 : 1);
  const targetChannel = layout.channels[targetIndex];
  if (!targetChannel) return { selection, boundary: direction };

  const targetAiring = nearestAiring(targetChannel, selection.anchorMs);
  return {
    selection: {
      channelId: targetChannel.source.channelId,
      scheduleBlockId: targetAiring?.scheduleBlockId,
      anchorMs: selection.anchorMs,
    },
  };
};

export {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  formatGuideEpisode,
  formatGuideTime,
  formatGuideTimeRange,
  GUIDE_BUCKET_MS,
  guideAiringLabel,
  guideChannelState,
  guideSelectionForChannel,
  guideTimeFormatter,
  guideWindow,
  LOOKBACK_MINUTES,
  layoutGuide,
  moveGuideSelection,
};
