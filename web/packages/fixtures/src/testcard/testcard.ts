import type { PoolDTO } from "@loomarr/api/models/poolDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import type { SearchResult } from "@loomarr/core/contracts";

// The "test card" — deterministic demo data shared by Storybook stories and tests, on
// both web and the future mobile app (§4.2, §5.2). Typed against the orval-generated
// DTOs (ClipDTO, Proposal) so the fixtures track the real contract 1:1 (§12). No
// `Date.now` / random anywhere, so the visual suite's frozen clock stays honest.
//
// The repo is public: every programme title comes from the invented demo catalogue
// (internal/demolibrary), and every brand, person and episode name is invented (#1592).
// internal/demolibrary/guard_test.go fails if a real one comes back.
//
// The clip, guide and filler-source fixtures live in sibling folders and are re-exported here.

export type { DrawerFixtureBlock, DrawerFixtureChannel } from "./channel-drawer";
export { drawerChannels } from "./channel-drawer";
export {
  aiTaggedClip,
  bumperClip,
  channelFits,
  discoveredClips,
  fallbackCardEntry,
  podClips,
  podEntries,
  splitProposal,
  taggedClip,
  thumbnailedClip,
  untaggedClip,
} from "./clips";
export {
  fillerSources,
  fillerSourcesEmptyProvider,
  fillerSourcesGrouped,
  fillerSourcesManyGrouped,
  fillerSourcesWithRemotes,
} from "./filler-sources";
export { guideChannels, guideFrom, guideNow, guidePod, guideTo } from "./guide-timeline";

const sampleIntent = "90s comedies and mysteries, easy-going, keep it PG-13";

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
  // proposedBy is the raw user id; the card renders only the server-resolved name (#1430).
  proposedBy: "usr_ada",
  proposedByName: "ada",
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
  { label: "90s comedies", value: sampleIntent },
  { label: "Cozy mysteries", value: "cozy sunday-afternoon mysteries" },
  { label: "Saturday cartoons", value: "saturday-morning cartoons for the kids" },
];

const proposal: Proposal = {
  intent: { description: sampleIntent },
  rationale: "An easy-going 90s block, front-loaded with the crowd-pleasers you already own.",
  lineup: [
    {
      name: "The Velvet Cartographer",
      year: 1995,
      mediaType: "movie",
      inLibrary: true,
      confidence: 0.92,
      rationale: "The library's best-loved mystery; anchors the block.",
    },
    {
      name: "Pelican Point Motel",
      year: 1993,
      mediaType: "movie",
      inLibrary: true,
      confidence: 0.84,
      rationale: "Warm, sun-bleached, quotable.",
    },
  ],
  acquisitions: [
    {
      name: "Pepperpot Summer",
      year: 1998,
      mediaType: "movie",
      inLibrary: false,
      confidence: 0.81,
      rationale: "Fills the late-block slot; not in the library yet.",
    },
  ],
  alternates: [
    { name: "The Seventh Satellite Choir", year: 1992, mediaType: "movie", inLibrary: false },
    { name: "Marigold Protocol", year: 1989, mediaType: "movie", inLibrary: false },
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
      qualifiers: [{ term: "easy-going", supportedItems: 2 }],
    },
    era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
  },
  trace: { version: 1, surfacedTotal: 0, recordedTotal: 0, truncated: false, candidates: [] },
};

const searchResults: SearchResult[] = [
  { id: "ch-42", scope: "channels", name: "90s Comedies", meta: "#42" },
  { id: "lib-velvet", scope: "library", name: "The Velvet Cartographer", meta: "1995", inLibrary: true },
  { id: "lib-pelican", scope: "library", name: "Pelican Point Motel", meta: "1993", inLibrary: true },
  { id: "tmdb-pepperpot", scope: "tmdb", name: "Pepperpot Summer", meta: "1998" },
  { id: "help-webhooks", scope: "help", name: "Configuring Sonarr/Radarr webhooks", meta: "Docs" },
];

export {
  emptyPool,
  healthyPool,
  intentTemplates,
  pendingPull,
  proposal,
  sampleIntent,
  searchResults,
  thinPool,
  unplaceablePool,
};
