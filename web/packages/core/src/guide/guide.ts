import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import type { GuideAiringLayout, GuideLayout, GuideWindow, GuideWindowArgs } from "./guide.type";

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

export {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  GUIDE_BUCKET_MS,
  guideWindow,
  LOOKBACK_MINUTES,
  layoutGuide,
};
