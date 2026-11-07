import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { useEffect, useRef } from "react";
import { clientDiagnostics } from "@/diagnostics/client-reporter";

const PLAYHEAD_DRIFT_THRESHOLD_MS = 30_000;

type WatchTelemetry = {
  channelId: string;
  playbackSessionId: string;
  // The tuned channel's timeline.
  airings: readonly GuideAiring[];
  // Where the viewer's picture is, on the channel's clock.
  viewerTimeMs: number;
  // The live edge, projected forward from the server's last read; undefined before the first read.
  serverNowMs?: number;
};

// What the Watch player reports about a session (client diagnostics): each schedule block the
// picture enters, and a playhead that has drifted half a minute from the live edge, once per block.
// Both start afresh when the channel changes.
const useWatchTelemetry = ({
  channelId,
  playbackSessionId,
  airings,
  viewerTimeMs,
  serverNowMs,
}: WatchTelemetry) => {
  const airing = airings.find(
    (candidate) => !candidate.nominal && viewerTimeMs >= candidate.startMs && viewerTimeMs < candidate.stopMs,
  );
  const scheduleBlockId = airing?.scheduleBlockId;
  const previousBlockRef = useRef<{ channelId: string; scheduleBlockId?: string }>({ channelId });
  const driftedBlockRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (previousBlockRef.current.channelId !== channelId) {
      previousBlockRef.current = { channelId };
      driftedBlockRef.current = undefined;
    }
    if (!airing || !scheduleBlockId || serverNowMs === undefined) return;
    if (previousBlockRef.current.scheduleBlockId === scheduleBlockId) return;
    clientDiagnostics.record({
      event: "player.schedule_block_changed",
      playbackSessionId,
      channelId,
      scheduleBlockId,
      previousScheduleBlockId: previousBlockRef.current.scheduleBlockId,
      blockKind: airing.kind === "program" || airing.kind === "filler" ? airing.kind : "flex",
      viewerTimeMs: Math.round(viewerTimeMs),
      serverTimeMs: Math.round(serverNowMs),
    });
    previousBlockRef.current.scheduleBlockId = scheduleBlockId;
  }, [channelId, airing, playbackSessionId, scheduleBlockId, serverNowMs, viewerTimeMs]);

  useEffect(() => {
    if (!scheduleBlockId || serverNowMs === undefined || driftedBlockRef.current === scheduleBlockId) return;
    const driftMs = Math.round(viewerTimeMs - serverNowMs);
    if (Math.abs(driftMs) < PLAYHEAD_DRIFT_THRESHOLD_MS) return;
    clientDiagnostics.record({
      event: "player.playhead_drift",
      playbackSessionId,
      channelId,
      viewerTimeMs: Math.round(viewerTimeMs),
      serverTimeMs: Math.round(serverNowMs),
      driftMs,
    });
    driftedBlockRef.current = scheduleBlockId;
  }, [channelId, playbackSessionId, scheduleBlockId, serverNowMs, viewerTimeMs]);
};

export { useWatchTelemetry };
