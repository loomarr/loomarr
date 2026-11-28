import type { PlayerBandData, WatchProgramme } from "@loomarr/ui";
import type { WatchLineupChannel } from "../../watch-lineup";

/**
 * What the lower third says is on a channel. The watched channel reads its own schedule at the
 * playhead (`band`, as `playbarFromTimeline` reads it), so a break says when the programme is back
 * and a viewer behind live reads what is on screen. Any other channel, or the watched one before
 * its schedule is read, says what the guide has on now.
 */
const programmeOf = (
  channel: WatchLineupChannel,
  watched: { id: string; band: PlayerBandData; playheadMs: number } | undefined,
  formatTime: (ms: number) => string,
): WatchProgramme => {
  const band = watched?.id === channel.id && watched.band.title ? watched.band : undefined;
  if (!band?.title) {
    return channel.onNow ? { kind: "on", minutesLeft: channel.minutesLeft ?? 1, title: channel.onNow } : { kind: "nothing" };
  }
  const next = band.next ? { next: { startsAt: formatTime(band.next.startMs), title: band.next.name } } : {};
  if (band.inBreak) return { backInMinutes: band.breakMinutesLeft ?? 1, kind: "break", title: band.title, ...next };
  const minutesLeft = band.stopMs === undefined ? 1 : (band.stopMs - (watched?.playheadMs ?? 0)) / 60_000;
  return {
    kind: "on",
    minutesLeft,
    title: band.title,
    ...(band.line ? { episode: band.line } : {}),
    ...(band.rating ? { rating: band.rating } : {}),
    ...next,
  };
};

export { programmeOf };
