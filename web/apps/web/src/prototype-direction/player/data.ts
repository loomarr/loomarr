// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Stub data for the player-chrome prototypes: channel 7 "Coast 90s" and one evening on it. Invented
// titles only (the repo is public); stills are token-drawn stand-ins, never remote images.
import { standInStill } from "../shared";

const MINUTE = 60_000;
const NOW = Date.UTC(2026, 10, 8, 20, 44);

const formatTime = (ms: number) =>
  new Intl.DateTimeFormat("en-US", { hour: "numeric", minute: "2-digit", timeZone: "UTC" }).format(ms);

const at = (minutes: number) => NOW + minutes * MINUTE;

interface Spot {
  durationMs: number;
  id: string;
  name: string;
  detail: string;
  still: string;
}

interface Programme {
  chips: string[];
  id: string;
  kind: "programme";
  line: string;
  name: string;
  startMs: number;
  stillUri: string;
  stopMs: number;
  synopsis: string;
}

interface Break {
  id: string;
  kind: "break";
  spots: Spot[];
  startMs: number;
  stopMs: number;
}

type Segment = Break | Programme;

const spot = (id: string, name: string, detail: string, seconds: number, hue: number): Spot => ({
  detail,
  durationMs: seconds * 1000,
  id,
  name,
  still: standInStill(hue),
});

const programme = (
  id: string,
  from: number,
  to: number,
  name: string,
  line: string,
  hue: number,
  chips: string[],
  synopsis: string,
): Programme => ({
  chips,
  id,
  kind: "programme",
  line,
  name,
  startMs: at(from),
  stillUri: standInStill(hue, name),
  stopMs: at(to),
  synopsis,
});

const breakOf = (id: string, from: number, spots: Spot[]): Break => {
  const startMs = at(from);
  return {
    id,
    kind: "break",
    spots,
    startMs,
    stopMs: startMs + spots.reduce((sum, s) => sum + s.durationMs, 0),
  };
};

const b1 = breakOf("b1", -12, [
  spot("b1a", "Breakfast cereal", "A cereal brand · 1996", 45, 40),
  spot("b1b", "Family car", "A car maker · 1995", 45, 200),
  spot("b1c", "Station ID", "", 30, 150),
  spot("b1d", "Soft drink", "A soda brand · 1997", 30, 0),
  spot("b1e", "Sneakers", "A shoe brand · 1994", 30, 300),
]);
const b2 = breakOf("b2", 13, [
  spot("b2a", "Toy robot", "A toy maker · 1996", 30, 260),
  spot("b2b", "We'll be right back", "", 5, 180),
  spot("b2c", "Pizza night", "A pizza chain · 1995", 30, 20),
]);

const segments: Segment[] = [
  programme(
    "p1",
    -40,
    -12,
    "The Long Shift",
    'S2 E3 "Overtime"',
    210,
    ["TV-PG", "Sitcom"],
    "A night crew bluffs through inspection day.",
  ),
  b1,
  programme(
    "p2",
    -9,
    13,
    "The Diner Years",
    'S2 E4 "Opening Night"',
    20,
    ["S2 E4", "TV-PG", "Comedy"],
    "Two friends open a diner and learn the hard way who their regulars are.",
  ),
  b2,
  {
    ...programme(
      "p3",
      14,
      126,
      "A detective film",
      "1994 · Drama",
      280,
      ["1994", "TV-PG", "Drama"],
      "A detective takes one last case.",
    ),
    startMs: b2.stopMs,
  },
];

const WINDOW = { endMs: at(120), nowMs: NOW, startMs: at(-15) };

const channel = { era: "1990s", name: "Coast 90s", number: "7", place: "a coastal town" };

interface ChannelRow {
  era: string;
  name: string;
  number: string;
  nowPlaying: string;
}

const channels: ChannelRow[] = [
  { era: "1980s", name: "Saturday Mornings", nowPlaying: "Cartoon block", number: "3" },
  { era: "1970s", name: "Late Show Reruns", nowPlaying: "A talk show", number: "5" },
  { era: "1990s", name: "Coast 90s", nowPlaying: "The Diner Years", number: "7" },
  { era: "1980s", name: "Movie Matinee", nowPlaying: "A heist film", number: "9" },
  { era: "1990s", name: "Game Night", nowPlaying: "A quiz show", number: "12" },
  { era: "1970s", name: "Sunday Westerns", nowPlaying: "A cattle drive", number: "14" },
  { era: "1980s", name: "Sports Replay", nowPlaying: "A baseball classic", number: "21" },
];

export type { Break, ChannelRow, Programme, Segment, Spot };
export { at, b1, channel, channels, formatTime, MINUTE, NOW, segments, WINDOW };

// Variant D's richer title block: what an Emby / Plex / Apple TV player sets under the title.
const nowMeta = { genre: "Comedy", rating: "TV-PG", year: "1996" };

// The channel the viewer came from. Absent (null) on a first tune-in, and then no control is drawn.
const lastChannel: { name: string; number: string } | null = { name: "Saturday Mornings", number: "3" };

export { lastChannel, nowMeta };
