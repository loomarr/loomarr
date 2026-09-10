import type { Assessment } from "@loomarr/api/models/assessment";

const outlook = (overrides: Partial<Assessment> = {}): Assessment => ({
  fingerprint: "fixture-proposal-outlook",
  observedAt: "2026-09-09T12:00:00Z",
  state: "ready",
  titles: 3,
  scheduledTitles: 3,
  missingAcquisitions: 0,
  missingLibrary: 0,
  unknownTitles: 0,
  programs: 12,
  seasons: 2,
  uniqueRuntimeMs: 4 * 3_600_000,
  firstRepeatMs: 4 * 3_600_000,
  windowMs: 0,
  windowLimited: false,
  thin: false,
  ordering: "syndication",
  relaxations: [],
  mix: { core: 2, adjacent: 1, discovery: 0, unknown: 0 },
  ...overrides,
});

export { outlook };
