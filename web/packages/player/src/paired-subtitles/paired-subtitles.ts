import type { SubtitleAiringDTO } from "@loomarr/api/models/subtitleAiringDTO";
import type { ChannelSubtitlesSource } from "@loomarr/core/channel-subtitles";
import { useEffect, useMemo, useState } from "react";

import {
  type LiveSubtitleCue,
  type LiveSubtitles,
  type SubtitlesAtPlayhead,
  subtitlesAtPlayhead,
  subtitleWindow,
} from "../live-subtitles";

interface PairedSubtitles extends SubtitlesAtPlayhead {
  /** The playing track's cues around the playhead; empty while subtitles are off or none play here. */
  cues: readonly LiveSubtitleCue[];
}

/**
 * The subtitles at the playhead (#2127), read from the sidecar the way web Watch reads it: one
 * window per step of the playhead, the chosen track's cues with it. The last window stays while the
 * next loads, so a cue on screen never blinks at a step; another channel drops it at once.
 */
const usePairedSubtitles = (
  source: ChannelSubtitlesSource,
  channelId: string | undefined,
  viewerTimeMs: number,
  choice: LiveSubtitles,
): PairedSubtitles => {
  const window = subtitleWindow(viewerTimeMs, choice);
  const query = JSON.stringify(window);
  const [loaded, setLoaded] = useState<{ channelId: string; airings: readonly SubtitleAiringDTO[] }>();
  useEffect(() => {
    if (!channelId) return;
    const request = new AbortController();
    source.load(channelId, JSON.parse(query), request.signal).then(
      (body) => {
        if (!request.signal.aborted) setLoaded({ airings: body.airings, channelId });
      },
      () => undefined,
    );
    return () => request.abort();
  }, [channelId, query, source]);
  const airings = loaded?.channelId === channelId ? loaded?.airings : undefined;
  const atPlayhead = subtitlesAtPlayhead(airings, viewerTimeMs, choice);
  const playing = atPlayhead.playing !== undefined;
  const cues = useMemo(() => (playing && airings ? airings.flatMap((a) => a.cues) : []), [airings, playing]);
  return { ...atPlayhead, cues };
};

export type { PairedSubtitles };
export { usePairedSubtitles };
