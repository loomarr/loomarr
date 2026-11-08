// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Stub guide data for the three states, and the small reads every variant shares ("what is on this
// channel now / next"). Invented titles only: the repo is public.
import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { type GuideLayout, layoutGuide } from "@loomarr/core/guide";
import { guidePod } from "@loomarr/fixtures";

const STATES = ["normal", "shorts", "downloading"] as const;
type GuideState = (typeof STATES)[number];

const TIMEZONE = "America/New_York";
// 8:00 PM on a summer evening in New York; every minute below is an offset from here.
const T0 = Date.UTC(2026, 6, 26, 0, 0);
const at = (minutes: number) => T0 + minutes * 60_000;
const NOW_MS = at(67); // 9:07 PM
const FROM_MS = at(30); // 8:30 PM, the guide's 30-minute lookback
const END_MS = at(330);

const ZOOMS = [120, 240] as const;
type Zoom = (typeof ZOOMS)[number];

interface Show {
  title: string;
  series?: string;
  mins: number;
}

interface ChannelSpec {
  id: string;
  name: string;
  number: number;
  era?: "1970s" | "1980s" | "1990s";
  start: number;
  shows: Show[];
  pods: number[];
  // A channel still downloading its titles airs commercials end to end.
  downloading?: boolean;
}

const episodic = (series: string, mins: number, titles: string[]): Show[] =>
  titles.map((title) => ({ title, series, mins }));
const movies = (list: [string, number][]): Show[] => list.map(([title, mins]) => ({ title, mins }));

const cartoons: ChannelSpec = {
  id: "cartoons",
  name: "Cartoon Classics",
  number: 2,
  era: "1990s",
  start: -9,
  pods: [3, 4, 2],
  shows: episodic("Kettle Kids", 22, [
    "The Case of the Missing Mitten",
    "Pip and the Paper Boat",
    "A Very Loud Library",
    "The Upside-Down Picnic",
    "Snow Day for Gulls",
  ]),
};
const scifi: ChannelSpec = {
  id: "scifi",
  name: "Sci-Fi Classics",
  number: 3,
  era: "1980s",
  start: -20,
  pods: [4, 3],
  shows: episodic("Cold Relay", 30, [
    "Static Over Harbour Nine",
    "The Lantern Colony",
    "Signal From Below",
    "Orbit of the Quiet Moon",
  ]),
};
const thrillers: ChannelSpec = {
  id: "thrillers",
  name: "1980s Thrillers",
  number: 4,
  era: "1980s",
  start: -40,
  pods: [4, 4, 3],
  shows: movies([
    ["Marigold Protocol", 92],
    ["Midnight at the Mapleton", 104],
    ["The Long Way to Ferris Dock", 88],
    ["Quiet Hands", 96],
  ]),
};
const sitcoms: ChannelSpec = {
  id: "sitcoms",
  name: "Sitcom Corner",
  number: 5,
  era: "1990s",
  start: -5,
  pods: [3, 2, 4],
  shows: episodic("Two Doors Down", 22, [
    "The Spare Key",
    "Casserole Diplomacy",
    "Who Borrowed the Ladder",
    "Block Party Blues",
  ]),
};
const saturday: ChannelSpec = {
  id: "saturday",
  name: "Saturday Morning Cartoons",
  number: 6,
  era: "1980s",
  start: -18,
  pods: [3, 4],
  shows: [
    { series: "Captain Crumb", title: "The Great Cereal Chase", mins: 22 },
    { series: "Robo Pup", title: "Fetch Protocol", mins: 22 },
    { series: "Captain Crumb", title: "Milk Moon", mins: 30 },
    { series: "Robo Pup", title: "The Squeaky Menace", mins: 22 },
  ],
};
const westerns: ChannelSpec = {
  id: "westerns",
  name: "Matinee Westerns",
  number: 7,
  era: "1970s",
  start: -55,
  pods: [4, 3],
  shows: movies([
    ["Dust on the Long Road", 94],
    ["Two Horses to Redwater", 88],
    ["The Quiet Sheriff", 91],
  ]),
};
const lateNight: ChannelSpec = {
  id: "latenight",
  name: "Late Night Movies",
  number: 8,
  start: -25,
  pods: [4, 3, 4],
  shows: movies([
    ["The Last Tram Home", 101],
    ["Harbour Lights", 87],
    ["Paper Moon Motel", 108],
  ]),
};
const cooking: ChannelSpec = {
  id: "cooking",
  name: "Home Cooking",
  number: 9,
  start: -12,
  pods: [3, 2, 3],
  shows: episodic("Supper in Thirty", 30, [
    "One-Pan Wonders",
    "The Humble Dumpling",
    "Soup Weather",
    "Pancakes for Dinner",
  ]),
};
const tinyTales: ChannelSpec = {
  id: "tinytales",
  name: "Tiny Tales",
  number: 3,
  era: "1970s",
  start: -6,
  pods: [2, 2, 3],
  shows: [
    ["The Snail Who Hurried", 4],
    ["Moth Opera", 11],
    ["A Hat for Winter", 2],
    ["Button's Big Day", 7],
    ["The Quiet Kite", 3],
    ["Two Socks", 9],
    ["Lighthouse Lullaby", 5],
    ["Fig and the Moon", 2],
    ["Paper Planes", 6],
    ["The Tiny Parade", 10],
  ].map(([title, mins]) => ({ title: String(title), mins: Number(mins) })),
};

const specsFor = (state: GuideState): ChannelSpec[] => {
  if (state === "shorts")
    return [cartoons, tinyTales, thrillers, sitcoms, saturday, westerns, lateNight, cooking];
  const base = [cartoons, scifi, thrillers, sitcoms, saturday, westerns, lateNight, cooking];
  if (state === "downloading")
    return base.map((spec) =>
      spec.id === "westerns" || spec.id === "cooking" ? { ...spec, downloading: true, start: -20 } : spec,
    );
  return base;
};

const airingsFor = (spec: ChannelSpec): GuideAiring[] => {
  const out: GuideAiring[] = [];
  let t = spec.start;
  let i = 0;
  const pod = (mins: number) => {
    out.push({
      kind: "filler",
      pod: guidePod,
      scheduleBlockId: `${spec.id}-${i}-pod`,
      startMs: at(t),
      stopMs: at(t + mins),
      title: "Commercials",
    });
    t += mins;
  };
  while (t < 330) {
    const pods = spec.pods[i % spec.pods.length] ?? 3;
    if (spec.downloading) {
      pod(pods);
    } else {
      const show = spec.shows[i % spec.shows.length];
      if (!show) break;
      out.push({
        kind: "program",
        scheduleBlockId: `${spec.id}-${i}`,
        series: show.series,
        startMs: at(t),
        stopMs: at(t + show.mins),
        title: show.title,
      });
      t += show.mins;
      pod(pods);
    }
    i += 1;
  }
  return out;
};

const timeline = (spec: ChannelSpec): GuideChannelTimeline => ({
  airings: airingsFor(spec),
  channelId: spec.id,
  name: spec.name,
  number: spec.number,
  pendingCount: 0,
  status: spec.downloading ? "building" : "live",
});

const buildGuide = (
  state: GuideState,
  zoom: Zoom,
): { eras: Record<string, ChannelSpec["era"]>; layout: GuideLayout } => {
  const specs = specsFor(state);
  const body: GuideOutputBody = {
    channels: specs.map(timeline),
    fromMs: FROM_MS,
    timezone: TIMEZONE,
    toMs: FROM_MS + zoom * 60_000,
  };
  return {
    eras: Object.fromEntries(specs.map((spec) => [spec.id, spec.era])),
    layout: layoutGuide(body, NOW_MS),
  };
};

// ---------------------------------------------------------------------------------------------
// What a channel is doing right now, in words a household uses.

type NowKind = "show" | "break" | "downloading";

interface ChannelNow {
  kind: NowKind;
  airing?: GuideAiring;
  title: string;
  series?: string;
  minutesLeft: number;
  progress: number;
  next?: GuideAiring;
}

const channelNow = (channel: GuideChannelTimeline, nowMs: number): ChannelNow => {
  if (channel.status === "building") {
    return {
      airing: channel.airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs),
      kind: "downloading",
      minutesLeft: 0,
      progress: 0,
      title: "Getting its shows ready",
    };
  }
  const index = channel.airings.findIndex((a) => a.startMs <= nowMs && nowMs < a.stopMs);
  const airing = channel.airings[index];
  const next = channel.airings.slice(index + 1).find((a) => a.kind === "program");
  const base = {
    airing,
    minutesLeft: airing ? Math.ceil((airing.stopMs - nowMs) / 60_000) : 0,
    next,
    progress: airing ? (nowMs - airing.startMs) / (airing.stopMs - airing.startMs) : 0,
  };
  if (airing?.kind !== "program") return { ...base, kind: "break", title: "Commercial break" };
  return { ...base, kind: "show", series: airing.series, title: airing.title };
};

const minutesText = (minutes: number) => {
  if (minutes < 60) return `${minutes} min`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m === 0 ? `${h} hr` : `${h} hr ${m} min`;
};

const DOWNLOADING_SENTENCE = "Still downloading its shows. Commercials play until they arrive.";

export type { ChannelNow, GuideState, NowKind, Zoom };
export {
  buildGuide,
  channelNow,
  DOWNLOADING_SENTENCE,
  END_MS,
  FROM_MS,
  minutesText,
  NOW_MS,
  STATES,
  TIMEZONE,
  ZOOMS,
};
