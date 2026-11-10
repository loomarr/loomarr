import type { PlayerDetailRow } from "@loomarr/ui";
import { type RefObject, useEffect, useState } from "react";
import type { PlaybackFacts } from "./use-playback-details.type";

const POLL_MS = 1_000;

const HDR: Record<string, string> = { HLG: "HLG", PQ: "HDR10 (PQ)", SDR: "None (SDR)" };

const frameRate = (fps: number) => `${Number.parseFloat(fps.toFixed(2))} fps`;

// The rows Playback details shows, from what the browser knows. Technical terms are allowed here and
// nowhere else; a figure the player cannot read (native HLS has no level, a browser may lack frame
// counts) leaves its row out rather than inventing it.
const detailRows = (
  { lagSeconds, level, sessionId, video }: PlaybackFacts,
  host: string,
): PlayerDetailRow[] => {
  const rows: PlayerDetailRow[] = [];
  const size = video && video.width > 0 ? `${video.width}×${video.height}` : undefined;
  const rate = level?.frameRate ? frameRate(level.frameRate) : undefined;
  if (size) rows.push({ label: "Resolution", value: [size, rate].filter(Boolean).join(" · ") });
  if (level?.codecs) rows.push({ label: "Video", value: level.codecs });
  if (level)
    rows.push({ label: "HDR", value: HDR[level.videoRange ?? "SDR"] ?? level.videoRange ?? "None (SDR)" });
  if (level?.bitrate)
    rows.push({ label: "Bitrate", value: `${(level.bitrate / 1_000_000).toFixed(1)} Mbps` });
  if (video?.bufferedAheadSeconds !== undefined)
    rows.push({ label: "Buffer ahead", value: `${video.bufferedAheadSeconds.toFixed(1)} s` });
  if (video?.droppedFrames !== undefined && video.totalFrames !== undefined)
    rows.push({
      label: "Dropped frames",
      value: `${video.droppedFrames} of ${video.totalFrames.toLocaleString("en-US")}`,
    });
  rows.push({ label: "Behind live", value: `${lagSeconds.toFixed(1)} s` });
  rows.push({ label: "Server", value: host });
  if (sessionId) rows.push({ label: "Session", value: sessionId.replace(/^playback_/, "").slice(0, 8) });
  return rows;
};

const readVideo = (el: HTMLVideoElement): PlaybackFacts["video"] => {
  const quality = el.getVideoPlaybackQuality?.();
  let ahead: number | undefined;
  for (let i = 0; i < el.buffered.length; i++) {
    if (el.currentTime >= el.buffered.start(i) && el.currentTime <= el.buffered.end(i)) {
      ahead = Math.max(0, el.buffered.end(i) - el.currentTime);
    }
  }
  return {
    ...(ahead === undefined ? {} : { bufferedAheadSeconds: ahead }),
    ...(quality ? { droppedFrames: quality.droppedVideoFrames, totalFrames: quality.totalVideoFrames } : {}),
    height: el.videoHeight,
    width: el.videoWidth,
  };
};

// The details rows, read from the player's own <video> every second while the panel is open.
const usePlaybackDetails = (
  open: boolean,
  video: RefObject<HTMLVideoElement | null>,
  facts: Omit<PlaybackFacts, "video">,
): PlayerDetailRow[] => {
  const [read, setRead] = useState<PlaybackFacts["video"]>(undefined);
  useEffect(() => {
    if (!open) return;
    const tick = () => setRead(video.current ? readVideo(video.current) : undefined);
    tick();
    const id = window.setInterval(tick, POLL_MS);
    return () => window.clearInterval(id);
  }, [open, video]);
  return detailRows({ ...facts, ...(read ? { video: read } : {}) }, window.location.host);
};

export { detailRows, usePlaybackDetails };
