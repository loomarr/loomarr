import type { PlayerSnapshot } from "@loomarr/player";

/**
 * The moment Watching's programme lines describe: the frame on screen while there is one (playing
 * or paused), otherwise the wall clock. A tune that is still starting or has failed shows no frame,
 * and its tune-time placeholder would keep "Up next" naming a programme that has already begun.
 */
const watchingScheduleTimeMs = (snapshot: PlayerSnapshot, wallClockMs: number) =>
  (snapshot.status === "playing" || snapshot.status === "paused") && snapshot.livePlayback
    ? snapshot.livePlayback.viewerTimeMs
    : wallClockMs;

export { watchingScheduleTimeMs };
