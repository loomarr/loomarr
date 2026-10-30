// The viewer's subtitles on the <video>'s own text track (#2088 step 7). The cues come from the
// subtitles sidecar on the channel's wall clock; the element plays media time, so each cue is placed
// at wall clock − offset, where offset is the player's wall-clock anchor (PROGRAM-DATE-TIME). A text
// track rather than an overlay because the platform renders it everywhere the picture goes,
// including a phone's native fullscreen, which no overlay reaches.

interface LiveSubtitleCue {
  startMs: number;
  endMs: number;
  /** WebVTT cue text: lines joined by "\n", inline <i>/<b> kept. */
  text: string;
}

interface LiveSubtitleCues {
  cues: readonly LiveSubtitleCue[];
  /** The track's ISO 639-2 language and its label, given to the element's track. */
  language: string;
  label: string;
}

// A re-place within this drift would only churn cues: the anchor wobbles by a frame or two.
const OFFSET_TOLERANCE_MS = 250;

// A track's label and language are fixed when it is added, and a platform's own captions menu (a
// phone's native fullscreen) lists them, so each choice gets its own track and the others are off.
// An element cannot drop a track it added; a viewer's few choices are a few tracks.
interface PlacedTrack {
  track: TextTrack;
  cues?: LiveSubtitleCues;
  offsetMs?: number;
}
const placed = new WeakMap<HTMLVideoElement, Map<string, PlacedTrack>>();

/**
 * Shows `subtitles` on the element's text track for that choice, placed by `offsetMs` (wall clock −
 * media time, ms); undefined hides them. Cheap to call every tick: it re-places only when the cues or
 * the offset changed.
 */
const syncSubtitleTrack = (
  video: HTMLVideoElement,
  subtitles: LiveSubtitleCues | undefined,
  offsetMs: number | undefined,
) => {
  const tracks = placed.get(video) ?? new Map<string, PlacedTrack>();
  const key = subtitles ? `${subtitles.language}\n${subtitles.label}` : undefined;
  for (const [k, other] of tracks) if (k !== key) other.track.mode = "disabled";
  if (!subtitles || !key) return;
  if (offsetMs === undefined || typeof video.addTextTrack !== "function" || typeof VTTCue === "undefined")
    return;
  let entry = tracks.get(key);
  if (!entry) {
    entry = { track: video.addTextTrack("subtitles", subtitles.label, subtitles.language) };
    tracks.set(key, entry);
    placed.set(video, tracks);
  }
  entry.track.mode = "showing";
  if (
    entry.cues === subtitles &&
    entry.offsetMs !== undefined &&
    Math.abs(entry.offsetMs - offsetMs) < OFFSET_TOLERANCE_MS
  )
    return;
  for (const cue of Array.from(entry.track.cues ?? [])) entry.track.removeCue(cue);
  for (const c of subtitles.cues) {
    entry.track.addCue(new VTTCue((c.startMs - offsetMs) / 1_000, (c.endMs - offsetMs) / 1_000, c.text));
  }
  entry.cues = subtitles;
  entry.offsetMs = offsetMs;
};

export type { LiveSubtitleCue, LiveSubtitleCues };
export { syncSubtitleTrack };
