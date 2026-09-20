import type { ChannelFitDTO } from "@loomarr/api/models/channelFitDTO";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { DiscoveredClip } from "@loomarr/api/models/discoveredClip";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { PodEntryDTO } from "@loomarr/api/models/podEntryDTO";
import type { PodPoolDTO } from "@loomarr/api/models/podPoolDTO";
import type { PoolDTO } from "@loomarr/api/models/poolDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import type { SplitReviewProposalDTO } from "@loomarr/api/models/splitReviewProposalDTO";
import type { SearchResult } from "@loomarr/core/contracts";

// The "test card" — deterministic demo data shared by Storybook stories and tests, on
// both web and the future mobile app (§4.2, §5.2). Typed against the orval-generated
// DTOs (ClipDTO, Proposal) so the fixtures track the real contract 1:1 (§12). No
// `Date.now` / random anywhere, so the visual suite's frozen clock stays honest.

const sampleIntent = "90s action movies, high energy, keep it PG-13";

// --- V35: catalog health, the Incoming queue, and filler pulls ---

// A catalog in good shape: everything tagged, every channel matching its own era exactly.
const healthyPool: PoolDTO = {
  clips: 412,
  breakBody: 380,
  eligible: 374,
  untagged: 0,
  channels: [
    {
      channelId: "ch-42",
      name: "Saturday Mornings",
      number: 42,
      level: "exact",
      total: 88,
      durationMs: 2_640_000,
      categories: 8,
      brands: 24,
    },
    {
      channelId: "ch-7",
      name: "Late Night Sci-Fi",
      number: 7,
      level: "exact",
      total: 61,
      durationMs: 1_830_000,
      categories: 6,
      brands: 19,
    },
  ],
};

// The interesting shape: one channel whose breaks fall through to the built-in card. The server
// sorts worst-first, so that channel leads — the strip reads `channels[0]` positionally.
const thinPool: PoolDTO = {
  clips: 120,
  breakBody: 90,
  eligible: 61,
  untagged: 14,
  channels: [
    {
      channelId: "ch-3",
      name: "Newsreel",
      number: 3,
      level: "bumper_card",
      total: 0,
      durationMs: 0,
      categories: 0,
      brands: 0,
    },
    {
      channelId: "ch-42",
      name: "Saturday Mornings",
      number: 42,
      level: "exact",
      total: 44,
      durationMs: 1_320_000,
      categories: 5,
      brands: 13,
    },
  ],
};

// ⚠ A catalog that reads as healthy by clip count and can fill nothing, because none of it is
// short enough for a break. The case the "fits a break" stat exists to make visible.
const unplaceablePool: PoolDTO = {
  clips: 500,
  breakBody: 500,
  eligible: 0,
  untagged: 500,
  channels: [
    {
      channelId: "ch-3",
      name: "Newsreel",
      number: 3,
      level: "bumper_card",
      total: 0,
      durationMs: 0,
      categories: 0,
      brands: 0,
    },
  ],
};

const emptyPool: PoolDTO = { clips: 0, breakBody: 0, eligible: 0, untagged: 0, channels: [] };

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
  candidateCount: 2,
  intent: {
    version: "filler-acquisition-intent/v2",
    count: 2,
    catalogReason: "Saturday Mornings falls back to bumpers, because nothing in the catalog matches its era.",
  },
  rejected: [],
  sources: [],
  createdAt: "2026-08-01T12:00:00Z",
  plan: [
    {
      sourceId: "classic",
      candidateId: "candidate_classic",
      tag: "archive",
      name: "Classic TV commercials",
      why: "A source you added and left switched on.",
      estimateClips: 40,
      dropped: false,
    },
    {
      sourceId: "psa",
      candidateId: "candidate_psa",
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
  hash: "hash-bumper-open",
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
    hash: "hash-sunnyd",
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
    hash: "hash-gushers",
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
    hash: "hash-bumper-close",
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
  hash: "hash-sunnyd-tagged",
  tunarrProgramId: "clip-sunnyd-tagged",
};

// A clip whose frame was extracted (V17b/V30) and adopted into the image service (§22).
//
// ⚠ **The `hash` used to carry an inline data URI, and that hack retired with V52 phase 8.** It
// existed because `clipThumbURL` passed `data:` values through unchanged, which was the only way a
// story could show a card WITH a frame while rendering offline against storybook-static. Artwork
// is an image record now, so the hash is an ordinary hash again and the assets come from
// `.storybook/story-assets/` via `staticDirs`.
//
// ⚠ Same-origin asset paths, never data URIs — a base64 data URI always contains a comma, which is
// `srcset`'s candidate separator, so it is unloadable there (#210). Each ladder rung is a
// different colour so a baseline shows which rung a given box actually chose.
const thumbnailedClip: ClipDTO = {
  ...taggedClip,
  name: "Frosted Flakes — They're Grrreat!",
  hash: "3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a",
  thumbImage: {
    hash: "3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a",
    role: "thumb",
    width: 500,
    height: 750,
    placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
    dominantHex: "#2b4a5e",
    animated: false,
    srcSetWebp: "/poster-154.webp 154w, /poster-342.webp 342w, /poster-500.webp 500w",
    // Empty: AVIF is job-produced, so freshly-adopted artwork has none — the ordinary state.
    srcSetAvif: "",
    src: "/poster-fallback.jpg",
  },
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
  hash: "hash-unlabeled",
  tunarrProgramId: "clip-unlabeled",
};

const aiTaggedClip: ClipDTO = {
  ...taggedClip,
  tagged: false,
  aiTagged: true,
  playCount: 0,
  playsCounted: true,
  hash: "hash-ai",
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
  hash: "hash-suggested-era",
  tunarrProgramId: "clip-suggested-era",
};

// A persisted split proposal (§10 V34) mid-review, covering every segment state the
// editor must render honestly: a clean classified segment, one with an unconfirmed era
// suggestion, a dHash duplicate flag, an unsplittable over-long span, and a transcript.
const splitProposal: SplitReviewProposalDTO = {
  id: "split-testcard",
  clipHash: "c5e2000000000000000000000000000000000000000000000000000000000080",
  createdAt: "2026-07-25T20:00:00Z",
  segments: [
    {
      index: 0,
      startMs: 0,
      endMs: 30500,
      name: "Sunny D — Dude!",
      nameOrigin: "model-proposed",
      nameEvidence: "SUNNY D — DUDE!",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      artwork: thumbnailedClip.thumbImage,
      transcript: "[00:01] Sunny D, dude!\n[00:12] Packed with sunshine.",
    },
    {
      index: 1,
      startMs: 30500,
      endMs: 61000,
      name: "Rotoscoped tech spot",
      nameOrigin: "model-proposed",
      nameEvidence: "THE FUTURE IS HERE",
      audience: "general",
      category: "tech",
      artwork: thumbnailedClip.thumbImage,
      // The classifier guessed 1985 from tone; the year is in no text signal, so the
      // validator refused to persist it (§10 era grounding). The operator confirms.
      suggestedEra: 1985,
    },
    {
      index: 2,
      startMs: 61000,
      endMs: 91500,
      name: "Gushers — Fruit by the Foot",
      nameOrigin: "source-authored",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      artwork: thumbnailedClip.thumbImage,
      // dHash match against an existing catalog row — a FLAG, never a silent drop.
      dupOf: "clip-gushers.mp4",
    },
    {
      index: 3,
      startMs: 91500,
      endMs: 240000,
      name: "Clip 4 from 80s TV commercials",
      nameOrigin: "fallback",
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
  scores: {
    version: 1,
    themeFit: 1,
    availabilityRatio: 0.67,
    eraBalance: null,
    theme: {
      status: "supported",
      basis: "qualifiers",
      assessedItems: 2,
      unknownItems: 0,
      qualifiers: [{ term: "action", supportedItems: 2 }],
    },
    era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
  },
  trace: { version: 1, surfacedTotal: 0, recordedTotal: 0, truncated: false, candidates: [] },
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
      {
        kind: "program",
        scheduleBlockId: "block_action_heat",
        title: "Heat",
        startMs: guideAt(-25),
        stopMs: guideAt(35),
        year: 1995,
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
        scheduleBlockId: "block_action_point_break",
        title: "Point Break",
        startMs: guideAt(39),
        stopMs: guideAt(111),
        year: 1991,
      },
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
        description:
          "Bart nurses an injured bird and learns how much responsibility comes with caring for it.",
        genres: ["Animation", "Comedy"],
        kind: "program",
        rating: "TV-PG",
        scheduleBlockId: "block_springfield_bart_mother",
        title: "Bart the Mother",
        series: "The Simpsons",
        season: 10,
        episode: 3,
        startMs: guideAt(0),
        stopMs: guideAt(22),
        thumbUrl: "/v1/images/storybook-springfield",
        year: 1998,
      },
      {
        kind: "filler",
        scheduleBlockId: "block_springfield_break_1",
        title: "Commercials",
        startMs: guideAt(22),
        stopMs: guideAt(26),
      },
      {
        kind: "program",
        scheduleBlockId: "block_springfield_lisa_a",
        title: "Lisa Gets an A",
        series: "The Simpsons",
        season: 10,
        episode: 7,
        startMs: guideAt(26),
        stopMs: guideAt(48),
      },
      {
        kind: "flex",
        scheduleBlockId: "block_springfield_flex_1",
        title: "",
        startMs: guideAt(48),
        stopMs: guideAt(60),
      },
      {
        kind: "program",
        scheduleBlockId: "block_springfield_kidney",
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
        scheduleBlockId: "block_scifi_best_worlds",
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
        scheduleBlockId: "block_scifi_first_contact",
        title: "Star Trek: First Contact",
        nominal: true,
        startMs: guideAt(45),
        stopMs: guideAt(75),
        // The line that turns a pending slot from a mystery into a status.
        provenance: "acquiring · 62% · 8m left",
      },
      {
        kind: "program",
        scheduleBlockId: "block_scifi_family",
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

// --- V35: the Sources tab ---

// The three derived rows the read-model always returns (§10). ⚠ Not a table: the server
// derives these from `filler.dir` plus the library connection, which is why `library` is not
// switchable — nothing scans a media-server library for clips, so a toggle would change
// nothing — and why an unconfigured row is still a row.
// ⚠ V37: ONE FLAT LIST. The `remote` CONTAINER row is gone — each registered collection is a
// peer row carrying its own kind, switch and fetch time. What survives the flattening is the
// property the old model got for free: `folder`/`library` are still rows even when unconfigured,
// because "you could set up a drop-folder but have not" is §10's answer to "why is my catalog
// empty?", and a list of things-that-exist cannot say it.
const readySource = {
  readiness: "ready" as const,
  ready: true,
  locationSource: "installation" as const,
  effectiveCountry: "US",
  actions: ["fetch", "disable"],
  effectiveEnabled: true,
  providerEnabled: true,
  incoming: 0,
};
const offSource = {
  readiness: "off" as const,
  ready: false,
  locationSource: "installation" as const,
  effectiveCountry: "US",
  actions: ["enable"],
  effectiveEnabled: false,
  providerEnabled: true,
  incoming: 0,
};

const fillerSources: FillerSourceDTO[] = [
  {
    ...readySource,
    id: "folder",
    enabled: true,
    switchable: true,
    removable: false,
    kind: "folder",
    target: "/data/filler",
    detail: "watched directly — new files appear on the next pass",
    count: 412,
    configured: true,
    fetchable: true,
    // Scanned, not searched: there is no upstream catalog to query for a local folder.
    searchable: false,
  },
  {
    ...readySource,
    id: "library",
    enabled: true,
    switchable: false,
    removable: false,
    kind: "library",
    target: "media server filler library",
    detail: "scanned by the media server",
    count: 6,
    configured: true,
    fetchable: true,
    searchable: false,
  },
];

// The flat list with two registered sources added as PEERS (V37) — one archive collection and
// one YouTube playlist, which is the shape the redesigned Sources tab draws.
//
// ⚠ Still a SEPARATE fixture from the two rows above, for the same reason as before: adding
// them to `fillerSources` would change what every existing FillerSources story renders, churning
// baselines for what is meant to be an additive change.
const fillerSourcesWithRemotes: FillerSourceDTO[] = [
  ...fillerSources,
  {
    ...readySource,
    id: "archive:classic_tv_commercials",
    enabled: true,
    switchable: true,
    removable: true,
    kind: "archive",
    target: "Classic TV Commercials",
    detail: "an archive.org collection — searchable here, downloaded when you queue or approve",
    count: 137,
    configured: true,
    fetchable: true,
    // Only archive can be searched in place — the per-row search expander's condition.
    searchable: true,
    lastCheckedAt: "2026-07-30T09:14:00Z",
    automaticDownloads: {
      mode: "defaults",
      everySeconds: 21600,
      maxPerCheck: 10,
      summary: "Uses your defaults: every 6 hours, up to 10 clips each check.",
    },
  },
  // ⚠ Switched off AND never fetched: the row must read as "stopped fetching", not "gone", and
  // render "never" rather than an epoch date nobody meant.
  //
  // ⚠ Also `searchable: false` while being a remote source — yt-dlp ENUMERATES a playlist, it
  // does not search YouTube, so a search box here would return nothing forever. That is the
  // distinction this fixture exists to keep visible.
  {
    ...offSource,
    id: "youtube:PLvintage",
    enabled: false,
    switchable: true,
    removable: true,
    kind: "youtube",
    target: "Vintage Ad Reels",
    detail: "a playlist you added — titles and descriptions are kept for tagging",
    count: 0,
    configured: true,
    fetchable: true,
    searchable: false,
    automaticDownloads: {
      mode: "never",
      everySeconds: 0,
      maxPerCheck: 10,
      summary: "Doesn’t download automatically. You can still look for clips yourself.",
    },
  },
];

// The PROVIDER ROLL-UP shape (§10 V51c) — what `GET /v1/filler/sources` has actually returned
// since PR #201, and what nothing rendered until V54 phase B.
//
// ⚠ **No fixture anywhere carried `group`/`parentId` before this one**, so every unit and visual
// test in the tree was green over the flat V37 list while the server had been sending a nested one
// for weeks. That is the same shape as the untyped story stubs (#281): a suite that agrees with
// itself about a shape the server stopped sending.
//
// Faithful to `fillersources.go` in the three ways that matter to the renderer:
//
//   1. **Pre-ordered, flat.** Each group node is immediately followed by its own children. There is
//      no `children: []` array — a twirl-down renders from exactly this by hiding rows whose parent
//      is collapsed, and the BE chose flat deliberately (orval handles recursive types badly).
//   2. **A group owns provider policy, not child policy.** It is switchable but cannot be removed,
//      fetched, or searched directly. Pausing it leaves each child's own choice intact.
//   3. **`configured` means "anything behind it"**, which is how an empty provider is told apart
//      from a broken one. The YouTube group here has children; `fillerSourcesEmptyProvider` below
//      is the zero-child case.
//
// ⚠ A separate fixture from `fillerSourcesWithRemotes` rather than a replacement, per this file's
// standing convention — folding the shape into the existing one would churn every FillerSources
// baseline for what the server intends as an ADDITIVE change.
const fillerSourcesGrouped: FillerSourceDTO[] = [
  ...fillerSources,
  // ── Archive.org: two collections, one of them switched off ──────────────────────────────────
  {
    ...readySource,
    id: "provider:archive",
    group: true,
    enabled: true,
    switchable: true,
    removable: false,
    kind: "archive",
    target: "Archive.org",
    detail: "collections you've added — searchable here, downloaded when you queue or approve",
    // ⚠ The group's own count and time are ROLL-UPS the server computes: the count sums its
    // children and `lastCheckedAt` is the MAX over them, which is why it equals the newer of the
    // two below rather than either one in particular.
    count: 137 + 42,
    configured: true,
    fetchable: false,
    searchable: false,
    lastCheckedAt: "2026-07-30T09:14:00Z",
  },
  {
    ...readySource,
    id: "archive:classic_tv_commercials",
    parentId: "provider:archive",
    enabled: true,
    switchable: true,
    removable: true,
    kind: "archive",
    target: "Classic TV Commercials",
    detail: "an archive.org collection — searchable here, downloaded when you queue or approve",
    count: 137,
    configured: true,
    fetchable: true,
    searchable: true,
    lastCheckedAt: "2026-07-30T09:14:00Z",
    automaticDownloads: {
      mode: "defaults",
      everySeconds: 21600,
      maxPerCheck: 10,
      summary: "Uses your defaults: every 6 hours, up to 10 clips each check.",
    },
  },
  // One child is off while provider policy remains on. Child readiness reports the mixed state.
  {
    ...offSource,
    id: "archive:vintage_psas",
    parentId: "provider:archive",
    enabled: false,
    switchable: true,
    removable: true,
    kind: "archive",
    target: "Vintage PSAs",
    detail: "an archive.org collection — searchable here, downloaded when you queue or approve",
    count: 42,
    configured: true,
    fetchable: true,
    searchable: true,
    lastCheckedAt: "2026-07-28T11:02:00Z",
    automaticDownloads: {
      mode: "custom",
      everySeconds: 86400,
      maxPerCheck: 4,
      summary: "Every day, up to 4 clips each check.",
    },
  },
  // ── YouTube: provider on, with its single playlist switched off ─────────────────────────────
  {
    ...readySource,
    id: "provider:youtube",
    group: true,
    enabled: true,
    switchable: true,
    removable: false,
    kind: "youtube",
    target: "YouTube",
    detail: "playlists you've added — titles and descriptions are kept for tagging",
    count: 0,
    configured: true,
    fetchable: false,
    searchable: false,
  },
  {
    ...offSource,
    id: "youtube:PLvintage",
    parentId: "provider:youtube",
    enabled: false,
    switchable: true,
    removable: true,
    kind: "youtube",
    target: "Vintage Ad Reels",
    detail: "a playlist you added — titles and descriptions are kept for tagging",
    count: 0,
    configured: true,
    fetchable: true,
    searchable: false,
    automaticDownloads: {
      mode: "never",
      everySeconds: 0,
      maxPerCheck: 10,
      summary: "Doesn’t download automatically. You can still look for clips yourself.",
    },
  },
];

// A deliberately crowded provider for the source-index scaling contract. Two attention rows are
// placed well below the first five in wire order so the story proves the UI promotes them instead
// of merely benefiting from favorable fixture ordering.
const manyArchiveSourceNames = [
  "Classic TV Commercials",
  "TV Ads",
  "Commercials From The Vault",
  "Natural History Favorites",
  "Vintage PSAs",
  "Station IDs",
  "Local Car Dealer Ads",
  "Saturday Morning Spots",
  "Retro Food Commercials",
  "Toy Commercials",
  "Movie Theater Bumpers",
  "Public Information Films",
  "Regional Station Breaks",
  "Holiday Commercials",
  "Household Product Ads",
  "Department Store Commercials",
  "Classic Radio Spots",
  "Network Promos",
  "Drive-In Intermissions",
  "Educational Shorts",
];

const fillerSourcesManyGrouped: FillerSourceDTO[] = [
  ...fillerSources,
  {
    ...readySource,
    id: "provider:archive",
    group: true,
    enabled: true,
    switchable: true,
    removable: false,
    kind: "archive",
    target: "Archive.org",
    detail: "collections you've added",
    count: 486,
    configured: true,
    fetchable: false,
    searchable: false,
  },
  ...manyArchiveSourceNames.map((target, index): FillerSourceDTO => {
    const attention = index === 8 || index === 16;
    return {
      ...(attention ? offSource : readySource),
      id: `archive:many-${index + 1}`,
      parentId: "provider:archive",
      enabled: !attention,
      switchable: true,
      removable: true,
      kind: "archive",
      target,
      detail: "an Archive.org collection",
      count: attention ? 0 : 18 + index,
      configured: true,
      fetchable: true,
      searchable: true,
    };
  }),
];

// A provider with NO children — the fresh-install state, and an INVITATION rather than a fault.
//
// ⚠ `configured: false` on a group means "nothing behind it yet", which §10 and
// `store/fillersources.go` both describe as an invitation to add one. The tab rendered it with the
// same red `not configured` caution Badge a broken drop-folder gets, which tells an operator
// something is wrong when nothing is.
const fillerSourcesEmptyProvider: FillerSourceDTO[] = [
  ...fillerSources,
  {
    ...readySource,
    id: "provider:archive",
    group: true,
    enabled: true,
    switchable: true,
    removable: false,
    kind: "archive",
    target: "Archive.org",
    detail: "collections you've added — searchable here, downloaded when you queue or approve",
    count: 0,
    configured: false,
    fetchable: false,
    searchable: false,
  },
];

// Per-clip channel fit (V35 item 1.7) — one row per channel, number-sorted as the server sends.
//
// ⚠ Deliberately covers all FIVE renderings the picker has to distinguish, because four of them
// look alike from a distance: a plain ladder match, an automatic non-match WITH a reason, a pin
// (no rung and no reason — a pin is placed ahead of the ladder), an exclusion, and the
// contradictory both-flags state the server reports as stored.
const channelFits: ChannelFitDTO[] = [
  { channelId: "ch-1", name: "Saturday Mornings", number: 1, level: "exact", pinned: false, excluded: false },
  {
    channelId: "ch-2",
    name: "Late Night Sci-Fi",
    number: 2,
    level: "widened",
    pinned: false,
    excluded: false,
  },
  // Automatic, and it would never be picked — the row the fit note exists for.
  {
    channelId: "ch-3",
    name: "Kids Block",
    number: 3,
    level: "bumper_card",
    reason: "audience",
    pinned: false,
    excluded: false,
  },
  // ⚠ Pinned: bumper_card with NO reason. Rendered as "always played here", never as
  // "won't be picked" — a pin has no rung because it bypasses the ladder entirely.
  { channelId: "ch-4", name: "Retro Movies", number: 4, level: "bumper_card", pinned: true, excluded: false },
  {
    channelId: "ch-5",
    name: "Newsreel",
    number: 5,
    level: "bumper_card",
    reason: "excluded",
    pinned: false,
    excluded: true,
  },
];

// archive.org search results, in the shapes the live API really returns.
//
// ⚠ `thumbnailUrl` is an INLINE data: URI, never the real https://archive.org/services/img/<id>
// these carry in production. A remote URL in a fixture bypasses the visual suite's stubbed
// fetch and races the snapshot, which is a flaky baseline rather than a network policy
// question — the same reason `thumbnailedClip` above is a 2×1 PNG.
//
// ⚠ The variety is deliberate and matches the pinned Go fixture: one row has everything, one
// has NO date, and one has neither duration nor height. Absence is the common case — archive
// has not probed every item — and a row that renders "0:00" for an unprobed clip claims it is
// empty. A uniform fixture would let that bug pass here and fail on the first real search.
const discoveredClips: DiscoveredClip[] = [
  {
    id: "kelloggs-bran-flakes",
    title: "Kellogg's Bran Flakes — 1989 television commercial",
    url: "https://archive.org/details/kelloggs-bran-flakes",
    thumbnailUrl:
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
    date: "1989-01-01T00:00:00Z",
    year: 1989,
    durationMs: 10_800,
    height: 720,
  },
  {
    id: "saturday-morning-cartoons-v-110",
    title: "Saturday Morning Cartoons Vol. 110 (full block)",
    url: "https://archive.org/details/saturday-morning-cartoons-v-110",
    thumbnailUrl:
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
    // No date: 2 of the 5 pinned docs declare none.
    durationMs: 11_032_100,
    height: 360,
  },
  {
    id: "youtube-IaEBrNaHfUs",
    title: "1980s cereal commercial compilation",
    url: "https://archive.org/details/youtube-IaEBrNaHfUs",
    thumbnailUrl:
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
    date: "2015-09-06T00:00:00Z",
    // ⚠ Neither duration nor height: archive never probed this item's files. The row must
    // render "—", never "0:00" or "0p".
  },
];

export {
  aiTaggedClip,
  bumperClip,
  channelFits,
  discoveredClips,
  emptyPool,
  fallbackCardEntry,
  fillerSources,
  fillerSourcesEmptyProvider,
  fillerSourcesGrouped,
  fillerSourcesManyGrouped,
  fillerSourcesWithRemotes,
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
  untaggedClip,
};
