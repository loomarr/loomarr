import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import {
  type LiveSubtitleCues,
  type LiveSubtitles,
  type LiveSubtitleTrack,
  SUBTITLE_STEP_MS,
  subtitleLabel,
  subtitlesAtPlayhead,
  subtitleWindow,
} from "@loomarr/player/browser";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo } from "react";

// The subtitles at the playhead (#2088 step 7), from the sidecar beside the stream: the text tracks
// of the programme the viewer is watching (live or seeked back), which of them the viewer's choice
// plays, and that track's cues for the player's text track. A programme without text subtitles, or
// a break, offers none. The window and the pick are the shared player's, as the phone reads them.

interface LiveSubtitlesAtPlayhead {
  /** The tracks the programme at the playhead carries; empty for a break or no text subtitles. */
  tracks: readonly LiveSubtitleTrack[];
  /** Which of `tracks` the viewer's choice plays; -1 for none. */
  active: number;
  /** A track's label in the player: its language, and its title beside it when it has one. */
  labelOf: (track: LiveSubtitleTrack) => string;
  /** The playing track's cues; undefined while subtitles are off or none play here. */
  cues?: LiveSubtitleCues;
}

const useLiveSubtitles = (
  channelId: string,
  viewerTimeMs: number,
  choice: LiveSubtitles,
  enabled: boolean,
): LiveSubtitlesAtPlayhead => {
  const window = channelsApi.useChannelSubtitles(channelId, subtitleWindow(viewerTimeMs, choice), {
    query: { enabled, placeholderData: keepPreviousData, retry: false, staleTime: SUBTITLE_STEP_MS },
  });
  const airings = unwrap(window.data)?.airings;
  const { active, playing, tracks } = subtitlesAtPlayhead(airings, viewerTimeMs, choice);
  // Stable while the window's data is: the player re-places cues only when they change.
  const cues = useMemo<LiveSubtitleCues | undefined>(
    () =>
      playing && airings
        ? { cues: airings.flatMap((a) => a.cues), label: subtitleLabel(playing), language: playing.language }
        : undefined,
    [airings, playing],
  );
  return { active, cues, labelOf: subtitleLabel, tracks };
};

export type { LiveSubtitlesAtPlayhead };
export { useLiveSubtitles };
