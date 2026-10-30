import type { ChannelTracksOutputBody } from "@loomarr/api/models/channelTracksOutputBody";
import type { ChannelTracksSource } from "@loomarr/core/channel-tracks";
import { useEffect, useState } from "react";

/**
 * The tracks of the programme at the playhead (#2127), for Settings' audio: asked once per programme,
 * by its start, as web Watch asks. Another programme or channel drops the old answer at once, so the
 * sheet never offers one programme's languages over another; a failed answer offers none.
 */
const usePairedTracks = (
  source: ChannelTracksSource,
  channelId: string | undefined,
  programmeStartMs: number | undefined,
): ChannelTracksOutputBody | undefined => {
  const key = channelId && programmeStartMs !== undefined ? `${channelId}@${programmeStartMs}` : undefined;
  const [loaded, setLoaded] = useState<{ key: string; tracks: ChannelTracksOutputBody }>();
  useEffect(() => {
    if (!channelId || programmeStartMs === undefined) return;
    const request = new AbortController();
    source.load(channelId, programmeStartMs, request.signal).then(
      (tracks) => {
        if (!request.signal.aborted) setLoaded({ key: `${channelId}@${programmeStartMs}`, tracks });
      },
      () => undefined,
    );
    return () => request.abort();
  }, [channelId, programmeStartMs, source]);
  return key !== undefined && loaded?.key === key ? loaded.tracks : undefined;
};

export { usePairedTracks };
