// PROTOTYPE — throwaway (#2088 step 0). Stub data for the /prototype/watch variants. Not product code:
// it lives on the prototype branch only and never reaches main.
import type { GuideAiring } from "@loomarr/api/models/guideAiring";

const NOW = Date.now();
const MIN = 60_000;

// The tuned channel, mid-programme: an episode started 9 minutes ago (inside the 15-min window, so
// "Start over" is offered), a commercial break next, then two more programmes.
const airings: GuideAiring[] = [
  {
    kind: "program",
    scheduleBlockId: "b1",
    startMs: NOW - 9 * MIN,
    stopMs: NOW + 13 * MIN,
    title: "The Last Lighthouse",
    series: "Coastline Mysteries",
    season: 2,
    episode: 4,
    year: 1994,
    rating: "TV-PG",
    genres: ["Mystery", "Drama"],
    description:
      "A storm cuts the island off from the mainland, and the keeper's logbook turns up with a page missing — the night the supply boat never arrived.",
  },
  {
    kind: "filler",
    scheduleBlockId: "b2",
    startMs: NOW + 13 * MIN,
    stopMs: NOW + 16 * MIN,
    title: "Commercials",
  },
  {
    kind: "program",
    scheduleBlockId: "b3",
    startMs: NOW + 16 * MIN,
    stopMs: NOW + 46 * MIN,
    title: "Pilot",
    series: "Night Shift Diner",
    season: 1,
    episode: 1,
    year: 1997,
    rating: "TV-14",
    genres: ["Comedy"],
    description: "A new cook, a broken neon sign, and the regulars who come in at 3 a.m.",
  },
  {
    kind: "filler",
    scheduleBlockId: "b4",
    startMs: NOW + 46 * MIN,
    stopMs: NOW + 49 * MIN,
    title: "Commercials",
  },
  {
    kind: "program",
    scheduleBlockId: "b5",
    startMs: NOW + 49 * MIN,
    stopMs: NOW + 109 * MIN,
    title: "Harbour Lights",
    year: 1989,
    rating: "PG",
    genres: ["Romance"],
    description: "A lighthouse inspector and a ferry captain, one summer on the north coast.",
  },
];

const channel = { id: "ch-7", number: 7, name: "Coast 90s", era: "1990s", place: "Portland, ME" };

// Filler state: the clip airing in the break.
const commercial = { clipTitle: "Sunny Delight — 'Purple Stuff' (1996)" };

const channels = [
  { number: 3, name: "Saturday Toons", now: "DuckTales", favourite: true },
  { number: 5, name: "Late Night Movies", now: "Harbour Lights", favourite: false },
  { number: 7, name: "Coast 90s", now: "Coastline Mysteries", favourite: true },
  { number: 9, name: "Sitcom Gold", now: "Night Shift Diner", favourite: false },
  { number: 12, name: "Nature Hour", now: "Tidepools", favourite: false },
  { number: 14, name: "Classic Sci-Fi", now: "Starward", favourite: true },
];

const quality = { auto: true, current: "720p", rungs: ["1080p", "720p", "480p", "360p"] };
const audio = { current: "English · stereo", options: ["English · stereo", "Español · stereo"] };
const subtitles = { current: "Off", options: ["Off", "English", "English SDH", "Español"] };

const playbackInfo = {
  viewer: [
    ["Quality", "Auto · 720p (rung 2 of 4)"],
    ["Resolution", "1280×720 @ 29.97"],
    ["Codec", "H.264 High · AAC-LC stereo"],
    ["Bitrate", "3.1 Mbps"],
    ["Buffer", "12.4 s"],
    ["Behind live", "0 s"],
  ],
  admin: [
    ["Encoder", "h264_nvenc · ladder 4 rungs"],
    ["Transcode reason", "Source HEVC 10-bit; browser profile h264"],
    ["Session", "ps_8f2c…a91"],
  ],
} as const;

// What each break holds, by schedule block: the guide's `pod.entries` (PodEntryDTO name, kind, brand,
// era, durationMs). A real preview tile is the clip's thumbnail (clip-thumb by hash); here a hue
// stands in for it. Station material (station_id, bumper) is part of the break like any ad.
interface BreakClip {
  name: string;
  kind: "commercial" | "station_id" | "bumper" | "trailer";
  brand?: string;
  era?: number;
  durationS: number;
  hue: number;
}
const breaks: Record<string, BreakClip[]> = {
  b2: [
    {
      name: "Sunny Delight — 'Purple Stuff'",
      kind: "commercial",
      brand: "Sunny Delight",
      era: 1996,
      durationS: 30,
      hue: 285,
    },
    {
      name: "Nintendo 64 — 'Get N or Get Out'",
      kind: "commercial",
      brand: "Nintendo",
      era: 1996,
      durationS: 30,
      hue: 0,
    },
    { name: "Coast 90s station ID", kind: "station_id", durationS: 10, hue: 38 },
    {
      name: "Blockbuster — 'Make It a Blockbuster Night'",
      kind: "commercial",
      brand: "Blockbuster",
      era: 1997,
      durationS: 30,
      hue: 220,
    },
    { name: "Night Shift Diner — tonight at 8:25", kind: "trailer", durationS: 20, hue: 170 },
  ],
  b4: [
    {
      name: "Pizza Hut — 'Stuffed Crust'",
      kind: "commercial",
      brand: "Pizza Hut",
      era: 1995,
      durationS: 30,
      hue: 10,
    },
    { name: "Coast 90s bumper", kind: "bumper", durationS: 8, hue: 38 },
    {
      name: "Ford Explorer — 'No Boundaries'",
      kind: "commercial",
      brand: "Ford",
      era: 1997,
      durationS: 30,
      hue: 200,
    },
  ],
};
// Programme stills (the airing's thumbUrl) — hue stand-ins, keyed by schedule block.
const stillHue: Record<string, number> = { b1: 245, b3: 330, b5: 25 };

export type { BreakClip };
export {
  airings,
  audio,
  breaks,
  channel,
  channels,
  commercial,
  MIN,
  NOW,
  playbackInfo,
  quality,
  stillHue,
  subtitles,
};
