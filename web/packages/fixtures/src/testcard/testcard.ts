import type { ClipDTO, PodEntryDTO, Proposal } from "@loomarr/api";
import type { SearchResult } from "@loomarr/core";

// The "test card" — deterministic demo data shared by Storybook stories and tests, on
// both web and the future mobile app (§4.2, §5.2). Typed against the orval-generated
// DTOs (ClipDTO, Proposal) so the fixtures track the real contract 1:1 (§12). No
// `Date.now` / random anywhere, so the visual suite's frozen clock stays honest.

const sampleIntent = "90s action movies, high energy, keep it PG-13";

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
    tunarrProgramId: "clip-bumper-open",
  },
  {
    name: "Sunny D — Dude!",
    kind: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    tunarrProgramId: "clip-sunnyd",
  },
  {
    name: "Gushers — Fruit by the Foot",
    kind: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    tunarrProgramId: "clip-gushers",
  },
  {
    name: "Back after these",
    kind: "bumper",
    durationMs: 5000,
    isFallbackCard: false,
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
  tunarrProgramId: "clip-sunnyd-tagged",
};

const untaggedClip: ClipDTO = {
  name: "Unlabeled 30s spot",
  kind: "commercial",
  durationMs: 30000,
  tagged: false,
  aiTagged: false,
  tunarrProgramId: "clip-unlabeled",
};

const aiTaggedClip: ClipDTO = { ...taggedClip, tagged: false, aiTagged: true, tunarrProgramId: "clip-ai" };

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

export {
  aiTaggedClip,
  bumperClip,
  fallbackCardEntry,
  intentTemplates,
  podClips,
  podEntries,
  proposal,
  sampleIntent,
  searchResults,
  taggedClip,
  untaggedClip,
};
