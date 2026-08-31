import {
  formatGuideEpisode,
  formatGuideTime,
  formatGuideTimeRange,
  type GuideAiringLayout,
  type GuideLayout,
  guideAiringLabel,
} from "@loomarr/core/guide";
import type { SurfChannelData, SurfGroupData, SurfSelection } from "../surf-rail.type";

const programmeFacts = (airing: GuideAiringLayout) =>
  [
    airing.source.year ? String(airing.source.year) : undefined,
    airing.source.rating,
    airing.source.genres?.slice(0, 2).join(" · "),
  ].filter((fact): fact is string => Boolean(fact));

const surfChannelData = (
  channel: GuideLayout["channels"][number],
  nowMs: number,
  timezone?: string,
): SurfChannelData => {
  const now = channel.airings.find(
    (airing) => airing.source.startMs <= nowMs && airing.source.stopMs > nowMs,
  );
  const next = channel.airings.find((airing) => airing.source.startMs >= (now?.source.stopMs ?? nowMs));
  return {
    channelLogoUri: channel.source.logo,
    channelLogoState: channel.source.logo ? "ready" : "missing",
    channelName: channel.source.name,
    channelNumber: String(channel.source.number),
    id: channel.source.channelId,
    next: next
      ? {
          timeLabel: formatGuideTime(next.source.startMs, timezone),
          title: guideAiringLabel(next.source),
        }
      : undefined,
    now: now
      ? {
          artworkState: now.source.thumbImage || now.source.thumbUrl ? "ready" : "missing",
          artworkUri: now.source.thumbImage?.src ?? now.source.thumbUrl,
          badge: { label: "On now", tone: "live" },
          description: now.source.description,
          episodeLabel: formatGuideEpisode(now.source.season, now.source.episode),
          facts: programmeFacts(now),
          progressPercent: now.progressRatio === undefined ? undefined : now.progressRatio * 100,
          remainingLabel: `${Math.max(1, Math.ceil((now.source.stopMs - nowMs) / 60_000))}m left`,
          seriesTitle: now.source.series,
          timeLabel: formatGuideTimeRange(now.source.startMs, now.source.stopMs, timezone),
          title: guideAiringLabel(now.source),
        }
      : undefined,
  };
};

const surfGroupsFromGuide = (
  layout: GuideLayout,
  playableChannelIds: readonly string[],
  recentChannelIds: readonly string[],
  nowMs: number,
): SurfGroupData[] => {
  const playable = new Set(playableChannelIds);
  const all = layout.channels
    .filter((channel) => playable.has(channel.source.channelId))
    .map((channel) => surfChannelData(channel, nowMs, layout.timezone));
  const byId = new Map(all.map((channel) => [channel.id, channel]));
  return [
    { channels: [], kind: "favourites", label: "Favourites" },
    {
      channels: recentChannelIds.flatMap((id) => {
        const channel = byId.get(id);
        return channel ? [channel] : [];
      }),
      kind: "recent",
      label: "Recent",
    },
    { channels: all, kind: "all", label: "All channels" },
  ];
};

const watchingScheduleFromGuide = (
  layout: GuideLayout | undefined,
  channelId: string | undefined,
  nowMs: number,
): Pick<SurfChannelData, "next" | "now"> | undefined => {
  const channel = layout?.channels.find((candidate) => candidate.source.channelId === channelId);
  return channel ? surfChannelData(channel, nowMs, layout?.timezone) : undefined;
};

const restoreSurfSelection = (
  groups: readonly SurfGroupData[],
  selection?: SurfSelection,
): SurfSelection | undefined => {
  const selections = groups.flatMap((group) =>
    group.channels.map((channel) => ({ channelId: channel.id, group: group.kind })),
  );
  if (!selection) return selections[0];
  return (
    selections.find(
      (candidate) => candidate.group === selection.group && candidate.channelId === selection.channelId,
    ) ??
    selections.find((candidate) => candidate.channelId === selection.channelId) ??
    selections[0]
  );
};

export { restoreSurfSelection, surfChannelData, surfGroupsFromGuide, watchingScheduleFromGuide };
