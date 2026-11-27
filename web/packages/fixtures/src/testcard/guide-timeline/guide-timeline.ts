import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { PodPoolDTO } from "@loomarr/api/models/podPoolDTO";

// The guide grid's window (§12). FIXED epoch ms, never derived from a clock: the grid
// positions every block against `guideFrom`, so a moving origin would move every block and
// the visual suite would diff on nothing but the time of day.
//
// 20:00–22:00 UTC on 2026-07-25, the same instant the store fixtures use.
const guideFrom = Date.UTC(2026, 6, 25, 20, 0, 0);
const guideTo = Date.UTC(2026, 6, 25, 22, 0, 0);
// Halfway in, so the now-line lands mid-window in every story rather than at an edge.
const guideNow = Date.UTC(2026, 6, 25, 21, 0, 0);

const guideAt = (minutes: number) => guideFrom + minutes * 60_000;

// One assembled break, for the grid's inline clip rendering and the detail card. Era and
// quality are present because they are exactly what the card exists to show — a 1994 480p
// capture is authentic, not a playback fault.
const guidePod: PodPoolDTO = {
  matchLevel: "exact",
  totalMs: 70_000,
  entries: [
    {
      name: "Channel bumper",
      role: "bumper",
      durationMs: 5_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
    {
      name: "Fizzbolt — Dude!",
      role: "commercial",
      durationMs: 30_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
    {
      name: "Gloopers",
      role: "commercial",
      durationMs: 30_000,
      era: 1993,
      quality: "360p",
      isFallbackCard: false,
    },
    {
      name: "Back after these",
      role: "bumper",
      durationMs: 5_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
  ],
};

// Three channels covering the four kinds a grid must distinguish (§12): a movie channel, an
// episodic channel whose blocks carry series + SxxExx, and one still acquiring. Deliberately
// includes a programme that STARTED BEFORE the window (Marigold Protocol, at -25m) — the
// in-progress case that must clip to the axis while keeping its true end.
const guideChannels: GuideChannelTimeline[] = [
  {
    channelId: "ch-action",
    name: "1980s Thrillers",
    number: 3,
    status: "live",
    pendingCount: 0,
    airings: [
      {
        kind: "program",
        scheduleBlockId: "block_action_marigold",
        title: "Marigold Protocol",
        startMs: guideAt(-25),
        stopMs: guideAt(35),
        year: 1989,
      },
      {
        kind: "filler",
        scheduleBlockId: "block_action_break_1",
        title: "Commercials",
        startMs: guideAt(35),
        stopMs: guideAt(39),
        pod: guidePod,
      },
      {
        kind: "program",
        scheduleBlockId: "block_action_mapleton",
        title: "Midnight at the Mapleton",
        startMs: guideAt(39),
        stopMs: guideAt(111),
        year: 1984,
      },
    ],
  },
  {
    channelId: "ch-cartoons",
    name: "Cartoon Classics",
    number: 1,
    status: "live",
    pendingCount: 0,
    airings: [
      {
        description:
          "Pip nurses an injured gull and learns how much responsibility comes with caring for it.",
        genres: ["Animation", "Comedy"],
        kind: "program",
        rating: "TV-PG",
        scheduleBlockId: "block_cartoons_borrowed_teapot",
        title: "The Borrowed Teapot",
        series: "Kettle Kids",
        season: 10,
        episode: 3,
        startMs: guideAt(0),
        stopMs: guideAt(22),
        thumbUrl: "/v1/images/storybook-cartoons",
        year: 1998,
      },
      {
        kind: "filler",
        scheduleBlockId: "block_cartoons_break_1",
        title: "Commercials",
        startMs: guideAt(22),
        stopMs: guideAt(26),
      },
      {
        kind: "program",
        scheduleBlockId: "block_cartoons_runaway_kite",
        title: "The Runaway Kite",
        series: "Kettle Kids",
        season: 10,
        episode: 7,
        startMs: guideAt(26),
        stopMs: guideAt(48),
      },
      {
        kind: "flex",
        scheduleBlockId: "block_cartoons_flex_1",
        title: "",
        startMs: guideAt(48),
        stopMs: guideAt(60),
      },
      {
        kind: "program",
        scheduleBlockId: "block_cartoons_staircase",
        title: "The Upside-Down Staircase, Part Two",
        series: "Kettle Kids",
        season: 10,
        episode: 8,
        startMs: guideAt(60),
        stopMs: guideAt(82),
      },
    ],
  },
  {
    channelId: "ch-scifi",
    name: "Sci-Fi Classics",
    number: 2,
    status: "live",
    // This channel has a pending acquisition on its timeline, so it reads "Filling in" — on
    // air, but not yet what was asked for. Keeps the chip visible in the default story rather
    // than only in a contrived one.
    pendingCount: 1,
    airings: [
      {
        kind: "program",
        scheduleBlockId: "block_scifi_dead_channel",
        title: "Voices on a Dead Channel",
        series: "Cold Relay",
        season: 3,
        episode: 26,
        startMs: guideAt(0),
        stopMs: guideAt(45),
      },
      // Still acquiring: drawn at a nominal width and cued as an estimate, never as a
      // promise that something airs then.
      {
        kind: "pending",
        scheduleBlockId: "block_scifi_hollow_moon",
        title: "Beacon of the Hollow Moon",
        nominal: true,
        startMs: guideAt(45),
        stopMs: guideAt(75),
        // The line that turns a pending slot from a mystery into a status.
        provenance: "acquiring · 62% · 8m left",
      },
      {
        kind: "program",
        scheduleBlockId: "block_scifi_borrowed_antenna",
        title: "The Borrowed Antenna",
        series: "Cold Relay",
        season: 4,
        episode: 2,
        startMs: guideAt(75),
        stopMs: guideAt(120),
      },
    ],
  },
];

export { guideChannels, guideFrom, guideNow, guidePod, guideTo };
