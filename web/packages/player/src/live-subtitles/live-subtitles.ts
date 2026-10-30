import type { ChannelSubtitlesParams } from "@loomarr/api/models/channelSubtitlesParams";
import type { SubtitleAiringDTO } from "@loomarr/api/models/subtitleAiringDTO";

// The viewer's subtitles (#2088 step 7): off, or a text track of the programme at the playhead,
// remembered across channels. The subtitles sidecar (GET /v1/channels/{id}/subtitles) lists each
// airing's tracks by language and title, so a choice is the pair. A programme without that exact
// track plays the first in the same language; one without the language plays none, and the choice
// is kept for the next programme that has it. Every player reads the sidecar the same way; how the
// cues reach the picture is the player's own (a <video> text track, or a native overlay).

interface LiveSubtitleTrack {
  /** ISO 639-2, e.g. `eng`; empty when the media left it untagged. */
  language: string;
  /** The language named in itself, the server's ("Español"); empty when it names no one language. */
  languageName?: string;
  /** The stream's title ("SDH"); empty when the media carries none. */
  title: string;
}

type LiveSubtitles = "off" | LiveSubtitleTrack;

interface LiveSubtitleCue {
  startMs: number;
  endMs: number;
  /** WebVTT cue text: lines joined by "\n", inline <i>/<b> kept. */
  text: string;
}

interface LiveSubtitleCues {
  cues: readonly LiveSubtitleCue[];
  /** The track's ISO 639-2 language and its label, given to the player's track. */
  language: string;
  label: string;
}

/** A stored choice as a client reads it back: anything but a track (language and title) is off. */
const subtitlesPreferenceOf = (stored: unknown): LiveSubtitles =>
  typeof stored === "object" &&
  stored !== null &&
  "language" in stored &&
  "title" in stored &&
  typeof stored.language === "string" &&
  typeof stored.title === "string"
    ? { language: stored.language, title: stored.title }
    : "off";

/** The index in `tracks` that `choice` plays, or -1 for none. */
const selectSubtitleTrack = (tracks: readonly LiveSubtitleTrack[], choice: LiveSubtitles): number => {
  if (choice === "off") return -1;
  const exact = tracks.findIndex((t) => t.language === choice.language && t.title === choice.title);
  return exact >= 0 ? exact : tracks.findIndex((t) => t.language === choice.language);
};

/** `C` and the CC tool: off → the first track → … → the last → off. Nothing to cycle stays off. */
const nextSubtitles = (tracks: readonly LiveSubtitleTrack[], choice: LiveSubtitles): LiveSubtitles => {
  const next = selectSubtitleTrack(tracks, choice) + 1;
  const track = tracks[next];
  return track ? { language: track.language, title: track.title } : "off";
};

/**
 * A track's label in the player: its language named in itself, as the server names it so every
 * platform reads alike, and its title beside it when it has one. An unnamed language reads by its
 * code, or as "Subtitles" when it has none.
 */
const subtitleLabel = (track: LiveSubtitleTrack) => {
  const language = track.languageName || track.language.toUpperCase() || "Subtitles";
  return track.title ? `${language} · ${track.title}` : language;
};

// The window is read in steps, so a playing viewer refetches once per step rather than per tick: a
// little behind the playhead (a cue already on screen) and a step and more ahead (the next cues in
// hand before they show).
const SUBTITLE_STEP_MS = 30_000;
const BEHIND_MS = 15_000;
const SPAN_MS = 90_000;

/** The sidecar query for the playhead: its window, and the chosen track's cues (none while off). */
const subtitleWindow = (viewerTimeMs: number, choice: LiveSubtitles): ChannelSubtitlesParams => {
  const from = Math.floor(viewerTimeMs / SUBTITLE_STEP_MS) * SUBTITLE_STEP_MS - BEHIND_MS;
  const pick = choice === "off" ? {} : { language: choice.language, title: choice.title };
  return { from, to: from + SPAN_MS, ...pick };
};

interface SubtitlesAtPlayhead {
  /** The tracks the programme at the playhead carries; empty for a break or no text subtitles. */
  tracks: readonly LiveSubtitleTrack[];
  /** Which of `tracks` the viewer's choice plays; -1 for none. */
  active: number;
  /** The track playing, when one is. */
  playing?: LiveSubtitleTrack;
}

/** What the sidecar's window offers at the playhead, and which of it the viewer's choice plays. */
const subtitlesAtPlayhead = (
  airings: readonly SubtitleAiringDTO[] | undefined,
  viewerTimeMs: number,
  choice: LiveSubtitles,
): SubtitlesAtPlayhead => {
  const airing = airings?.find((a) => viewerTimeMs >= a.startMs && viewerTimeMs < a.stopMs);
  const tracks = airing?.tracks ?? [];
  const active = selectSubtitleTrack(tracks, choice);
  return { active, playing: active >= 0 ? tracks[active] : undefined, tracks };
};

/** The lines on screen at an instant, inline tags dropped: for a player that draws its own captions. */
const cueLinesAt = (cues: readonly LiveSubtitleCue[], atMs: number): string[] => {
  const showing = cues.filter((cue) => cue.startMs <= atMs && atMs < cue.endMs);
  return showing.flatMap((cue) => cue.text.replace(/<[^>]*>/g, "").split("\n"));
};

export type { LiveSubtitleCue, LiveSubtitleCues, LiveSubtitles, LiveSubtitleTrack, SubtitlesAtPlayhead };
export {
  cueLinesAt,
  nextSubtitles,
  SUBTITLE_STEP_MS,
  selectSubtitleTrack,
  subtitleLabel,
  subtitlesAtPlayhead,
  subtitlesPreferenceOf,
  subtitleWindow,
};
