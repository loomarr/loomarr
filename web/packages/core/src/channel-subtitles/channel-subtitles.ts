import { getChannelSubtitlesUrl } from "@loomarr/api/endpoints/channels";
import type { ChannelSubtitlesOutputBody } from "@loomarr/api/models/channelSubtitlesOutputBody";
import type { SubtitleCueDTO } from "@loomarr/api/models/subtitleCueDTO";
import type { SubtitleTrackDTO } from "@loomarr/api/models/subtitleTrackDTO";
import { useEffect, useRef, useState } from "react";

import { SUBTITLES_OFF } from "./subtitle-choices";

// A channel's subtitles are served beside its stream, on its wall clock (#2088 decision 5): the
// player asks for a stretch from its playhead and draws the cue under the playhead over the picture.

/** How much of the clock one ask covers. */
const SUBTITLE_WINDOW_MS = 2 * 60_000;
/** The next stretch is asked for this long before the current one runs out. */
const SUBTITLE_LEAD_MS = 30_000;
/** A track still being read from the media is asked for again this soon... */
const SUBTITLE_PENDING_RETRY_MS = 3_000;
/** ...and a failed ask this soon. */
const SUBTITLE_FAILED_RETRY_MS = 15_000;

interface ChannelSubtitlesSource {
  load: (
    channelId: string,
    stretch: { from: number; to: number; track?: string },
    signal: AbortSignal,
  ) => Promise<ChannelSubtitlesOutputBody>;
}

/** The subtitles read from the server with the client's own request (a cookie on the web, a token on a device). */
const createChannelSubtitlesSource = (request: typeof globalThis.fetch): ChannelSubtitlesSource => ({
  load: async (channelId, { from, to, track }, signal) => {
    const response = await request(getChannelSubtitlesUrl(channelId, { from, to, track }), {
      method: "GET",
      signal,
    });
    if (!response.ok) throw new Error(`Couldn't read the channel's subtitles (${response.status}).`);
    return (await response.json()) as ChannelSubtitlesOutputBody;
  },
});

/** What one ask answered, and for which channel, track and stretch. */
interface SubtitleStretch {
  channelId: string;
  cues: readonly SubtitleCueDTO[];
  from: number;
  to: number;
  track: string;
  tracks: readonly SubtitleTrackDTO[] | undefined;
}

/**
 * Whether the player needs to ask again: nothing asked yet, another channel or track, or a playhead
 * outside the stretch or near its end. A pending or failed ask is held as a short stretch, so the
 * playhead moving past it is the retry and no timer is needed (a paused picture asks nothing).
 */
const needsStretch = (
  held: SubtitleStretch | undefined,
  at: { channelId: string; playheadMs: number; track: string },
): boolean =>
  !held ||
  held.channelId !== at.channelId ||
  held.track !== at.track ||
  at.playheadMs < held.from ||
  at.playheadMs >= held.to - SUBTITLE_LEAD_MS;

/** The lines on screen at the playhead: every cue showing then, each split at its newlines. */
const cueLines = (cues: readonly SubtitleCueDTO[], playheadMs: number): string[] =>
  cues
    .filter((cue) => cue.startMs <= playheadMs && playheadMs < cue.endMs)
    .flatMap((cue) => cue.text.split("\n"))
    .filter((line) => line.trim() !== "");

interface ChannelSubtitles {
  /** The lines to draw now; empty when Off, between cues, or still reading the track. */
  lines: readonly string[];
  /** The text tracks of the programme on air, for the menu; undefined until the first answer. */
  tracks: readonly SubtitleTrackDTO[] | undefined;
}

/**
 * Follows a channel's subtitles along the playhead. `playheadMs` is the picture's wall-clock time
 * (the live clock's viewerTimeMs), so a viewer behind live sees the lines of what is on screen.
 * With `track` Off it still reads the programme's tracks (a cheap ask), so the menu and `C` know
 * what there is; a track is read from the media only once one is chosen.
 */
const useChannelSubtitles = (
  source: ChannelSubtitlesSource,
  channelId: string | undefined,
  playheadMs: number,
  track: string,
  enabled: boolean,
): ChannelSubtitles => {
  const [held, setHeld] = useState<SubtitleStretch>();
  const playhead = useRef(playheadMs);
  playhead.current = playheadMs;
  const need = enabled && channelId !== undefined && needsStretch(held, { channelId, playheadMs, track });

  useEffect(() => {
    if (!need || channelId === undefined) return;
    const pending = new AbortController();
    const from = Math.floor(playhead.current);
    const chosen = track === SUBTITLES_OFF ? undefined : track;
    const hold = (body: ChannelSubtitlesOutputBody | undefined, retryMs?: number) =>
      setHeld({
        channelId,
        cues: body?.cues ?? [],
        from,
        to: retryMs === undefined ? from + SUBTITLE_WINDOW_MS : from + SUBTITLE_LEAD_MS + retryMs,
        track,
        tracks: body?.tracks,
      });
    source.load(channelId, { from, to: from + SUBTITLE_WINDOW_MS, track: chosen }, pending.signal).then(
      (body) => {
        if (!pending.signal.aborted) hold(body, body.pending ? SUBTITLE_PENDING_RETRY_MS : undefined);
      },
      () => {
        if (!pending.signal.aborted) hold(undefined, SUBTITLE_FAILED_RETRY_MS);
      },
    );
    return () => pending.abort();
  }, [channelId, need, source, track]);

  const current = enabled && held !== undefined && held.channelId === channelId;
  return {
    lines: current && held.track === track && track !== SUBTITLES_OFF ? cueLines(held.cues, playheadMs) : [],
    tracks: current ? held.tracks : undefined,
  };
};

export type { ChannelSubtitles, ChannelSubtitlesSource, SubtitleStretch };
export {
  createChannelSubtitlesSource,
  cueLines,
  needsStretch,
  SUBTITLE_FAILED_RETRY_MS,
  SUBTITLE_LEAD_MS,
  SUBTITLE_PENDING_RETRY_MS,
  SUBTITLE_WINDOW_MS,
  useChannelSubtitles,
};
