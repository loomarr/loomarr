import { formatClipDuration } from "@loomarr/core/format";
import {
  formatGuideEpisode,
  formatProgrammeTitle,
  type GuideAiringLayout,
  guideBreakProgramme,
  guideFillerNowLabel,
} from "@loomarr/core/guide";

import type {
  PlaybarSegment,
  PlaybarSpot,
  PlaybarWindow,
  PlayerBandData,
  PlayerInfoData,
  PlayerLiveState,
} from "../player-chrome.type";

type TimelineAiring = GuideAiringLayout["source"];
type PodEntry = NonNullable<TimelineAiring["pod"]>["entries"][number];

const MINUTE = 60_000;

/** The seekable past the stream still holds behind live (`playout.DVRHorizon`). */
const PLAYBAR_PAST_MS = 15 * MINUTE;
/** How far ahead the playbar previews: about two hours (§9.3). */
const PLAYBAR_AHEAD_MS = 120 * MINUTE;

// What a spot is when it has no viewer name, or what it is beside a commercial's brand (§9.3).
const SPOT_ROLE: Record<PodEntry["role"], string> = {
  bumper: "Bumper",
  commercial: "Commercial",
  interstitial: "Interstitial",
  news: "News",
  political: "Political ad",
  psa: "PSA",
  station_id: "Station ID",
  trailer: "Coming up",
};

// The spot's line under its name: a commercial reads by `brand · era` when it has them, everything
// else by its role. A spot already named by its role has nothing more to say.
const spotOf = (entry: PodEntry, index: number): PlaybarSpot => {
  const role = SPOT_ROLE[entry.role];
  const name = entry.name.trim() || role;
  const brandEra = [entry.brand, entry.era ? String(entry.era) : undefined].filter(Boolean).join(" · ");
  const detail = entry.role === "commercial" && brandEra ? brandEra : name === role ? "" : role;
  return {
    detail,
    durationLabel: formatClipDuration(entry.durationMs),
    durationMs: entry.durationMs,
    id: `${entry.hash ?? "card"}-${index}`,
    name,
    stillUri: entry.thumbImage?.src,
  };
};

// A series airing's second line: its episode number and the episode's own name.
const episodeLine = (airing: TimelineAiring) =>
  [formatGuideEpisode(airing.season, airing.episode), airing.title.trim() ? `“${airing.title.trim()}”` : ""]
    .filter(Boolean)
    .join(" ");

const segmentOf = (airing: TimelineAiring): PlaybarSegment => {
  if (airing.kind === "filler")
    return {
      id: airing.scheduleBlockId,
      kind: "break",
      spots: (airing.pod?.entries ?? []).map(spotOf),
      startMs: airing.startMs,
      stopMs: airing.stopMs,
    };
  const series = airing.series?.trim();
  const name = series || formatProgrammeTitle(airing);
  const line = series
    ? episodeLine(airing)
    : [airing.year ? String(airing.year) : "", airing.genres?.[0] ?? ""].filter(Boolean).join(" · ");
  const se = series ? episodeLine({ ...airing, title: "" }) : "";
  return {
    chips: [
      se || (airing.year ? String(airing.year) : ""),
      airing.rating ?? "",
      ...(airing.genres ?? []),
    ].filter(Boolean),
    id: airing.scheduleBlockId,
    kind: "programme",
    line: line || undefined,
    name,
    startMs: airing.startMs,
    stillUri: airing.thumbImage?.src ?? (airing.thumbUrl || undefined),
    stopMs: airing.stopMs,
    synopsis: airing.description?.trim() || undefined,
  };
};

const minutes = (ms: number) => Math.max(0, Math.round(ms / MINUTE));

// The station's era as its ident reads it: a whole decade is "1990s", any other span its years. A span
// open at either end says nothing a viewer can read, so it is left off.
const eraLabel = (era: { from?: number; to?: number } | undefined) => {
  const { from, to } = era ?? {};
  if (!from || !to) return undefined;
  if (from % 10 === 0 && to === from + 9) return `${from}s`;
  return from === to ? String(from) : `${from}–${to}`;
};

interface ChromeChannel {
  name: string;
  number: string;
  /** The channel's era decade from the API, for the channel block's colour. */
  decade?: number;
  /** The years the channel airs as (its filler era) and where (its filler place), for the ident. */
  era?: { from?: number; to?: number };
  place?: string;
}

// The lower third reads the block under the PLAYHEAD, not under live: a viewer behind is watching
// what aired then.
const fraction = (at: number, from: number, to: number) =>
  to > from ? Math.min(1, Math.max(0, (at - from) / (to - from))) : 0;

// A programme's name, second line and art, as the band and the playbar read them. The art is the title's
// own clear logo and backdrop (an episode's show's), absent when the Library holds none.
const programmeOf = (airing: TimelineAiring) => {
  const segment = segmentOf(airing);
  const art = {
    ...(airing.logoImage ? { logo: airing.logoImage } : {}),
    ...(airing.backdropImage ? { backdrop: airing.backdropImage } : {}),
  };
  return segment.kind === "programme"
    ? { ...art, line: segment.line, name: segment.name }
    : { line: undefined, name: airing.title };
};

const bandOf = (
  channel: ChromeChannel,
  airings: readonly TimelineAiring[],
  airing: TimelineAiring | undefined,
  playheadMs: number,
  live: PlayerLiveState,
  nowMs: number,
): PlayerBandData => {
  const station = [eraLabel(channel.era), channel.place?.trim()].filter(Boolean).join(" · ");
  const base = {
    channelName: channel.name,
    channelNumber: channel.number,
    clockMs: playheadMs,
    ...(channel.decade === undefined ? {} : { decade: channel.decade }),
    station: station || undefined,
  };
  if (!airing) return base;
  const behindLabel =
    live.mode === "live" ? undefined : `${formatClipDuration(live.lagSeconds * 1000)} behind`;
  const nextAfter = (stopMs: number) => {
    const next = airings.find((a) => a.kind !== "filler" && a.startMs >= stopMs);
    return next ? { name: programmeOf(next).name, startMs: next.startMs } : undefined;
  };
  if (airing.kind === "filler") {
    // A break is a state of the programme it leads back to (#2761): its title, Back in n min.
    const returning = guideBreakProgramme(
      airings.filter((a) => a.kind !== "filler"),
      airing,
    );
    const shown: ReturnType<typeof programmeOf> = returning
      ? programmeOf(returning)
      : { line: undefined, name: airing.title };
    return {
      ...base,
      ...(behindLabel ? { behindLabel } : {}),
      breakMinutesLeft: Math.max(1, minutes(airing.stopMs - playheadMs)),
      inBreak: true,
      next: returning ? nextAfter(returning.stopMs) : undefined,
      progress: returning ? 0 : fraction(playheadMs, airing.startMs, airing.stopMs),
      startMs: returning?.startMs ?? airing.startMs,
      stopMs: returning?.stopMs ?? airing.stopMs,
      title: shown.name,
      ...("logo" in shown ? { logo: shown.logo } : {}),
      ...("backdrop" in shown ? { backdrop: shown.backdrop } : {}),
    };
  }
  const { line, name, ...art } = programmeOf(airing);
  const behind = playheadMs < nowMs - 1000;
  return {
    ...base,
    ...(behindLabel ? { behindLabel } : {}),
    ...(behind ? { liveProgress: fraction(nowMs, airing.startMs, airing.stopMs) } : {}),
    line,
    next: nextAfter(airing.stopMs),
    progress: fraction(playheadMs, airing.startMs, airing.stopMs),
    startMs: airing.startMs,
    stopMs: airing.stopMs,
    title: name,
    ...art,
  };
};

// The clip on air in a break: walk the pod by duration from the break's start.
const podEntryAt = (airing: TimelineAiring, playheadMs: number): PodEntry | undefined => {
  let at = airing.startMs;
  return airing.pod?.entries.find((e) => {
    at += e.durationMs;
    return playheadMs < at;
  });
};

// The Info sheet (§9, mock "info"): the block at the playhead as the band reads it, with the
// programme's still, facts and synopsis, then the next programme — a break is never "up next".
const infoOf = (
  airings: readonly TimelineAiring[],
  airing: TimelineAiring | undefined,
  band: PlayerBandData,
  playheadMs: number,
): PlayerInfoData | undefined => {
  if (!airing) return undefined;
  const next = airings.find((a) => a.kind !== "filler" && a.startMs >= airing.stopMs);
  const nextSegment = next ? segmentOf(next) : undefined;
  const facts =
    airing.kind === "filler"
      ? ""
      : [
          formatGuideEpisode(airing.season, airing.episode) ?? "",
          airing.year ? String(airing.year) : "",
          airing.rating ?? "",
          ...(airing.genres ?? []),
        ]
          .filter(Boolean)
          .join(" · ");
  const segment = segmentOf(airing);
  return {
    facts: facts || undefined,
    progress: band.progress ?? 0,
    startMs: airing.startMs,
    stillUri: segment.kind === "programme" ? segment.stillUri : undefined,
    stopMs: airing.stopMs,
    synopsis: segment.kind === "programme" ? segment.synopsis : undefined,
    timeLabel:
      airing.kind === "filler"
        ? (band.behindLabel ?? `Break · ${minutes(airing.stopMs - airing.startMs)} min`)
        : `${minutes(playheadMs - airing.startMs)}m in · ${band.behindLabel ?? `${minutes(airing.stopMs - playheadMs)}m left`}`,
    title:
      airing.kind === "filler"
        ? guideFillerNowLabel(podEntryAt(airing, playheadMs))
        : formatProgrammeTitle(airing),
    upNext:
      next && nextSegment?.kind === "programme"
        ? { line: nextSegment.line, name: nextSegment.name, startMs: next.startMs }
        : undefined,
  };
};

interface PlaybarFromTimelineArgs {
  airings: readonly TimelineAiring[];
  channel: ChromeChannel;
  live: PlayerLiveState;
  /** The live edge, on the server's clock. */
  nowMs: number;
  /** Where the viewer is watching; behind the live edge when paused or behind. */
  playheadMs: number;
}

/**
 * The player chrome's data, read from a channel's timeline the same way on every client (§9): the
 * lower third and the Info sheet for the block at the playhead, and the playbar's blocks over its window — fifteen
 * minutes behind live to about two hours ahead. Blocks outside the window are dropped; the ones it
 * cuts are clipped to it. `startOverMs` is where Start over goes, while it has somewhere to go.
 */
const playbarFromTimeline = ({ airings, channel, live, nowMs, playheadMs }: PlaybarFromTimelineArgs) => {
  const window: PlaybarWindow = { endMs: nowMs + PLAYBAR_AHEAD_MS, nowMs, startMs: nowMs - PLAYBAR_PAST_MS };
  const segments = airings
    .filter((a) => a.stopMs > window.startMs && a.startMs < window.endMs)
    .map((a) => ({
      ...segmentOf(a),
      startMs: Math.max(a.startMs, window.startMs),
      stopMs: Math.min(a.stopMs, window.endMs),
    }));
  const atPlayhead = airings.find((a) => playheadMs >= a.startMs && playheadMs < a.stopMs);
  // Start over (#2088 decision 2): the programme at the playhead, when its start is still held.
  const startOverMs =
    atPlayhead && atPlayhead.kind !== "filler" && atPlayhead.startMs >= window.startMs
      ? atPlayhead.startMs
      : undefined;
  const band = bandOf(channel, airings, atPlayhead, playheadMs, live, nowMs);
  return { band, info: infoOf(airings, atPlayhead, band, playheadMs), segments, startOverMs, window };
};

export { playbarFromTimeline };
