// PROTOTYPE — throwaway (#2151 step 2). Stub data for /prototype/library, sized to the reference
// household measured in #2151 (33,135 videos, 2,812 titles). Artwork is a hue-lit stand-in
// (`hue`), never a remote image. Every variant reads the same data; nothing here is fetched.
import type { ScanState } from "./prototype-state";

// ── Scan status: what GET /v1/library/status would return (#2151 decision 35) ────────────────
type StageKey = "walk" | "identify" | "headers" | "metadata";
interface Stage {
  key: StageKey;
  label: string; // "Finding files"
  state: "done" | "running" | "waiting";
}
interface Root {
  path: string; // Loomarr's path
  foundIn: string[]; // where discovery saw it: Emby, Sonarr, Radarr
  readable: boolean;
  confirmed: boolean;
  videos: number;
  walked: string; // "12m ago"
}
interface LiveSignal {
  label: string; // "Change notify", "Sonarr webhook"
  ok: boolean;
  detail: string; // "last event 2h ago"
}
interface ScanStatus {
  headline: string; // one line for the health strip
  tone: "ok" | "busy" | "warn" | "bad";
  stages: Stage[];
  progress?: { done: number; total: number; eta: string; rate: string };
  paused?: string; // "while CH 4 Saturday Morning plays"
  lastWalk: string;
  lastChange: string;
  unmatched: number;
  totalFiles: number;
  totalTitles: number;
  live: LiveSignal[];
  roots: Root[];
  held?: { root: string; wouldRemove: number; of: number; since: string };
}

const ROOTS: Root[] = [
  {
    path: "/media/tv",
    foundIn: ["Emby", "Sonarr"],
    readable: true,
    confirmed: true,
    videos: 30729,
    walked: "12m ago",
  },
  {
    path: "/media/movies",
    foundIn: ["Emby", "Radarr"],
    readable: true,
    confirmed: true,
    videos: 2406,
    walked: "12m ago",
  },
];

const LIVE_OK: LiveSignal[] = [
  { label: "Change notify", ok: true, detail: "2 shares watched · last event 3m ago" },
  { label: "Sonarr webhook", ok: true, detail: "registered · last event 2h ago" },
  { label: "Radarr webhook", ok: true, detail: "registered · last event 1d ago" },
];

const STAGES = (running: StageKey | null, doneUpTo: number): Stage[] =>
  (
    [
      ["walk", "Finding files"],
      ["identify", "Identifying"],
      ["headers", "Reading file headers"],
      ["metadata", "Metadata"],
    ] as const
  ).map(([key, label], i) => ({
    key,
    label,
    state: key === running ? "running" : i < doneUpTo ? "done" : "waiting",
  }));

const scanStatus = (scan: ScanState): ScanStatus => {
  const base = {
    lastWalk: "12m ago",
    lastChange: "3m ago",
    unmatched: 3,
    totalFiles: 33135,
    totalTitles: 2812,
    live: LIVE_OK,
    roots: ROOTS,
  };
  switch (scan) {
    case "idle":
      return { ...base, headline: "Up to date", tone: "ok", stages: STAGES(null, 4) };
    case "importing":
      return {
        ...base,
        headline: "Importing your library",
        tone: "busy",
        stages: STAGES("headers", 2),
        progress: { done: 12400, total: 33135, eta: "~8 min", rate: "41 files/s" },
        lastChange: "just now",
      };
    case "paused":
      return {
        ...base,
        headline: "Import paused",
        tone: "busy",
        stages: STAGES("headers", 2),
        progress: { done: 12400, total: 33135, eta: "resumes when the channel stops", rate: "0 files/s" },
        paused: "while CH 4 Saturday Morning plays",
      };
    case "offline-root":
      return {
        ...base,
        headline: "/media/movies is offline",
        tone: "bad",
        stages: STAGES(null, 4),
        roots: [ROOTS[0] as Root, { ...(ROOTS[1] as Root), readable: false, walked: "failed 2m ago" }],
        live: [
          { label: "Change notify", ok: false, detail: "movies share unreachable" },
          ...LIVE_OK.slice(1),
        ],
      };
    case "held-removal":
      return {
        ...base,
        headline: "2,406 movies look removed",
        tone: "warn",
        stages: STAGES(null, 4),
        held: { root: "/media/movies", wouldRemove: 2406, of: 2406, since: "4m ago" },
      };
    case "fresh":
      return {
        ...base,
        headline: "Confirm where your media is",
        tone: "warn",
        stages: STAGES(null, 0),
        unmatched: 0,
        totalFiles: 0,
        totalTitles: 0,
        lastWalk: "never",
        lastChange: "never",
        roots: ROOTS.map((r) => ({ ...r, confirmed: false, walked: "not yet" })),
      };
  }
};

// ── Items (#2151 decision 24) ─────────────────────────────────────────────────────────────────
type IdentitySource = "Sonarr" | "Radarr" | "NFO" | "Linked" | "Embedded id";
type FactsState = "waiting" | "reading" | "read" | "measured" | "failed";
interface ItemRow {
  id: string;
  title: string;
  year: number;
  kind: "Movie" | "Series";
  identity: IdentitySource;
  certification: string;
  facts: FactsState;
  factsDetail?: string; // series: "154/154 read"
  problems?: string[]; // "1 probe failed", "stray copy 8.1 GB"
  hue: number; // artwork stand-in
  offline?: boolean; // its root is unreadable
}

const ITEMS: ItemRow[] = [
  {
    id: "west-wing",
    title: "The West Wing",
    year: 1999,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-14",
    facts: "read",
    factsDetail: "154/154 read",
    hue: 214,
  },
  {
    id: "mad-max",
    title: "Mad Max: Fury Road",
    year: 2015,
    kind: "Movie",
    identity: "Radarr",
    certification: "R",
    facts: "measured",
    hue: 28,
  },
  {
    id: "fargo",
    title: "Fargo",
    year: 1996,
    kind: "Movie",
    identity: "Radarr",
    certification: "R",
    facts: "read",
    problems: ["stray copy · 8.1 GB"],
    hue: 196,
  },
  {
    id: "lost",
    title: "Lost",
    year: 2004,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-14",
    facts: "failed",
    factsDetail: "120/121 read",
    problems: ["1 file unreadable"],
    hue: 150,
  },
  {
    id: "bluey",
    title: "Bluey",
    year: 2018,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-Y",
    facts: "read",
    factsDetail: "151/151 read",
    hue: 200,
  },
  {
    id: "simpsons",
    title: "The Simpsons",
    year: 1989,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-PG",
    facts: "read",
    factsDetail: "768/768 read",
    hue: 52,
  },
  {
    id: "exorcist",
    title: "The Exorcist",
    year: 1973,
    kind: "Movie",
    identity: "Radarr",
    certification: "R",
    facts: "measured",
    hue: 4,
  },
  {
    id: "reality-bites",
    title: "Reality Bites",
    year: 1994,
    kind: "Movie",
    identity: "Linked",
    certification: "PG-13",
    facts: "read",
    hue: 330,
  },
  {
    id: "frenzy",
    title: "Frenzy",
    year: 1972,
    kind: "Movie",
    identity: "Embedded id",
    certification: "R",
    facts: "read",
    problems: ["stray copy · 31 GB"],
    hue: 12,
  },
  {
    id: "aaotd",
    title: "Are You Afraid of the Dark?",
    year: 1990,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-Y7",
    facts: "read",
    factsDetail: "91/91 read",
    hue: 268,
  },
  {
    id: "doraemon",
    title: "Doraemon",
    year: 2005,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-Y7",
    facts: "read",
    factsDetail: "939/939 read",
    hue: 192,
  },
  {
    id: "homicide",
    title: "Homicide: Life on the Street",
    year: 1993,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-14",
    facts: "read",
    factsDetail: "122/122 read",
    hue: 220,
  },
  {
    id: "elmer-gantry",
    title: "Elmer Gantry",
    year: 1960,
    kind: "Movie",
    identity: "Radarr",
    certification: "NR",
    facts: "read",
    hue: 40,
  },
  {
    id: "killing-bookie",
    title: "The Killing of a Chinese Bookie",
    year: 1976,
    kind: "Movie",
    identity: "Radarr",
    certification: "R",
    facts: "read",
    hue: 350,
  },
  {
    id: "magicians",
    title: "The Magicians",
    year: 2015,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-MA",
    facts: "read",
    factsDetail: "65/65 read",
    hue: 280,
  },
  {
    id: "mr-robot",
    title: "Mr. Robot",
    year: 2015,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-MA",
    facts: "read",
    factsDetail: "45/45 read",
    hue: 0,
  },
  {
    id: "walking-dead",
    title: "The Walking Dead",
    year: 2010,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-MA",
    facts: "read",
    factsDetail: "177/177 read",
    hue: 90,
  },
  {
    id: "train",
    title: "The Train",
    year: 1964,
    kind: "Movie",
    identity: "Radarr",
    certification: "NR",
    facts: "read",
    hue: 30,
  },
  {
    id: "footloose",
    title: "Footloose",
    year: 1984,
    kind: "Movie",
    identity: "Radarr",
    certification: "PG",
    facts: "read",
    hue: 60,
  },
  {
    id: "ducktales",
    title: "DuckTales",
    year: 1987,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-Y",
    facts: "read",
    factsDetail: "100/100 read",
    hue: 210,
  },
  {
    id: "far-heaven",
    title: "Far from Heaven",
    year: 2002,
    kind: "Movie",
    identity: "Radarr",
    certification: "PG-13",
    facts: "read",
    hue: 170,
  },
  {
    id: "preacher",
    title: "Preacher",
    year: 2016,
    kind: "Series",
    identity: "Sonarr",
    certification: "TV-MA",
    facts: "read",
    factsDetail: "43/43 read",
    hue: 18,
  },
];

// While importing, facts are still arriving: what the per-item state looks like mid-import.
const itemsFor = (scan: ScanState): ItemRow[] => {
  if (scan === "fresh") return [];
  if (scan === "importing" || scan === "paused") {
    return ITEMS.map((it, i) =>
      i % 3 === 0
        ? { ...it, facts: "waiting", factsDetail: it.kind === "Series" ? "0/154 read" : undefined }
        : i % 3 === 1
          ? {
              ...it,
              facts: scan === "paused" ? "waiting" : "reading",
              factsDetail: it.kind === "Series" ? "37/91 read" : undefined,
            }
          : it,
    );
  }
  if (scan === "offline-root")
    return ITEMS.map((it) => (it.kind === "Movie" ? { ...it, offline: true } : it));
  return ITEMS;
};

// ── Item view (#2151 decision 25) ─────────────────────────────────────────────────────────────
type Provider = "TMDB" | "Sonarr" | "Radarr" | "NFO" | "Local" | "Header" | "Measured";
interface FieldValue {
  label: string;
  value: string;
  source: Provider;
  // Every provider's value + why the winner won — the provenance popover.
  candidates?: { source: Provider; value: string }[];
  rule?: string; // "Certification: strictest grounded value wins"
}
interface Episode {
  code: string; // "S04E10"
  title: string;
  runtime: string;
  rating: string;
  ratingInherited: boolean;
  facts: FactsState;
  special?: string; // "Special · airs before S05"
}
interface SourceFile {
  path: string;
  size: string;
  container: string;
  video: string;
  audio: string[];
  subtitles: string[];
  chapters: number;
  duration: string;
  revision: string;
  facts: FactsState;
  current: boolean; // the arr's current file — plays
  stray?: boolean; // an alternate that never plays
  edition?: string;
}
interface ItemDetail {
  id: string;
  title: string;
  year: number;
  kind: "Movie" | "Series";
  status?: string;
  hue: number;
  identity: IdentitySource;
  ids: { namespace: string; value: string }[];
  fields: FieldValue[];
  seasons?: { number: number; label: string; episodes: number; read: number }[];
  openSeason?: Episode[]; // the expanded season's episodes
  files?: SourceFile[];
  usedBy: { channel: string; how: string }[];
  artwork: { role: string; source: Provider }[];
}

const WEST_WING: ItemDetail = {
  id: "west-wing",
  title: "The West Wing",
  year: 1999,
  kind: "Series",
  status: "Ended",
  hue: 214,
  identity: "Sonarr",
  ids: [
    { namespace: "tvdb", value: "72521" },
    { namespace: "tmdb", value: "688" },
    { namespace: "imdb", value: "tt0200276" },
  ],
  fields: [
    {
      label: "Overview",
      value:
        "Insider look at the lives of the dedicated men and women who serve in the West Wing of the White House…",
      source: "TMDB",
      candidates: [
        {
          source: "TMDB",
          value: "Insider look at the lives of the dedicated men and women who serve in the West Wing…",
        },
        {
          source: "Sonarr",
          value: "This Emmy Award-winning drama series reveals the inner workings of the White House…",
        },
      ],
      rule: "Series text: TMDB wins, then Sonarr, then NFO",
    },
    { label: "Genres", value: "Drama, Political", source: "TMDB" },
    {
      label: "Certification",
      value: "TV-14",
      source: "TMDB",
      candidates: [
        { source: "TMDB", value: "TV-14" },
        { source: "Sonarr", value: "TV-PG" },
      ],
      rule: "Certification: the strictest grounded value wins",
    },
    { label: "Status", value: "Ended", source: "TMDB" },
    { label: "Network", value: "NBC", source: "TMDB" },
    { label: "Episode text", value: "From Sonarr (TVDB numbering)", source: "Sonarr" },
  ],
  seasons: [1, 2, 3, 4, 5, 6, 7].map((n) => ({ number: n, label: `Season ${n}`, episodes: 22, read: 22 })),
  openSeason: [
    {
      code: "S04E01",
      title: "20 Hours in America (1)",
      runtime: "44:02",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E02",
      title: "20 Hours in America (2)",
      runtime: "43:51",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E03",
      title: "College Kids",
      runtime: "43:40",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E04",
      title: "The Red Mass",
      runtime: "43:12",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E05",
      title: "Debate Camp",
      runtime: "43:33",
      rating: "TV-14",
      ratingInherited: true,
      facts: "measured",
    },
    {
      code: "S04E06",
      title: "Game On",
      runtime: "43:58",
      rating: "TV-14",
      ratingInherited: true,
      facts: "measured",
    },
    {
      code: "S04E07",
      title: "Election Night",
      runtime: "43:21",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E08",
      title: "Process Stories",
      runtime: "43:45",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E09",
      title: "Swiss Diplomacy",
      runtime: "43:10",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S04E10",
      title: "Arctic Radar",
      runtime: "43:12",
      rating: "TV-14",
      ratingInherited: true,
      facts: "read",
    },
    {
      code: "S00E03",
      title: "Documentary Special",
      runtime: "44:30",
      rating: "TV-PG",
      ratingInherited: false,
      facts: "read",
      special: "Special · airs before S03E01",
    },
  ],
  files: [
    {
      path: "/media/tv/The West Wing/Season 04/The West Wing (1999) - S04E10 - Arctic Radar [Bluray-1080p Remux][DTS-HD MA 2.0][h264]-BTN.mkv",
      size: "7.9 GB",
      container: "Matroska",
      video: "H.264 · 1920×1080 · 23.976 fps · SDR",
      audio: ["DTS-HD MA 2.0 · English"],
      subtitles: ["English", "English (SDH)"],
      chapters: 6,
      duration: "43:12.4",
      revision: "7.9 GB · modified 2024-04-22",
      facts: "read",
      current: true,
    },
  ],
  usedBy: [
    { channel: "CH 7 · Prestige Drama", how: "in the lineup · aired 3× this week" },
    { channel: "CH 12 · Sunday Politics", how: "in the lineup" },
  ],
  artwork: [
    { role: "Poster", source: "Local" },
    { role: "Backdrop", source: "Local" },
    { role: "Logo", source: "Local" },
    { role: "Episode stills", source: "Local" },
  ],
};

const FARGO: ItemDetail = {
  id: "fargo",
  title: "Fargo",
  year: 1996,
  kind: "Movie",
  hue: 196,
  identity: "Radarr",
  ids: [
    { namespace: "tmdb", value: "275" },
    { namespace: "imdb", value: "tt0116282" },
  ],
  fields: [
    {
      label: "Overview",
      value: "Jerry, a small-town Minnesota car salesman, is bursting at the seams with debt…",
      source: "TMDB",
    },
    { label: "Genres", value: "Crime, Thriller", source: "TMDB" },
    { label: "Certification", value: "R", source: "TMDB" },
    { label: "Collection", value: "—", source: "TMDB" },
    {
      label: "Runtime",
      value: "1:38:01",
      source: "Header",
      candidates: [
        { source: "Header", value: "1:38:01" },
        { source: "Radarr", value: "1:38" },
        { source: "TMDB", value: "98 min" },
      ],
      rule: "Technical facts: measured, then header, then the arr summary",
    },
  ],
  files: [
    {
      path: "/media/movies/Fargo (1996)/Fargo (1996) [imdbid-tt0116282] - [Remux-2160p][DV HDR10][DTS-HD MA 5.1][HEVC]-CiNEPHiLES.mkv",
      size: "61.2 GB",
      container: "Matroska",
      video: "HEVC · 3840×2160 · 23.976 fps · Dolby Vision + HDR10",
      audio: ["DTS-HD MA 5.1 · English", "AC-3 2.0 · English (commentary)"],
      subtitles: ["English", "Spanish"],
      chapters: 16,
      duration: "1:38:01.2",
      revision: "61.2 GB · modified 2025-02-11",
      facts: "read",
      current: true,
    },
    {
      path: "/media/movies/Fargo (1996)/Fargo (1996) - X265 AC3 5.1.mkv",
      size: "8.1 GB",
      container: "Matroska",
      video: "HEVC · 1920×1040 · 23.976 fps · SDR",
      audio: ["AC-3 5.1 · English"],
      subtitles: [],
      chapters: 0,
      duration: "1:38:00.9",
      revision: "8.1 GB · modified 2019-06-03",
      facts: "read",
      current: false,
      stray: true,
    },
  ],
  usedBy: [{ channel: "CH 21 · Coen Brothers", how: "in the lineup · aired 1× this week" }],
  artwork: [
    { role: "Poster", source: "Local" },
    { role: "Backdrop", source: "Local" },
    { role: "Logo", source: "TMDB" },
  ],
};

// ── Unmatched (#2151 decision 27) ─────────────────────────────────────────────────────────────
interface Candidate {
  title: string;
  year: number;
  kind: "Movie" | "Series";
  tmdbId: number;
  overview: string;
  hue: number;
  agreement: { title: boolean; year: boolean; runtime: boolean };
}
interface UnmatchedGroup {
  id: string;
  folder: string;
  files: string[]; // file names
  parsed: string; // "Reality Bites · 2004 (year twice in name)"
  best?: Candidate;
  others: Candidate[];
  found: string; // "found 2d ago"
}

const UNMATCHED: UnmatchedGroup[] = [
  {
    id: "home-movies",
    folder: "/media/tv/Home Movies/Season 01",
    files: Array.from({ length: 13 }, (_, i) => `Home Movies - 1x${String(i + 1).padStart(2, "0")}.avi`),
    parsed: "Home Movies · S01E01–E13",
    best: {
      title: "Home Movies",
      year: 1999,
      kind: "Series",
      tmdbId: 2363,
      overview: "Eight-year-old Brendon Small makes home movies with his friends Melissa and Jason…",
      hue: 120,
      agreement: { title: true, year: false, runtime: true },
    },
    others: [
      {
        title: "Home Movies",
        year: 1979,
        kind: "Movie",
        tmdbId: 37514,
        overview: "A film-making class…",
        hue: 40,
        agreement: { title: true, year: false, runtime: false },
      },
    ],
    found: "found 2d ago",
  },
  {
    id: "reality-bites-copy",
    folder: "/media/movies/Reality Bites (1994)",
    files: ["Reality Bites (2004) (2004) 1080p DTS 6ch.mkv"],
    parsed: "Reality Bites · 2004",
    best: {
      title: "Reality Bites",
      year: 1994,
      kind: "Movie",
      tmdbId: 9489,
      overview: "A small circle of friends suddenly come face to face with the harsh realities of the 1990s…",
      hue: 330,
      agreement: { title: true, year: false, runtime: true },
    },
    others: [],
    found: "found 5h ago",
  },
  {
    id: "unknown",
    folder: "/media/movies/incoming",
    files: ["VTS_01_1.mkv"],
    parsed: "nothing recognisable",
    others: [],
    found: "found 1h ago",
  },
];

// ── Collections (#2151 decision 28) ───────────────────────────────────────────────────────────
interface CollectionRow {
  id: string;
  kind: "TMDB" | "Mine";
  name: string;
  owned: number;
  total?: number; // TMDB: how many exist
  fromEmby?: boolean;
  usedBy?: string[];
  hue: number;
}
const COLLECTIONS: CollectionRow[] = [
  { id: "alien", kind: "TMDB", name: "Alien Collection", owned: 4, total: 6, hue: 150 },
  { id: "bttf", kind: "TMDB", name: "Back to the Future Collection", owned: 3, total: 3, hue: 24 },
  { id: "indy", kind: "TMDB", name: "Indiana Jones Collection", owned: 4, total: 5, hue: 36 },
  { id: "godfather", kind: "TMDB", name: "The Godfather Collection", owned: 3, total: 3, hue: 10 },
  {
    id: "xmas",
    kind: "Mine",
    name: "Christmas Movies",
    owned: 18,
    fromEmby: true,
    usedBy: ["CH 12 · Holiday"],
    hue: 0,
  },
  {
    id: "saturday",
    kind: "Mine",
    name: "Saturday Morning Cartoons",
    owned: 9,
    usedBy: ["CH 4 · Saturday Morning"],
    hue: 200,
  },
  { id: "hitchcock", kind: "Mine", name: "Hitchcock, in order", owned: 7, hue: 260 },
];
const COLLECTION_EDIT = {
  id: "hitchcock",
  name: "Hitchcock, in order",
  description: "The run I like to watch in October.",
  members: [
    { title: "Rebecca", year: 1940, hue: 220 },
    { title: "Notorious", year: 1946, hue: 30 },
    { title: "Rear Window", year: 1954, hue: 12 },
    { title: "Vertigo", year: 1958, hue: 290 },
    { title: "North by Northwest", year: 1959, hue: 180 },
    { title: "Psycho", year: 1960, hue: 0 },
    { title: "Frenzy", year: 1972, hue: 12 },
  ],
  usedBy: [] as string[],
};

// ── Settings → Library extras (#2151 decision 29) ─────────────────────────────────────────────
const IDENTITY_SOURCES = [
  { name: "Sonarr", useAsLibrarySource: true, webhook: "registered · last event 2h ago", ok: true },
  { name: "Radarr", useAsLibrarySource: true, webhook: "registered · last event 1d ago", ok: true },
];
const METADATA = { language: "English (US)", country: "United States", languageFrom: "taken from Emby" };
const PATH_MAP = [
  { authority: "Emby", from: "/mnt/media/tv", to: "/media/tv" },
  { authority: "Emby", from: "/mnt/media/movies", to: "/media/movies" },
  { authority: "Sonarr", from: "/tv", to: "/media/tv" },
  { authority: "Radarr", from: "/movies", to: "/media/movies" },
];

export type {
  Candidate,
  CollectionRow,
  Episode,
  FactsState,
  FieldValue,
  IdentitySource,
  ItemDetail,
  ItemRow,
  LiveSignal,
  Provider,
  Root,
  ScanStatus,
  SourceFile,
  Stage,
  UnmatchedGroup,
};
export {
  COLLECTION_EDIT,
  COLLECTIONS,
  FARGO,
  IDENTITY_SOURCES,
  itemsFor,
  METADATA,
  PATH_MAP,
  scanStatus,
  UNMATCHED,
  WEST_WING,
};
