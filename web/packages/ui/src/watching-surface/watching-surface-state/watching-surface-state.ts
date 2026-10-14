import type { PlayerSnapshot } from "@loomarr/player";

const behindLabel = (seconds: number) => {
  const rounded = Math.max(0, Math.round(seconds));
  const minutes = Math.floor(rounded / 60);
  const remainder = String(rounded % 60).padStart(2, "0");
  return `${minutes}:${remainder} behind`;
};

const playbackMessage = (snapshot: PlayerSnapshot, loadError?: string) =>
  loadError ??
  (snapshot.status === "empty"
    ? "No playable channels are on this Loomarr yet."
    : snapshot.status === "idle"
      ? "Choose a channel from the Guide or Surf."
      : snapshot.status === "failed"
        ? snapshot.error
        : undefined);

/**
 * The moment Watching's programme lines describe: the frame on screen while there is one (playing
 * or paused), otherwise the wall clock. A tune that is still starting or has failed shows no frame,
 * and its tune-time placeholder would keep "Up next" naming a programme that has already begun.
 */
const watchingScheduleTimeMs = (snapshot: PlayerSnapshot, wallClockMs: number) =>
  (snapshot.status === "playing" || snapshot.status === "paused") && snapshot.livePlayback
    ? snapshot.livePlayback.viewerTimeMs
    : wallClockMs;

export { behindLabel, playbackMessage, watchingScheduleTimeMs };
