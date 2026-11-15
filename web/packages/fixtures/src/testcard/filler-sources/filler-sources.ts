import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";

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

export {
  fillerSources,
  fillerSourcesEmptyProvider,
  fillerSourcesGrouped,
  fillerSourcesManyGrouped,
  fillerSourcesWithRemotes,
};
