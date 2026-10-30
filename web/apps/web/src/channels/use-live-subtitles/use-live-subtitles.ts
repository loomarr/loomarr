import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import {
  type LiveSubtitleCues,
  type LiveSubtitles,
  type LiveSubtitleTrack,
  selectSubtitleTrack,
} from "@loomarr/player/browser";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo } from "react";
import { languageLabel } from "@/channels/channel-default-audio/language-label";

// The subtitles at the playhead (#2088 step 7), from the sidecar beside the stream: the text tracks
// of the programme the viewer is watching (live or seeked back), which of them the viewer's choice
// plays, and that track's cues for the player's text track. A programme without text subtitles, or
// a break, offers none.

// The window is read in steps, so a playing viewer refetches once per step rather than per tick: a
// little behind the playhead (a cue already on screen) and a step and more ahead (the next cues in
// hand before they show).
const STEP_MS = 30_000;
const BEHIND_MS = 15_000;
const SPAN_MS = 90_000;

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

const labelOf = (track: LiveSubtitleTrack) =>
  track.title ? `${languageLabel(track.language)} · ${track.title}` : languageLabel(track.language);

const useLiveSubtitles = (
  channelId: string,
  viewerTimeMs: number,
  choice: LiveSubtitles,
  enabled: boolean,
): LiveSubtitlesAtPlayhead => {
  const from = Math.floor(viewerTimeMs / STEP_MS) * STEP_MS - BEHIND_MS;
  const pick = choice === "off" ? {} : { language: choice.language, title: choice.title };
  const window = channelsApi.useChannelSubtitles(
    channelId,
    { from, to: from + SPAN_MS, ...pick },
    { query: { enabled, placeholderData: keepPreviousData, retry: false, staleTime: STEP_MS } },
  );
  const airings = unwrap(window.data)?.airings;
  const airing = airings?.find((a) => viewerTimeMs >= a.startMs && viewerTimeMs < a.stopMs);
  const tracks = airing?.tracks ?? [];
  const active = selectSubtitleTrack(tracks, choice);
  const playing = active >= 0 ? tracks[active] : undefined;
  // Stable while the window's data is: the player re-places cues only when they change.
  const cues = useMemo<LiveSubtitleCues | undefined>(
    () =>
      playing && airings
        ? { cues: airings.flatMap((a) => a.cues), label: labelOf(playing), language: playing.language }
        : undefined,
    [airings, playing],
  );
  return { active, cues, labelOf, tracks };
};

export type { LiveSubtitlesAtPlayhead };
export { useLiveSubtitles };
