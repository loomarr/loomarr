import type { CoverageDTO } from "@loomarr/api/models/coverageDTO";
import type { FillerDefectReportDTO } from "@loomarr/api/models/fillerDefectReportDTO";
import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerReadinessDTO } from "@loomarr/api/models/fillerReadinessDTO";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";

// Filler fixtures, typed against the generated DTOs so a new required field breaks the build here
// rather than surfacing as `undefined` in one test's screen. Same reasoning as `./channels`.
//
// `readiness()` is a healthy, ready install with nothing to do: 25 clips, no live channels.
// Tests override only the part they are about.
const readiness = (over: Partial<FillerReadinessDTO> = {}): FillerReadinessDTO => {
  const { repairs, ...rest } = over;
  return {
    ready: true,
    nextAction: "none",
    repairs: repairs ?? { count: 0 },
    fetch: { enabled: true, catalogClips: 25 },
    storage: {
      automatic: true,
      state: "healthy",
      totalBytes: 500 * 1024 ** 3,
      freeBytes: 200 * 1024 ** 3,
      managedBytes: 2 * 1024 ** 3,
      reservedBytes: 0,
      filesystemReservedBytes: 0,
      softBudgetBytes: 20 * 1024 ** 3,
      hardReserveBytes: 10 * 1024 ** 3,
      availableBytes: 18 * 1024 ** 3,
    },
    pipeline: {
      runnable: 0,
      scheduled: 0,
      inProgress: 0,
      needsDecision: 0,
      recoverable: 0,
      ready: 25,
      complete: 0,
      rejected: 0,
      dismissed: 0,
    },
    pool: { clips: 25, breakBody: 20, eligible: 18, untagged: 0, channels: [] },
    acquisitions: [],
    ...rest,
  };
};

// `defectReport()` is the defect floor's report (#2034) on an install it has measured, with nothing
// it would retire and nothing left to label.
const defectReport = (over: Partial<FillerDefectReportDTO> = {}): FillerDefectReportDTO => ({
  floorVersion: "floor-v3-provisional",
  clips: 25,
  unmeasured: 0,
  labelled: 0,
  verdicts: { pass: 25, fail: 0, undecided: 0, pending: 0 },
  scores: [],
  total: 0,
  rows: [],
  ...over,
});

// `DEFAULT_KINDS` is what the server's coverage answer says a channel that chose no kinds airs:
// every kind except the opt-in political and news (#2037).
const DEFAULT_KINDS: CoverageDTO["defaultKinds"] = [
  "commercial",
  "bumper",
  "station_id",
  "psa",
  "trailer",
  "interstitial",
];

// `fillerSource()` is an enabled, ready watched folder. Override `kind`, `uri`, `actions` and the
// rest for the source a test is about.
const fillerSource = (over: Partial<FillerSourceDTO> = {}): FillerSourceDTO => ({
  id: "folder",
  kind: "folder",
  target: "/data/filler",
  detail: "watched directly",
  count: 0,
  incoming: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  effectiveEnabled: true,
  providerEnabled: true,
  switchable: true,
  removable: false,
  searchable: false,
  readiness: "ready",
  ready: true,
  locationSource: "installation",
  actions: ["fetch", "disable"],
  ...over,
});

// `fillerProviders` is the registry `GET /v1/filler/providers` answers on a stock install: the two
// fetched, discoverable services and the two scanned local kinds.
const fillerProviders: FillerProviderDescriptionDTO[] = [
  {
    id: "archive",
    label: "Archive.org",
    addressHint: "https://archive.org/details/collection",
    fetched: true,
    listable: true,
    searchable: true,
    discoverable: true,
    openSearchable: false,
    addable: true,
    blurb: "Public collections of commercials, bumpers, and other broadcast material.",
    sourceNoun: "Archive.org collection",
    previewHeading: "Popular videos in this collection",
    findLabel: "Find an Archive.org collection",
    findPlaceholder: "Search collections or paste an archive.org URL",
    findAdd: "Add collection",
    findNoMatches: "No collections found. Paste the exact URL to check it.",
    findMinQuery: 2,
  },
  {
    id: "youtube",
    label: "YouTube",
    addressHint: "https://www.youtube.com/@channel/videos",
    fetched: true,
    listable: true,
    searchable: false,
    discoverable: true,
    openSearchable: false,
    addable: true,
    blurb: "Channels and playlists you choose from YouTube.",
    sourceNoun: "YouTube channel or playlist",
    previewHeading: "A few videos from this source",
    findLabel: "Find a YouTube channel or playlist",
    findPlaceholder: "Search channels or paste a channel or playlist URL",
    findAdd: "Add source",
    findNoMatches: "No channels found. Paste a channel or playlist URL to check it.",
    findMinQuery: 3,
  },
  {
    id: "library",
    label: "Media library",
    addressHint: "Commercials — as it appears on your media server",
    addressLabel: "Library name",
    sourceNoun: "Media server library",
    fetched: false,
    listable: false,
    searchable: false,
    discoverable: false,
    openSearchable: false,
    addable: true,
  },
  {
    id: "folder",
    label: "Folder",
    addressHint: "/data/filler — a full path Loomarr can read",
    addressLabel: "Folder path",
    sourceNoun: "Folder on this server",
    fetched: false,
    listable: false,
    searchable: false,
    discoverable: false,
    openSearchable: false,
    addable: true,
  },
];

export { DEFAULT_KINDS, defectReport, fillerProviders, fillerSource, readiness };
