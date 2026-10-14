import type { FillerReadinessDTO } from "@loomarr/api/models/fillerReadinessDTO";

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

export { readiness };
