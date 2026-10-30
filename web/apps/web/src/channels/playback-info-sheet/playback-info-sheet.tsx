import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import { unwrap } from "@loomarr/api/unwrap";
import type { Density } from "@loomarr/design-system";
import { type LivePlaybackFacts, streamRows, useSampledPlaybackFacts } from "@loomarr/player/browser";
import { PlayerPlaybackInfo } from "@loomarr/ui";
import { useEffect, useState } from "react";
import { adminRows } from "@/channels/playback-info-rows";

// The server's encode changes slowly; its status is read again at this pace while open.
const STATUS_MS = 5_000;
// How long Copy stream URL reads "Copied".
const COPIED_MS = 2_000;

interface PlaybackInfoSheetProps {
  auto: boolean;
  channelId: string;
  density: Density;
  isAdmin: boolean;
  lagSeconds: number;
  playbackFacts: () => LivePlaybackFacts | undefined;
  sessionId: string;
}

// PLAYBACK INFO (#2088 decision 13): the Stream rows any viewer reads, sampled from the player while
// open, and for an admin the server's side of the channel's browser encode and the signed stream URL
// to open in another player.
const PlaybackInfoSheet = ({
  auto,
  channelId,
  density,
  isAdmin,
  lagSeconds,
  playbackFacts,
  sessionId,
}: PlaybackInfoSheetProps) => {
  const facts = useSampledPlaybackFacts(playbackFacts);

  const status = dashboardApi.useGetPlayoutStatus({
    query: { enabled: isAdmin, refetchInterval: STATUS_MS, retry: false },
  });
  // A channel has a row per format it airs (`playout.md`): the 1080p baseline, and a premium
  // `4k-…` one where it has 4K. The row is the one for the rung this browser is playing.
  const premium = (facts?.height ?? 0) >= 2160;
  const health = unwrap(status.data)?.channels.find(
    (row) => row.channelId === channelId && row.target.startsWith("4k-") === premium,
  );

  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), COPIED_MS);
    return () => window.clearTimeout(timer);
  }, [copied]);
  const url = facts?.url;
  const copyStreamUrl = url
    ? () => {
        // Absolute, so it opens in another player; a refused clipboard just stays "Copy".
        void navigator.clipboard
          ?.writeText(new URL(url, window.location.href).href)
          .then(() => setCopied(true))
          .catch(() => undefined);
      }
    : undefined;

  return (
    <PlayerPlaybackInfo
      admin={
        isAdmin ? { copied, onCopyStreamUrl: copyStreamUrl, rows: adminRows(health, sessionId) } : undefined
      }
      density={density}
      stream={streamRows({ auto, facts, lagSeconds })}
    />
  );
};

export type { PlaybackInfoSheetProps };
export { PlaybackInfoSheet };
