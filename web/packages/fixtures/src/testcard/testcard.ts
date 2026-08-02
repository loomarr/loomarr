import type {
  ClipDTO,
  GuideChannelTimeline,
  IncomingAskDTO,
  IncomingReelDTO,
  PodEntryDTO,
  PodPoolDTO,
  PoolDTO,
  Proposal,
  PullDTO,
  SplitProposal,
} from "@loomarr/api";
import type { SearchResult } from "@loomarr/core";

// The "test card" — deterministic demo data shared by Storybook stories and tests, on
// both web and the future mobile app (§4.2, §5.2). Typed against the orval-generated
// DTOs (ClipDTO, Proposal) so the fixtures track the real contract 1:1 (§12). No
// `Date.now` / random anywhere, so the visual suite's frozen clock stays honest.

const sampleIntent = "90s action movies, high energy, keep it PG-13";

// --- V35: catalog health, the Incoming queue, and filler pulls ---

// A catalog in good shape: everything tagged, every channel matching its own era exactly.
const healthyPool: PoolDTO = {
  clips: 412,
  commercials: 380,
  eligible: 374,
  untagged: 0,
  channels: [
    { channelId: "ch-42", name: "Saturday Mornings", number: 42, level: "exact", total: 88 },
    { channelId: "ch-7", name: "Late Night Sci-Fi", number: 7, level: "exact", total: 61 },
  ],
};

// The interesting shape: one channel whose breaks fall through to the built-in card. The server
// sorts worst-first, so that channel leads — the strip reads `channels[0]` positionally.
const thinPool: PoolDTO = {
  clips: 120,
  commercials: 90,
  eligible: 61,
  untagged: 14,
  channels: [
    { channelId: "ch-3", name: "Newsreel", number: 3, level: "bumper_card", total: 0 },
    { channelId: "ch-42", name: "Saturday Mornings", number: 42, level: "exact", total: 44 },
  ],
};

// ⚠ A catalog that reads as healthy by clip count and can fill nothing, because none of it is
// short enough for a break. The case the "fits a break" stat exists to make visible.
const unplaceablePool: PoolDTO = {
  clips: 500,
  commercials: 500,
  eligible: 0,
  untagged: 500,
  channels: [{ channelId: "ch-3", name: "Newsreel", number: 3, level: "bumper_card", total: 0 }],
};

const emptyPool: PoolDTO = { clips: 0, commercials: 0, eligible: 0, untagged: 0, channels: [] };

// An era the tagger proposed but could NOT ground in the clip's text — a decision with a
// proposed answer the operator confirms or rejects.
const guessedEraAsk: IncomingAskDTO = {
  path: "1988/toys.mp4",
  name: "Transformers holiday spot",
  from: "archive",
  durationMs: 30_000,
  kind: "commercial",
  audience: "kids",
  category: "toys",
  suggestedEra: 1988,
  reason: "The year isn't written anywhere in this clip's name or description, so Loomarr guessed it.",
};

// ⚠ A DIFFERENT question from the one above: nothing to confirm, so no proposed answer.
const untaggedAsk: IncomingAskDTO = {
  path: "mystery.mp4",
  name: "mystery.mp4",
  durationMs: 25_000,
  kind: "commercial",
  reason: "Loomarr couldn't work out what this is, so it will only match broadly.",
};

// A compilation mid-split, with the count of segments an operator cannot simply accept.
const compilationReel: IncomingReelDTO = {
  proposalId: "sp_1",
  clipPath: "comps/1987-saturday.mp4",
  segments: 12,
  needsAttention: 3,
  createdAt: "2026-08-01T12:00:00Z",
};

const cleanReel: IncomingReelDTO = {
  proposalId: "sp_2",
  clipPath: "comps/1993-toys.mp4",
  segments: 8,
  needsAttention: 0,
  createdAt: "2026-08-01T13:00:00Z",
};

// A proposed acquisition awaiting a human. ⚠ `estimateClips` is an ESTIMATE and the composer
// reports 0 where it has measured nothing — both cases matter to the card, so the fixture
// carries real numbers and a caller zeroes them for the unmeasured story.
const pendingPull: PullDTO = {
  id: "pull_1",
  title: "Top up the 1990s",
  reason: "Saturday Mornings falls back to bumpers, because nothing in the catalog matches its era.",
  proposedBy: "ada",
  status: "pending",
  estimateClips: 52,
  createdAt: "2026-08-01T12:00:00Z",
  plan: [
    {
      sourceId: "classic",
      tag: "archive",
      name: "Classic TV commercials",
      why: "A source you added and left switched on.",
      estimateClips: 40,
      dropped: false,
    },
    {
      sourceId: "psa",
      tag: "archive",
      name: "Public service announcements",
      why: "A source you added and left switched on.",
      estimateClips: 12,
      dropped: false,
    },
  ],
};

const intentTemplates = [
  { label: "90s action", value: sampleIntent },
  { label: "Cozy mysteries", value: "cozy sunday-afternoon mysteries" },
  { label: "Saturday cartoons", value: "saturday-morning cartoons for the kids" },
];

const bumperClip: ClipDTO = {
  name: "Channel bumper",
  kind: "bumper",
  durationMs: 5000,
  era: 1990,
  audience: "kids",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  path: "clip-bumper-open.mp4",
  tunarrProgramId: "clip-bumper-open",
};

// GET /v1/channels/{id}/pods returns PodEntryDTO, NOT ClipDTO: a pod entry is a placed
// clip (it can be the embedded fallback card, which has no Tunarr program id at all),
// while a ClipDTO is a catalog row carrying tags. Typed to the shape the endpoint really
// returns so the timeline tracks the contract 1:1 (§12).
const podEntries: PodEntryDTO[] = [
  {
    name: "Channel bumper",
    kind: "bumper",
    durationMs: 5000,
    isFallbackCard: false,
    path: "clip-bumper-open.mp4",
    tunarrProgramId: "clip-bumper-open",
  },
  {
    name: "Sunny D — Dude!",
    kind: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    path: "clip-sunnyd.mp4",
    tunarrProgramId: "clip-sunnyd",
  },
  {
    name: "Gushers — Fruit by the Foot",
    kind: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    path: "clip-gushers.mp4",
    tunarrProgramId: "clip-gushers",
  },
  {
    name: "Back after these",
    kind: "bumper",
    durationMs: 5000,
    isFallbackCard: false,
    path: "clip-bumper-close.mp4",
    tunarrProgramId: "clip-bumper-close",
  },
];

// The bottom of the §10 fallback ladder: nothing matched, so the embedded card stands in
// rather than dead air. It has no program id because it is not a Tunarr program.
const fallbackCardEntry: PodEntryDTO = {
  name: "Loomarr — We'll be right back",
  kind: "bumper",
  durationMs: 5000,
  isFallbackCard: true,
};

const podClips: ClipDTO[] = [
  bumperClip,
  {
    name: "Sunny D — Dude!",
    kind: "commercial",
    durationMs: 30000,
    era: 1990,
    audience: "kids",
    tagged: true,
    aiTagged: false,
    playCount: 0,
    playsCounted: true,
    path: "clip-sunnyd.mp4",
    tunarrProgramId: "clip-sunnyd",
  },
  {
    name: "Gushers — Fruit by the Foot",
    kind: "commercial",
    durationMs: 30000,
    era: 1990,
    audience: "kids",
    tagged: true,
    aiTagged: false,
    playCount: 0,
    playsCounted: true,
    path: "clip-gushers.mp4",
    tunarrProgramId: "clip-gushers",
  },
  {
    name: "Back after these",
    kind: "bumper",
    durationMs: 5000,
    era: 1990,
    audience: "kids",
    tagged: true,
    aiTagged: false,
    playCount: 0,
    playsCounted: true,
    path: "clip-bumper-close.mp4",
    tunarrProgramId: "clip-bumper-close",
  },
];

const taggedClip: ClipDTO = {
  name: "Sunny D — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food & drink",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  path: "clip-sunnyd-tagged.mp4",
  tunarrProgramId: "clip-sunnyd-tagged",
};

// A clip whose frame was extracted (V17b/V30). The `path` carries an inline data URI rather
// than a real clip path because `clipThumbURL` passes those through unchanged — stories render
// offline against storybook-static with no server behind them, so a `/v1/filler/thumb/…` src
// would be a broken image in the gallery and a flaky pixel in the visual suite.
//
// A 2×1 amber PNG: large enough that `object-cover` on a 16:9 box produces a solid fill, small
// enough to read as a fixture rather than as artwork.
const thumbnailedClip: ClipDTO = {
  ...taggedClip,
  name: "Frosted Flakes — They're Grrreat!",
  path: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
  thumbnail: "clip-frosted.jpg",
  tunarrProgramId: "clip-frosted",
};

const untaggedClip: ClipDTO = {
  name: "Unlabeled 30s spot",
  kind: "commercial",
  durationMs: 30000,
  tagged: false,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  path: "clip-unlabeled.mp4",
  tunarrProgramId: "clip-unlabeled",
};

const aiTaggedClip: ClipDTO = {
  ...taggedClip,
  tagged: false,
  aiTagged: true,
  playCount: 0,
  playsCounted: true,
  path: "clip-ai.mp4",
  tunarrProgramId: "clip-ai",
};

// A clip whose AI era guess was REFUSED by the grounding validator (§10, V34): the year
// appears nowhere in the clip's text signals, so it arrives as `suggestedEra` — a question
// for the operator, not a tag.
const suggestedEraClip: ClipDTO = {
  ...untaggedClip,
  name: "Rotoscoped spot, year unknown",
  audience: "general",
  category: "tech",
  suggestedEra: 1985,
  path: "clip-suggested-era.mp4",
  tunarrProgramId: "clip-suggested-era",
};

// A persisted split proposal (§10 V34) mid-review, covering every segment state the
// editor must render honestly: a clean classified segment, one with an unconfirmed era
// suggestion, a dHash duplicate flag, an unsplittable over-long span, and a transcript.
const splitProposal: SplitProposal = {
  id: "split-testcard",
  clipPath: "compilations/80s-tv-commercials.mp4",
  createdAt: "2026-07-25T20:00:00Z",
  segments: [
    {
      index: 0,
      startMs: 0,
      endMs: 30500,
      name: "Sunny D — Dude!",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      transcript: "[00:01] Sunny D, dude!\n[00:12] Packed with sunshine.",
    },
    {
      index: 1,
      startMs: 30500,
      endMs: 61000,
      name: "Rotoscoped tech spot",
      audience: "general",
      category: "tech",
      // The classifier guessed 1985 from tone; the year is in no text signal, so the
      // validator refused to persist it (§10 era grounding). The operator confirms.
      suggestedEra: 1985,
    },
    {
      index: 2,
      startMs: 61000,
      endMs: 91500,
      name: "Gushers — Fruit by the Foot",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      // dHash match against an existing catalog row — a FLAG, never a silent drop.
      dupOf: "clip-gushers.mp4",
    },
    {
      index: 3,
      startMs: 91500,
      endMs: 240000,
      name: "80s-tv-commercials part 4",
      // Over-long AND the rescue found nothing (no whisper, or no breaks in the text).
      // The operator cuts it by hand or drops it — Loomarr does not guess.
      unsplittable: true,
    },
  ],
};

const proposal: Proposal = {
  intent: { description: sampleIntent },
  rationale: "A high-energy 90s action block, front-loaded with the crowd-pleasers you already own.",
  lineup: [
    {
      name: "Heat",
      year: 1995,
      mediaType: "movie",
      inLibrary: true,
      confidence: 0.92,
      rationale: "Peak-era Michael Mann; anchors the block.",
    },
    {
      name: "Point Break",
      year: 1991,
      mediaType: "movie",
      inLibrary: true,
      confidence: 0.84,
      rationale: "Kinetic, sun-bleached, quotable.",
    },
  ],
  acquisitions: [
    {
      name: "Con Air",
      year: 1997,
      mediaType: "movie",
      inLibrary: false,
      confidence: 0.81,
      rationale: "Fills the late-block slot; not in the library yet.",
    },
  ],
  alternates: [
    { name: "Face/Off", year: 1997, mediaType: "movie", inLibrary: false },
    { name: "The Rock", year: 1996, mediaType: "movie", inLibrary: false },
  ],
  scores: { themeFit: 0.88, availabilityRatio: 0.67, eraBalance: 0.71, overall: 0.82 },
};

const searchResults: SearchResult[] = [
  { id: "ch-42", scope: "channels", name: "90s Action", meta: "#42" },
  { id: "lib-heat", scope: "library", name: "Heat", meta: "1995", inLibrary: true },
  { id: "lib-pb", scope: "library", name: "Point Break", meta: "1991", inLibrary: true },
  { id: "tmdb-conair", scope: "tmdb", name: "Con Air", meta: "1997" },
  { id: "help-webhooks", scope: "help", name: "Configuring Sonarr/Radarr webhooks", meta: "Docs" },
];

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
      kind: "bumper",
      durationMs: 5_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
    {
      name: "Sunny D — Dude!",
      kind: "commercial",
      durationMs: 30_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
    {
      name: "Gushers",
      kind: "commercial",
      durationMs: 30_000,
      era: 1993,
      quality: "360p",
      isFallbackCard: false,
    },
    {
      name: "Back after these",
      kind: "bumper",
      durationMs: 5_000,
      era: 1994,
      quality: "480p",
      isFallbackCard: false,
    },
  ],
};

// Three channels covering the four kinds a grid must distinguish (§12): a movie channel, an
// episodic channel whose blocks carry series + SxxExx, and one still acquiring. Deliberately
// includes a programme that STARTED BEFORE the window (Heat, at -25m) — the in-progress case
// that must clip to the axis while keeping its true end.
const guideChannels: GuideChannelTimeline[] = [
  {
    channelId: "ch-action",
    name: "1980s Action Heroes",
    number: 3,
    status: "live",
    pendingCount: 0,
    airings: [
      { kind: "program", title: "Heat", startMs: guideAt(-25), stopMs: guideAt(35), year: 1995 },
      {
        kind: "filler",
        title: "Commercials",
        startMs: guideAt(35),
        stopMs: guideAt(39),
        pod: guidePod,
      },
      { kind: "program", title: "Point Break", startMs: guideAt(39), stopMs: guideAt(111), year: 1991 },
    ],
  },
  {
    channelId: "ch-springfield",
    name: "Springfield Classics",
    number: 1,
    status: "live",
    pendingCount: 0,
    airings: [
      {
        kind: "program",
        title: "Bart the Mother",
        series: "The Simpsons",
        season: 10,
        episode: 3,
        startMs: guideAt(0),
        stopMs: guideAt(22),
      },
      { kind: "filler", title: "Commercials", startMs: guideAt(22), stopMs: guideAt(26) },
      {
        kind: "program",
        title: "Lisa Gets an A",
        series: "The Simpsons",
        season: 10,
        episode: 7,
        startMs: guideAt(26),
        stopMs: guideAt(48),
      },
      { kind: "flex", title: "", startMs: guideAt(48), stopMs: guideAt(60) },
      {
        kind: "program",
        title: "Homer Simpson in: Kidney Trouble",
        series: "The Simpsons",
        season: 10,
        episode: 8,
        startMs: guideAt(60),
        stopMs: guideAt(82),
      },
    ],
  },
  {
    channelId: "ch-scifi",
    name: "Star Trek Classics",
    number: 2,
    status: "live",
    // This channel has a pending acquisition on its timeline, so it reads "Filling in" — on
    // air, but not yet what was asked for. Keeps the chip visible in the default story rather
    // than only in a contrived one.
    pendingCount: 1,
    airings: [
      {
        kind: "program",
        title: "The Best of Both Worlds",
        series: "Star Trek: TNG",
        season: 3,
        episode: 26,
        startMs: guideAt(0),
        stopMs: guideAt(45),
      },
      // Still acquiring: drawn at a nominal width and cued as an estimate, never as a
      // promise that something airs then.
      {
        kind: "pending",
        title: "Star Trek: First Contact",
        nominal: true,
        startMs: guideAt(45),
        stopMs: guideAt(75),
        // The line that turns a pending slot from a mystery into a status.
        provenance: "acquiring · 62% · 8m left",
      },
      {
        kind: "program",
        title: "Family",
        series: "Star Trek: TNG",
        season: 4,
        episode: 2,
        startMs: guideAt(75),
        stopMs: guideAt(120),
      },
    ],
  },
];

export {
  aiTaggedClip,
  bumperClip,
  cleanReel,
  compilationReel,
  emptyPool,
  fallbackCardEntry,
  guessedEraAsk,
  guideChannels,
  guideFrom,
  guideNow,
  guidePod,
  guideTo,
  healthyPool,
  intentTemplates,
  pendingPull,
  podClips,
  podEntries,
  proposal,
  sampleIntent,
  searchResults,
  splitProposal,
  suggestedEraClip,
  taggedClip,
  thinPool,
  thumbnailedClip,
  unplaceablePool,
  untaggedAsk,
  untaggedClip,
};
