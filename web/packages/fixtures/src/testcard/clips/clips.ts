import type { ChannelFitDTO } from "@loomarr/api/models/channelFitDTO";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { DiscoveredClip } from "@loomarr/api/models/discoveredClip";
import type { PodEntryDTO } from "@loomarr/api/models/podEntryDTO";
import type { SplitReviewProposalDTO } from "@loomarr/api/models/splitReviewProposalDTO";

const bumperClip: ClipDTO = {
  name: "Channel bumper",
  role: "bumper",
  durationMs: 5000,
  era: 1990,
  audience: "kids",
  tagged: true,
  needsDetails: false,
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
    role: "bumper",
    durationMs: 5000,
    isFallbackCard: false,
    path: "clip-bumper-open.mp4",
    tunarrProgramId: "clip-bumper-open",
  },
  {
    name: "Fizzbolt — Dude!",
    role: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    path: "clip-fizzbolt.mp4",
    tunarrProgramId: "clip-fizzbolt",
  },
  {
    name: "Gloopers — Snack Attack!",
    role: "commercial",
    durationMs: 30000,
    isFallbackCard: false,
    path: "clip-gloopers.mp4",
    tunarrProgramId: "clip-gloopers",
  },
  {
    name: "Back after these",
    role: "bumper",
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
  role: "bumper",
  durationMs: 5000,
  isFallbackCard: true,
};

const podClips: ClipDTO[] = [
  bumperClip,
  {
    name: "Fizzbolt — Dude!",
    role: "commercial",
    durationMs: 30000,
    era: 1990,
    audience: "kids",
    tagged: true,
    needsDetails: false,
    playCount: 0,
    playsCounted: true,
    hash: "hash-fizzbolt",
    tunarrProgramId: "clip-fizzbolt",
  },
  {
    name: "Gloopers — Snack Attack!",
    role: "commercial",
    durationMs: 30000,
    era: 1990,
    audience: "kids",
    tagged: true,
    needsDetails: false,
    playCount: 0,
    playsCounted: true,
    hash: "hash-gloopers",
    tunarrProgramId: "clip-gloopers",
  },
  {
    name: "Back after these",
    role: "bumper",
    durationMs: 5000,
    era: 1990,
    audience: "kids",
    tagged: true,
    needsDetails: false,
    playCount: 0,
    playsCounted: true,
    hash: "hash-bumper-close",
    tunarrProgramId: "clip-bumper-close",
  },
];

const taggedClip: ClipDTO = {
  name: "Fizzbolt — Dude!",
  role: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food & drink",
  tagged: true,
  needsDetails: false,
  playCount: 0,
  playsCounted: true,
  hash: "hash-fizzbolt-tagged",
  tunarrProgramId: "clip-fizzbolt-tagged",
};

// A clip whose frame was extracted (V17b/V30) and adopted into the image service (§22).
//
// ⚠ **The `hash` is an ordinary hash, never an inline data URI.** Artwork is an image record, and
// a story rendering offline against storybook-static gets its assets from
// `.storybook/story-assets/` via `staticDirs`.
//
// ⚠ Same-origin asset paths, never data URIs — a base64 data URI always contains a comma, which is
// `srcset`'s candidate separator, so it is unloadable there (#210). Each ladder rung is a
// different colour so a baseline shows which rung a given box actually chose.
const thumbnailedClip: ClipDTO = {
  ...taggedClip,
  name: "Crunchlings — Morning Crunch!",
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
  tunarrProgramId: "clip-crunchlings",
};

const untaggedClip: ClipDTO = {
  name: "Unlabeled 30s spot",
  role: "commercial",
  durationMs: 30000,
  tagged: false,
  // A commercial with no match tags: the Library's one exception on a ready clip (#1817).
  needsDetails: true,
  playCount: 0,
  playsCounted: true,
  hash: "hash-unlabeled",
  tunarrProgramId: "clip-unlabeled",
};

const aiTaggedClip: ClipDTO = {
  ...taggedClip,
  tagged: false,
  playCount: 0,
  playsCounted: true,
  hash: "hash-ai",
  tunarrProgramId: "clip-ai",
};

// A persisted split proposal (§10 V34) mid-review, covering every segment state the
// editor must render honestly: a clean classified segment, one with an unconfirmed era
// suggestion, a dHash duplicate flag, an unsplittable over-long span, and a transcript.
const splitProposal: SplitReviewProposalDTO = {
  id: "split-testcard",
  clipHash: "c5e2000000000000000000000000000000000000000000000000000000000080",
  createdAt: "2026-07-25T20:00:00Z",
  checkedLanguages: [],
  currentLanguages: [],
  segments: [
    {
      index: 0,
      startMs: 0,
      endMs: 30500,
      name: "Fizzbolt — Dude!",
      nameOrigin: "model-proposed",
      nameEvidence: "FIZZBOLT — DUDE!",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      artwork: thumbnailedClip.thumbImage,
      transcript: "[00:01] Fizzbolt, dude!\n[00:12] Packed with sunshine.",
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
      name: "Gloopers — Snack Attack!",
      nameOrigin: "source-authored",
      era: 1990,
      audience: "kids",
      category: "food & drink",
      artwork: thumbnailedClip.thumbImage,
      // dHash match against an existing catalog row — a FLAG, never a silent drop.
      dupOf: "clip-gloopers.mp4",
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
// ⚠ The variety is deliberate and mirrors the pinned Go fixture's shapes (its titles and ids are
// invented here): one row has everything, one has NO date, and one has neither duration nor
// height. Absence is the common case — archive has not probed every item — and a row that renders
// "0:00" for an unprobed clip claims it is empty. A uniform fixture would let that bug pass here
// and fail on the first real search.
const discoveredClips: DiscoveredClip[] = [
  {
    id: "crunchlings-1989",
    title: "Crunchlings Cereal — 1989 television commercial",
    url: "https://archive.org/details/crunchlings-1989",
    thumbnailUrl:
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
    date: "1989-01-01T00:00:00Z",
    year: 1989,
    durationMs: 10_800,
    height: 720,
  },
  {
    id: "saturday-morning-block-v-7",
    title: "Saturday Morning Block Vol. 7 (full block)",
    url: "https://archive.org/details/saturday-morning-block-v-7",
    thumbnailUrl:
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAQAAABeK7cBAAAADklEQVR42mP8z8AARIQZADIAAv/kx0EAAAAASUVORK5CYII=",
    // No date: 2 of the 5 pinned docs declare none.
    durationMs: 11_032_100,
    height: 360,
  },
  {
    id: "youtube-dEm0cLiP001",
    title: "1980s cereal commercial compilation",
    url: "https://archive.org/details/youtube-dEm0cLiP001",
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
  fallbackCardEntry,
  podClips,
  podEntries,
  splitProposal,
  taggedClip,
  thumbnailedClip,
  untaggedClip,
};
