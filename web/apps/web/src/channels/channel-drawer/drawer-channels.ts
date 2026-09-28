import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { GuideChannelTimelineStatus } from "@loomarr/api/models/guideChannelTimelineStatus";
import type { DrawerChannel } from "./channel-drawer.type";

// The drawer's strip is "NOW → NEXT 2H" with a little of the show already airing before the now
// line, which the console mock draws at 17% of the row.
const STRIP_BEFORE_MS = 24 * 60_000;
const STRIP_AFTER_MS = 120 * 60_000;
const DRAWER_NOW_PERCENT = (STRIP_BEFORE_MS / (STRIP_BEFORE_MS + STRIP_AFTER_MS)) * 100;

// The mock's programme line: "Series — “Episode”" for an episode, the title alone for a film.
const programmeLine = (airing: GuideAiring): string =>
  airing.series ? `${airing.series} — “${airing.title}”` : airing.title;

// drawerChannels reads the guide the Watch page already shares with Home and the Guide into the
// drawer's rows: what's on now, the minutes left, and the blocks from just before now to two hours
// on. A paused or still-building channel says so instead.
const drawerChannels = (timelines: GuideChannelTimeline[], nowMs: number): DrawerChannel[] => {
  const from = nowMs - STRIP_BEFORE_MS;
  const to = nowMs + STRIP_AFTER_MS;
  return [...timelines]
    .sort((a, b) => a.number - b.number || a.channelId.localeCompare(b.channelId))
    .map((timeline) => {
      const offAir =
        timeline.status === GuideChannelTimelineStatus.paused
          ? "paused"
          : timeline.status === GuideChannelTimelineStatus.building ||
              timeline.status === GuideChannelTimelineStatus.empty
            ? "building"
            : undefined;
      const current = offAir
        ? undefined
        : timeline.airings.find(
            (airing) =>
              airing.kind === GuideAiringKind.program && airing.startMs <= nowMs && nowMs < airing.stopMs,
          );
      const blocks = offAir
        ? []
        : timeline.airings
            .filter(
              (airing) =>
                airing.stopMs > from && airing.startMs < to && airing.kind !== GuideAiringKind.pending,
            )
            .map((airing) => {
              const pod = airing.kind !== GuideAiringKind.program;
              return {
                key: airing.scheduleBlockId,
                label: pod ? undefined : (airing.series ?? airing.title),
                minutes: (Math.min(airing.stopMs, to) - Math.max(airing.startMs, from)) / 60_000,
                pod,
              };
            });
      return {
        id: timeline.channelId,
        number: timeline.number,
        name: timeline.name,
        now: current ? programmeLine(current) : undefined,
        minutesLeft: current ? Math.max(1, Math.round((current.stopMs - nowMs) / 60_000)) : undefined,
        offAir,
        blocks,
      };
    });
};

export { DRAWER_NOW_PERCENT, drawerChannels };
