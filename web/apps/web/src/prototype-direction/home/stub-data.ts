// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Stub data for the three Home variants: six invented channels at a fixed clock (8:17 PM), in three
// states. Programme titles are invented (the repo is public).

import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import { GuideHighlightDTOReason } from "@loomarr/api/models/guideHighlightDTOReason";

const HOME_STATES = ["normal", "first-run", "downloading"] as const;
type HomeState = (typeof HOME_STATES)[number];

const MIN = 60_000;
// 8:17 PM on a Sunday, shown in UTC so every viewer reads the same clock.
const NOW_MS = Date.UTC(2026, 10, 8, 20, 17);
const TIME_ZONE = "UTC";

interface ProtoChannel {
  id: string;
  number: number;
  name: string;
  era?: string;
  hue: number;
  // What is on this minute when the channel is ready.
  series?: string;
  title: string;
  // "S2 E4" for an episode, the year for a film.
  episode?: string;
  minsLeft: number;
  runtimeMins: number;
  timeRange: string;
  // The next programme (also what a still-downloading channel is waiting to play).
  next: { series?: string; title: string; at: string };
  // Titles downloaded of the titles the channel needs; ready when have === total.
  have: number;
  total: number;
  ready: boolean;
  // The ad on screen while a channel has no titles yet, and how long the break runs.
  adSpot: string;
  adRange: string;
}

interface ProtoTitle {
  key: string;
  name: string;
  hue: number;
  channelName: string;
  channelNumber: number;
}

interface HomeData {
  channels: ProtoChannel[];
  // The caller's last-tuned channel; undefined for a member who has never tuned.
  lastTuned?: ProtoChannel;
  // The lowest-numbered channel: where a first-time member lands.
  lowest: ProtoChannel;
  highlights: GuideHighlightDTO[];
  titles: ProtoTitle[];
  newChannel: { name: string; number: number; era?: string; line: string };
  readyCount: number;
}

const base = [
  {
    id: "ch-3",
    number: 3,
    name: "Saturday Toons",
    hue: 28,
    series: "Rocket Pup",
    title: "Moon Cheese",
    episode: "S1 E12",
    minsLeft: 9,
    runtimeMins: 22,
    timeRange: "8:04–8:26 PM",
    next: { series: "Captain Pickle", title: "The Great Jar Escape", at: "8:26 PM" },
    total: 12,
    adSpot: "Brightside Cereal",
    adRange: "8:15–8:20 PM",
    downloaded: 3,
  },
  {
    id: "ch-5",
    number: 5,
    name: "Late Night Movies",
    era: "1980s",
    hue: 262,
    title: "Midnight at the Lantern Hotel",
    episode: "1986",
    minsLeft: 54,
    runtimeMins: 104,
    timeRange: "7:27–9:11 PM",
    next: { title: "The Velvet Detour", at: "9:11 PM" },
    total: 9,
    adSpot: "Maple Mart Savings",
    adRange: "8:16–8:21 PM",
    downloaded: 2,
  },
  {
    id: "ch-7",
    number: 7,
    name: "Coast 90s",
    era: "1990s",
    hue: 205,
    series: "Coastline Mysteries",
    title: "The Last Lighthouse",
    episode: "S2 E4",
    minsLeft: 13,
    runtimeMins: 30,
    timeRange: "8:00–8:30 PM",
    next: { series: "Coastline Mysteries", title: "Fog on the Pier", at: "8:30 PM" },
    total: 12,
    adSpot: "Sparkle Soda",
    adRange: "8:14–8:19 PM",
    downloaded: 3,
  },
  {
    id: "ch-9",
    number: 9,
    name: "Sitcom Gold",
    era: "1990s",
    hue: 18,
    series: "The Brightons",
    title: "Neighbour Wars",
    episode: "S3 E9",
    minsLeft: 13,
    runtimeMins: 30,
    timeRange: "8:00–8:30 PM",
    next: { series: "The Brightons", title: "The Surprise Party", at: "8:30 PM" },
    total: 12,
    adSpot: "Hearth Insurance",
    adRange: "8:12–8:17 PM",
    downloaded: 12,
  },
  {
    id: "ch-12",
    number: 12,
    name: "Nature Hour",
    hue: 142,
    series: "Tidepool Diaries",
    title: "Low Tide",
    episode: "S1 E3",
    minsLeft: 41,
    runtimeMins: 60,
    timeRange: "7:58–8:58 PM",
    next: { series: "Tidepool Diaries", title: "The Kelp Forest", at: "8:58 PM" },
    total: 8,
    adSpot: "Cobalt Bikes",
    adRange: "8:10–8:15 PM",
    downloaded: 1,
  },
  {
    id: "ch-14",
    number: 14,
    name: "Classic Sci-Fi",
    era: "1970s",
    hue: 232,
    series: "Starbound Frontier",
    title: "Dust Planet",
    episode: "S1 E7",
    minsLeft: 28,
    runtimeMins: 50,
    timeRange: "7:55–8:45 PM",
    next: { series: "Starbound Frontier", title: "The Silent Relay", at: "8:45 PM" },
    total: 10,
    adSpot: "Orbit Toothpaste",
    adRange: "8:13–8:18 PM",
    downloaded: 4,
  },
] as const;

const highlight = (
  channel: (typeof base)[number],
  startMins: number,
  series: string,
  title: string,
  reason: GuideHighlightDTO["reason"],
  extra: Partial<GuideHighlightDTO> & { season?: number } = {},
): GuideHighlightDTO => {
  const { season, ...rest } = extra;
  return {
    airing: {
      kind: GuideAiringKind.program,
      scheduleBlockId: `blk-${channel.id}-${startMins}`,
      series,
      title,
      season,
      startMs: Date.UTC(2026, 10, 8, 20, 0) + startMins * MIN,
      stopMs: Date.UTC(2026, 10, 8, 20, 0) + (startMins + 30) * MIN,
    },
    channelId: channel.id,
    channelName: channel.name,
    channelNumber: channel.number,
    reason,
    ...rest,
  };
};

const byId = (id: string) => base.find((c) => c.id === id) ?? base[0];

const titlesAll: ProtoTitle[] = [
  {
    key: "t1",
    name: "The Lamplighter’s Daughter",
    hue: 250,
    channelName: "Late Night Movies",
    channelNumber: 5,
  },
  { key: "t2", name: "Harbor Lights", hue: 200, channelName: "Coast 90s", channelNumber: 7 },
  { key: "t3", name: "Pudding Planet", hue: 30, channelName: "Saturday Toons", channelNumber: 3 },
  { key: "t4", name: "Orbit Station Nine", hue: 230, channelName: "Classic Sci-Fi", channelNumber: 14 },
  { key: "t5", name: "Marsh Walkers", hue: 140, channelName: "Nature Hour", channelNumber: 12 },
];

const buildHome = (state: HomeState): HomeData => {
  const downloading = state === "downloading";
  const channels: ProtoChannel[] = base.map((c) => {
    const have = downloading ? c.downloaded : c.total;
    return {
      id: c.id,
      number: c.number,
      name: c.name,
      era: "era" in c ? c.era : undefined,
      hue: c.hue,
      series: "series" in c ? c.series : undefined,
      title: c.title,
      episode: c.episode,
      minsLeft: c.minsLeft,
      runtimeMins: c.runtimeMins,
      timeRange: c.timeRange,
      next: c.next,
      have,
      total: c.total,
      ready: have === c.total,
      adSpot: c.adSpot,
      adRange: c.adRange,
    };
  });
  const lowest = channels[0] as ProtoChannel;
  const lastTuned = state === "first-run" ? undefined : channels.find((c) => c.id === "ch-7");

  const all = [
    highlight(
      byId("ch-9"),
      30,
      "The Brightons",
      "The Surprise Party",
      GuideHighlightDTOReason.season_premiere,
      {
        season: 4,
      },
    ),
    highlight(byId("ch-14"), 60, "Starbound Frontier", "Edge of the Map", GuideHighlightDTOReason.marathon, {
      untilMs: Date.UTC(2026, 10, 8, 23, 0),
      episodes: 4,
    }),
    highlight(
      byId("ch-12"),
      100,
      "Tidepool Diaries",
      "Welcome to the Shallows",
      GuideHighlightDTOReason.series_premiere,
    ),
  ];
  // While titles download only a channel with its shows can honestly promise anything tonight.
  const highlights = downloading ? all.filter((h) => h.channelId === "ch-9") : all;

  return {
    channels,
    lastTuned,
    lowest,
    highlights,
    titles: downloading ? titlesAll.slice(0, 3) : titlesAll,
    newChannel: {
      name: "Nature Hour",
      number: 12,
      line: "Requested by Maya · added Tuesday",
    },
    readyCount: channels.filter((c) => c.ready).length,
  };
};

const pad = (n: number) => String(n).padStart(2, "0");

export type { HomeData, HomeState, ProtoChannel, ProtoTitle };
export { buildHome, HOME_STATES, NOW_MS, pad, TIME_ZONE };
